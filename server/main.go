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

// core math vectors
type Vector2 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Inputs struct {
	A     bool `json:"a"`
	D     bool `json:"d"`
	W     bool `json:"w"`
	S     bool `json:"s"`
	DashL bool `json:"dashL"`
	DashR bool `json:"dashR"`
}

// advanced player state
type PlayerState struct {
	Id              string  `json:"id"`
	Pos             Vector2 `json:"pos"`
	VelocityX       float64 `json:"velocityX"`
	VelocityY       float64 `json:"velocityY"`
	DashVelocity    float64 `json:"-"`
	Rotation        float64 `json:"rotation"`
	IsJumping       bool    `json:"isJumping"`
	CanDoubleJump   bool    `json:"-"`
	IsSomersaulting bool    `json:"isSomersaulting"`
	IsBlocking      bool    `json:"isBlocking"`
	DashesLeft      int     `json:"-"`
	DashCooldown    int     `json:"-"`
	PrevW           bool    `json:"-"` // track edge detection for double jump
	Width           float64 `json:"width"`
	Height          float64 `json:"height"`
	Color           string  `json:"color"`
	Side            string  `json:"side"`
	Inputs          Inputs  `json:"-"`
}

type BallState struct {
	Pos      Vector2 `json:"pos"`
	Velocity Vector2 `json:"velocity"`
	Radius   float64 `json:"radius"`
	HitCount int     `json:"hitCount"`
}

type GameState struct {
	Players map[string]*PlayerState `json:"players"`
	Ball    BallState               `json:"ball"`
	Score   map[string]int          `json:"score"`
	State   string                  `json:"state"`
}

// global game state
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
	resetTicks = 0
)

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

		// 1. process players
		for _, p := range state.Players {
			playerGroundY := groundY - p.Height

			// cooldowns
			if p.DashCooldown > 0 {
				p.DashCooldown--
			}

			// dash mechanics
			if p.Inputs.DashL && p.DashesLeft > 0 && p.DashCooldown <= 0 {
				p.DashVelocity = -35.0
				p.DashesLeft--
				p.DashCooldown = 30 // 0.5 sec
				p.IsSomersaulting = true
			}
			if p.Inputs.DashR && p.DashesLeft > 0 && p.DashCooldown <= 0 {
				p.DashVelocity = 35.0
				p.DashesLeft--
				p.DashCooldown = 30
				p.IsSomersaulting = true
			}

			// horizontal movement
			baseSpeed := 0.0
			if p.Inputs.A {
				baseSpeed = -10.0
			}
			if p.Inputs.D {
				baseSpeed = 10.0
			}

			p.VelocityX = baseSpeed + p.DashVelocity
			p.Pos.X += p.VelocityX
			p.DashVelocity *= 0.85 // rapid decay for snappy dashes

			// bounds
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

			// vertical & jump mechanics (edge detection on W)
			justPressedW := p.Inputs.W && !p.PrevW
			p.PrevW = p.Inputs.W

			if justPressedW {
				if !p.IsJumping {
					p.VelocityY = -18.0
					p.IsJumping = true
					p.CanDoubleJump = true
				} else if p.CanDoubleJump {
					p.VelocityY = -16.0
					p.CanDoubleJump = false
					p.IsSomersaulting = true
				}
			}

			// block mechanic
			p.IsBlocking = p.Inputs.S && p.IsJumping
			if p.IsBlocking {
				p.VelocityY += 1.8 // fast fall while blocking
			} else {
				p.VelocityY += 0.6 // floatier standard gravity
			}

			p.Pos.Y += p.VelocityY

			// ground collision
			if p.Pos.Y >= playerGroundY {
				p.Pos.Y = playerGroundY
				p.VelocityY = 0
				p.IsJumping = false
				p.IsSomersaulting = false
				p.IsBlocking = false
				p.CanDoubleJump = true
				p.DashesLeft = 2 // reset dashes on ground
			}

			// animation state (rotation)
			if p.IsSomersaulting {
				if p.Side == "left" {
					p.Rotation += 0.35
				} else {
					p.Rotation -= 0.35
				}
			} else if p.IsBlocking {
				if p.Side == "left" {
					p.Rotation = math.Pi / 4 // 45 degrees
				} else {
					p.Rotation = -math.Pi / 4
				}
			} else {
				// reset rotation
				p.Rotation = 0
			}
		}

		if state.State == "scored" {
			resetTicks--
			state.Ball.Pos.X += state.Ball.Velocity.X
			state.Ball.Velocity.X *= 0.95
			if resetTicks <= 0 {
				resetRound(canvasWidth, groundY)
			}
			stateMutex.Unlock()
			continue
		}

		// 2. ball physics
		dynamicGravity := 0.2 + (float64(state.Ball.HitCount) * 0.02)
		if dynamicGravity > 1.2 {
			dynamicGravity = 1.2
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

		if state.Ball.Pos.Y > groundY-state.Ball.Radius {
			state.Ball.Pos.Y = groundY - state.Ball.Radius
			state.Ball.Velocity.Y *= -0.3

			if state.Ball.Pos.X < canvasWidth/2 {
				state.Score["right"]++
			} else {
				state.Score["left"]++
			}
			state.State = "scored"
			resetTicks = 120
		}

		// 3. ball vs player collisions (proper impulse physics)
		for _, p := range state.Players {
			closestX := math.Max(p.Pos.X, math.Min(state.Ball.Pos.X, p.Pos.X+p.Width))
			closestY := math.Max(p.Pos.Y, math.Min(state.Ball.Pos.Y, p.Pos.Y+p.Height))

			distX := state.Ball.Pos.X - closestX
			distY := state.Ball.Pos.Y - closestY
			distSquared := (distX * distX) + (distY * distY)

			if distSquared < (state.Ball.Radius * state.Ball.Radius) {
				dist := math.Sqrt(distSquared)
				if dist == 0 {
					dist = 0.1
				}

				// collision normal (pointing from player to ball)
				nx := distX / dist
				ny := distY / dist

				// push ball out of player to prevent sticking
				penetration := state.Ball.Radius - dist
				state.Ball.Pos.X += nx * penetration
				state.Ball.Pos.Y += ny * penetration

				// calculate relative velocity
				relVelX := state.Ball.Velocity.X - p.VelocityX
				relVelY := state.Ball.Velocity.Y - p.VelocityY

				// velocity along the normal
				dot := relVelX*nx + relVelY*ny

				// only resolve if objects are moving towards each other
				if dot < 0 {
					restitution := 0.8 // default bounce

					if p.IsBlocking {
						restitution = 0.2 // deaden the ball
						// force deflection forward and down
						if p.Side == "left" {
							nx = 0.8
							ny = 0.5
						} else {
							nx = -0.8
							ny = 0.5
						}
					} else if p.IsSomersaulting {
						restitution = 1.6 // smash multiplier!
					}

					// calculate impulse and apply to ball
					impulse := -(1 + restitution) * dot
					state.Ball.Velocity.X += impulse * nx
					state.Ball.Velocity.Y += impulse * ny

					// transfer a portion of player's momentum for control (allows juggling)
					state.Ball.Velocity.X += p.VelocityX * 0.3
					state.Ball.Velocity.Y += p.VelocityY * 0.3

					state.Ball.HitCount++
				}
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
			if dist == 0 {
				dist = 0.1
			}

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

func resetRound(canvasWidth float64, groundY float64) {
	state.Ball.Pos = Vector2{X: canvasWidth / 2, Y: 200}
	state.Ball.Velocity = Vector2{X: 0, Y: 0}
	state.Ball.HitCount = 0

	for _, p := range state.Players {
		if p.Side == "left" {
			p.Pos = Vector2{X: 300, Y: groundY - p.Height}
		} else {
			p.Pos = Vector2{X: canvasWidth - 300 - p.Width, Y: groundY - p.Height}
		}
		p.VelocityY = 0
		p.VelocityX = 0
		p.Rotation = 0
		p.IsJumping = false
		p.CanDoubleJump = true
		p.DashesLeft = 2
	}
	state.State = "playing"
}

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

	startX := 300.0
	color := "#4caf50"
	side := "left"
	if len(state.Players)%2 != 0 {
		startX = 1600.0 - 300.0 - 120.0
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
				p.Inputs.S = inputs["s"].(bool)
				p.Inputs.DashL = inputs["dashL"].(bool)
				p.Inputs.DashR = inputs["dashR"].(bool)
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
