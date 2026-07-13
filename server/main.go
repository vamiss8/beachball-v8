package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
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
	Side      string  `json:"side"`
	Inputs    Inputs  `json:"-"`
}

type BallState struct {
	Pos      Vector2 `json:"pos"`
	Velocity Vector2 `json:"velocity"`
	Radius   float64 `json:"radius"`
	HitCount int     `json:"hitCount"` // tracks rally length for acceleration
}

type GameState struct {
	Players map[string]*PlayerState `json:"players"`
	Ball    BallState               `json:"ball"`
	Score   map[string]int          `json:"score"`
	State   string                  `json:"state"` // "playing" or "scored"
}

// global state
var (
	state = GameState{
		Players: make(map[string]*PlayerState),
		Ball:    BallState{Pos: Vector2{X: 800, Y: 200}, Velocity: Vector2{X: 0, Y: 0}, Radius: 45, HitCount: 0},
		Score:   map[string]int{"left": 0, "right": 0},
		State:   "playing",
	}
	stateMutex sync.Mutex
	clients    = make(map[*websocket.Conn]string)
	nextID     = 1
	resetTicks = 0 // timer for round reset
)

// run physics loop at 60 fps
func gameLoop() {
	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()

	canvasWidth, canvasHeight := 1600.0, 900.0
	groundY := canvasHeight - 100.0

	netWidth, netHeight := 20.0, 240.0
	netX := canvasWidth/2 - netWidth/2
	netY := groundY - netHeight

	for range ticker.C {
		stateMutex.Lock()

		// 1. process players (can move even during score pause)
		for _, p := range state.Players {
			playerGroundY := groundY - p.Height
			speed := 14.0

			if p.Inputs.A {
				p.Pos.X -= speed
			}
			if p.Inputs.D {
				p.Pos.X += speed
			}

			if p.Pos.X < 0 {
				p.Pos.X = 0
			}
			if p.Pos.X > canvasWidth-p.Width {
				p.Pos.X = canvasWidth - p.Width
			}

			if p.Side == "left" {
				if p.Pos.X > canvasWidth/2-p.Width-10 {
					p.Pos.X = canvasWidth/2 - p.Width - 10
				}
			} else {
				if p.Pos.X < canvasWidth/2+10 {
					p.Pos.X = canvasWidth/2 + 10
				}
			}

			if p.Inputs.W && !p.IsJumping {
				p.VelocityY = -24
				p.IsJumping = true
			}

			p.Pos.Y += p.VelocityY
			p.VelocityY += 1.2 // player gravity

			if p.Pos.Y >= playerGroundY {
				p.Pos.Y = playerGroundY
				p.VelocityY = 0
				p.IsJumping = false
			}
		}

		// handle round reset state
		if state.State == "scored" {
			resetTicks--
			// apply friction so ball stops rolling
			state.Ball.Pos.X += state.Ball.Velocity.X
			state.Ball.Velocity.X *= 0.95
			if resetTicks <= 0 {
				resetRound(canvasWidth, groundY)
			}
			stateMutex.Unlock()
			continue // skip ball physics until reset
		}

		// 2. ball physics
		// ball gravity increases with rally length
		dynamicGravity := 0.8 + (float64(state.Ball.HitCount) * 0.05)
		if dynamicGravity > 2.5 {
			dynamicGravity = 2.5 // cap max gravity
		}

		state.Ball.Velocity.Y += dynamicGravity
		state.Ball.Pos.X += state.Ball.Velocity.X
		state.Ball.Pos.Y += state.Ball.Velocity.Y

		if state.Ball.Pos.X < state.Ball.Radius {
			state.Ball.Pos.X = state.Ball.Radius
			state.Ball.Velocity.X *= -0.8
		}
		if state.Ball.Pos.X > canvasWidth-state.Ball.Radius {
			state.Ball.Pos.X = canvasWidth - state.Ball.Radius
			state.Ball.Velocity.X *= -0.8
		}

		// ball hits floor - scoring logic!
		if state.Ball.Pos.Y > groundY-state.Ball.Radius {
			state.Ball.Pos.Y = groundY - state.Ball.Radius
			state.Ball.Velocity.Y *= -0.3 // weak bounce on sand

			if state.Ball.Pos.X < canvasWidth/2 {
				state.Score["right"]++
			} else {
				state.Score["left"]++
			}
			state.State = "scored"
			resetTicks = 120 // wait 2 seconds (120 ticks at 60fps)
		}

		// 3. ball vs player collisions
		for _, p := range state.Players {
			closestX := math.Max(p.Pos.X, math.Min(state.Ball.Pos.X, p.Pos.X+p.Width))
			closestY := math.Max(p.Pos.Y, math.Min(state.Ball.Pos.Y, p.Pos.Y+p.Height))

			distX := state.Ball.Pos.X - closestX
			distY := state.Ball.Pos.Y - closestY
			distSquared := (distX * distX) + (distY * distY)

			if distSquared < (state.Ball.Radius * state.Ball.Radius) {
				playerCenterX := p.Pos.X + p.Width/2
				playerCenterY := p.Pos.Y + p.Height/2

				diffX := state.Ball.Pos.X - playerCenterX
				diffY := state.Ball.Pos.Y - playerCenterY

				length := math.Sqrt(diffX*diffX + diffY*diffY)
				if length > 0 {
					diffX /= length
					diffY /= length
				}

				// bounce force increases with rally length
				bounceForce := 14.0 + (float64(state.Ball.HitCount) * 0.8)
				if bounceForce > 35.0 {
					bounceForce = 35.0 // cap max speed
				}

				state.Ball.Velocity.X = diffX * bounceForce
				state.Ball.Velocity.Y = diffY * bounceForce - 3.0
				
				state.Ball.HitCount++ // increase rally hits
			}
		}

		// 4. ball vs net
		closestNetX := math.Max(netX, math.Min(state.Ball.Pos.X, netX+netWidth))
		closestNetY := math.Max(netY, math.Min(state.Ball.Pos.Y, netY+netHeight))

		distNetX := state.Ball.Pos.X - closestNetX
		distNetY := state.Ball.Pos.Y - closestNetY
		distNetSquared := (distNetX * distNetX) + (distNetY * distNetY)

		if distNetSquared < (state.Ball.Radius * state.Ball.Radius) {
			dist := math.Sqrt(distNetSquared)
			if dist == 0 { dist = 0.1 }

			nx := distNetX / dist
			ny := distNetY / dist
			penetration := state.Ball.Radius - dist
			state.Ball.Pos.X += nx * penetration
			state.Ball.Pos.Y += ny * penetration

			dotProduct := state.Ball.Velocity.X*nx + state.Ball.Velocity.Y*ny
			bounce := 0.4
			state.Ball.Velocity.X = (state.Ball.Velocity.X - 2*dotProduct*nx) * bounce
			state.Ball.Velocity.Y = (state.Ball.Velocity.Y - 2*dotProduct*ny) * bounce
		}

		stateMutex.Unlock()
	}
}

// resets positions and ball after a score
func resetRound(canvasWidth float64, groundY float64) {
	state.Ball.Pos = Vector2{X: canvasWidth / 2, Y: 200}
	state.Ball.Velocity = Vector2{X: 0, Y: 0}
	state.Ball.HitCount = 0

	for _, p := range state.Players {
		if p.Side == "left" {
			p.Pos = Vector2{X: 300, Y: groundY - p.Height}
		} else {
			p.Pos = Vector2{X: 1600 - 300 - p.Width, Y: groundY - p.Height}
		}
		p.VelocityY = 0
		p.IsJumping = false
	}
	state.State = "playing"
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

		for conn := range clients {
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

	stateMutex.Lock()
	playerID := fmt.Sprintf("player_%d", nextID)
	nextID++

	startX := 200.0
	color := "#4caf50"
	side := "left"
	if len(state.Players)%2 != 0 {
		startX = 1600 - 300 - 120 // spawn second player on the right
		color = "#2196f3"
		side = "right"
	}

	state.Players[playerID] = &PlayerState{
		Id:     playerID,
		Pos:    Vector2{X: startX, Y: 0},
		Width:  120,
		Height: 120,
		Color:  color,
		Side:   side,
	}
	clients[ws] = playerID
	stateMutex.Unlock()

	fmt.Println(playerID, "connected")

	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			fmt.Println(playerID, "disconnected")
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
	go broadcastLoop()

	http.HandleFunc("/ws", handleConnections)
	fmt.Println("server running on http://localhost:8080")

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("server error: ", err)
	}
}
