package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/joho/godotenv"

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
	// Load leaves anything already in the environment alone, so Fly's secrets win and the
	// missing file in production is not an error.
	godotenv.Load()
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
		_, err := tea.NewProgram(tui.New(games, db, me()), tea.WithAltScreen()).Run()
		return err
	}

	s, err := server.New(addr, hostKey, games, db)
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

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errs:
		return err
	case <-stop:
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func me() lobby.Player {
	name := "player"
	if u, err := user.Current(); err == nil && u.Username != "" {
		name = lobby.CleanName(u.Username)
	}
	return lobby.Player{ID: "local", Name: name}
}
