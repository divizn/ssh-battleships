package game

import (
	"errors"
	"math/rand"
	"slices"
)

const Size = 16

// FleetCells is how many cells a full fleet covers, which is what the roster and the win
// condition count against.
var FleetCells = func() int {
	n := 0
	for _, c := range Fleet {
		n += len(c.Shape())
	}
	return n
}()

var (
	ErrOutOfBounds = errors.New("ship or shot outside the board")
	ErrOverlap     = errors.New("ship overlaps another ship")
	ErrTouching    = errors.New("ships cannot touch, not even at the corners")
	ErrAlreadyShot = errors.New("cell already marked")
	ErrDuplicate   = errors.New("ship class already placed")
)

type ShipClass int

const (
	Carrier ShipClass = iota
	Battleship
	Cruiser
	Submarine
	Destroyer
	Corvette
	Tender
	Cutter
)

var Fleet = []ShipClass{Carrier, Battleship, Cruiser, Submarine, Destroyer, Corvette, Tender, Cutter}

// Shape is the class's cells as offsets from the origin, unrotated. Not every ship is a line:
// Corvette and Cutter are L shaped, Tender is a T.
func (c ShipClass) Shape() []Coord {
	return shapes[c]
}

var shapes = [...][]Coord{
	Carrier:    line(5),
	Battleship: line(4),
	Cruiser:    line(3),
	Submarine:  line(3),
	Destroyer:  line(2),
	Corvette:   {{0, 0}, {1, 0}, {2, 0}, {2, 1}},
	Tender:     {{0, 0}, {0, 1}, {0, 2}, {1, 1}},
	Cutter:     {{0, 0}, {1, 0}, {1, 1}},
}

func line(n int) []Coord {
	cells := make([]Coord, n)
	for i := range cells {
		cells[i] = Coord{Col: i}
	}
	return cells
}

func (c ShipClass) String() string {
	return [...]string{
		"Carrier", "Battleship", "Cruiser", "Submarine", "Destroyer",
		"Corvette", "Tender", "Cutter",
	}[c]
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
	Rotation int
	Hits     int
}

// Cells turns the class's shape into board coordinates: rotate a quarter turn at a time, pull
// the result back into positive space so Origin stays the shape's top-left corner whichever way
// it faces, then translate.
func (s Ship) Cells() []Coord {
	shape := s.Class.Shape()
	cells := make([]Coord, len(shape))
	for i, o := range shape {
		for range s.Rotation & 3 {
			o = Coord{Row: o.Col, Col: -o.Row}
		}
		cells[i] = o
	}
	top, left := cells[0].Row, cells[0].Col
	for _, c := range cells {
		top, left = min(top, c.Row), min(left, c.Col)
	}
	for i := range cells {
		cells[i] = Coord{
			Row: cells[i].Row - top + s.Origin.Row,
			Col: cells[i].Col - left + s.Origin.Col,
		}
	}
	return cells
}

func (s Ship) Sunk() bool {
	return s.Hits >= len(s.Class.Shape())
}

// Halo is the ring of cells around a ship, corners included. No other ship may sit there,
// so once this one sinks the whole ring is known to be water.
func (s Ship) Halo() []Coord {
	own := s.Cells()
	var ring []Coord
	for _, c := range own {
		for dr := -1; dr <= 1; dr++ {
			for dc := -1; dc <= 1; dc++ {
				n := Coord{Row: c.Row + dr, Col: c.Col + dc}
				if n.Valid() && !slices.Contains(own, n) && !slices.Contains(ring, n) {
					ring = append(ring, n)
				}
			}
		}
	}
	return ring
}

type Shot uint8

const (
	Unshot Shot = iota
	Miss
	Hit
	// Water is a cell revealed as empty by a neighbouring ship sinking, never fired at.
	Water
)

// Known reports whether anything is already known about the cell.
func (s Shot) Known() bool { return s != Unshot }

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
	for _, c := range s.Halo() {
		if _, ok := b.ShipAt(c); ok {
			return ErrTouching
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
	_, ok := b.Ship(class)
	return ok
}

func (b *Board) Ship(class ShipClass) (Ship, bool) {
	for _, s := range b.Ships {
		if s.Class == class {
			return s, true
		}
	}
	return Ship{}, false
}

// Tally counts the shots taken at this board and how many of them landed.
func (b *Board) Tally() (fired, hits int) {
	for row := range Size {
		for col := range Size {
			switch b.shots[row][col] {
			case Hit:
				fired++
				hits++
			case Miss:
				fired++
			}
		}
	}
	return fired, hits
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
	if b.shots[c.Row][c.Col].Known() {
		return Result{}, ErrAlreadyShot
	}
	for i := range b.Ships {
		if !slices.Contains(b.Ships[i].Cells(), c) {
			continue
		}
		b.shots[c.Row][c.Col] = Hit
		b.Ships[i].Hits++
		if b.Ships[i].Sunk() {
			b.reveal(b.Ships[i])
		}
		return Result{Hit: true, Sunk: b.Ships[i].Sunk(), Class: b.Ships[i].Class}, nil
	}
	b.shots[c.Row][c.Col] = Miss
	return Result{}, nil
}

// reveal marks the water around a ship that has just gone down. Nothing can be hiding there,
// so neither player should waste a shot on it.
func (b *Board) reveal(s Ship) {
	for _, c := range s.Halo() {
		if !b.shots[c.Row][c.Col].Known() {
			b.shots[c.Row][c.Col] = Water
		}
	}
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

// AutoPlace fills out the fleet, keeping any ships already placed by hand. It reports false
// if those manual placements leave nowhere legal for the rest.
func AutoPlace(b *Board, rng *rand.Rand) bool {
	kept := len(b.Ships)
	for range 100 {
		if fill(b, rng) {
			return true
		}
		b.Ships = b.Ships[:kept]
	}
	return false
}

func fill(b *Board, rng *rand.Rand) bool {
	for _, class := range Fleet {
		if b.Placed(class) {
			continue
		}
		if !scatter(b, class, rng) {
			return false
		}
	}
	return true
}

// scatter drops one ship at random spots until it lands legally, giving up rather than
// spinning forever on a board that has no room left for it.
func scatter(b *Board, class ShipClass, rng *rand.Rand) bool {
	for range 500 {
		s := Ship{
			Class:    class,
			Origin:   Coord{rng.Intn(Size), rng.Intn(Size)},
			Rotation: rng.Intn(4),
		}
		if b.Place(s) == nil {
			return true
		}
	}
	return false
}
