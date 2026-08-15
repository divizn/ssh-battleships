package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/divizn/ssh-battleships/internal/game"
	"github.com/divizn/ssh-battleships/internal/lobby"
	"github.com/divizn/ssh-battleships/internal/store"
)

type screen int

const (
	menu screen = iota
	joining
	playing
	naming
)

type choice int

const (
	playBot choice = iota
	createRoom
	joinRoom
)

var menuItems = []string{"Play the bot", "Create a room", "Join with a code"}

// topPlayers is how many leaderboard places fit beside the menu.
const topPlayers = 8

// roomGone arrives when the room shuts down under us.
type roomGone struct{}

// loaded carries what the database knows about this player, or nothing at all when it is
// unreachable. A game is playable either way, so the error only ever becomes a notice.
type loaded struct {
	profile store.Profile
	top     []store.Entry
	err     error
}

type Model struct {
	lobby *lobby.Lobby
	db    *store.Store
	me    lobby.Player
	st    styles

	screen screen
	choice choice
	typed  string
	notice string

	profile store.Profile
	top     []store.Entry

	sess *lobby.Session
	snap lobby.Snapshot
	live bool

	cursor        game.Coord
	vertical      bool
	bell          bool
	width, height int
}

func New(l *lobby.Lobby, db *store.Store, me lobby.Player) Model {
	return NewWithRenderer(l, db, me, lipgloss.DefaultRenderer())
}

// NewWithRenderer builds a model that draws through r, which for an SSH session must be the
// renderer bound to that session rather than the server's own terminal.
func NewWithRenderer(l *lobby.Lobby, db *store.Store, me lobby.Player, r *lipgloss.Renderer) Model {
	return Model{lobby: l, db: db, me: me, st: newStyles(r)}
}

func (m Model) Init() tea.Cmd {
	return m.load()
}

// load fetches the player's record and the leaderboard together, since the menu shows both.
func (m Model) load() tea.Cmd {
	if m.db == nil {
		return nil
	}
	db, id := m.db, m.me.ID
	return func() tea.Msg {
		profile, err := db.Profile(id)
		top, topErr := db.Top(topPlayers)
		if err == nil {
			err = topErr
		}
		return loaded{profile: profile, top: top, err: err}
	}
}

func (m Model) saveName(name string) tea.Cmd {
	db, id := m.db, m.me.ID
	return func() tea.Msg {
		if err := db.SetName(id, name); err != nil {
			return loaded{profile: store.Profile{Name: name}, err: err}
		}
		return nil
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case lobby.Snapshot:
		m.bell = sunkSince(m.snap, msg) && m.live
		m.snap, m.live = msg, true
		return m, listen(m.sess)

	case roomGone:
		return m.leave("That room has closed.")

	case loaded:
		return m.settle(msg), nil

	case tea.KeyMsg:
		m.bell = false
		switch msg.String() {
		case "ctrl+c":
			return m, m.quit()
		}
		switch m.screen {
		case menu:
			return m.updateMenu(msg.String())
		case joining:
			return m.updateJoining(msg.String())
		case naming:
			return m.updateNaming(msg.String())
		default:
			return m.updatePlaying(msg.String())
		}
	}
	return m, nil
}

// settle takes what the database returned. A player with no name stored has never been here
// before, so they are asked for one.
func (m Model) settle(msg loaded) Model {
	if msg.err != nil {
		m.notice = "Scores are unavailable right now."
		return m
	}
	m.profile, m.top = msg.profile, msg.top
	if msg.profile.Name != "" {
		m.me.Name = msg.profile.Name
		return m
	}
	if m.screen == menu && m.db.Tracks(m.me.ID) {
		m.screen, m.typed, m.notice = naming, m.me.Name, ""
	}
	return m
}

func (m Model) updateNaming(key string) (tea.Model, tea.Cmd) {
	switch {
	case key == "backspace":
		if m.typed != "" {
			m.typed = m.typed[:len(m.typed)-1]
		}
	case key == "enter":
		name := lobby.CleanName(m.typed)
		if name == "" {
			m.notice = "Pick something with a letter or two in it."
			return m, nil
		}
		m.me.Name, m.profile.Name = name, name
		m.screen, m.typed, m.notice = menu, "", ""
		return m, m.saveName(name)
	case len(key) == 1 && len(m.typed) < lobby.NameLimit:
		m.typed += key
	}
	return m, nil
}

func (m Model) updateMenu(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q":
		return m, m.quit()
	case "up", "k":
		m.choice = choice(max(int(m.choice)-1, 0))
	case "down", "j":
		m.choice = choice(min(int(m.choice)+1, len(menuItems)-1))
	case "enter", " ":
		switch m.choice {
		case playBot:
			return m.enter(m.lobby.Bot(m.me))
		case createRoom:
			return m.enter(m.lobby.Create(m.me))
		default:
			m.screen, m.typed, m.notice = joining, "", ""
		}
	}
	return m, nil
}

func (m Model) updateJoining(key string) (tea.Model, tea.Cmd) {
	switch {
	case key == "esc":
		m.screen, m.notice = menu, ""
	case key == "backspace":
		if m.typed != "" {
			m.typed = m.typed[:len(m.typed)-1]
		}
	case key == "enter":
		if len(m.typed) < 4 {
			m.notice = "A room code is four letters."
			return m, nil
		}
		return m.enter(m.lobby.Join(m.typed, m.me))
	case len(key) == 1 && isLetter(key[0]) && len(m.typed) < 4:
		m.typed += strings.ToUpper(key)
	}
	return m, nil
}

func (m Model) updatePlaying(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q":
		return m, m.quit()
	case "esc":
		return m.leave("")
	}
	if !m.live {
		return m, nil
	}

	if step, ok := steps[key]; ok {
		m.cursor.Row = clamp(m.cursor.Row + step.Row)
		m.cursor.Col = clamp(m.cursor.Col + step.Col)
		return m, nil
	}

	switch m.snap.Phase {
	case lobby.Placing:
		return m.updatePlacing(key), nil
	case lobby.Firing:
		if key == "enter" || key == " " {
			m.notice = errText(m.sess.Fire(m.cursor))
		}
	case lobby.Over:
		if key == "n" {
			return m.leave("")
		}
	}
	return m, nil
}

func (m Model) updatePlacing(key string) Model {
	switch key {
	case "r":
		m.vertical = !m.vertical
	case "R":
		m.notice = errText(m.sess.AutoPlace())
	case "enter", " ":
		if ship, ok := m.pending(); ok {
			m.notice = errText(m.sess.Place(ship))
		}
	}
	return m
}

// enter attaches to a freshly opened or joined room.
func (m Model) enter(s *lobby.Session, err error) (tea.Model, tea.Cmd) {
	if err != nil {
		m.notice = capitalise(err.Error()) + "."
		return m, nil
	}
	m.sess, m.live = s, false
	m.screen, m.notice = playing, ""
	m.cursor, m.vertical = game.Coord{}, false
	return m, listen(s)
}

// leave drops the room but keeps the connection, so a player can start another game. The
// reload picks up whatever that game just did to their record.
func (m Model) leave(notice string) (Model, tea.Cmd) {
	if m.sess != nil {
		m.sess.Close()
		m.sess = nil
	}
	m.live, m.screen, m.notice = false, menu, notice
	m.snap = lobby.Snapshot{}
	return m, m.load()
}

func (m Model) quit() tea.Cmd {
	if m.sess != nil {
		m.sess.Close()
	}
	return tea.Quit
}

func listen(s *lobby.Session) tea.Cmd {
	if s == nil {
		return nil
	}
	return func() tea.Msg {
		snap, ok := <-s.Events()
		if !ok {
			return roomGone{}
		}
		return snap
	}
}

// sunkSince reports whether the new snapshot carries a sinking the old one did not.
func sunkSince(old, next lobby.Snapshot) bool {
	for i := range next.Last {
		shot := next.Last[i]
		if shot.Set && shot.Res.Sunk && shot != old.Last[i] {
			return true
		}
	}
	return false
}

// pending is the next ship still to be positioned, if any are left.
func (m Model) pending() (game.Ship, bool) {
	board := m.snap.Game.Board(m.snap.Seat)
	for _, class := range game.Fleet {
		if !board.Placed(class) {
			return game.Ship{Class: class, Origin: m.cursor, Vertical: m.vertical}, true
		}
	}
	return game.Ship{}, false
}

func (m Model) mine() game.Player   { return m.snap.Seat }
func (m Model) theirs() game.Player { return m.snap.Seat.Other() }

var steps = map[string]game.Coord{
	"up": {Row: -1}, "k": {Row: -1},
	"down": {Row: 1}, "j": {Row: 1},
	"left": {Col: -1}, "h": {Col: -1},
	"right": {Col: 1}, "l": {Col: 1},
}

func clamp(n int) int {
	return min(max(n, 0), game.Size-1)
}

func isLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return capitalise(err.Error()) + "."
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func label(c game.Coord) string {
	return fmt.Sprintf("%c%d", 'A'+rune(c.Col), c.Row+1)
}
