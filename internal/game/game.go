package game

import "errors"

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

// Fire resolves a shot by the player whose turn it is, against their opponent.
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
	g.Turn = g.Turn.Other()
	return res, nil
}
