package ai

import (
	"math/rand"

	"github.com/divizn/ssh-battleships/internal/game"
)

// Bot plays hunt-and-target: fire at parity-filtered cells until a hit, then probe
// the neighbours of that hit, following the axis once two hits line up.
type Bot struct {
	rng     *rand.Rand
	fired   [game.Size][game.Size]bool
	targets []game.Coord
	cluster []game.Coord
}

func New(rng *rand.Rand) *Bot {
	return &Bot{rng: rng}
}

func (b *Bot) NextShot() game.Coord {
	for len(b.targets) > 0 {
		c := b.targets[len(b.targets)-1]
		b.targets = b.targets[:len(b.targets)-1]
		if b.open(c) {
			return c
		}
	}
	return b.hunt()
}

func (b *Bot) Record(c game.Coord, res game.Result) {
	b.fired[c.Row][c.Col] = true
	switch {
	case res.Sunk:
		// ponytail: a sunk ship drops the whole cluster, so hits on a ship
		// adjacent to it are forgotten. Track per-ship clusters if the bot
		// needs to be stronger.
		b.cluster = nil
		b.targets = nil
	case res.Hit:
		b.cluster = append(b.cluster, c)
		b.targets = b.follow()
	}
}

func (b *Bot) open(c game.Coord) bool {
	return c.Valid() && !b.fired[c.Row][c.Col]
}

func (b *Bot) hunt() game.Coord {
	var parity, any []game.Coord
	for row := range game.Size {
		for col := range game.Size {
			c := game.Coord{Row: row, Col: col}
			if !b.open(c) {
				continue
			}
			any = append(any, c)
			if (row+col)%2 == 0 {
				parity = append(parity, c)
			}
		}
	}
	if len(parity) > 0 {
		return parity[b.rng.Intn(len(parity))]
	}
	return any[b.rng.Intn(len(any))]
}

// follow returns the cells worth trying next given the current cluster of hits:
// the four neighbours of a lone hit, or the two ends of an established line.
func (b *Bot) follow() []game.Coord {
	first := b.cluster[0]
	var candidates []game.Coord
	if len(b.cluster) == 1 {
		candidates = []game.Coord{
			{Row: first.Row - 1, Col: first.Col},
			{Row: first.Row + 1, Col: first.Col},
			{Row: first.Row, Col: first.Col - 1},
			{Row: first.Row, Col: first.Col + 1},
		}
	} else {
		lo, hi := first, first
		vertical := b.cluster[1].Col == first.Col
		for _, c := range b.cluster {
			if vertical {
				lo.Row = min(lo.Row, c.Row)
				hi.Row = max(hi.Row, c.Row)
			} else {
				lo.Col = min(lo.Col, c.Col)
				hi.Col = max(hi.Col, c.Col)
			}
		}
		if vertical {
			candidates = []game.Coord{{Row: lo.Row - 1, Col: lo.Col}, {Row: hi.Row + 1, Col: hi.Col}}
		} else {
			candidates = []game.Coord{{Row: lo.Row, Col: lo.Col - 1}, {Row: hi.Row, Col: hi.Col + 1}}
		}
	}

	open := candidates[:0]
	for _, c := range candidates {
		if b.open(c) {
			open = append(open, c)
		}
	}
	return open
}
