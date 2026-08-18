package room

import (
	"errors"
	"log"
	"sync"
)

// MaxRooms caps how many matches exist at once. without it, anyone could open
// endless codes and the server would happily allocate a simulation for each.
const MaxRooms = 500

var (
	// ErrBadCode means the code could never have been issued by this server.
	ErrBadCode = errors.New("invalid room code")
	// ErrTooManyRooms means the server is already at MaxRooms.
	ErrTooManyRooms = errors.New("too many rooms")
)

// Manager owns every live room and hands each connection to the right one.
//
// this is the only place in the package that takes a lock: rooms themselves
// stay lock-free because each one is driven by its own goroutine; the map of
// rooms is not game state, so guarding it with a mutex is fine.
type Manager struct {
	mu    sync.Mutex
	rooms map[string]*Room
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{rooms: make(map[string]*Room)}
}

// Join resolves a room code to a room, opening it if nobody has yet. an empty
// code asks for a fresh room, which is what someone gets when they open the
// site without an invite link.
//
// an unknown but well-formed code opens a room under that name rather than
// failing: a shared link keeps working even after the server restarts.
func (m *Manager) Join(rawCode string) (*Room, error) {
	if rawCode == "" {
		return m.open()
	}

	code, ok := NormalizeCode(rawCode)
	if !ok {
		return nil, ErrBadCode
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if r, ok := m.rooms[code]; ok {
		return r, nil
	}
	return m.insert(code)
}

// count reports how many rooms are live. unexported: nothing outside the
// package has a use for it, and the tests live in here anyway.
func (m *Manager) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.rooms)
}

// CloseAll stops every room, used on shutdown.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, r := range m.rooms {
		r.Close()
	}
	m.rooms = make(map[string]*Room)
}

// open picks a code nobody is using and starts a room behind it.
func (m *Manager) open() (*Room, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// collisions are rare enough that a handful of tries is plenty; giving up
	// beats looping forever once the space is genuinely crowded
	for attempt := 0; attempt < 10; attempt++ {
		code, err := NewCode()
		if err != nil {
			return nil, err
		}
		if _, taken := m.rooms[code]; taken {
			continue
		}
		return m.insert(code)
	}
	return nil, ErrTooManyRooms
}

// insert starts a room and registers it. the caller must hold the lock.
func (m *Manager) insert(code string) (*Room, error) {
	if len(m.rooms) >= MaxRooms {
		return nil, ErrTooManyRooms
	}

	r := newRoom(code, m.forget)
	m.rooms[code] = r
	log.Printf("room %s: opened (%d live)", code, len(m.rooms))
	return r, nil
}

// forget drops a room that closed itself. called from the room's own
// goroutine, so it must never call back into the room.
func (m *Manager) forget(code string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.rooms, code)
}
