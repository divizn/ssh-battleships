package main

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	// reads .env before main, leaving anything already in the environment alone
	_ "github.com/joho/godotenv/autoload"

	"github.com/divizn/ssh-battleships/internal/lobby"
	"github.com/divizn/ssh-battleships/internal/server"
	"github.com/divizn/ssh-battleships/internal/store"
	"github.com/divizn/ssh-battleships/internal/tui"
)

func main() {
	addr := flag.String("addr", ":2222", "address to listen on")
	hostKey := flag.String("host-key", ".ssh/battleships_ed25519", "path to the ssh host key")
	local := flag.Bool("local", false, "play in this terminal instead of serving ssh")
	flag.Parse()

	if err := run(*addr, *hostKey, *local); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(addr, hostKey string, local bool) error {
	db := store.New(os.Getenv("UPSTASH_REDIS_REST_URL"), os.Getenv("UPSTASH_REDIS_REST_TOKEN"))
	if db == nil {
		fmt.Fprintln(os.Stderr, "no redis configured, names and scores will not be kept")
	}

	games := lobby.New()
	games.OnResult = func(winner, loser lobby.Player, ranked bool) {
		if err := db.Record(winner.ID, loser.ID, ranked); err != nil {
			fmt.Fprintln(os.Stderr, "recording result:", err)
		}
	}

	if local {
		_, err := tea.NewProgram(tui.New(games, db, me()), tea.WithAltScreen(), tea.WithFilter(tui.CloseOnQuit)).Run()
		return err
	}

	keyPath, err := hostKeyPath(hostKey)
	if err != nil {
		return err
	}

	s, err := server.New(addr, keyPath, games, db)
	if err != nil {
		return err
	}

	errs := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "battleships listening on %s\n", addr)
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			errs <- err
		}
	}()

	stopBeating := heartbeat(db)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errs:
		return err
	case <-stop:
	}
	stopBeating()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// heartbeat keeps the landing page's "live" badge honest, and returns the function that stops
// it. The key outlives one beat but not two, so an instance that is stopped on a schedule, or
// simply dies, shows as offline without anything else having to notice.
func heartbeat(db *store.Store) func() {
	const every = time.Minute

	beat := func() {
		if err := db.Heartbeat(150 * time.Second); err != nil {
			fmt.Fprintln(os.Stderr, "heartbeat:", err)
		}
	}

	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		tick := time.NewTicker(every)
		defer tick.Stop()
		beat()
		for {
			select {
			case <-tick.C:
				beat()
			case <-done:
				return
			}
		}
	}()

	// the DEL waits for the last beat to land, or it could be overwritten by it
	return func() {
		close(done)
		<-stopped
		if err := db.Offline(); err != nil {
			fmt.Fprintln(os.Stderr, "retiring the heartbeat:", err)
		}
	}
}

// hostKeyPath writes the base64 SSH_HOST_KEY secret to disk and returns that path, so a
// deploy that has no persistent volume still presents the same key it did last time.
func hostKeyPath(path string) (string, error) {
	secret := os.Getenv("SSH_HOST_KEY")
	if secret == "" {
		return path, nil
	}
	pem, err := base64.StdEncoding.DecodeString(strings.TrimSpace(secret))
	if err != nil {
		return "", fmt.Errorf("SSH_HOST_KEY is not base64: %w", err)
	}
	// the scratch image this ships in has no /tmp of its own
	if err := os.MkdirAll(os.TempDir(), 0o700); err != nil {
		return "", err
	}
	f := filepath.Join(os.TempDir(), "battleships_host_key")
	return f, os.WriteFile(f, pem, 0o600)
}

func me() lobby.Player {
	name := "player"
	if u, err := user.Current(); err == nil && u.Username != "" {
		name = lobby.CleanName(u.Username)
	}
	return lobby.Player{ID: "local", Name: name}
}
