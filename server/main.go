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

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

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

// Обновленный стейт игрока (убрали IsSomersaulting, добавили RotationVel)
type PlayerState struct {
	Id            string  `json:"id"`
	Pos           Vector2 `json:"pos"`
	VelocityX     float64 `json:"velocityX"`
	VelocityY     float64 `json:"velocityY"`
	DashVelocity  float64 `json:"-"`
	Rotation      float64 `json:"rotation"`
	RotationVel   float64 `json:"-"` // Скорость текущего переката/сальто
	IsJumping     bool    `json:"isJumping"`
	CanDoubleJump bool    `json:"-"`
	IsBlocking    bool    `json:"isBlocking"`
	DashesLeft    int     `json:"-"`
	DashCooldown  int     `json:"-"`
	PrevW         bool    `json:"-"`
	Width         float64 `json:"width"`
	Height        float64 `json:"height"`
	Color         string  `json:"color"`
	Side          string  `json:"side"`
	Inputs        Inputs  `json:"-"`
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

		// 1. Обработка игроков
		for _, p := range state.Players {
			playerGroundY := groundY - p.Height

			if p.DashCooldown > 0 {
				p.DashCooldown--
			}

			// Логика дэшей (назначаем скорость движения и скорость вращения)
			if p.Inputs.DashL && p.DashesLeft > 0 && p.DashCooldown <= 0 {
				p.DashVelocity = -35.0
				p.DashesLeft--
				p.DashCooldown = 30
				p.RotationVel = -0.35 // Кувырок влево (против часовой)
			}
			if p.Inputs.DashR && p.DashesLeft > 0 && p.DashCooldown <= 0 {
				p.DashVelocity = 35.0
				p.DashesLeft--
				p.DashCooldown = 30
				p.RotationVel = 0.35 // Кувырок вправо (по часовой)
			}

			baseSpeed := 0.0
			if p.Inputs.A { baseSpeed = -10.0 }
			if p.Inputs.D { baseSpeed = 10.0 }

			p.VelocityX = baseSpeed + p.DashVelocity
			p.Pos.X += p.VelocityX
			p.DashVelocity *= 0.85

			// Границы карты
			if p.Pos.X < 0 { p.Pos.X = 0 }
			if p.Pos.X > canvasWidth-p.Width { p.Pos.X = canvasWidth - p.Width }
			if p.Side == "left" {
				if p.Pos.X > canvasWidth/2-p.Width-10 { p.Pos.X = canvasWidth/2 - p.Width - 10 }
			} else {
				if p.Pos.X < canvasWidth/2+10 { p.Pos.X = canvasWidth/2 + 10 }
			}

			// Логика прыжков и двойных прыжков (с сальто вперед)
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
					// Сальто вперед в зависимости от стороны
					if p.Side == "left" {
						p.RotationVel = 0.25 // Лицом направо -> крутим по часовой
					} else {
						p.RotationVel = -0.25 // Лицом налево -> крутим против часовой
					}
				}
			}

			// Блок в воздухе
			p.IsBlocking = p.Inputs.S && p.IsJumping
			if p.IsBlocking {
				p.VelocityY += 1.8 
			} else {
				p.VelocityY += 0.6 
			}
			
			p.Pos.Y += p.VelocityY

			// Приземление
			if p.Pos.Y >= playerGroundY {
				if p.IsJumping {
					// Если мы ТОЛЬКО ЧТО приземлились, сбрасываем сальто на ноги
					p.Rotation = 0
					p.RotationVel = 0
				}
				p.Pos.Y = playerGroundY
				p.VelocityY = 0
				p.IsJumping = false
				p.IsBlocking = false
				p.CanDoubleJump = true
				p.DashesLeft = 2 // На земле дэши бесконечные (сразу восстанавливаются)
			}

			// АНИМАЦИЯ: Точное вычисление углов
			if p.IsBlocking {
				// Блок наклоняет на 45 градусов и отменяет любое сальто
				if p.Side == "left" {
					p.Rotation = math.Pi / 4
				} else {
					p.Rotation = -math.Pi / 4
				}
				p.RotationVel = 0
			} else if p.RotationVel != 0 {
				// Крутимся
				p.Rotation += p.RotationVel
				// Проверяем, сделали ли мы полный оборот (360 градусов = 2*Pi)
				if math.Abs(p.Rotation) >= 2*math.Pi {
					p.Rotation = 0
					p.RotationVel = 0 // Останавливаем вращение точно на ногах
				}
			} else {
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

		// 2. Физика мяча
		dynamicGravity := 0.2 + (float64(state.Ball.HitCount) * 0.02)
		if dynamicGravity > 1.2 { dynamicGravity = 1.2 }

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

		// 3. Столкновение мяча и игроков
		for _, p := range state.Players {
			closestX := math.Max(p.Pos.X, math.Min(state.Ball.Pos.X, p.Pos.X+p.Width))
			closestY := math.Max(p.Pos.Y, math.Min(state.Ball.Pos.Y, p.Pos.Y+p.Height))

			distX := state.Ball.Pos.X - closestX
			distY := state.Ball.Pos.Y - closestY
			distSquared := (distX * distX) + (distY * distY)

			if distSquared < (state.Ball.Radius * state.Ball.Radius) {
				dist := math.Sqrt(distSquared)
				if dist == 0 { dist = 0.1 }

				nx := distX / dist
				ny := distY / dist

				penetration := state.Ball.Radius - dist
				state.Ball.Pos.X += nx * penetration
				state.Ball.Pos.Y += ny * penetration

				relVelX := state.Ball.Velocity.X - p.VelocityX
				relVelY := state.Ball.Velocity.Y - p.VelocityY

				dot := relVelX*nx + relVelY*ny

				if dot < 0 {
					restitution := 0.8 
					
					if p.IsBlocking {
						restitution = 0.2 // Мягкий блок гасит скорость
						if p.Side == "left" { nx = 0.8; ny = 0.5 } else { nx = -0.8; ny = 0.5 }
					} else if p.RotationVel != 0 {
						restitution = 1.6 // Если в момент удара мы крутимся - это мощный смэш!
					}

					impulse := -(1 + restitution) * dot
					state.Ball.Velocity.X += impulse * nx
					state.Ball.Velocity.Y += impulse * ny

					state.Ball.Velocity.X += p.VelocityX * 0.3
					state.Ball.Velocity.Y += p.VelocityY * 0.3
					
					state.Ball.HitCount++
				}
			}
		}

		// 4. Мяч и сетка
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
		p.DashVelocity = 0
		p.Rotation = 0
		p.RotationVel = 0
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