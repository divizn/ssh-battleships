package game

import (
	"errors"
	"slices"
)

var ErrGameOver = errors.New("game is over")

type Player int

const (
	P1 Player = iota
	P2
)

func (p Player) Other() Player {
	return 1 - p
}

type Game struct {
	Boards [2]Board
	Turn   Player
	Over   bool
	Winner Player
}

func (g *Game) Board(p Player) *Board {
	return &g.Boards[p]
}

// Clone returns a copy sharing nothing with g, so it can be handed to another goroutine.
func (g Game) Clone() Game {
	for i := range g.Boards {
		g.Boards[i].Ships = slices.Clone(g.Boards[i].Ships)
	}
	return g
}

// Fire resolves a shot by the player whose turn it is, against their opponent. A hit earns
// another go, so the turn only passes on a miss.
func (g *Game) Fire(c Coord) (Result, error) {
	if g.Over {
		return Result{}, ErrGameOver
	}
	target := &g.Boards[g.Turn.Other()]
	res, err := target.Fire(c)
	if err != nil {
		return res, err
	}
	if target.Defeated() {
		g.Over = true
		g.Winner = g.Turn
		return res, nil
	}
	if !res.Hit {
		g.Turn = g.Turn.Other()
	}
	return res, nil
}
