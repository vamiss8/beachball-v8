package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// websocket upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// game state structures
type Vector2 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Inputs struct {
	A bool `json:"a"`
	D bool `json:"d"`
	W bool `json:"w"`
}

type PlayerState struct {
	Id        string  `json:"id"`
	Pos       Vector2 `json:"pos"`
	VelocityY float64 `json:"velocityY"`
	IsJumping bool    `json:"isJumping"`
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
	Color     string  `json:"color"`
	Side      string  `json:"side"` // "left" or "right"
	Inputs    Inputs  `json:"-"`    // hide inputs from json payload
}

type BallState struct {
	Pos    Vector2 `json:"pos"`
	Radius float64 `json:"radius"`
}

type GameState struct {
	Players map[string]*PlayerState `json:"players"` // changed to map of players
	Ball    BallState               `json:"ball"`
}

// global state
var (
	state = GameState{
		Players: make(map[string]*PlayerState),
		Ball:    BallState{Pos: Vector2{X: 400, Y: 100}, Radius: 20},
	}
	stateMutex sync.Mutex
	clients    = make(map[*websocket.Conn]string) // track connections
	nextID     = 1
)

// run physics loop at 60 fps
func gameLoop() {
	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()

	canvasWidth, canvasHeight := 800.0, 600.0

	for range ticker.C {
		stateMutex.Lock()

		// 1. process all players
		for _, p := range state.Players {
			groundY := canvasHeight - 100.0 - p.Height
			speed := 7.0

			// horizontal movement
			if p.Inputs.A {
				p.Pos.X -= speed
			}
			if p.Inputs.D {
				p.Pos.X += speed
			}

			// general canvas boundaries
			if p.Pos.X < 0 {
				p.Pos.X = 0
			}
			if p.Pos.X > canvasWidth-p.Width {
				p.Pos.X = canvasWidth - p.Width
			}

			// net collision based on player side
			if p.Side == "left" {
				if p.Pos.X > canvasWidth/2-p.Width-5 {
					p.Pos.X = canvasWidth/2 - p.Width - 5
				}
			} else {
				if p.Pos.X < canvasWidth/2+5 {
					p.Pos.X = canvasWidth/2 + 5
				}
			}

			// vertical movement & gravity
			if p.Inputs.W && !p.IsJumping {
				p.VelocityY = -15
				p.IsJumping = true
			}

			p.Pos.Y += p.VelocityY
			p.VelocityY += 0.8 // gravity

			if p.Pos.Y >= groundY {
				p.Pos.Y = groundY
				p.VelocityY = 0
				p.IsJumping = false
			}
		}

		// 2. simple ball gravity
		state.Ball.Pos.Y += 2
		if state.Ball.Pos.Y > canvasHeight-100-state.Ball.Radius {
			state.Ball.Pos.Y = 100
		}

		stateMutex.Unlock()
	}
}

// broadcast state to all clients at 60 fps
func broadcastLoop() {
	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()

	for range ticker.C {
		stateMutex.Lock()
		msg, _ := json.Marshal(map[string]interface{}{
			"type":  "state",
			"state": state,
		})

		// send to all connected clients
		for conn := range clients {
			// ignore errors for now, handled in read loop
			conn.WriteMessage(websocket.TextMessage, msg)
		}
		stateMutex.Unlock()
	}
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer ws.Close()

	// register new player
	stateMutex.Lock()
	playerID := fmt.Sprintf("player_%d", nextID)
	nextID++

	// determine side and color
	startX := 100.0
	color := "#4caf50" // green for left
	side := "left"
	if len(state.Players)%2 != 0 {
		startX = 700.0 - 60.0
		color = "#2196f3" // blue for right
		side = "right"
	}

	state.Players[playerID] = &PlayerState{
		Id:     playerID,
		Pos:    Vector2{X: startX, Y: 0},
		Width:  60,
		Height: 60,
		Color:  color,
		Side:   side,
	}
	clients[ws] = playerID
	stateMutex.Unlock()

	fmt.Println(playerID, "connected")

	// listen for inputs from this client
	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			fmt.Println(playerID, "disconnected")
			// cleanup on disconnect
			stateMutex.Lock()
			delete(state.Players, playerID)
			delete(clients, ws)
			stateMutex.Unlock()
			break
		}

		var inputData map[string]interface{}
		if err := json.Unmarshal(msg, &inputData); err == nil && inputData["type"] == "input" {
			inputs := inputData["keys"].(map[string]interface{})

			stateMutex.Lock()
			if p, exists := state.Players[playerID]; exists {
				p.Inputs.A = inputs["a"].(bool)
				p.Inputs.D = inputs["d"].(bool)
				p.Inputs.W = inputs["w"].(bool)
			}
			stateMutex.Unlock()
		}
	}
}

func main() {
	go gameLoop()
	go broadcastLoop() // global broadcast routine

	http.HandleFunc("/ws", handleConnections)
	fmt.Println("server running on http://localhost:8080")

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("server error: ", err)
	}
}