package tui

import (
	"fmt"
	"math/rand"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/divizn/ssh-battleships/internal/ai"
	"github.com/divizn/ssh-battleships/internal/game"
)

const (
	you = game.P1
	foe = game.P2
)

type phase int

const (
	placing phase = iota
	firing
	over
)

type Model struct {
	g        game.Game
	bot      *ai.Bot
	rng      *rand.Rand
	phase    phase
	cursor   game.Coord
	vertical bool
	next     int
	yourShot string
	botShot  string
}

func New() Model {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	m := Model{rng: rng, bot: ai.New(rng)}
	game.AutoPlace(m.g.Board(foe), rng)
	return m
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	}

	switch m.phase {
	case placing:
		return m.updatePlacing(key.String()), nil
	case firing:
		return m.updateFiring(key.String()), nil
	case over:
		if key.String() == "n" {
			return New(), nil
		}
	}
	return m, nil
}

func (m Model) updatePlacing(key string) Model {
	if moved, ok := m.move(key); ok {
		return moved
	}
	switch key {
	case "r":
		m.vertical = !m.vertical
	case "R":
		game.AutoPlace(m.g.Board(you), m.rng)
		m.next = len(game.Fleet)
	case "enter", " ":
		if err := m.g.Board(you).Place(m.pending()); err != nil {
			return m
		}
		m.next++
	}
	if m.next == len(game.Fleet) {
		m.phase = firing
		m.cursor = game.Coord{}
	}
	return m
}

func (m Model) updateFiring(key string) Model {
	if moved, ok := m.move(key); ok {
		return moved
	}
	if key != "enter" && key != " " {
		return m
	}

	res, err := m.g.Fire(m.cursor)
	if err != nil {
		m.yourShot = "you have already fired at " + label(m.cursor)
		return m
	}
	m.yourShot = describe("You", res, m.cursor)
	if m.g.Over {
		m.phase = over
		return m
	}

	shot := m.bot.NextShot()
	res, err = m.g.Fire(shot)
	if err != nil {
		m.botShot = fmt.Sprintf("bot fired at %s: %v", label(shot), err)
		return m
	}
	m.bot.Record(shot, res)
	m.botShot = describe("Bot", res, shot)
	if m.g.Over {
		m.phase = over
	}
	return m
}

var steps = map[string]game.Coord{
	"up": {Row: -1}, "k": {Row: -1},
	"down": {Row: 1}, "j": {Row: 1},
	"left": {Col: -1}, "h": {Col: -1},
	"right": {Col: 1}, "l": {Col: 1},
}

func (m Model) move(key string) (Model, bool) {
	step, ok := steps[key]
	if !ok {
		return m, false
	}
	m.cursor.Row = clamp(m.cursor.Row + step.Row)
	m.cursor.Col = clamp(m.cursor.Col + step.Col)
	return m, true
}

func clamp(n int) int {
	return min(max(n, 0), game.Size-1)
}

// pending is the ship being positioned during placement.
func (m Model) pending() game.Ship {
	return game.Ship{Class: game.Fleet[m.next], Origin: m.cursor, Vertical: m.vertical}
}

func label(c game.Coord) string {
	return fmt.Sprintf("%c%d", 'A'+rune(c.Col), c.Row+1)
}

func describe(who string, res game.Result, c game.Coord) string {
	switch {
	case res.Sunk:
		return fmt.Sprintf("%s sank the %s at %s", who, res.Class, label(c))
	case res.Hit:
		return fmt.Sprintf("%s hit the %s at %s", who, res.Class, label(c))
	default:
		return fmt.Sprintf("%s missed at %s", who, label(c))
	}
}
