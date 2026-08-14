package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/divizn/ssh-battleships/internal/game"
)

var specials = map[string]tea.KeyType{
	"enter": tea.KeyEnter,
	"up":    tea.KeyUp,
	"down":  tea.KeyDown,
	"left":  tea.KeyLeft,
	"right": tea.KeyRight,
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

func TestAutoPlaceStartsTheGame(t *testing.T) {
	m := press(New(), "R")
	mm := m.(Model)

	if mm.phase != firing {
		t.Fatalf("phase after auto-place = %v, want firing", mm.phase)
	}
	if got := len(mm.g.Board(you).Ships); got != len(game.Fleet) {
		t.Errorf("placed %d ships, want %d", got, len(game.Fleet))
	}
	if !strings.Contains(m.View(), "enter fire") {
		t.Error("help text still shows placement keys")
	}
}

func TestManualPlacementWalksTheFleet(t *testing.T) {
	m := press(New(), "r", "enter") // carrier, rotated, at A1
	mm := m.(Model)

	carrier := mm.g.Board(you).Ships[0]
	if want := (game.Ship{Class: game.Carrier, Origin: game.Coord{}, Vertical: true}); carrier != want {
		t.Fatalf("placed %+v, want %+v", carrier, want)
	}
	if mm.phase != placing {
		t.Errorf("phase = %v, want to still be placing four more ships", mm.phase)
	}
	if !strings.Contains(m.View(), "Place your Battleship") {
		t.Error("prompt did not advance to the next ship")
	}
}

func TestIllegalPlacementIsRejected(t *testing.T) {
	m := press(New(), "r", "down", "down", "down", "down", "down", "down", "enter")
	mm := m.(Model)

	if got := len(mm.g.Board(you).Ships); got != 0 {
		t.Errorf("placed %d ships, want the off-board carrier refused", got)
	}
}

func TestEnemyFleetStaysHiddenUntilTheGameEnds(t *testing.T) {
	m := press(New(), "R")

	if got := strings.Count(m.View(), "#"); got != game.FleetCells {
		t.Errorf("view shows %d ship cells, want only your own %d", got, game.FleetCells)
	}
}

// sweep fires at every cell in turn until the game ends.
func sweep(t *testing.T, m tea.Model) tea.Model {
	t.Helper()
	for range game.Size {
		for col := range game.Size {
			m = press(m, "enter")
			if m.(Model).phase == over {
				return m
			}
			if col < game.Size-1 {
				m = press(m, "right")
			}
		}
		m = press(m, "down")
		for range game.Size - 1 {
			m = press(m, "left")
		}
	}
	t.Fatal("fired at every cell without the game ending")
	return nil
}

func TestPlayingOutTheBoardEndsTheGame(t *testing.T) {
	m := sweep(t, press(New(), "R"))

	if !strings.Contains(m.View(), "wins.") && !strings.Contains(m.View(), "You win.") {
		t.Fatalf("game over but the view announces nothing:\n%s", m.View())
	}
}

func TestFrameKeepsOneSizeInEveryPhase(t *testing.T) {
	placingFrame := New().View()
	firingFrame := press(New(), "R").View()
	overFrame := sweep(t, press(New(), "R")).View()

	for _, f := range []string{firingFrame, overFrame} {
		if lipgloss.Width(f) != lipgloss.Width(placingFrame) || lipgloss.Height(f) != lipgloss.Height(placingFrame) {
			t.Errorf("frame is %dx%d, want a constant %dx%d so the layout does not jump",
				lipgloss.Width(f), lipgloss.Height(f),
				lipgloss.Width(placingFrame), lipgloss.Height(placingFrame))
		}
	}
	if w, h := lipgloss.Width(placingFrame), lipgloss.Height(placingFrame); w > 80 || h > 24 {
		t.Errorf("frame is %dx%d, want it to fit a stock 80x24 terminal", w, h)
	}
}

func TestSmallTerminalAsksForAResize(t *testing.T) {
	m, _ := New().Update(tea.WindowSizeMsg{Width: 40, Height: 12})

	if !strings.Contains(m.View(), "Terminal too small") {
		t.Errorf("view in a 40x12 window did not ask for a resize:\n%s", m.View())
	}
}

func TestSinkIsAnnouncedAndRingsTheBell(t *testing.T) {
	m := New()
	m.g.Boards[foe] = game.Board{}
	for _, s := range []game.Ship{
		{Class: game.Destroyer, Origin: game.Coord{}},
		{Class: game.Carrier, Origin: game.Coord{Row: 9}}, // so the destroyer sinking is not the whole war
	} {
		if err := m.g.Board(foe).Place(s); err != nil {
			t.Fatal(err)
		}
	}
	after := press(m, "R", "enter", "right", "enter")

	if !strings.Contains(after.View(), "SANK the Destroyer at B1") {
		t.Errorf("sinking the destroyer was not announced:\n%s", after.View())
	}
	if !after.(Model).bell {
		t.Error("bell did not ring on a sink")
	}
	if after = press(after, "right"); after.(Model).bell {
		t.Error("bell still ringing a keypress later")
	}
}

func TestRosterStrikesSunkShipsOnly(t *testing.T) {
	m := New()
	m.g.Boards[foe] = game.Board{}
	for _, s := range []game.Ship{
		{Class: game.Destroyer, Origin: game.Coord{}},
		{Class: game.Carrier, Origin: game.Coord{Row: 9}}, // so the destroyer sinking is not the whole war
	} {
		if err := m.g.Board(foe).Place(s); err != nil {
			t.Fatal(err)
		}
	}
	after := press(m, "R", "enter").(Model)

	if got := after.roster(foe); got != plainRoster {
		t.Errorf("roster after a hit but no sink = %q, want %q", got, plainRoster)
	}
	if got := press(after, "right", "enter").(Model).roster(foe); got == plainRoster {
		t.Error("roster is unchanged after the destroyer sank")
	}
}

const plainRoster = "·Carrier  ·Battleship  ·Cruiser\n·Submarine  ·Destroyer"
