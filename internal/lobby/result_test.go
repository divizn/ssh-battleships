package lobby

import (
	"fmt"
	"testing"
	"time"

	"github.com/divizn/ssh-battleships/internal/game"
)

// testFleet is a legal layout kept clear of row 8 and below, which leaves the loser somewhere
// harmless to shoot while the winner works through the cells above.
var testFleet = []game.Ship{
	{Class: game.Carrier, Origin: game.Coord{Row: 0, Col: 0}},
	{Class: game.Destroyer, Origin: game.Coord{Row: 0, Col: 6}},
	{Class: game.Battleship, Origin: game.Coord{Row: 2, Col: 0}},
	{Class: game.Cruiser, Origin: game.Coord{Row: 4, Col: 0}},
	{Class: game.Submarine, Origin: game.Coord{Row: 6, Col: 0}},
	{Class: game.Corvette, Origin: game.Coord{Row: 0, Col: 9}},
	{Class: game.Cutter, Origin: game.Coord{Row: 0, Col: 12}},
	{Class: game.Tender, Origin: game.Coord{Row: 4, Col: 9}},
}

func results(l *Lobby) chan string {
	out := make(chan string, 4)
	l.OnResult = func(winner, loser Player, ranked bool) {
		out <- fmt.Sprintf("%s beat %s ranked=%v", winner.Name, loser.Name, ranked)
	}
	return out
}

func reported(t *testing.T, out chan string, want string) {
	t.Helper()
	select {
	case got := <-out:
		if got != want {
			t.Errorf("reported %q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no result was reported, want %q", want)
	}
	select {
	case got := <-out:
		t.Errorf("the same game was reported twice, the second time as %q", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func placeFleet(t *testing.T, s *Session) {
	t.Helper()
	for _, ship := range testFleet {
		if err := s.Place(ship); err != nil {
			t.Fatalf("placing the %s: %v", ship.Class, err)
		}
	}
}

func cells(ships []game.Ship) []game.Coord {
	var all []game.Coord
	for _, s := range ships {
		all = append(all, s.Cells()...)
	}
	return all
}

// safeCells are the bottom rows, which testFleet never reaches into.
func safeCells() []game.Coord {
	var all []game.Coord
	for row := 8; row < game.Size; row++ {
		for col := range game.Size {
			all = append(all, game.Coord{Row: row, Col: col})
		}
	}
	return all
}

func TestAWonGameIsReportedOnceAndCountsForTheLeaderboard(t *testing.T) {
	l := New()
	out := results(l)
	host, _ := l.Create(alice)
	defer host.Close()
	guest, _ := l.Join(host.Code(), bob)
	defer guest.Close()

	placeFleet(t, host)
	placeFleet(t, guest)
	await(t, host, "firing", func(s Snapshot) bool { return s.Phase == Firing })

	targets, spare := cells(testFleet), safeCells()
	for i, c := range targets {
		if err := host.Fire(c); err != nil {
			t.Fatalf("shot %d at %v: %v", i, c, err)
		}
		if i == len(targets)-1 {
			break
		}
		if err := guest.Fire(spare[i]); err != nil {
			t.Fatalf("reply %d at %v: %v", i, spare[i], err)
		}
	}

	snap := await(t, host, "the win", func(s Snapshot) bool { return s.Phase == Over })
	if snap.Winner != game.P1 || snap.Forfeit {
		t.Fatalf("winner = %v forfeit = %v, want the host winning outright", snap.Winner, snap.Forfeit)
	}
	reported(t, out, "Alice beat Bob ranked=true")
}

func TestABotGameCountsForTheRecordButNotTheLeaderboard(t *testing.T) {
	l := New()
	out := results(l)
	s, _ := l.Bot(alice)
	defer s.Close()

	placeFleet(t, s)
	snap := await(t, s, "firing", func(s Snapshot) bool { return s.Phase == Firing })

	// The bot needs 17 shots of its own to win and only gets 16, so this always ends here.
	for i, c := range cells(snap.Game.Board(game.P2).Ships) {
		if err := s.Fire(c); err != nil {
			t.Fatalf("shot %d at %v: %v", i, c, err)
		}
	}

	if snap = await(t, s, "the win", func(s Snapshot) bool { return s.Phase == Over }); snap.Winner != game.P1 {
		t.Fatalf("winner = %v, want the player", snap.Winner)
	}
	reported(t, out, "Alice beat The bot ranked=false")
}

func TestAForfeitIsReportedForTheOpponent(t *testing.T) {
	l := NewWithGrace(50 * time.Millisecond)
	out := results(l)
	host, _ := l.Create(alice)
	defer host.Close()
	guest, _ := l.Join(host.Code(), bob)
	bothReady(t, host, guest)
	await(t, host, "firing", func(s Snapshot) bool { return s.Phase == Firing })

	guest.Close()
	await(t, host, "the forfeit", func(s Snapshot) bool { return s.Phase == Over })
	reported(t, out, "Alice beat Bob ranked=true")
}

func TestAnAbandonedRoomReportsNothing(t *testing.T) {
	l := New()
	out := results(l)
	host, _ := l.Create(alice)
	host.Close()

	select {
	case got := <-out:
		t.Errorf("a room nobody ever played in reported %q", got)
	case <-time.After(100 * time.Millisecond):
	}
}
