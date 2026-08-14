package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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

func TestPlayingOutTheBoardEndsTheGame(t *testing.T) {
	m := press(New(), "R")

	for range game.Size {
		for col := range game.Size {
			m = press(m, "enter")
			if m.(Model).phase == over {
				if !strings.Contains(m.View(), "wins.") && !strings.Contains(m.View(), "You win.") {
					t.Fatalf("game over but the view announces nothing:\n%s", m.View())
				}
				return
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
}
