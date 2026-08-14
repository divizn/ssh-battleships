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

// shotLog is the last thing one side did, kept as a result so the view can colour it.
type shotLog struct {
	set  bool
	res  game.Result
	at   game.Coord
	note string
}

type Model struct {
	g             game.Game
	bot           *ai.Bot
	rng           *rand.Rand
	phase         phase
	cursor        game.Coord
	vertical      bool
	next          int
	yourShot      shotLog
	botShot       shotLog
	bell          bool
	width, height int
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
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	m.bell = false

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
			fresh := New()
			fresh.width, fresh.height = m.width, m.height
			return fresh, nil
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
		m.yourShot = shotLog{set: true, note: "You have already fired at " + label(m.cursor) + "."}
		return m
	}
	m.yourShot = shotLog{set: true, res: res, at: m.cursor}
	m.bell = res.Sunk
	if m.g.Over {
		m.phase = over
		return m
	}

	shot := m.bot.NextShot()
	res, err = m.g.Fire(shot)
	if err != nil {
		m.botShot = shotLog{set: true, note: fmt.Sprintf("bot fired at %s: %v", label(shot), err)}
		return m
	}
	m.bot.Record(shot, res)
	m.botShot = shotLog{set: true, res: res, at: shot}
	m.bell = m.bell || res.Sunk
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
