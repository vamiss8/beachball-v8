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

type PlayerState struct {
	Pos       Vector2 `json:"pos"`
	VelocityY float64 `json:"velocityY"`
	IsJumping bool    `json:"isJumping"`
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
}

type BallState struct {
	Pos    Vector2 `json:"pos"`
	Radius float64 `json:"radius"`
}

type GameState struct {
	Player PlayerState `json:"player"`
	Ball   BallState   `json:"ball"`
}

// global state and mutex for thread safety
var (
	state = GameState{
		Player: PlayerState{Pos: Vector2{X: 100, Y: 0}, Width: 60, Height: 60},
		Ball:   BallState{Pos: Vector2{X: 400, Y: 100}, Radius: 20},
	}
	stateMutex sync.Mutex
	
	// simple input tracking for the current player
	keys = struct {
		A, D, W bool
	}{}
)

// run physics loop at 60 fps
func gameLoop() {
	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()

	// constants (match frontend dimensions for now)
	canvasWidth, canvasHeight := 800.0, 600.0 // we will handle dynamic resizing later
	groundY := canvasHeight - 100.0 - state.Player.Height

	for range ticker.C {
		stateMutex.Lock()

		// 1. player horizontal movement
		speed := 7.0
		if keys.A {
			state.Player.Pos.X -= speed
		}
		if keys.D {
			state.Player.Pos.X += speed
		}

		// player boundaries
		if state.Player.Pos.X < 0 {
			state.Player.Pos.X = 0
		}
		if state.Player.Pos.X > canvasWidth/2-state.Player.Width-5 {
			state.Player.Pos.X = canvasWidth/2 - state.Player.Width - 5
		}

		// 2. player vertical movement & gravity
		if keys.W && !state.Player.IsJumping {
			state.Player.VelocityY = -15
			state.Player.IsJumping = true
		}

		state.Player.Pos.Y += state.Player.VelocityY
		state.Player.VelocityY += 0.8 // gravity

		if state.Player.Pos.Y >= groundY {
			state.Player.Pos.Y = groundY
			state.Player.VelocityY = 0
			state.Player.IsJumping = false
		}

		// 3. simple ball gravity
		state.Ball.Pos.Y += 2
		if state.Ball.Pos.Y > canvasHeight-100-state.Ball.Radius {
			state.Ball.Pos.Y = 100
		}

		stateMutex.Unlock()
	}
}

// broadcast state to client at 60 fps
func broadcastLoop(ws *websocket.Conn) {
	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()

	for range ticker.C {
		stateMutex.Lock()
		msg, _ := json.Marshal(map[string]interface{}{
			"type":  "state",
			"state": state,
		})
		stateMutex.Unlock()

		if err := ws.WriteMessage(websocket.TextMessage, msg); err != nil {
			break
		}
	}
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer ws.Close()

	fmt.Println("player connected")

	// start broadcasting state to this client
	go broadcastLoop(ws)

	// listen for inputs from client
	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			fmt.Println("player disconnected:", err)
			break
		}

		// parse inputs
		var inputData map[string]interface{}
		if err := json.Unmarshal(msg, &inputData); err == nil && inputData["type"] == "input" {
			inputs := inputData["keys"].(map[string]interface{})
			
			stateMutex.Lock()
			keys.A = inputs["a"].(bool)
			keys.D = inputs["d"].(bool)
			keys.W = inputs["w"].(bool)
			stateMutex.Unlock()
		}
	}
}

func main() {
	// start physics engine
	go gameLoop()

	http.HandleFunc("/ws", handleConnections)
	fmt.Println("server running on http://localhost:8080")
	
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("server error: ", err)
	}
}