package game

import (
	"errors"
	"math/rand"
	"slices"
)

const (
	Size       = 10
	FleetCells = 17
)

var (
	ErrOutOfBounds = errors.New("ship or shot outside the board")
	ErrOverlap     = errors.New("ship overlaps another ship")
	ErrAlreadyShot = errors.New("cell already fired at")
	ErrDuplicate   = errors.New("ship class already placed")
)

type ShipClass int

const (
	Carrier ShipClass = iota
	Battleship
	Cruiser
	Submarine
	Destroyer
)

var Fleet = []ShipClass{Carrier, Battleship, Cruiser, Submarine, Destroyer}

func (c ShipClass) Length() int {
	return [...]int{5, 4, 3, 3, 2}[c]
}

func (c ShipClass) String() string {
	return [...]string{"Carrier", "Battleship", "Cruiser", "Submarine", "Destroyer"}[c]
}

type Coord struct {
	Row, Col int
}

func (c Coord) Valid() bool {
	return c.Row >= 0 && c.Row < Size && c.Col >= 0 && c.Col < Size
}

type Ship struct {
	Class    ShipClass
	Origin   Coord
	Vertical bool
	Hits     int
}

func (s Ship) Cells() []Coord {
	cells := make([]Coord, s.Class.Length())
	for i := range cells {
		if s.Vertical {
			cells[i] = Coord{s.Origin.Row + i, s.Origin.Col}
		} else {
			cells[i] = Coord{s.Origin.Row, s.Origin.Col + i}
		}
	}
	return cells
}

func (s Ship) Sunk() bool {
	return s.Hits >= s.Class.Length()
}

type Shot uint8

const (
	Unshot Shot = iota
	Miss
	Hit
)

type Result struct {
	Hit   bool
	Sunk  bool
	Class ShipClass
}

type Board struct {
	Ships []Ship
	shots [Size][Size]Shot
}

func (b *Board) CanPlace(s Ship) error {
	for _, c := range s.Cells() {
		if !c.Valid() {
			return ErrOutOfBounds
		}
		if _, ok := b.ShipAt(c); ok {
			return ErrOverlap
		}
	}
	if b.Placed(s.Class) {
		return ErrDuplicate
	}
	return nil
}

func (b *Board) Place(s Ship) error {
	if err := b.CanPlace(s); err != nil {
		return err
	}
	s.Hits = 0
	b.Ships = append(b.Ships, s)
	return nil
}

func (b *Board) Placed(class ShipClass) bool {
	for _, s := range b.Ships {
		if s.Class == class {
			return true
		}
	}
	return false
}

func (b *Board) ShipAt(c Coord) (ShipClass, bool) {
	for _, s := range b.Ships {
		if slices.Contains(s.Cells(), c) {
			return s.Class, true
		}
	}
	return 0, false
}

func (b *Board) At(c Coord) Shot {
	if !c.Valid() {
		return Unshot
	}
	return b.shots[c.Row][c.Col]
}

func (b *Board) Fire(c Coord) (Result, error) {
	if !c.Valid() {
		return Result{}, ErrOutOfBounds
	}
	if b.shots[c.Row][c.Col] != Unshot {
		return Result{}, ErrAlreadyShot
	}
	for i := range b.Ships {
		if !slices.Contains(b.Ships[i].Cells(), c) {
			continue
		}
		b.shots[c.Row][c.Col] = Hit
		b.Ships[i].Hits++
		return Result{Hit: true, Sunk: b.Ships[i].Sunk(), Class: b.Ships[i].Class}, nil
	}
	b.shots[c.Row][c.Col] = Miss
	return Result{}, nil
}

func (b *Board) Defeated() bool {
	if len(b.Ships) == 0 {
		return false
	}
	for _, s := range b.Ships {
		if !s.Sunk() {
			return false
		}
	}
	return true
}

func AutoPlace(b *Board, rng *rand.Rand) {
	for _, class := range Fleet {
		if b.Placed(class) {
			continue
		}
		for {
			s := Ship{
				Class:    class,
				Origin:   Coord{rng.Intn(Size), rng.Intn(Size)},
				Vertical: rng.Intn(2) == 0,
			}
			if b.Place(s) == nil {
				break
			}
		}
	}
}
