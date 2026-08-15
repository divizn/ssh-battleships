package tui

import "github.com/charmbracelet/lipgloss"

// styles is built per session. Over SSH the colour profile and background belong to the
// client's terminal, which only that session's renderer knows about.
type styles struct {
	dim, water, title, heading lipgloss.Style
	ship, hurt, hit, sunk      lipgloss.Style
	legal, broken, here        lipgloss.Style
	win, lose, code            lipgloss.Style
	yours, theirs              lipgloss.Style
	box, column, banner, body  lipgloss.Style
	chosen, unchosen, prompt   lipgloss.Style
}

func newStyles(r *lipgloss.Renderer) styles {
	// Two greys, both adaptive: the terminal background decides whether muted means darker
	// or lighter. dim carries text that still has to be read, water only suggests an edge.
	grey := lipgloss.AdaptiveColor{Light: "241", Dark: "248"}
	greyer := lipgloss.AdaptiveColor{Light: "249", Dark: "242"}
	red := lipgloss.Color("9")
	green := lipgloss.Color("10")

	return styles{
		dim:      r.NewStyle().Foreground(grey),
		water:    r.NewStyle().Foreground(greyer),
		title:    r.NewStyle().Bold(true).Foreground(lipgloss.Color("14")),
		heading:  r.NewStyle().Bold(true),
		ship:     r.NewStyle().Foreground(lipgloss.Color("12")),
		hurt:     r.NewStyle().Foreground(lipgloss.Color("11")),
		hit:      r.NewStyle().Foreground(red).Bold(true),
		sunk:     r.NewStyle().Foreground(red).Strikethrough(true),
		legal:    r.NewStyle().Foreground(green),
		broken:   r.NewStyle().Foreground(red),
		here:     r.NewStyle().Reverse(true),
		win:      r.NewStyle().Bold(true).Foreground(green),
		lose:     r.NewStyle().Bold(true).Foreground(red),
		yours:    r.NewStyle().Bold(true).Foreground(green),
		theirs:   r.NewStyle().Bold(true).Foreground(red),
		code:     r.NewStyle().Bold(true).Foreground(lipgloss.Color("14")),
		chosen:   r.NewStyle().Bold(true).Foreground(lipgloss.Color("14")),
		unchosen: r.NewStyle().Foreground(grey),
		prompt:   r.NewStyle().Bold(true),
		box: r.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(greyer).
			Padding(0, 2),
		column: r.NewStyle().Width(colWidth),
		banner: r.NewStyle().Width(bodyWidth).Align(lipgloss.Center),
		body:   r.NewStyle().Width(bodyWidth),
	}
}
