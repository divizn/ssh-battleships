package ai

import (
	"maps"
	"math/rand"
	"testing"

	"github.com/divizn/ssh-battleships/internal/game"
)

// playOut runs the bot against a random fleet and returns the shots it took, in order.
func playOut(t *testing.T, seed int64) []game.Coord {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	var b game.Board
	game.AutoPlace(&b, rng)
	bot := New(rng)

	var shots []game.Coord
	for range game.Size * game.Size {
		c := bot.NextShot()
		res, err := b.Fire(c)
		if err != nil {
			t.Fatalf("seed %d: shot %d at %v: %v", seed, len(shots), c, err)
		}
		bot.Record(c, res)
		shots = append(shots, c)
		if b.Defeated() {
			return shots
		}
	}
	t.Fatalf("seed %d: bot did not sink the fleet in %d shots", seed, game.Size*game.Size)
	return nil
}

func TestBotNeverRepeatsACell(t *testing.T) {
	for seed := range 200 {
		seen := map[game.Coord]bool{}
		for _, c := range playOut(t, int64(seed)) {
			if seen[c] {
				t.Fatalf("seed %d: fired at %v twice", seed, c)
			}
			seen[c] = true
		}
	}
}

func TestBotSinksEveryFleetWithinBounds(t *testing.T) {
	const games = 200
	worst, total := 0, 0
	for seed := range games {
		n := len(playOut(t, int64(seed)))
		worst = max(worst, n)
		total += n
	}

	// half the board: hunting dominates on a board this size, but a bot doing worse than this
	// has stopped targeting and is close to firing at random.
	budget := float64(game.Size*game.Size) / 2
	mean := float64(total) / games
	if mean > budget {
		t.Errorf("mean %.1f shots, want a hunt-and-target bot under %.0f", mean, budget)
	}
	t.Logf("worst %d shots, mean %.1f", worst, mean)
}

func TestBotChasesAHit(t *testing.T) {
	bot := New(rand.New(rand.NewSource(1)))
	hit := game.Coord{Row: 4, Col: 4}
	bot.Record(hit, game.Result{Hit: true, Class: game.Cruiser})

	next := bot.NextShot()
	if dist := abs(next.Row-hit.Row) + abs(next.Col-hit.Col); dist != 1 {
		t.Errorf("shot after a hit = %v, want a neighbour of %v", next, hit)
	}
}

func TestBotFollowsTheAxisOfTwoHits(t *testing.T) {
	bot := New(rand.New(rand.NewSource(1)))
	bot.Record(game.Coord{Row: 4, Col: 4}, game.Result{Hit: true, Class: game.Cruiser})
	bot.Record(game.Coord{Row: 4, Col: 5}, game.Result{Hit: true, Class: game.Cruiser})

	got := map[game.Coord]bool{}
	for range 2 {
		c := bot.NextShot()
		got[c] = true
		bot.Record(c, game.Result{})
	}

	want := map[game.Coord]bool{{Row: 4, Col: 3}: true, {Row: 4, Col: 6}: true}
	if !maps.Equal(got, want) {
		t.Errorf("shots after two in-line hits = %v, want the two ends %v", got, want)
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
