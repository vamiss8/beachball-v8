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
	Velocity Vector2 `json:"velocity"` // added velocity
	Radius   float64 `json:"radius"`
}

type GameState struct {
	Players map[string]*PlayerState `json:"players"`
	Ball    BallState               `json:"ball"`
}

// global state
var (
	state = GameState{
		Players: make(map[string]*PlayerState),
		// initial ball drop
		Ball: BallState{Pos: Vector2{X: 400, Y: 100}, Velocity: Vector2{X: 0, Y: 0}, Radius: 20},
	}
	stateMutex sync.Mutex
	clients    = make(map[*websocket.Conn]string)
	nextID     = 1
)

// run physics loop at 60 fps
func gameLoop() {
	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()

	canvasWidth, canvasHeight := 800.0, 600.0
	groundY := canvasHeight - 100.0

	// net dimensions (matches frontend: x = center-5, y = height-300, w = 10, h = 200)
	netWidth, netHeight := 10.0, 200.0
	netX := canvasWidth/2 - netWidth/2
	netY := canvasHeight - 300.0

	for range ticker.C {
		stateMutex.Lock()

		// 1. process players
		for _, p := range state.Players {
			playerGroundY := groundY - p.Height
			speed := 7.0

			if p.Inputs.A {
				p.Pos.X -= speed
			}
			if p.Inputs.D {
				p.Pos.X += speed
			}

			// horizontal bounds
			if p.Pos.X < 0 {
				p.Pos.X = 0
			}
			if p.Pos.X > canvasWidth-p.Width {
				p.Pos.X = canvasWidth - p.Width
			}

			// net collision for players
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

			if p.Pos.Y >= playerGroundY {
				p.Pos.Y = playerGroundY
				p.VelocityY = 0
				p.IsJumping = false
			}
		}

		// 2. ball physics
		state.Ball.Velocity.Y += 0.4 // ball gravity
		state.Ball.Pos.X += state.Ball.Velocity.X
		state.Ball.Pos.Y += state.Ball.Velocity.Y

		// ball boundaries (walls)
		if state.Ball.Pos.X < state.Ball.Radius {
			state.Ball.Pos.X = state.Ball.Radius
			state.Ball.Velocity.X *= -0.8 // bounce
		}
		if state.Ball.Pos.X > canvasWidth-state.Ball.Radius {
			state.Ball.Pos.X = canvasWidth - state.Ball.Radius
			state.Ball.Velocity.X *= -0.8
		}

		// ball boundaries (floor)
		if state.Ball.Pos.Y > groundY-state.Ball.Radius {
			state.Ball.Pos.Y = groundY - state.Ball.Radius
			state.Ball.Velocity.Y *= -0.7 // bounce
			state.Ball.Velocity.X *= 0.98 // friction
		}

		// 3. ball vs player collisions (circle vs AABB)
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

				bounceForce := 12.0
				state.Ball.Velocity.X = diffX * bounceForce
				state.Ball.Velocity.Y = diffY * bounceForce - 3.0 // extra lift
			}
		}

		// 4. ball vs net collision (circle vs AABB)
		closestNetX := math.Max(netX, math.Min(state.Ball.Pos.X, netX+netWidth))
		closestNetY := math.Max(netY, math.Min(state.Ball.Pos.Y, netY+netHeight))

		distNetX := state.Ball.Pos.X - closestNetX
		distNetY := state.Ball.Pos.Y - closestNetY
		distNetSquared := (distNetX * distNetX) + (distNetY * distNetY)

		if distNetSquared < (state.Ball.Radius * state.Ball.Radius) {
			dist := math.Sqrt(distNetSquared)
			if dist == 0 {
				dist = 0.1 // prevent division by zero
				distNetX = 0.1
				distNetY = 0
			}

			// normalize collision normal
			nx := distNetX / dist
			ny := distNetY / dist

			// calculate penetration depth and push the ball out
			penetration := state.Ball.Radius - dist
			state.Ball.Pos.X += nx * penetration
			state.Ball.Pos.Y += ny * penetration

			// reflect velocity vector: v = v - 2 * dot(v, n) * n
			dotProduct := state.Ball.Velocity.X*nx + state.Ball.Velocity.Y*ny
			
			// net absorbs a lot of energy, so bounce coefficient is low (0.4)
			bounce := 0.4
			state.Ball.Velocity.X = (state.Ball.Velocity.X - 2*dotProduct*nx) * bounce
			state.Ball.Velocity.Y = (state.Ball.Velocity.Y - 2*dotProduct*ny) * bounce
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

	startX := 100.0
	color := "#4caf50"
	side := "left"
	if len(state.Players)%2 != 0 {
		startX = 700.0 - 60.0
		color = "#2196f3"
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
