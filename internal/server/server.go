package server

import (
	"crypto/rand"
	"encoding/hex"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
	gossh "golang.org/x/crypto/ssh"

	"github.com/divizn/ssh-battleships/internal/lobby"
	"github.com/divizn/ssh-battleships/internal/store"
	"github.com/divizn/ssh-battleships/internal/tui"
)

// New builds the SSH server. hostKey must survive redeploys: a new one makes every returning
// player's client shout REMOTE HOST IDENTIFICATION HAS CHANGED.
func New(addr, hostKey string, l *lobby.Lobby, db *store.Store) (*ssh.Server, error) {
	return wish.NewServer(
		wish.WithAddress(addr),
		wish.WithHostKeyPath(hostKey),
		wish.WithPublicKeyAuth(func(ssh.Context, ssh.PublicKey) bool { return true }),
		wish.WithKeyboardInteractiveAuth(func(ssh.Context, gossh.KeyboardInteractiveChallenge) bool { return true }),
		wish.WithMiddleware(
			bubbletea.Middleware(handler(l, db)),
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
}

func handler(l *lobby.Lobby, db *store.Store) bubbletea.Handler {
	return func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
		model := tui.NewWithRenderer(l, db, playerFor(s), bubbletea.MakeRenderer(s))
		return model, []tea.ProgramOption{tea.WithAltScreen()}
	}
}

// playerFor identifies a connection by its public key fingerprint, which is what lets a
// dropped player reconnect into their own seat. Without a key there is nothing stable to
// recognise, so the session gets a throwaway id and simply cannot reconnect.
func playerFor(s ssh.Session) lobby.Player {
	name := lobby.CleanName(s.User())
	if name == "" {
		name = "player"
	}
	if key := s.PublicKey(); key != nil {
		return lobby.Player{ID: gossh.FingerprintSHA256(key), Name: name}
	}
	return lobby.Player{ID: "anon:" + nonce(), Name: name}
}

func nonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "0"
	}
	return hex.EncodeToString(b)
}
