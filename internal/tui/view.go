package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/divizn/ssh-battleships/internal/game"
	"github.com/divizn/ssh-battleships/internal/lobby"
)

const (
	colWidth  = 36
	gap       = "      "
	bodyWidth = colWidth*2 + len(gap)
)

func (m Model) View() string {
	frame := m.st.box.Render(m.st.body.Render(lipgloss.JoinVertical(lipgloss.Left,
		m.st.banner.Render(m.st.title.Render("B A T T L E S H I P S")),
		m.pane(),
		"",
		m.status(),
		"",
		m.st.dim.Render(m.help()),
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

// pane is the tall middle of the frame, a fixed height so the border never jumps.
func (m Model) pane() string {
	var content string
	switch {
	case m.screen == menu:
		content = m.menuPane()
	case m.screen == naming:
		content = m.namePane()
	case m.screen == joining:
		content = m.joinPane()
	case m.screen == unranked:
		content = m.unrankedPane()
	case !m.live || m.snap.Phase == lobby.Waiting:
		content = m.waitPane()
	default:
		content = lipgloss.JoinHorizontal(lipgloss.Top,
			m.side(m.mine(), "YOUR FLEET"),
			gap,
			m.side(m.theirs(), enemyHeading(m.snap)),
		)
	}
	return lipgloss.NewStyle().Width(bodyWidth).Height(paneHeight).Render(content)
}

// paneHeight is what a pair of boards needs: heading, letters, the rows, a gap, the roster
// and the tally.
var paneHeight = 1 + 1 + game.Size + 1 + rosterLines + 1

func enemyHeading(snap lobby.Snapshot) string {
	if snap.Opponent.Name == "" {
		return "ENEMY WATERS"
	}
	return strings.ToUpper(snap.Opponent.Name) + "'S WATERS"
}

func (m Model) menuPane() string {
	rows := []string{m.st.heading.Render("Playing as " + m.me.Name), m.st.dim.Render(m.record()), ""}
	for i, item := range menuItems {
		if choice(i) == m.choice {
			rows = append(rows, m.st.chosen.Render("  > "+item))
			continue
		}
		rows = append(rows, m.st.unchosen.Render("    "+item))
	}
	left := lipgloss.JoinVertical(lipgloss.Left, rows...)
	if len(m.top) == 0 {
		return left
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, m.st.column.Render(left), gap, m.board())
}

// record is this player's own tally, bot games included. A keyless session is told why it will
// have none, since otherwise a whole ranked game is played and silently thrown away. With no
// database at all it stays blank, because then there is nothing the player could do about it.
func (m Model) record() string {
	if m.db == nil {
		return ""
	}
	if !m.db.Tracks(m.me.ID) {
		if m.localWins+m.localLosses == 0 {
			return "Anonymous session, nothing is recorded."
		}
		return fmt.Sprintf("%d won · %d lost this session, not recorded.", m.localWins, m.localLosses)
	}
	if m.profile.Games == 0 {
		return "No games played yet."
	}
	return fmt.Sprintf("%d won · %d lost · %s", m.profile.Wins, m.profile.Losses, plural(m.profile.Games, "game"))
}

// board is the leaderboard, which counts wins against people only.
func (m Model) board() string {
	rows := []string{m.st.heading.Render("LEADERBOARD"), ""}
	for i, e := range m.top {
		rows = append(rows, fmt.Sprintf("%s %-*s %s",
			m.st.dim.Render(fmt.Sprintf("%d.", i+1)), lobby.NameLimit, e.Name, m.st.code.Render(strconv.Itoa(e.Wins))))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// unrankedPane is shown once, before the menu, to a session with no SSH key. The same facts
// are on the site, which is why the address is here rather than a longer explanation.
func (m Model) unrankedPane() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		m.st.heading.Render("This session is anonymous"),
		"",
		m.st.dim.Render("Your terminal offered no SSH public key, and that key is"),
		m.st.dim.Render("what a profile is kept under here. Everything is playable"),
		m.st.dim.Render("and this session keeps its own tally, but nothing is saved:"),
		m.st.dim.Render("no leaderboard place, and no reconnecting into your seat"),
		m.st.dim.Render("if you drop mid-game."),
		"",
		m.st.dim.Render("To be recorded, run this once and connect again:"),
		m.st.code.Render("ssh-keygen -t ed25519"),
		"",
		m.st.dim.Render("More at ")+m.st.code.Render("battleships.phons.dev"),
	)
}

func (m Model) namePane() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		m.st.heading.Render("What should the fleet call you?"),
		"",
		"  "+m.st.code.Render(m.typed)+m.st.prompt.Render("_"),
		"",
		m.st.dim.Render(fmt.Sprintf("  Letters, digits and spaces, up to %d of them.", lobby.NameLimit)),
	)
}

func (m Model) joinPane() string {
	boxes := make([]string, 4)
	for i := range boxes {
		boxes[i] = m.st.dim.Render("_")
		if i < len(m.typed) {
			boxes[i] = m.st.code.Render(string(m.typed[i]))
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.st.heading.Render("Room code"),
		"",
		"  "+strings.Join(boxes, " "),
	)
}

func (m Model) waitPane() string {
	if !m.live {
		return m.st.dim.Render("Opening the room...")
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.st.heading.Render("Your room is open."),
		"",
		"  "+m.st.code.Render(strings.Join(strings.Split(m.snap.Code, ""), " ")),
		"",
		m.st.dim.Render("  Tell a friend to pick \"Join with a code\" and type that in."),
	)
}

// side stacks one player's grid, fleet roster and shot tally into a single column.
func (m Model) side(p game.Player, name string) string {
	return m.st.column.Render(lipgloss.JoinVertical(lipgloss.Left,
		m.st.heading.Render(name),
		m.grid(p),
		"",
		m.roster(p),
		m.tally(p),
	))
}

func (m Model) grid(p game.Player) string {
	var b strings.Builder
	letters := make([]string, game.Size)
	for i := range letters {
		letters[i] = string(rune('A' + i))
	}
	b.WriteString(m.st.dim.Render("    "+strings.Join(letters, " ")) + "\n")

	ghost := m.ghost(p)
	for row := range game.Size {
		b.WriteString(m.st.dim.Render(pad(row+1)) + " ")
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
	board := m.snap.Game.Board(p)
	style, glyph := m.st.water, "·"
	switch {
	case slices.Contains(ghost, c):
		style, glyph = m.st.legal, "#"
		if ship, ok := m.pending(); ok && board.CanPlace(ship) != nil {
			style = m.st.broken
		}
	case board.At(c) == game.Hit:
		style, glyph = m.st.hit, "X"
	case board.At(c) == game.Miss:
		style, glyph = m.st.dim, "o"
	case board.At(c) == game.Water:
		glyph = "×"
	default:
		if _, ok := board.ShipAt(c); ok && m.reveals(p) {
			style, glyph = m.st.ship, "#"
		}
	}
	if c == m.cursor && p == m.active() {
		style = m.st.here
	}
	return style.Render(glyph)
}

// roster names each ship behind a marker: afloat, damaged, or sunk. The marker carries the
// state on its own, since plenty of SSH clients drop colour and strikethrough.
func (m Model) roster(p game.Player) string {
	board := m.snap.Game.Board(p)
	names := make([]string, 0, len(game.Fleet))
	for _, class := range game.Fleet {
		s, ok := board.Ship(class)
		switch {
		case !ok:
			names = append(names, m.st.dim.Render("·"+class.String()))
		case s.Sunk():
			names = append(names, m.st.broken.Render("x")+m.st.sunk.Render(class.String()))
		case s.Hits > 0 && p == m.mine():
			names = append(names, m.st.hurt.Render("!"+class.String()))
		default:
			names = append(names, m.st.water.Render("·")+class.String())
		}
	}
	lines := make([]string, 0, rosterLines)
	for i := 0; i < len(names); i += rosterPerLine {
		lines = append(lines, strings.Join(names[i:min(i+rosterPerLine, len(names))], "  "))
	}
	return strings.Join(lines, "\n")
}

const rosterPerLine = 3

var rosterLines = (len(game.Fleet) + rosterPerLine - 1) / rosterPerLine

// tally summarises the shots taken at one board, named for whoever is firing them.
func (m Model) tally(p game.Player) string {
	fired, hits := m.snap.Game.Board(p).Tally()
	pct := 0
	if fired > 0 {
		pct = hits * 100 / fired
	}
	who := "you:"
	if p == m.mine() {
		who = "them:"
	}
	return m.st.dim.Render(fmt.Sprintf("%s %s · %s · %d%%", who, plural(hits, "hit"), plural(fired, "shot"), pct))
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func (m Model) status() string {
	second := m.subline()
	if m.notice != "" {
		second = m.st.dim.Render(m.notice)
		if badge := m.badge(); badge != "" {
			second = badge + " " + second
		}
	}
	return m.headline() + "\n" + second
}

// badge names whose go it is. It shares its line with the notice, so it is rendered
// alongside one rather than being replaced by it.
func (m Model) badge() string {
	if m.screen != playing || !m.live || m.snap.Phase != lobby.Firing || m.snap.Away {
		return ""
	}
	if m.snap.Game.Turn != m.mine() {
		return m.st.theirs.Render("THEIR GO")
	}
	return m.st.yours.Render("YOUR GO")
}

func (m Model) headline() string {
	switch {
	case m.screen == menu:
		return "Pick a game."
	case m.screen == naming:
		return "First time here. Pick a name other players will see."
	case m.screen == joining:
		return "Type the four letters your friend was given."
	case m.screen == unranked:
		return "Nothing this session does will be recorded."
	case !m.live:
		return ""
	}

	switch m.snap.Phase {
	case lobby.Waiting:
		return "Waiting for someone to join."
	case lobby.Placing:
		if ship, ok := m.pending(); ok {
			return fmt.Sprintf("Place your %s.", ship.Class)
		}
		return "Fleet ready. Waiting for your opponent to place theirs."
	case lobby.Over:
		return m.result()
	default:
		return m.shot(m.snap.Last[m.mine()], true)
	}
}

func (m Model) subline() string {
	if m.screen != playing || !m.live {
		return ""
	}
	switch m.snap.Phase {
	case lobby.Placing:
		return m.st.dim.Render([...]string{"0°", "90°", "180°", "270°"}[m.rotation])
	case lobby.Firing:
		if m.snap.Away {
			return m.st.hurt.Render(fmt.Sprintf("%s dropped out, back within %s or they forfeit.",
				m.snap.Opponent.Name, remaining(m.snap.AwayUntil)))
		}
		return m.badge() + " " + m.st.dim.Render(m.shot(m.snap.Last[m.theirs()], false))
	case lobby.Over:
		return m.tally(m.theirs())
	}
	return ""
}

func (m Model) result() string {
	won := m.snap.Winner == m.mine()
	switch {
	case m.snap.Forfeit && won:
		return m.st.win.Render(m.snap.Opponent.Name + " never came back. You win by forfeit.")
	case m.snap.Forfeit:
		return m.st.lose.Render("You forfeited that one.")
	case won:
		return m.st.win.Render("You win. Their fleet is on the bottom.")
	default:
		return m.st.lose.Render("You lose. Your fleet is on the bottom.")
	}
}

func (m Model) shot(s lobby.Shot, mine bool) string {
	if !s.Set {
		return ""
	}
	who := m.snap.Opponent.Name
	if mine {
		who = "You"
	}
	switch {
	case s.Res.Sunk:
		style := m.st.lose
		if mine {
			style = m.st.win
		}
		return style.Render(fmt.Sprintf("%s SANK the %s at %s.", who, s.Res.Class, label(s.At)))
	case s.Res.Hit:
		return m.st.hurt.Render(fmt.Sprintf("%s hit the %s at %s.", who, s.Res.Class, label(s.At)))
	default:
		return m.st.dim.Render(fmt.Sprintf("%s missed at %s.", who, label(s.At)))
	}
}

func remaining(deadline time.Time) string {
	left := time.Until(deadline).Round(time.Second)
	if left < 0 {
		left = 0
	}
	return left.String()
}

func (m Model) help() string {
	switch {
	case m.screen == menu:
		return "up/down choose · enter start · q quit"
	case m.screen == naming:
		return "type a name · enter save"
	case m.screen == joining:
		return "type four letters · enter join · esc back"
	case m.screen == unranked:
		return "enter continue · q quit"
	case !m.live:
		return "esc back · q quit"
	}
	switch m.snap.Phase {
	case lobby.Placing:
		if _, ok := m.pending(); ok {
			return "hjkl/arrows move · r rotate · enter place · R auto-place · esc leave"
		}
		return "esc leave · q quit"
	case lobby.Over:
		return "n back to the menu · q quit"
	case lobby.Waiting:
		return "esc leave · q quit"
	default:
		return "hjkl/arrows move · enter fire · esc leave · q quit"
	}
}

// ghost is the outline of the ship currently being positioned, empty outside placement.
func (m Model) ghost(p game.Player) []game.Coord {
	if m.snap.Phase != lobby.Placing || p != m.mine() {
		return nil
	}
	ship, ok := m.pending()
	if !ok {
		return nil
	}
	var cells []game.Coord
	for _, c := range ship.Cells() {
		if c.Valid() {
			cells = append(cells, c)
		}
	}
	return cells
}

func (m Model) reveals(p game.Player) bool {
	return p == m.mine() || m.snap.Phase == lobby.Over
}

// active is the board the cursor sits on: your own while placing, the enemy's while firing.
func (m Model) active() game.Player {
	if m.snap.Phase == lobby.Placing {
		return m.mine()
	}
	return m.theirs()
}

func pad(n int) string {
	return fmt.Sprintf("%3d", n)
}
