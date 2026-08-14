package game

import (
	"errors"
	"math/rand"
	"testing"
)

func TestCanPlace(t *testing.T) {
	tests := []struct {
		name string
		ship Ship
		want error
	}{
		{"top left horizontal", Ship{Class: Carrier, Origin: Coord{0, 0}}, nil},
		{"flush right edge", Ship{Class: Carrier, Origin: Coord{0, 5}}, nil},
		{"flush bottom edge", Ship{Class: Carrier, Origin: Coord{5, 0}, Vertical: true}, nil},
		{"off right edge", Ship{Class: Carrier, Origin: Coord{0, 6}}, ErrOutOfBounds},
		{"off bottom edge", Ship{Class: Carrier, Origin: Coord{6, 0}, Vertical: true}, ErrOutOfBounds},
		{"negative origin", Ship{Class: Destroyer, Origin: Coord{-1, 0}}, ErrOutOfBounds},
		{"crosses existing ship", Ship{Class: Battleship, Origin: Coord{2, 4}, Vertical: true}, ErrOverlap},
		{"same class twice", Ship{Class: Cruiser, Origin: Coord{8, 0}}, ErrDuplicate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b Board
			if err := b.Place(Ship{Class: Cruiser, Origin: Coord{4, 3}}); err != nil {
				t.Fatalf("setup: %v", err)
			}
			if err := b.CanPlace(tt.ship); !errors.Is(err, tt.want) {
				t.Errorf("CanPlace = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestPlaceRejectedShipIsNotStored(t *testing.T) {
	var b Board
	if err := b.Place(Ship{Class: Carrier, Origin: Coord{0, 8}}); err == nil {
		t.Fatal("expected out of bounds placement to fail")
	}
	if len(b.Ships) != 0 {
		t.Errorf("board holds %d ships after a rejected placement", len(b.Ships))
	}
}

func TestFire(t *testing.T) {
	tests := []struct {
		name    string
		shots   []Coord
		at      Coord
		want    Result
		wantErr error
	}{
		{"water", nil, Coord{9, 9}, Result{}, nil},
		{"hit", nil, Coord{0, 0}, Result{Hit: true, Class: Destroyer}, nil},
		{"sink", []Coord{{0, 0}}, Coord{0, 1}, Result{Hit: true, Sunk: true, Class: Destroyer}, nil},
		{"repeat hit", []Coord{{0, 0}}, Coord{0, 0}, Result{}, ErrAlreadyShot},
		{"repeat miss", []Coord{{9, 9}}, Coord{9, 9}, Result{}, ErrAlreadyShot},
		{"off board", nil, Coord{10, 0}, Result{}, ErrOutOfBounds},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b Board
			if err := b.Place(Ship{Class: Destroyer, Origin: Coord{0, 0}}); err != nil {
				t.Fatalf("setup: %v", err)
			}
			for _, c := range tt.shots {
				if _, err := b.Fire(c); err != nil {
					t.Fatalf("setup shot %v: %v", c, err)
				}
			}
			got, err := b.Fire(tt.at)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Fire error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Fire = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDefeatedNeedsEveryShipSunk(t *testing.T) {
	var b Board
	rng := rand.New(rand.NewSource(1))
	AutoPlace(&b, rng)

	if b.Defeated() {
		t.Fatal("a fresh fleet is not defeated")
	}
	var cells []Coord
	for _, s := range b.Ships {
		cells = append(cells, s.Cells()...)
	}
	if len(cells) != FleetCells {
		t.Fatalf("fleet covers %d cells, want %d", len(cells), FleetCells)
	}
	for i, c := range cells {
		if _, err := b.Fire(c); err != nil {
			t.Fatalf("fire %v: %v", c, err)
		}
		if want := i == len(cells)-1; b.Defeated() != want {
			t.Fatalf("after %d hits Defeated = %v, want %v", i+1, b.Defeated(), want)
		}
	}
}

func TestTallyCountsShotsAndHits(t *testing.T) {
	var b Board
	if err := b.Place(Ship{Class: Destroyer, Origin: Coord{0, 0}}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if fired, hits := b.Tally(); fired != 0 || hits != 0 {
		t.Fatalf("fresh board tallies %d/%d, want 0/0", hits, fired)
	}

	for _, c := range []Coord{{0, 0}, {5, 5}, {9, 9}} {
		if _, err := b.Fire(c); err != nil {
			t.Fatalf("fire %v: %v", c, err)
		}
	}
	if fired, hits := b.Tally(); fired != 3 || hits != 1 {
		t.Errorf("Tally = %d hits of %d shots, want 1 of 3", hits, fired)
	}
}

func TestAutoPlaceProducesALegalFleet(t *testing.T) {
	for seed := range 200 {
		var b Board
		AutoPlace(&b, rand.New(rand.NewSource(int64(seed))))

		if len(b.Ships) != len(Fleet) {
			t.Fatalf("seed %d: placed %d ships, want %d", seed, len(b.Ships), len(Fleet))
		}
		seen := map[Coord]bool{}
		for _, s := range b.Ships {
			for _, c := range s.Cells() {
				if !c.Valid() {
					t.Fatalf("seed %d: %v off board at %v", seed, s.Class, c)
				}
				if seen[c] {
					t.Fatalf("seed %d: overlap at %v", seed, c)
				}
				seen[c] = true
			}
		}
	}
}

func TestAutoPlaceKeepsManualPlacements(t *testing.T) {
	var b Board
	manual := Ship{Class: Carrier, Origin: Coord{0, 0}, Vertical: true}
	if err := b.Place(manual); err != nil {
		t.Fatalf("setup: %v", err)
	}
	AutoPlace(&b, rand.New(rand.NewSource(7)))

	if len(b.Ships) != len(Fleet) {
		t.Fatalf("placed %d ships, want %d", len(b.Ships), len(Fleet))
	}
	if b.Ships[0] != manual {
		t.Errorf("manual ship = %+v, want %+v", b.Ships[0], manual)
	}
}

func TestTurnAlternatesOnHitAndMiss(t *testing.T) {
	g := twoDestroyerGame(t)

	if _, err := g.Fire(Coord{5, 5}); err != nil { // P1 misses
		t.Fatal(err)
	}
	if g.Turn != P2 {
		t.Fatalf("turn after a miss = %v, want P2", g.Turn)
	}
	if _, err := g.Fire(Coord{0, 0}); err != nil { // P2 hits
		t.Fatal(err)
	}
	if g.Turn != P1 {
		t.Fatalf("turn after a hit = %v, want P1", g.Turn)
	}
}

func TestFireHitsTheOpponentsBoard(t *testing.T) {
	g := twoDestroyerGame(t)

	if _, err := g.Fire(Coord{0, 0}); err != nil {
		t.Fatal(err)
	}
	if got := g.Board(P2).At(Coord{0, 0}); got != Hit {
		t.Errorf("P2 board at 0,0 = %v, want Hit", got)
	}
	if got := g.Board(P1).At(Coord{0, 0}); got != Unshot {
		t.Errorf("P1 board at 0,0 = %v, want Unshot", got)
	}
}

func TestWinFreezesTheGame(t *testing.T) {
	g := twoDestroyerGame(t)

	for _, c := range []Coord{{0, 0}, {0, 0}, {0, 1}} { // P1, P2, P1
		if _, err := g.Fire(c); err != nil {
			t.Fatalf("fire %v: %v", c, err)
		}
	}
	if !g.Over || g.Winner != P1 {
		t.Fatalf("Over = %v, Winner = %v, want true and P1", g.Over, g.Winner)
	}
	if g.Turn != P1 {
		t.Errorf("turn = %v, want the winner to still hold it", g.Turn)
	}
	if _, err := g.Fire(Coord{9, 9}); !errors.Is(err, ErrGameOver) {
		t.Errorf("Fire after the win = %v, want ErrGameOver", err)
	}
}

func twoDestroyerGame(t *testing.T) *Game {
	t.Helper()
	var g Game
	for p := range g.Boards {
		if err := g.Boards[p].Place(Ship{Class: Destroyer, Origin: Coord{0, 0}}); err != nil {
			t.Fatalf("setup player %d: %v", p, err)
		}
	}
	return &g
}
