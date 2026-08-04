// Package room owns the live match: it runs the simulation loop and fans the
// resulting snapshots out to every connected client.
package room

import (
	"log"
	"strconv"
	"sync"
	"time"

	"beachball-v8/server/internal/game"
	"beachball-v8/server/internal/protocol"
)

// MaxPlayers is how many people can actually play; everyone else spectates.
const MaxPlayers = 2

// EmptyRoomTTL is how long a room waits alone before closing. the grace
// period is what lets someone reload the page, or arrive a moment before
// their opponent, without the match evaporating.
const EmptyRoomTTL = 60 * time.Second

// playerInput carries a client's key state into the room goroutine.
type playerInput struct {
	playerID string
	keys     game.Input
}

// Room is a single match. every field below is touched only by run(), so the
// simulation needs no locks at all; the outside world talks to it exclusively
// through channels.
type Room struct {
	ID string

	world   *game.World
	clients map[*Client]struct{}

	register   chan *Client
	unregister chan *Client
	inputs     chan playerInput
	quit       chan struct{}

	nextPlayerID uint64

	// when the last client left, zero while anyone is still here
	emptySince time.Time
	// told the room's own id once it closes itself, so the manager can drop
	// it from the registry
	onEmpty func(string)

	closeOnce sync.Once
}

// newRoom creates a room and starts its simulation loop. onEmpty may be nil,
// which is what tests that own a single room directly want.
func newRoom(id string, onEmpty func(string)) *Room {
	r := &Room{
		ID:         id,
		world:      game.NewWorld(),
		clients:    make(map[*Client]struct{}),
		register:   make(chan *Client),
		unregister: make(chan *Client, MaxPlayers),
		inputs:     make(chan playerInput, 64),
		quit:       make(chan struct{}),
		onEmpty:    onEmpty,
	}
	go r.run()
	return r
}

// Close stops the simulation loop. safe to call more than once, since a room
// can also close itself once it has sat empty.
func (r *Room) Close() {
	r.closeOnce.Do(func() { close(r.quit) })
}

// run is the single owner of the room's state.
func (r *Room) run() {
	ticker := time.NewTicker(game.TickDuration)
	defer ticker.Stop()

	for {
		select {
		case <-r.quit:
			return

		case c := <-r.register:
			r.add(c)

		case c := <-r.unregister:
			r.remove(c)

		case in := <-r.inputs:
			if p, ok := r.world.Players[in.playerID]; ok {
				p.SetInput(in.keys)
			}

		case <-ticker.C:
			// an empty room has nobody to simulate for, so it idles instead
			// of burning a tick sixty times a second until it expires
			if len(r.clients) == 0 {
				if r.idleTooLong() {
					r.shutdown()
					return
				}
				continue
			}
			r.emptySince = time.Time{}

			r.world.Step()
			r.broadcastState()
		}
	}
}

// idleTooLong reports whether the room has been empty past its grace period.
func (r *Room) idleTooLong() bool {
	if r.emptySince.IsZero() {
		r.emptySince = time.Now()
		return false
	}
	return time.Since(r.emptySince) >= EmptyRoomTTL
}

// shutdown retires the room. the registry is told first so that nobody is
// handed a room that is already on its way out, and quit is closed second so
// a connection caught mid-join is released instead of blocking forever.
func (r *Room) shutdown() {
	if r.onEmpty != nil {
		r.onEmpty(r.ID)
	}
	r.Close()
	log.Printf("room %s: closed after %s empty", r.ID, EmptyRoomTTL)
}

// add seats a new client, as a player if a side is free and as a spectator
// otherwise, then greets it with a welcome message.
func (r *Room) add(c *Client) {
	r.clients[c] = struct{}{}

	if side, ok := r.freeSide(); ok {
		c.playerID = r.newPlayerID()
		c.side = side
		r.world.AddPlayer(c.playerID, side)
	} else {
		c.spectator = true
	}

	msg, err := protocol.Encode(protocol.TypeWelcome, protocol.Welcome{
		PlayerID:  c.playerID,
		Side:      c.side,
		Spectator: c.spectator,
		RoomID:    r.ID,
		Arena:     protocol.CurrentArena(),
	})
	if err != nil {
		log.Printf("room %s: encode welcome: %v", r.ID, err)
		return
	}
	c.trySend(msg)

	log.Printf("room %s: %s joined (spectator=%v)", r.ID, c.describe(), c.spectator)
}

// remove drops a client and frees its side for the next joiner.
func (r *Room) remove(c *Client) {
	if _, ok := r.clients[c]; !ok {
		return
	}
	delete(r.clients, c)
	close(c.send)

	if c.playerID != "" {
		r.world.RemovePlayer(c.playerID)
	}
	log.Printf("room %s: %s left", r.ID, c.describe())
}

// freeSide finds an unoccupied half of the court.
func (r *Room) freeSide() (game.Side, bool) {
	// checked explicitly instead of by player count, so a mid-match
	// disconnect always frees exactly the side that opened up
	for _, side := range []game.Side{game.SideLeft, game.SideRight} {
		if !r.world.SideTaken(side) {
			return side, true
		}
	}
	return "", false
}

func (r *Room) newPlayerID() string {
	r.nextPlayerID++
	return "player_" + strconv.FormatUint(r.nextPlayerID, 10)
}

// broadcastState marshals the world once and hands the same bytes to everyone.
func (r *Room) broadcastState() {
	msg, err := protocol.Encode(protocol.TypeState, protocol.State{World: r.world})
	if err != nil {
		log.Printf("room %s: encode state: %v", r.ID, err)
		return
	}
	for c := range r.clients {
		// a client that cannot keep up is dropped rather than allowed to
		// stall the simulation for everybody else
		if !c.trySend(msg) {
			c.close()
		}
	}
}
