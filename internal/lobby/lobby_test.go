package lobby

import (
	"errors"
	"testing"
	"time"

	"github.com/divizn/ssh-battleships/internal/game"
)

var (
	alice = Player{ID: "key-a", Name: "Alice"}
	bob   = Player{ID: "key-b", Name: "Bob"}
	carol = Player{ID: "key-c", Name: "Carol"}
)

// await drains snapshots until one satisfies cond, so a test never depends on how many
// updates a single action happened to produce.
func await(t *testing.T, s *Session, what string, cond func(Snapshot) bool) Snapshot {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case snap, ok := <-s.Events():
			if !ok {
				t.Fatalf("room closed while waiting for %s", what)
			}
			if cond(snap) {
				return snap
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func bothReady(t *testing.T, a, b *Session) {
	t.Helper()
	if err := a.AutoPlace(); err != nil {
		t.Fatal(err)
	}
	if err := b.AutoPlace(); err != nil {
		t.Fatal(err)
	}
}

// water is an unshot cell with no ship on it, so firing there misses and the go passes over.
// Fleets are placed at random, so a test that needs a miss has to look one up.
func water(t *testing.T, b *game.Board) game.Coord {
	t.Helper()
	ship := map[game.Coord]bool{}
	for _, s := range b.Ships {
		for _, c := range s.Cells() {
			ship[c] = true
		}
	}
	for row := range game.Size {
		for col := range game.Size {
			c := game.Coord{Row: row, Col: col}
			if !ship[c] && !b.At(c).Known() {
				return c
			}
		}
	}
	t.Fatal("no unshot water left on the board")
	return game.Coord{}
}

func TestBotRoomIsPlayableImmediately(t *testing.T) {
	s, err := New().Bot(alice)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	snap := await(t, s, "the bot to be seated", func(s Snapshot) bool { return s.Phase == Placing })
	if !snap.VsBot || snap.Opponent.Name == "" {
		t.Fatalf("bot room snapshot = %+v, want an opponent already seated", snap)
	}
	if err := s.AutoPlace(); err != nil {
		t.Fatal(err)
	}
	snap = await(t, s, "firing", func(s Snapshot) bool { return s.Phase == Firing })

	if err := s.Fire(water(t, snap.Game.Board(game.P2))); err != nil {
		t.Fatal(err)
	}
	snap = await(t, s, "the bot to shoot back", func(s Snapshot) bool { return s.Last[game.P2].Set })
	if !snap.Last[game.P1].Set {
		t.Error("your own shot was not recorded")
	}
}

func TestCreateThenJoinSeatsBothPlayers(t *testing.T) {
	l := New()
	host, err := l.Create(alice)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()

	snap := await(t, host, "an empty room", func(s Snapshot) bool { return s.Phase == Waiting })
	if len(snap.Code) != codeLength {
		t.Errorf("room code %q, want %d letters", snap.Code, codeLength)
	}

	guest, err := l.Join(host.Code(), bob)
	if err != nil {
		t.Fatal(err)
	}
	defer guest.Close()

	snap = await(t, host, "bob to arrive", func(s Snapshot) bool { return s.Phase == Placing })
	if snap.Opponent != bob || snap.Seat != game.P1 {
		t.Errorf("host sees opponent %+v in seat %v, want Bob opposite seat P1", snap.Opponent, snap.Seat)
	}
	snap = await(t, guest, "guest to be seated", func(s Snapshot) bool { return s.Phase == Placing })
	if snap.Opponent != alice || snap.Seat != game.P2 {
		t.Errorf("guest sees opponent %+v in seat %v, want Alice opposite seat P2", snap.Opponent, snap.Seat)
	}
}

func TestJoinIsCaseAndSpaceInsensitive(t *testing.T) {
	l := New()
	host, err := l.Create(alice)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()

	guest, err := l.Join("  "+lower(host.Code())+" ", bob)
	if err != nil {
		t.Fatalf("join with a sloppily typed code: %v", err)
	}
	guest.Close()
}

func TestJoinRejectsAThirdPlayerAndAnUnknownCode(t *testing.T) {
	l := New()
	host, _ := l.Create(alice)
	defer host.Close()
	guest, _ := l.Join(host.Code(), bob)
	defer guest.Close()
	await(t, host, "bob to arrive", func(s Snapshot) bool { return s.Phase == Placing })

	if _, err := l.Join(host.Code(), carol); !errors.Is(err, ErrFull) {
		t.Errorf("third player got %v, want ErrFull", err)
	}
	if _, err := l.Join("ZZZZ", carol); !errors.Is(err, ErrNoSuchRoom) {
		t.Errorf("unknown code got %v, want ErrNoSuchRoom", err)
	}
}

func TestOnlyTheSideWhoseTurnItIsCanFire(t *testing.T) {
	l := New()
	host, _ := l.Create(alice)
	defer host.Close()
	guest, _ := l.Join(host.Code(), bob)
	defer guest.Close()
	bothReady(t, host, guest)
	snap := await(t, host, "firing", func(s Snapshot) bool { return s.Phase == Firing })

	if err := guest.Fire(game.Coord{Row: 0, Col: 0}); !errors.Is(err, ErrNotYourGo) {
		t.Errorf("guest firing first got %v, want ErrNotYourGo", err)
	}
	if err := host.Fire(water(t, snap.Game.Board(game.P2))); err != nil {
		t.Fatalf("host firing first: %v", err)
	}
	if err := host.Fire(game.Coord{Row: 1, Col: 1}); !errors.Is(err, ErrNotYourGo) {
		t.Errorf("host firing twice got %v, want ErrNotYourGo", err)
	}
	if err := guest.Fire(water(t, snap.Game.Board(game.P1))); err != nil {
		t.Fatalf("guest firing second: %v", err)
	}
}

func TestPlacingIsRefusedOnceTheGameStarts(t *testing.T) {
	l := New()
	host, _ := l.Create(alice)
	defer host.Close()
	guest, _ := l.Join(host.Code(), bob)
	defer guest.Close()
	bothReady(t, host, guest)
	await(t, host, "firing", func(s Snapshot) bool { return s.Phase == Firing })

	if err := host.Place(game.Ship{Class: game.Carrier}); !errors.Is(err, ErrNotPlacing) {
		t.Errorf("placing mid-game got %v, want ErrNotPlacing", err)
	}
}

func TestDroppingOutHoldsTheSeatThenForfeits(t *testing.T) {
	l := NewWithGrace(80 * time.Millisecond)
	host, _ := l.Create(alice)
	defer host.Close()
	guest, _ := l.Join(host.Code(), bob)
	bothReady(t, host, guest)
	await(t, host, "firing", func(s Snapshot) bool { return s.Phase == Firing })

	guest.Close()
	snap := await(t, host, "the opponent to go away", func(s Snapshot) bool { return s.Away })
	if snap.Phase != Firing {
		t.Errorf("phase during the grace period = %v, want the game still live", snap.Phase)
	}
	if snap.AwayUntil.IsZero() {
		t.Error("no deadline was published for the reconnect window")
	}

	snap = await(t, host, "the forfeit", func(s Snapshot) bool { return s.Phase == Over })
	if !snap.Forfeit || snap.Winner != game.P1 {
		t.Errorf("forfeit = %v, winner = %v, want a forfeit won by the host", snap.Forfeit, snap.Winner)
	}
}

func TestReconnectingWithTheSameKeyTakesTheSeatBack(t *testing.T) {
	l := NewWithGrace(2 * time.Second)
	host, _ := l.Create(alice)
	defer host.Close()
	guest, _ := l.Join(host.Code(), bob)
	bothReady(t, host, guest)
	await(t, host, "firing", func(s Snapshot) bool { return s.Phase == Firing })
	if err := host.Fire(game.Coord{Row: 3, Col: 3}); err != nil {
		t.Fatal(err)
	}

	guest.Close()
	await(t, host, "the opponent to go away", func(s Snapshot) bool { return s.Away })

	back, err := l.Join(host.Code(), bob)
	if err != nil {
		t.Fatalf("reconnecting: %v", err)
	}
	defer back.Close()

	snap := await(t, back, "the game in progress", func(s Snapshot) bool { return s.Phase == Firing })
	if snap.Seat != game.P2 {
		t.Errorf("reconnected into seat %v, want P2", snap.Seat)
	}
	if !snap.Last[game.P1].Set {
		t.Error("the shot fired while away was lost")
	}
	await(t, host, "the opponent to come back", func(s Snapshot) bool { return !s.Away })
}

func TestRoomIsForgottenOnceEveryoneHasGone(t *testing.T) {
	l := New()
	host, err := l.Create(alice)
	if err != nil {
		t.Fatal(err)
	}
	await(t, host, "an empty room", func(s Snapshot) bool { return s.Phase == Waiting })
	if l.Rooms() != 1 {
		t.Fatalf("lobby holds %d rooms, want 1", l.Rooms())
	}

	host.Close()
	for range 100 {
		if l.Rooms() == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("lobby still holds %d rooms after the last player left", l.Rooms())
}

func lower(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'A' && r <= 'Z' {
			out[i] = r + 32
		}
	}
	return string(out)
}
