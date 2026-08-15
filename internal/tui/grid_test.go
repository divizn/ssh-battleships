package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/divizn/ssh-battleships/internal/game"
)

// Every row carries its own number: the labels used to be capped at 10.
func TestGridLabelsEveryRow(t *testing.T) {
	view := botGame(t).View()
	for row := range game.Size {
		if label := pad(row + 1); !strings.Contains(view, label) {
			t.Errorf("row %d label %q missing from the grid", row+1, label)
		}
	}
	if n := strings.Count(view, pad(game.Size)+" "); n != 2 {
		t.Errorf("last row label appears %d times, want once per board", n)
	}
	if strings.Contains(view, strconv.Itoa(game.Size+1)+" ·") {
		t.Error("grid drew a row past the bottom of the board")
	}
}
