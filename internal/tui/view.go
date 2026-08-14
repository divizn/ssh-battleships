package tui

import (
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/divizn/ssh-battleships/internal/game"
)

var (
	dim    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	title  = lipgloss.NewStyle().Bold(true)
	ship   = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	hit    = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	legal  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	broken = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	here   = lipgloss.NewStyle().Reverse(true)
)

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(title.Render("BATTLESHIPS") + "\n\n")
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
		m.board(you, "YOUR FLEET"),
		"   ",
		m.board(foe, "ENEMY WATERS"),
	))
	b.WriteString("\n" + m.status() + "\n\n" + dim.Render(m.help()) + "\n")
	return b.String()
}

func (m Model) status() string {
	switch m.phase {
	case placing:
		return "Place your " + game.Fleet[m.next].String()
	case over:
		if m.g.Winner == you {
			return title.Render("You win.")
		}
		return title.Render("The bot wins.")
	default:
		return strings.TrimRight(m.yourShot+"\n"+m.botShot, "\n")
	}
}

func (m Model) help() string {
	switch m.phase {
	case placing:
		return "arrows/hjkl move · r rotate · enter place · R auto-place the rest · q quit"
	case over:
		return "n new game · q quit"
	default:
		return "arrows/hjkl move · enter fire · q quit"
	}
}

// board renders one grid. Ships show on your own board, and on the enemy's once the game is over.
func (m Model) board(p game.Player, heading string) string {
	var b strings.Builder
	b.WriteString(title.Render(heading) + "\n")
	b.WriteString(dim.Render("    A B C D E F G H I J") + "\n")

	ghost := m.ghost(p)
	for row := range game.Size {
		b.WriteString(dim.Render(pad(row+1)) + " ")
		for col := range game.Size {
			b.WriteString(m.cell(p, game.Coord{Row: row, Col: col}, ghost))
			if col < game.Size-1 {
				b.WriteString(" ")
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) cell(p game.Player, c game.Coord, ghost []game.Coord) string {
	style, glyph := dim, "·"
	switch {
	case slices.Contains(ghost, c):
		style, glyph = legal, "#"
		if m.g.Board(p).CanPlace(m.pending()) != nil {
			style = broken
		}
	case m.g.Board(p).At(c) == game.Hit:
		style, glyph = hit, "X"
	case m.g.Board(p).At(c) == game.Miss:
		glyph = "o"
	default:
		if _, ok := m.g.Board(p).ShipAt(c); ok && m.reveals(p) {
			style, glyph = ship, "#"
		}
	}
	if c == m.cursor && p == m.active() {
		style = here
	}
	return style.Render(glyph)
}

// ghost is the outline of the ship currently being positioned, empty outside placement.
func (m Model) ghost(p game.Player) []game.Coord {
	if m.phase != placing || p != you {
		return nil
	}
	var cells []game.Coord
	for _, c := range m.pending().Cells() {
		if c.Valid() {
			cells = append(cells, c)
		}
	}
	return cells
}

func (m Model) reveals(p game.Player) bool {
	return p == you || m.phase == over
}

// active is the board the cursor sits on: your own while placing, the enemy's while firing.
func (m Model) active() game.Player {
	if m.phase == placing {
		return you
	}
	return foe
}

func pad(n int) string {
	if n < 10 {
		return "  " + string(rune('0'+n))
	}
	return " 10"
}
