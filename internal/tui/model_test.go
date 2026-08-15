package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/divizn/ssh-battleships/internal/game"
	"github.com/divizn/ssh-battleships/internal/lobby"
)

var specials = map[string]tea.KeyType{
	"enter":     tea.KeyEnter,
	"esc":       tea.KeyEsc,
	"backspace": tea.KeyBackspace,
	"up":        tea.KeyUp,
	"down":      tea.KeyDown,
	"left":      tea.KeyLeft,
	"right":     tea.KeyRight,
}

func press(m tea.Model, keys ...string) tea.Model {
	for _, k := range keys {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		if t, ok := specials[k]; ok {
			msg = tea.KeyMsg{Type: t}
		}
		m, _ = m.Update(msg)
	}
	return m
}

// settle feeds the model every snapshot the room has published, standing in for the Bubble
// Tea runtime pumping the listen command.
func settle(t *testing.T, m tea.Model) tea.Model {
	t.Helper()
	for range 20 {
		sess := m.(Model).sess
		if sess == nil {
			return m
		}
		select {
		case snap, ok := <-sess.Events():
			if !ok {
				m, _ = m.Update(roomGone{})
				return m
			}
			m, _ = m.Update(snap)
		case <-time.After(100 * time.Millisecond):
			return m
		}
	}
	return m
}

func newModel(l *lobby.Lobby, name string) tea.Model {
	return New(l, lobby.Player{ID: "key-" + name, Name: name})
}

// botGame walks the menu into a bot game with both fleets down.
func botGame(t *testing.T) tea.Model {
	t.Helper()
	m := settle(t, press(newModel(lobby.New(), "Tester"), "enter"))
	m = settle(t, press(m, "R"))
	if got := m.(Model).snap.Phase; got != lobby.Firing {
		t.Fatalf("phase after auto-placing against the bot = %v, want Firing", got)
	}
	return m
}

func TestMenuOffersThreeGames(t *testing.T) {
	m := newModel(lobby.New(), "Tester")

	view := m.View()
	for _, item := range menuItems {
		if !strings.Contains(view, item) {
			t.Errorf("menu is missing %q", item)
		}
	}
	if got := press(m, "down", "down", "down").(Model).choice; got != joinRoom {
		t.Errorf("selection ran past the end of the menu to %v", got)
	}
	if got := press(m, "up").(Model).choice; got != playBot {
		t.Errorf("selection ran past the top of the menu to %v", got)
	}
}

func TestBotGameSeatsYouAndTheBot(t *testing.T) {
	m := settle(t, press(newModel(lobby.New(), "Tester"), "enter")).(Model)

	if m.screen != playing {
		t.Fatalf("screen = %v, want playing", m.screen)
	}
	if !m.snap.VsBot || m.snap.Phase != lobby.Placing {
		t.Errorf("snapshot = phase %v vsBot %v, want Placing against the bot", m.snap.Phase, m.snap.VsBot)
	}
	if !strings.Contains(m.View(), "Place your Carrier") {
		t.Errorf("placement prompt missing:\n%s", m.View())
	}
}

func TestPlacementWalksTheFleetThroughTheRoom(t *testing.T) {
	m := settle(t, press(newModel(lobby.New(), "Tester"), "enter"))
	m = settle(t, press(m, "r", "enter")) // carrier, rotated, at A1

	mm := m.(Model)
	ships := mm.snap.Game.Board(mm.snap.Seat).Ships
	if len(ships) != 1 {
		t.Fatalf("board holds %d ships, want the carrier only", len(ships))
	}
	if want := (game.Ship{Class: game.Carrier, Vertical: true}); ships[0] != want {
		t.Errorf("placed %+v, want %+v", ships[0], want)
	}
	if !strings.Contains(m.View(), "Place your Battleship") {
		t.Error("prompt did not advance to the next ship")
	}
}

func TestTouchingPlacementIsRefusedWithAReason(t *testing.T) {
	m := settle(t, press(newModel(lobby.New(), "Tester"), "enter"))
	m = settle(t, press(m, "enter"))         // carrier along row 1
	m = settle(t, press(m, "down", "enter")) // battleship directly underneath

	mm := m.(Model)
	if got := len(mm.snap.Game.Board(mm.snap.Seat).Ships); got != 1 {
		t.Fatalf("board holds %d ships, want the touching one refused", got)
	}
	if !strings.Contains(mm.notice, "touch") {
		t.Errorf("notice = %q, want it to explain the ships cannot touch", mm.notice)
	}
}

func TestFiringDrawsAReplyFromTheBot(t *testing.T) {
	m := settle(t, press(botGame(t), "enter")).(Model)

	if !m.snap.Last[m.mine()].Set {
		t.Error("your own shot was not recorded")
	}
	if !m.snap.Last[m.theirs()].Set {
		t.Error("the bot did not shoot back")
	}
	if m.snap.Game.Turn != m.mine() {
		t.Errorf("turn = %v, want it back with you", m.snap.Game.Turn)
	}
}

func TestEnemyFleetStaysHiddenWhileTheGameRuns(t *testing.T) {
	m := botGame(t)

	if got := strings.Count(m.View(), "#"); got != game.FleetCells {
		t.Errorf("view shows %d ship cells, want only your own %d", got, game.FleetCells)
	}
}

func TestCreatedRoomShowsACodeAFriendCanJoinWith(t *testing.T) {
	l := lobby.New()
	host := settle(t, press(newModel(l, "Alice"), "down", "enter"))

	code := host.(Model).snap.Code
	if len(code) != 4 {
		t.Fatalf("room code %q, want four letters", code)
	}
	if !strings.Contains(host.View(), strings.Join(strings.Split(code, ""), " ")) {
		t.Errorf("the room code is not on screen:\n%s", host.View())
	}

	guest := press(newModel(l, "Bob"), "down", "down", "enter")
	if got := guest.(Model).screen; got != joining {
		t.Fatalf("screen = %v, want the code entry", got)
	}
	guest = settle(t, press(guest, append(strings.Split(strings.ToLower(code), ""), "enter")...))

	gm := guest.(Model)
	if gm.screen != playing {
		t.Fatalf("guest screen = %v after typing %q, notice %q", gm.screen, code, gm.notice)
	}
	if gm.snap.Seat != game.P2 || gm.snap.Opponent.Name != "Alice" {
		t.Errorf("guest seated at %v against %q, want P2 against Alice", gm.snap.Seat, gm.snap.Opponent.Name)
	}
	if got := settle(t, host).(Model).snap.Opponent.Name; got != "Bob" {
		t.Errorf("host sees opponent %q, want Bob", got)
	}
}

func TestJoiningAMissingRoomSaysSo(t *testing.T) {
	m := press(newModel(lobby.New(), "Tester"), "down", "down", "enter")
	m = press(m, "z", "z", "z", "z", "enter")

	mm := m.(Model)
	if mm.screen != joining {
		t.Errorf("screen = %v, want to stay on the code entry", mm.screen)
	}
	if !strings.Contains(strings.ToLower(mm.notice), "no room") {
		t.Errorf("notice = %q, want it to say the room does not exist", mm.notice)
	}
}

func TestCodeEntryTakesFourLettersAndBackspace(t *testing.T) {
	m := press(newModel(lobby.New(), "Tester"), "down", "down", "enter")
	m = press(m, "a", "b", "c", "d", "e")

	if got := m.(Model).typed; got != "ABCD" {
		t.Errorf("typed %q, want it capped at ABCD", got)
	}
	if got := press(m, "backspace").(Model).typed; got != "ABC" {
		t.Errorf("after backspace typed %q, want ABC", got)
	}
}

func TestFrameKeepsOneSizeOnEveryScreen(t *testing.T) {
	l := lobby.New()
	frames := map[string]string{
		"menu":    newModel(l, "Tester").View(),
		"joining": press(newModel(l, "Tester"), "down", "down", "enter").View(),
		"waiting": settle(t, press(newModel(l, "Tester"), "down", "enter")).View(),
		"playing": botGame(t).View(),
	}

	want := frames["menu"]
	for name, f := range frames {
		if lipgloss.Width(f) != lipgloss.Width(want) || lipgloss.Height(f) != lipgloss.Height(want) {
			t.Errorf("%s frame is %dx%d, want a constant %dx%d", name,
				lipgloss.Width(f), lipgloss.Height(f), lipgloss.Width(want), lipgloss.Height(want))
		}
	}
	if w, h := lipgloss.Width(want), lipgloss.Height(want); w > 80 || h > 24 {
		t.Errorf("frame is %dx%d, want it to fit a stock 80x24 terminal", w, h)
	}
}

func TestSmallTerminalAsksForAResize(t *testing.T) {
	m, _ := newModel(lobby.New(), "Tester").Update(tea.WindowSizeMsg{Width: 40, Height: 12})

	if !strings.Contains(m.View(), "Terminal too small") {
		t.Errorf("view in a 40x12 window did not ask for a resize:\n%s", m.View())
	}
}

func TestLeavingAGameReturnsToTheMenu(t *testing.T) {
	m := press(botGame(t), "esc").(Model)

	if m.screen != menu || m.sess != nil {
		t.Errorf("screen = %v with session %v, want back at the menu with no room", m.screen, m.sess)
	}
}
