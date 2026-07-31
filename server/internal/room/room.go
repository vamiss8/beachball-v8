// Package room owns the live match: it runs the simulation loop and fans the
// resulting snapshots out to every connected client.
package room

import (
	"log"
	"strconv"
	"sync/atomic"
	"time"

	"beachball-v8/server/internal/game"
	"beachball-v8/server/internal/protocol"
)

// MaxPlayers is how many people can actually play; everyone else spectates.
const MaxPlayers = 2

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

	nextPlayerID atomic.Uint64
}

// New creates a room and starts its simulation loop.
func New(id string) *Room {
	r := &Room{
		ID:         id,
		world:      game.NewWorld(),
		clients:    make(map[*Client]struct{}),
		register:   make(chan *Client),
		unregister: make(chan *Client, MaxPlayers),
		inputs:     make(chan playerInput, 64),
		quit:       make(chan struct{}),
	}
	go r.run()
	return r
}

// Close stops the simulation loop.
func (r *Room) Close() { close(r.quit) }

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
			r.world.Step()
			r.broadcastState()
		}
	}
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
	return "player_" + strconv.FormatUint(r.nextPlayerID.Add(1), 10)
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
