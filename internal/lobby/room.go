package lobby

import (
	"errors"
	"math/rand"
	"slices"
	"time"

	"github.com/divizn/ssh-battleships/internal/ai"
	"github.com/divizn/ssh-battleships/internal/game"
)

// Grace is how long a room holds a game open for a player who dropped out.
const Grace = 60 * time.Second

var (
	ErrClosed     = errors.New("that room is gone")
	ErrFull       = errors.New("that room already has two players")
	ErrNotSeated  = errors.New("you are not in this room")
	ErrNotPlacing = errors.New("ships cannot be moved now")
	ErrNotFiring  = errors.New("the game has not started")
	ErrNotYourGo  = errors.New("it is not your turn")
)

type Phase int

const (
	Waiting Phase = iota // one seat still empty
	Placing
	Firing
	Over
)

type Player struct {
	ID   string // ssh public key fingerprint
	Name string
}

// Shot is the last thing one side did, kept so the view can narrate it.
type Shot struct {
	Set bool
	At  game.Coord
	Res game.Result
}

// Snapshot is the whole room state as one side sees it. Sessions render server side, so
// carrying both boards leaks nothing; the view decides what to draw.
type Snapshot struct {
	Code      string
	Phase     Phase
	Game      game.Game
	Seat      game.Player
	You       Player
	Opponent  Player
	VsBot     bool
	Away      bool // the opponent dropped and the clock is running
	AwayUntil time.Time
	Winner    game.Player
	Forfeit   bool
	Last      [2]Shot
}

// Room is the handle other goroutines hold. Every field of the game itself lives in room,
// which only the actor goroutine ever touches.
type Room struct {
	Code string

	cmds   chan func(*room)
	closed chan struct{}
}

type sub struct {
	id string
	ch chan Snapshot
}

type room struct {
	pub      *Room
	game     game.Game
	phase    Phase
	seats    [2]Player
	taken    [2]bool
	subs     []sub
	bot      *ai.Bot
	rng      *rand.Rand
	last     [2]Shot
	winner   game.Player
	forfeit  bool
	away     [2]bool
	awayAt   [2]time.Time
	timers   [2]*time.Timer
	grace    time.Duration
	expire   chan game.Player
	onClose  func()
	onResult Result
	done     bool
}

// Result is told who won a finished game. Ranked is false for bot games, which still count
// towards a player's own record. It runs off the actor goroutine, so it may block.
type Result func(winner, loser Player, ranked bool)

func newRoom(code string, host Player, bot *ai.Bot, rng *rand.Rand, grace time.Duration, onResult Result, onClose func()) *Room {
	pub := &Room{
		Code:   code,
		cmds:   make(chan func(*room)),
		closed: make(chan struct{}),
	}
	rm := &room{
		pub:      pub,
		phase:    Waiting,
		bot:      bot,
		rng:      rng,
		grace:    grace,
		expire:   make(chan game.Player, 2),
		onResult: onResult,
		onClose:  onClose,
	}
	rm.seats[game.P1], rm.taken[game.P1] = host, true
	if bot != nil {
		rm.seats[game.P2] = Player{ID: "bot", Name: "The bot"}
		rm.taken[game.P2] = true
		rm.phase = Placing
		game.AutoPlace(rm.game.Board(game.P2), rm.rng)
	}
	go rm.run()
	return pub
}

func (rm *room) run() {
	for !rm.done {
		select {
		case c := <-rm.pub.cmds:
			c(rm)
		case seat := <-rm.expire:
			rm.timeout(seat)
		}
	}
	close(rm.pub.closed)
	for _, s := range rm.subs {
		close(s.ch)
	}
	rm.onClose()
}

// call runs f on the actor goroutine and waits for its answer.
func (r *Room) call(f func(*room) error) error {
	reply := make(chan error, 1)
	select {
	case r.cmds <- func(rm *room) { reply <- f(rm) }:
		return <-reply
	case <-r.closed:
		return ErrClosed
	}
}

// join seats p if there is room, or reattaches them if they are already seated. A player
// returning on the same public key lands back in their own game.
func (r *Room) join(p Player) (*Session, error) {
	ch := make(chan Snapshot, 1)
	err := r.call(func(rm *room) error {
		seat, ok := rm.seat(p.ID)
		if !ok {
			if rm.taken[game.P2] {
				return ErrFull
			}
			rm.seats[game.P2], rm.taken[game.P2] = p, true
			rm.phase = Placing
			seat = game.P2
		}
		rm.subs = append(rm.subs, sub{p.ID, ch})
		rm.arrive(seat)
		rm.broadcast()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Session{room: r, id: p.ID, ch: ch}, nil
}

func (rm *room) seat(id string) (game.Player, bool) {
	for i := range rm.seats {
		if rm.taken[i] && rm.seats[i].ID == id {
			return game.Player(i), true
		}
	}
	return 0, false
}

func (rm *room) attached(seat game.Player) bool {
	return slices.ContainsFunc(rm.subs, func(s sub) bool { return s.id == rm.seats[seat].ID })
}

func (rm *room) arrive(seat game.Player) {
	if rm.timers[seat] != nil {
		rm.timers[seat].Stop()
		rm.timers[seat] = nil
	}
	rm.away[seat] = false
	rm.awayAt[seat] = time.Time{}
}

// depart starts the forfeit clock for a seat nobody is watching any more.
func (rm *room) depart(seat game.Player) {
	if rm.phase != Placing && rm.phase != Firing {
		return
	}
	if rm.bot != nil && seat == game.P2 {
		return
	}
	rm.away[seat] = true
	rm.awayAt[seat] = time.Now().Add(rm.grace)
	rm.timers[seat] = time.AfterFunc(rm.grace, func() {
		select {
		case rm.expire <- seat:
		case <-rm.pub.closed:
		}
	})
}

func (rm *room) timeout(seat game.Player) {
	if rm.phase == Over || rm.attached(seat) {
		return
	}
	rm.phase = Over
	rm.winner = seat.Other()
	rm.forfeit = true
	rm.report()
	rm.broadcast()
	rm.reap()
}

func (rm *room) ready(seat game.Player) bool {
	return len(rm.game.Board(seat).Ships) == len(game.Fleet)
}

func (rm *room) place(seat game.Player, s game.Ship) error {
	if rm.phase != Placing {
		return ErrNotPlacing
	}
	if err := rm.game.Board(seat).Place(s); err != nil {
		return err
	}
	rm.startIfReady()
	return nil
}

func (rm *room) autoPlace(seat game.Player) error {
	if rm.phase != Placing {
		return ErrNotPlacing
	}
	game.AutoPlace(rm.game.Board(seat), rm.rng)
	rm.startIfReady()
	return nil
}

func (rm *room) startIfReady() {
	if rm.ready(game.P1) && rm.ready(game.P2) {
		rm.phase = Firing
	}
}

func (rm *room) fire(seat game.Player, c game.Coord) error {
	if rm.phase != Firing {
		return ErrNotFiring
	}
	if rm.game.Turn != seat {
		return ErrNotYourGo
	}
	res, err := rm.game.Fire(c)
	if err != nil {
		return err
	}
	rm.last[seat] = Shot{Set: true, At: c, Res: res}
	if rm.game.Over {
		rm.finish()
		return nil
	}
	rm.botTurn()
	return nil
}

// botTurn plays the bot's whole go: a hit earns it another shot, so it fires until it misses.
func (rm *room) botTurn() {
	for rm.bot != nil && rm.phase == Firing && rm.game.Turn == game.P2 {
		c := rm.bot.NextShot()
		res, err := rm.game.Fire(c)
		if err != nil {
			return
		}
		rm.bot.Record(c, res)
		rm.last[game.P2] = Shot{Set: true, At: c, Res: res}
		if rm.game.Over {
			rm.finish()
		}
	}
}

func (rm *room) finish() {
	rm.phase = Over
	rm.winner = rm.game.Winner
	rm.report()
}

// report hands the result off in its own goroutine: writing it out is somebody else's
// network call and the actor must not sit and wait for it.
func (rm *room) report() {
	if rm.onResult == nil {
		return
	}
	winner, loser, ranked := rm.seats[rm.winner], rm.seats[rm.winner.Other()], rm.bot == nil
	go rm.onResult(winner, loser, ranked)
}

func (rm *room) snapshot(seat game.Player) Snapshot {
	other := seat.Other()
	return Snapshot{
		Code:      rm.pub.Code,
		Phase:     rm.phase,
		Game:      rm.game.Clone(),
		Seat:      seat,
		You:       rm.seats[seat],
		Opponent:  rm.seats[other],
		VsBot:     rm.bot != nil,
		Away:      rm.away[other],
		AwayUntil: rm.awayAt[other],
		Winner:    rm.winner,
		Forfeit:   rm.forfeit,
		Last:      rm.last,
	}
}

// broadcast pushes the latest state to every attached session, dropping any snapshot a slow
// session has not read yet. Snapshots are absolute, so only the newest one matters.
func (rm *room) broadcast() {
	for _, s := range rm.subs {
		seat, ok := rm.seat(s.id)
		if !ok {
			continue
		}
		snap := rm.snapshot(seat)
		select {
		case <-s.ch:
		default:
		}
		select {
		case s.ch <- snap:
		default:
		}
	}
}

// reap closes a room nobody is left to play in.
func (rm *room) reap() {
	if len(rm.subs) > 0 {
		return
	}
	if rm.bot != nil || rm.phase == Over || rm.phase == Waiting {
		rm.shut()
	}
}

func (rm *room) shut() {
	for i := range rm.timers {
		if rm.timers[i] != nil {
			rm.timers[i].Stop()
		}
	}
	rm.done = true
}
