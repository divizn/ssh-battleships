// Package store keeps player names and records in Upstash Redis over its REST API.
package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// idPrefix is what a real ssh key fingerprint starts with. Bot seats, local play and
	// keyless sessions have ids that cannot start with it, so this one test keeps all of
	// them out of Redis.
	idPrefix = "SHA256:"

	// The Upstash instance is shared with daily-quotes, which owns quote:<date>. Everything
	// here stays under one namespace so the two can never grow into each other.
	namespace   = "battleships:"
	leaderboard = namespace + "leaderboard"
	total       = namespace + "total"

	// live expires on its own, so a server that is stopped, killed or unplugged stops
	// claiming to be up without anyone having to tidy after it.
	live = namespace + "live"
)

type Profile struct {
	Name   string
	Wins   int
	Losses int
	Games  int
}

type Entry struct {
	Name string
	Wins int
}

// Store is nil when Redis is not configured. Every method tolerates a nil receiver, so no
// caller has to know whether the game is running with a database behind it.
type Store struct {
	url    string
	token  string
	client *http.Client
}

func New(url, token string) *Store {
	if url == "" || token == "" {
		return nil
	}
	return &Store{
		url:    strings.TrimSuffix(url, "/"),
		token:  token,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// Tracks reports whether id belongs to a player worth remembering.
func (s *Store) Tracks(id string) bool {
	return s != nil && strings.HasPrefix(id, idPrefix)
}

func (s *Store) Profile(id string) (Profile, error) {
	if !s.Tracks(id) {
		return Profile{}, nil
	}
	res, err := s.do([]any{"HMGET", key(id), "name", "wins", "losses", "games"})
	if err != nil {
		return Profile{}, err
	}
	var fields []*string
	if err := json.Unmarshal(res[0], &fields); err != nil {
		return Profile{}, fmt.Errorf("hmget %s: %w", id, err)
	}
	for len(fields) < 4 {
		fields = append(fields, nil)
	}
	return Profile{
		Name:   text(fields[0]),
		Wins:   number(fields[1]),
		Losses: number(fields[2]),
		Games:  number(fields[3]),
	}, nil
}

func (s *Store) SetName(id, name string) error {
	if !s.Tracks(id) || name == "" {
		return nil
	}
	_, err := s.do([]any{"HSET", key(id), "name", name})
	return err
}

// Record books a finished game. Every win counts on the total board; only ranked ones reach the
// leaderboard, because a board anyone can farm against the computer is not worth reading.
func (s *Store) Record(winner, loser string, ranked bool) error {
	var cmds [][]any
	if s.Tracks(winner) {
		cmds = append(cmds, []any{"HINCRBY", key(winner), "wins", 1}, []any{"HINCRBY", key(winner), "games", 1})
		cmds = append(cmds, []any{"ZINCRBY", total, 1, winner})
		if ranked {
			cmds = append(cmds, []any{"ZINCRBY", leaderboard, 1, winner})
		}
	}
	if s.Tracks(loser) {
		cmds = append(cmds, []any{"HINCRBY", key(loser), "losses", 1}, []any{"HINCRBY", key(loser), "games", 1})
	}
	if len(cmds) == 0 {
		return nil
	}
	_, err := s.do(cmds...)
	return err
}

// Heartbeat says the server is accepting games for the next ttl. The landing page reads this
// key to decide whether to tell a visitor the game is worth connecting to.
func (s *Store) Heartbeat(ttl time.Duration) error {
	if s == nil {
		return nil
	}
	_, err := s.do([]any{"SET", live, time.Now().Unix(), "EX", int(ttl.Seconds())})
	return err
}

// Offline retires the heartbeat on a clean shutdown, so a scheduled stop shows on the page in
// seconds rather than whenever the key happens to expire.
func (s *Store) Offline() error {
	if s == nil {
		return nil
	}
	_, err := s.do([]any{"DEL", live})
	return err
}

// Top ranks wins against people.
func (s *Store) Top(n int) ([]Entry, error) { return s.board(leaderboard, n) }

// TopTotal ranks every win a tracked player has, bot games included.
func (s *Store) TopTotal(n int) ([]Entry, error) { return s.board(total, n) }

func (s *Store) board(zset string, n int) ([]Entry, error) {
	if s == nil || n <= 0 {
		return nil, nil
	}
	res, err := s.do([]any{"ZRANGE", zset, 0, n - 1, "REV", "WITHSCORES"})
	if err != nil {
		return nil, err
	}
	var flat []json.RawMessage
	if err := json.Unmarshal(res[0], &flat); err != nil {
		return nil, fmt.Errorf("zrange %s: %w", zset, err)
	}

	entries := make([]Entry, 0, len(flat)/2)
	names := make([][]any, 0, len(flat)/2)
	for i := 0; i+1 < len(flat); i += 2 {
		var id string
		if err := json.Unmarshal(flat[i], &id); err != nil {
			return nil, fmt.Errorf("zrange %s: %w", zset, err)
		}
		entries = append(entries, Entry{Wins: score(flat[i+1])})
		names = append(names, []any{"HGET", key(id), "name"})
	}
	if len(entries) == 0 {
		return nil, nil
	}

	found, err := s.do(names...)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		var name *string
		json.Unmarshal(found[i], &name)
		entries[i].Name = text(name)
		if entries[i].Name == "" {
			entries[i].Name = "anonymous"
		}
	}
	return entries, nil
}

// do runs commands through the pipeline endpoint, which takes one command just as happily
// as several, and returns one raw result per command.
func (s *Store) do(cmds ...[]any) ([]json.RawMessage, error) {
	body, err := json.Marshal(cmds)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, s.url+"/pipeline", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("redis responded %s: %s", resp.Status, bytes.TrimSpace(msg))
	}

	var out []struct {
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out) != len(cmds) {
		return nil, fmt.Errorf("redis answered %d of %d commands", len(out), len(cmds))
	}
	results := make([]json.RawMessage, len(out))
	for i, r := range out {
		if r.Error != "" {
			return nil, fmt.Errorf("%s: %s", cmds[i][0], r.Error)
		}
		results[i] = r.Result
	}
	return results, nil
}

func key(id string) string { return namespace + "player:" + id }

func text(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func number(s *string) int {
	n, _ := strconv.Atoi(text(s))
	return n
}

// score reads a zset score, which the REST API returns as a bare number or as a string
// depending on the command.
func score(raw json.RawMessage) int {
	n, err := strconv.ParseFloat(strings.Trim(string(raw), `"`), 64)
	if err != nil {
		return 0
	}
	return int(n)
}
