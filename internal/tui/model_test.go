package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/divizn/ssh-battleships/internal/game"
	"github.com/divizn/ssh-battleships/internal/lobby"
	"github.com/divizn/ssh-battleships/internal/store"
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
	return New(l, nil, lobby.Player{ID: "key-" + name, Name: name})
}

// known is a model for a player Redis would recognise. The store is never actually reached:
// these tests hand the model the answers a lookup would have produced.
func known(l *lobby.Lobby) Model {
	db := store.New("http://redis.invalid", "token")
	return New(l, db, lobby.Player{ID: "SHA256:tester", Name: "sshname"})
}

// fullBoard is the widest leaderboard the pane ever has to hold.
var fullBoard = loaded{
	profile: store.Profile{Name: "Tester", Wins: 12, Losses: 3, Games: 15},
	top:     board(topPlayers),
}

func board(n int) []store.Entry {
	entries := make([]store.Entry, n)
	for i := range entries {
		entries[i] = store.Entry{Name: strings.Repeat("W", lobby.NameLimit), Wins: 999 - i}
	}
	return entries
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
	if want := (game.Ship{Class: game.Carrier, Rotation: 1}); ships[0] != want {
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
		"menu":        newModel(l, "Tester").View(),
		"joining":     press(newModel(l, "Tester"), "down", "down", "enter").View(),
		"waiting":     settle(t, press(newModel(l, "Tester"), "down", "enter")).View(),
		"playing":     botGame(t).View(),
		"naming":      known(l).settle(loaded{}).View(),
		"leaderboard": known(l).settle(fullBoard).View(),
	}

	want := frames["menu"]
	for name, f := range frames {
		if lipgloss.Width(f) != lipgloss.Width(want) || lipgloss.Height(f) != lipgloss.Height(want) {
			t.Errorf("%s frame is %dx%d, want a constant %dx%d", name,
				lipgloss.Width(f), lipgloss.Height(f), lipgloss.Width(want), lipgloss.Height(want))
		}
	}
	if w, h := lipgloss.Width(want), lipgloss.Height(want); w > 90 || h > 34 {
		t.Errorf("frame is %dx%d, want it to stay within 90x34", w, h)
	}
}

func TestSmallTerminalAsksForAResize(t *testing.T) {
	m, _ := newModel(lobby.New(), "Tester").Update(tea.WindowSizeMsg{Width: 40, Height: 12})

	if !strings.Contains(m.View(), "Terminal too small") {
		t.Errorf("view in a 40x12 window did not ask for a resize:\n%s", m.View())
	}
}

func TestAPlayerWithNoStoredNameIsAskedForOne(t *testing.T) {
	m := known(lobby.New()).settle(loaded{})

	if m.screen != naming {
		t.Fatalf("screen = %v, want the name prompt", m.screen)
	}
	if m.typed != "sshname" {
		t.Errorf("prompt starts at %q, want the ssh username offered as a default", m.typed)
	}

	after := press(m, "\x1b", "[", "3", "1", "m", "!", "enter").(Model)
	if after.screen != menu {
		t.Fatalf("screen = %v after naming, want the menu", after.screen)
	}
	if after.me.Name != "sshname31m" {
		t.Errorf("name = %q, want the escape sequence stripped out of it", after.me.Name)
	}
	if !strings.Contains(after.View(), "Playing as sshname31m") {
		t.Errorf("the menu does not greet the new name:\n%s", after.View())
	}
}

func TestAKnownPlayerGoesStraightToTheMenuWithTheirRecord(t *testing.T) {
	m := known(lobby.New()).settle(loaded{profile: store.Profile{Name: "Ada", Wins: 3, Losses: 1, Games: 4}})

	if m.screen != menu {
		t.Fatalf("screen = %v, want the menu", m.screen)
	}
	view := m.View()
	for _, want := range []string{"Playing as Ada", "3 won", "1 lost", "4 games"} {
		if !strings.Contains(view, want) {
			t.Errorf("menu is missing %q:\n%s", want, view)
		}
	}
}

func TestTheLeaderboardIsOnTheMenuWhenThereIsOne(t *testing.T) {
	l := lobby.New()
	if strings.Contains(known(l).settle(loaded{profile: store.Profile{Name: "Ada"}}).View(), "LEADERBOARD") {
		t.Error("an empty leaderboard was drawn anyway")
	}

	view := known(l).settle(fullBoard).View()
	if !strings.Contains(view, "LEADERBOARD") {
		t.Fatalf("the leaderboard is missing:\n%s", view)
	}
	if got := strings.Count(view, "999"); got != 1 {
		t.Errorf("the top score appears %d times, want once:\n%s", got, view)
	}
	if !strings.Contains(view, "Play the bot") {
		t.Error("the leaderboard pushed the menu off the pane")
	}
}

func TestAnUnreachableStoreStillLetsYouPlay(t *testing.T) {
	m := known(lobby.New()).settle(loaded{err: errUnreachable})

	if m.screen != menu {
		t.Errorf("screen = %v, want the menu even with no database", m.screen)
	}
	if !strings.Contains(m.View(), "Playing as sshname") {
		t.Errorf("the ssh username was not used as a fallback:\n%s", m.View())
	}
}

var errUnreachable = errors.New("dial tcp: no such host")

func TestLeavingAGameReturnsToTheMenu(t *testing.T) {
	m := press(botGame(t), "esc").(Model)

	if m.screen != menu || m.sess != nil {
		t.Errorf("screen = %v with session %v, want back at the menu with no room", m.screen, m.sess)
	}
}

// A keyless session plays perfectly well but nothing it does is kept, which is worth saying
// before the game rather than discovering afterwards that a real win vanished.
func TestKeylessSessionIsToldItIsUnranked(t *testing.T) {
	db := store.New("http://redis.invalid", "token")
	m := New(lobby.New(), db, lobby.Player{ID: "anon:deadbeef", Name: "juanm"})
	if got := m.View(); !strings.Contains(got, "No SSH key") {
		t.Errorf("a keyless session was not warned:\n%s", got)
	}
}

// With no database at all there is nothing the player could do, so the menu says nothing.
func TestNoDatabaseMeansNoWarning(t *testing.T) {
	m := New(lobby.New(), nil, lobby.Player{ID: "anon:deadbeef", Name: "juanm"})
	if got := m.View(); strings.Contains(got, "No SSH key") {
		t.Errorf("warned about keys with no database configured:\n%s", got)
	}
}
