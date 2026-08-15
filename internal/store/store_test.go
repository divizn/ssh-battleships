package store

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const alice = "SHA256:alice"
const bob = "SHA256:bob"

// fake stands in for Upstash: it answers each pipeline with the next canned reply and keeps
// the commands it was sent so a test can look at them.
type fake struct {
	replies []string
	sent    [][][]any
}

func (f *fake) serve(t *testing.T) *Store {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Errorf("authorization %q, want the bearer token", got)
		}
		body, _ := io.ReadAll(r.Body)
		var cmds [][]any
		if err := json.Unmarshal(body, &cmds); err != nil {
			t.Errorf("request body %s is not a pipeline: %v", body, err)
		}
		f.sent = append(f.sent, cmds)
		if len(f.replies) == 0 {
			t.Errorf("unexpected %d-command call: %s", len(cmds), body)
			http.Error(w, "no reply queued", http.StatusInternalServerError)
			return
		}
		reply := f.replies[0]
		f.replies = f.replies[1:]
		io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "token")
}

func (f *fake) command(t *testing.T, call, index int) string {
	t.Helper()
	if call >= len(f.sent) || index >= len(f.sent[call]) {
		t.Fatalf("no command %d in call %d of %v", index, call, f.sent)
	}
	parts := make([]string, len(f.sent[call][index]))
	for i, p := range f.sent[call][index] {
		parts[i] = toString(p)
	}
	return strings.Join(parts, " ")
}

func toString(v any) string {
	b, _ := json.Marshal(v)
	return strings.Trim(string(b), `"`)
}

func TestProfileReadsTheStoredRecord(t *testing.T) {
	f := &fake{replies: []string{`[{"result":["Alice","3","1","4"]}]`}}
	db := f.serve(t)

	got, err := db.Profile(alice)
	if err != nil {
		t.Fatalf("Profile: %v", err)
	}
	if want := (Profile{Name: "Alice", Wins: 3, Losses: 1, Games: 4}); got != want {
		t.Errorf("Profile = %+v, want %+v", got, want)
	}
	if want := "HMGET " + key(alice) + " name wins losses games"; f.command(t, 0, 0) != want {
		t.Errorf("sent %q, want %q", f.command(t, 0, 0), want)
	}
}

// The instance is shared with daily-quotes, which owns quote:<date>.
func TestEveryKeyIsNamespaced(t *testing.T) {
	for _, k := range []string{key(alice), leaderboard} {
		if !strings.HasPrefix(k, "battleships:") {
			t.Errorf("key %q is loose in a shared database", k)
		}
	}
}

func TestProfileOfANewPlayerIsEmptyRatherThanAnError(t *testing.T) {
	f := &fake{replies: []string{`[{"result":[null,null,null,null]}]`}}

	got, err := f.serve(t).Profile(alice)
	if err != nil || got != (Profile{}) {
		t.Errorf("Profile = %+v, %v, want a zero profile and no error", got, err)
	}
}

func TestRankedResultTouchesTheLeaderboardAndBotGamesDoNot(t *testing.T) {
	ranked := &fake{replies: []string{`[{"result":1},{"result":1},{"result":1},{"result":1},{"result":1}]`}}
	if err := ranked.serve(t).Record(alice, bob, true); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if got := len(ranked.sent[0]); got != 5 {
		t.Fatalf("ranked win sent %d commands, want wins, games, zincrby, losses, games", got)
	}
	if want := "ZINCRBY " + leaderboard + " 1 " + alice; ranked.command(t, 0, 2) != want {
		t.Errorf("sent %q, want %q", ranked.command(t, 0, 2), want)
	}

	bot := &fake{replies: []string{`[{"result":1},{"result":1}]`}}
	if err := bot.serve(t).Record(alice, "bot", false); err != nil {
		t.Fatalf("Record against the bot: %v", err)
	}
	for _, c := range bot.sent[0] {
		if toString(c[1]) == leaderboard {
			t.Errorf("a bot game reached the leaderboard: %v", bot.sent[0])
		}
		if strings.Contains(toString(c[1]), "bot") {
			t.Errorf("the bot got a player record: %v", bot.sent[0])
		}
	}
}

func TestUntrackedPlayersNeverReachRedis(t *testing.T) {
	f := &fake{}
	db := f.serve(t)

	for _, id := range []string{"local", "bot", "anon:deadbeef", ""} {
		if _, err := db.Profile(id); err != nil {
			t.Errorf("Profile(%q): %v", id, err)
		}
		if err := db.SetName(id, "Nobody"); err != nil {
			t.Errorf("SetName(%q): %v", id, err)
		}
		if err := db.Record(id, id, true); err != nil {
			t.Errorf("Record(%q): %v", id, err)
		}
	}
	if len(f.sent) != 0 {
		t.Errorf("untracked ids sent %v", f.sent)
	}
}

func TestTopPairsScoresWithNames(t *testing.T) {
	f := &fake{replies: []string{
		`[{"result":["` + alice + `",5,"` + bob + `","2"]}]`,
		`[{"result":"Alice"},{"result":null}]`,
	}}

	got, err := f.serve(t).Top(8)
	if err != nil {
		t.Fatalf("Top: %v", err)
	}
	want := []Entry{{Name: "Alice", Wins: 5}, {Name: "anonymous", Wins: 2}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Top = %+v, want %+v", got, want)
	}
}

func TestHeartbeatExpiresAndShutdownRetiresIt(t *testing.T) {
	f := &fake{replies: []string{`[{"result":"OK"}]`, `[{"result":1}]`}}
	db := f.serve(t)

	if err := db.Heartbeat(150 * time.Second); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	got := f.command(t, 0, 0)
	if !strings.HasPrefix(got, "SET "+live+" ") || !strings.HasSuffix(got, " EX 150") {
		t.Errorf("sent %q, want a SET on %s expiring in 150 seconds", got, live)
	}

	if err := db.Offline(); err != nil {
		t.Fatalf("Offline: %v", err)
	}
	if want := "DEL " + live; f.command(t, 1, 0) != want {
		t.Errorf("sent %q, want %q", f.command(t, 1, 0), want)
	}
}

func TestAnUnreachableStoreIsAnErrorNotAPanic(t *testing.T) {
	f := &fake{replies: []string{`[{"error":"WRONGTYPE"}]`}}

	if _, err := f.serve(t).Profile(alice); err == nil {
		t.Error("a redis error came back as success")
	}
}

func TestNoRedisMeansNoStoreAndNoCrash(t *testing.T) {
	db := New("", "")
	if db != nil {
		t.Fatal("New with no credentials returned a store")
	}
	if db.Tracks(alice) {
		t.Error("a nil store claims to track players")
	}
	if _, err := db.Profile(alice); err != nil {
		t.Errorf("Profile on a nil store: %v", err)
	}
	if err := db.SetName(alice, "Alice"); err != nil {
		t.Errorf("SetName on a nil store: %v", err)
	}
	if err := db.Record(alice, bob, true); err != nil {
		t.Errorf("Record on a nil store: %v", err)
	}
	if got, err := db.Top(8); got != nil || err != nil {
		t.Errorf("Top on a nil store = %v, %v", got, err)
	}
}
