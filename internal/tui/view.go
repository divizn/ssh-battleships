package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/divizn/ssh-battleships/internal/game"
)

const (
	colWidth  = 32
	gap       = "      "
	bodyWidth = colWidth*2 + len(gap)
)

// Two greys, both adaptive: the terminal background decides whether muted means darker or
// lighter. dim carries text that still has to be read, water only ever suggests an edge.
var (
	grey    = lipgloss.AdaptiveColor{Light: "241", Dark: "248"}
	greyer  = lipgloss.AdaptiveColor{Light: "249", Dark: "242"}
	dim     = lipgloss.NewStyle().Foreground(grey)
	water   = lipgloss.NewStyle().Foreground(greyer)
	title   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	heading = lipgloss.NewStyle().Bold(true)
	ship    = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	hurt    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	hit     = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	sunk    = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Strikethrough(true)
	legal   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	broken  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	here    = lipgloss.NewStyle().Reverse(true)
	win     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	lose    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
	box     = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(greyer).
		Padding(0, 2)
	column = lipgloss.NewStyle().Width(colWidth)
	banner = lipgloss.NewStyle().Width(bodyWidth).Align(lipgloss.Center)
	body   = lipgloss.NewStyle().Width(bodyWidth)
)

func (m Model) View() string {
	frame := box.Render(body.Render(lipgloss.JoinVertical(lipgloss.Left,
		banner.Render(title.Render("B A T T L E S H I P S")),
		lipgloss.JoinHorizontal(lipgloss.Top,
			m.side(you, "YOUR FLEET"),
			gap,
			m.side(foe, "ENEMY WATERS"),
		),
		"",
		m.status(),
		"",
		dim.Render(m.help()),
	)))

	if m.width == 0 {
		return frame
	}
	if m.width < lipgloss.Width(frame) || m.height < lipgloss.Height(frame) {
		return fmt.Sprintf("Terminal too small.\n\nBattleships needs %d x %d, this window is %d x %d.\n",
			lipgloss.Width(frame), lipgloss.Height(frame), m.width, m.height)
	}

	view := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, frame)
	if m.bell {
		view += "\a"
	}
	return view
}

// side stacks one player's grid, fleet roster and shot tally into a single column.
func (m Model) side(p game.Player, name string) string {
	return column.Render(lipgloss.JoinVertical(lipgloss.Left,
		heading.Render(name),
		m.grid(p),
		"",
		m.roster(p),
		m.tally(p),
	))
}

func (m Model) grid(p game.Player) string {
	var b strings.Builder
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
	style, glyph := water, "·"
	switch {
	case slices.Contains(ghost, c):
		style, glyph = legal, "#"
		if m.g.Board(p).CanPlace(m.pending()) != nil {
			style = broken
		}
	case m.g.Board(p).At(c) == game.Hit:
		style, glyph = hit, "X"
	case m.g.Board(p).At(c) == game.Miss:
		style, glyph = dim, "o"
	case m.g.Board(p).At(c) == game.Water:
		glyph = "×"
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

// roster names each ship behind a marker: afloat, damaged, or sunk. The marker carries the
// state on its own, since plenty of SSH clients drop colour and strikethrough.
func (m Model) roster(p game.Player) string {
	names := make([]string, 0, len(game.Fleet))
	for _, class := range game.Fleet {
		s, ok := m.g.Board(p).Ship(class)
		switch {
		case !ok:
			names = append(names, dim.Render("·"+class.String()))
		case s.Sunk():
			names = append(names, broken.Render("x")+sunk.Render(class.String()))
		case s.Hits > 0 && p == you:
			names = append(names, hurt.Render("!"+class.String()))
		default:
			names = append(names, water.Render("·")+class.String())
		}
	}
	return strings.Join(names[:3], "  ") + "\n" + strings.Join(names[3:], "  ")
}

// tally summarises the shots taken at one board, named for whoever is firing them.
func (m Model) tally(p game.Player) string {
	fired, hits := m.g.Board(p).Tally()
	pct := 0
	if fired > 0 {
		pct = hits * 100 / fired
	}
	who := "you:"
	if p == you {
		who = "bot:"
	}
	return dim.Render(fmt.Sprintf("%s %s · %s · %d%%", who, plural(hits, "hit"), plural(fired, "shot"), pct))
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func (m Model) status() string {
	var first, second string
	switch m.phase {
	case placing:
		first = fmt.Sprintf("Place your %s, %d of %d.", game.Fleet[m.next], m.next+1, len(game.Fleet))
		second = dim.Render(map[bool]string{true: "vertical", false: "horizontal"}[m.vertical])
	case over:
		if m.g.Winner == you {
			first = win.Render("You win. The enemy fleet is on the bottom.")
		} else {
			first = lose.Render("The bot wins. Your fleet is on the bottom.")
		}
		second = m.tally(foe)
	default:
		first = m.yourShot.render(true)
		second = m.botShot.render(false)
	}
	return first + "\n" + second
}

func (s shotLog) render(mine bool) string {
	if !s.set {
		return ""
	}
	if s.note != "" {
		return dim.Render(s.note)
	}

	who := map[bool]string{true: "You", false: "The bot"}[mine]
	switch {
	case s.res.Sunk:
		style := lose
		if mine {
			style = win
		}
		return style.Render(fmt.Sprintf("%s SANK the %s at %s.", who, s.res.Class, label(s.at)))
	case s.res.Hit:
		return hurt.Render(fmt.Sprintf("%s hit the %s at %s.", who, s.res.Class, label(s.at)))
	default:
		return dim.Render(fmt.Sprintf("%s missed at %s.", who, label(s.at)))
	}
}

func (m Model) help() string {
	switch m.phase {
	case placing:
		return "hjkl/arrows move · r rotate · enter place · R auto-place · q quit"
	case over:
		return "n new game · q quit"
	default:
		return "hjkl/arrows move · enter fire · q quit"
	}
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
