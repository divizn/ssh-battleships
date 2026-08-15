package lobby

import (
	"slices"

	"github.com/divizn/ssh-battleships/internal/game"
)

// Session is one attached viewer of one room. It is the only handle the UI needs.
type Session struct {
	room *Room
	id   string
	ch   chan Snapshot
}

func (s *Session) Code() string { return s.room.Code }

// Events yields a snapshot after every change. It is closed when the room shuts down.
func (s *Session) Events() <-chan Snapshot { return s.ch }

func (s *Session) Place(ship game.Ship) error {
	return s.act(func(rm *room, seat game.Player) error { return rm.place(seat, ship) })
}

func (s *Session) AutoPlace() error {
	return s.act(func(rm *room, seat game.Player) error { return rm.autoPlace(seat) })
}

func (s *Session) Fire(c game.Coord) error {
	return s.act(func(rm *room, seat game.Player) error { return rm.fire(seat, c) })
}

// Close detaches this viewer. The game survives it: the seat is only given up once the
// grace period runs out.
func (s *Session) Close() {
	s.room.call(func(rm *room) error {
		rm.subs = slices.DeleteFunc(rm.subs, func(x sub) bool { return x.ch == s.ch })
		if seat, ok := rm.seat(s.id); ok && !rm.attached(seat) {
			rm.depart(seat)
		}
		rm.broadcast()
		rm.reap()
		return nil
	})
}

func (s *Session) act(f func(*room, game.Player) error) error {
	return s.room.call(func(rm *room) error {
		seat, ok := rm.seat(s.id)
		if !ok {
			return ErrNotSeated
		}
		if err := f(rm, seat); err != nil {
			return err
		}
		rm.broadcast()
		return nil
	})
}
