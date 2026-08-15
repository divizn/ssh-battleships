package lobby

import (
	"errors"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/divizn/ssh-battleships/internal/ai"
)

var ErrNoSuchRoom = errors.New("no room with that code")

// codeLetters leaves out I and O, which are misread as 1 and 0 over a terminal.
const codeLetters = "ABCDEFGHJKLMNPQRSTUVWXYZ"

const codeLength = 4

type Lobby struct {
	mu    sync.Mutex
	rooms map[string]*Room
	rng   *rand.Rand
	grace time.Duration
}

func New() *Lobby {
	return NewWithGrace(Grace)
}

// NewWithGrace is New with a shorter reconnect window, for tests.
func NewWithGrace(grace time.Duration) *Lobby {
	return &Lobby{
		rooms: map[string]*Room{},
		rng:   rand.New(rand.NewSource(time.Now().UnixNano())),
		grace: grace,
	}
}

// Bot opens a private room already seated by the computer.
func (l *Lobby) Bot(p Player) (*Session, error) {
	return l.open(p, func(rng *rand.Rand) *ai.Bot { return ai.New(rng) })
}

// Create opens a room and returns its code for a friend to join with.
func (l *Lobby) Create(p Player) (*Session, error) {
	return l.open(p, func(*rand.Rand) *ai.Bot { return nil })
}

func (l *Lobby) open(p Player, bot func(*rand.Rand) *ai.Bot) (*Session, error) {
	l.mu.Lock()
	code := l.freeCode()
	rng := rand.New(rand.NewSource(l.rng.Int63()))
	room := newRoom(code, p, bot(rng), rng, l.grace, func() { l.forget(code) })
	l.rooms[code] = room
	l.mu.Unlock()

	return room.join(p)
}

// Join seats a player in an existing room, or reattaches them if they were already in it.
func (l *Lobby) Join(code string, p Player) (*Session, error) {
	l.mu.Lock()
	room, ok := l.rooms[Normalise(code)]
	l.mu.Unlock()
	if !ok {
		return nil, ErrNoSuchRoom
	}
	return room.join(p)
}

// Normalise turns whatever the player typed into a code this lobby would recognise.
func Normalise(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func (l *Lobby) Rooms() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.rooms)
}

func (l *Lobby) forget(code string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.rooms, code)
}

// freeCode must be called with the lock held.
func (l *Lobby) freeCode() string {
	for {
		b := make([]byte, codeLength)
		for i := range b {
			b[i] = codeLetters[l.rng.Intn(len(codeLetters))]
		}
		if code := string(b); l.rooms[code] == nil {
			return code
		}
	}
}
