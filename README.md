# beachball-v8

Two-player arcade volleyball in the browser · Аркадный волейбол в браузере на двоих

[English](#english) · [Русский](#russian)

---

# ENGLISH

A pet project: I'm building a clone of [beachball.online](https://beachball.online)
to figure out how realtime multiplayer games are actually put together.

The idea is simple. You open a link, send it to a friend, you play.

## How it works

All the physics runs on the server. The client doesn't simulate anything — it
sends which keys are held down and draws whatever comes back.

```
browser                          server (Go)
  input ──── {"type":"input"} ───────►  room.Client
                                          │
                                          ▼
                                       game.World.Step()   60 times/sec
                                          │
  render ◄─── {"type":"state"} ──────────┘
```

I started with physics on the client, using p2-es, and dropped it. Two browsers
drift apart from each other within seconds, and you can fix your own score
straight from the console. So I rewrote it in Go. The volleyball here is
arcade-y: the entire physics is gravity plus circle-versus-rectangle collision,
and a full engine is overkill for that. There's no decent p2 port for Go anyway.

The price is input lag. I plan to hide it with interpolation on the client.

## Server packages

```
server/
├── cmd/server/          entrypoint: http, static files, graceful shutdown
└── internal/
    ├── game/            the simulation itself, knows nothing about networking
    │   ├── config.go    every physics tunable in one place
    │   ├── types.go     Vec2, Side, Phase, Input
    │   ├── player.go    movement, jump, dash, block, spin
    │   ├── ball.go      gravity, integration, walls
    │   └── world.go     simulation step, collisions, scoring, rally phases
    ├── protocol/        every websocket message in one file
    └── room/            the match: simulation loop and snapshot broadcast
        ├── room.go      owns the state, talks to the outside only via channels
        └── client.go    one connection: readPump / writePump
```

`game` imports nothing from `room` or `protocol`. That's the whole point of the
split: the physics runs in ordinary unit tests, no sockets involved.

A room's state is touched only by its own goroutine, everything else talks to it
through channels. There are no mutexes on the game state.

## Running it

You'll need Go 1.26+ and Node 20+.

Server:

```bash
cd server
go run ./cmd/server
```

Client in dev mode, in a second terminal:

```bash
cd client
npm install
npm run dev
```

Then http://localhost:5173.

### From a single link

The server can serve the built client itself, no second port needed. This is how
it'll work in production:

```bash
cd client
npm run build
```

```bash
cd server
go run ./cmd/server
```

Open http://localhost:8080 and send the same link to your opponent.

### Flags

| Flag | Default | What it does |
| ---- | ------- | ------------ |
| `-addr` | `:8080` | listen address |
| `-static` | `../client/dist` | directory with the built client |
| `-dev-origin` | `http://localhost:5173` | allow connections from vite |

## Tests

```bash
cd server
go test ./...
```

## Controls

| Key | Action |
| --- | ------ |
| `A` / `D` | move |
| `W` | jump, press again for a double jump with spin |
| `S` in the air | block: kills the ball and drops you down |
| `Q` / `E` | dash left/right, two per airtime |

A spinning player smashes, a blocking one kills the ball. The ball's gravity
grows with every touch, so rallies can't go on forever.

## Rules

Before a serve the ball hangs in the air for a second so both players can get
into position. It lands on your half, the point goes to your opponent, and the
winner serves. First to 15.

## Roadmap

- [x] Server-side simulation, split into packages
- [x] Tests for the physics and the match rules
- [ ] Client: connect to the socket, render, interpolate
- [ ] Rooms by code and an invite link
- [ ] Lobby, nicknames, ready checks
- [ ] Sprites, sound, hit effects
- [ ] 2v2 mode
- [ ] Deployment

---

# RUSSIAN

Пет-проект: пишу клон [beachball.online](https://beachball.online), чтобы
разобраться, как вообще делаются реалтайм-игры на несколько человек.

Идея простая: открыл ссылку, кинул её другу, играете.

## Как устроено

Вся физика считается на сервере. Клиент не симулирует ничего: шлёт, какие
клавиши зажаты, и рисует то, что пришло в ответ.

```
браузер                          сервер (Go)
  ввод  ──── {"type":"input"} ───────►  room.Client
                                          │
                                          ▼
                                       game.World.Step()   60 раз/сек
                                          │
  рендер ◄─── {"type":"state"} ──────────┘
```

Начинал я с физики на клиенте, на p2-es. Отказался: два браузера расходятся
между собой уже через несколько секунд, а поправить себе счёт можно прямо из
консоли. Переписал на свою физику в Go. Волейбол тут аркадный, вся физика —
это гравитация и столкновение круга с прямоугольником, полноценный движок для
такого избыточен. Плюс нормального порта p2 под Go всё равно нет.

Платим за это задержкой ввода. Собираюсь гасить её интерполяцией на клиенте.

## Пакеты сервера

```
server/
├── cmd/server/          точка входа: http, статика, graceful shutdown
└── internal/
    ├── game/            сама симуляция, про сеть не знает
    │   ├── config.go    все настройки физики в одном месте
    │   ├── types.go     Vec2, Side, Phase, Input
    │   ├── player.go    движение, прыжок, даш, блок, вращение
    │   ├── ball.go      гравитация, интегрирование, стены
    │   └── world.go     шаг симуляции, коллизии, счёт, фазы раунда
    ├── protocol/        все сообщения WebSocket в одном файле
    └── room/            матч: цикл симуляции и рассылка снапшотов
        ├── room.go      владеет состоянием, наружу только через каналы
        └── client.go    одно соединение: readPump / writePump
```

`game` ничего не импортирует из `room` и `protocol`. Ради этого всё и
затевалось: физику можно гонять в обычных юнит-тестах, без сокетов.

Состояние комнаты трогает только её горутина, снаружи с ней общаются через
каналы. Мьютексов на игровом состоянии нет.

## Запуск

Нужны Go 1.26+ и Node 20+.

Сервер:

```bash
cd server
go run ./cmd/server
```

Клиент в дев-режиме, отдельным терминалом:

```bash
cd client
npm install
npm run dev
```

Дальше http://localhost:5173.

### Одной ссылкой

Сервер умеет сам раздавать собранный клиент, второй порт не нужен. Так это и
будет работать в проде:

```bash
cd client
npm run build
```

```bash
cd server
go run ./cmd/server
```

Открываем http://localhost:8080, туда же зовём второго.

### Флаги

| Флаг | По умолчанию | Зачем |
| ---- | ------------ | ----- |
| `-addr` | `:8080` | адрес прослушивания |
| `-static` | `../client/dist` | папка со сборкой клиента |
| `-dev-origin` | `http://localhost:5173` | разрешить подключения с vite |

## Тесты

```bash
cd server
go test ./...
```

## Управление

| Клавиша | Действие |
| ------- | -------- |
| `A` / `D` | движение |
| `W` | прыжок, второй раз — двойной, с вращением |
| `S` в воздухе | блок: гасит мяч и роняет тебя вниз |
| `Q` / `E` | рывок влево/вправо, два за один полёт |

Крутящийся игрок бьёт смэш, блокирующий — гасит. Гравитация мяча растёт с
каждым касанием, так что бесконечных розыгрышей не бывает.

## Правила

Перед подачей мяч висит в воздухе секунду, чтобы оба успели встать. Упал на
половину — очко сопернику, подаёт выигравший. Играем до 15.

## Что дальше

- [x] Симуляция на сервере, разбитая на пакеты
- [x] Тесты физики и правил
- [ ] Клиент: подключение к сокету, отрисовка, интерполяция
- [ ] Комнаты по коду и ссылка-приглашение
- [ ] Лобби, никнеймы, готовность
- [ ] Спрайты, звук, эффекты ударов
- [ ] Режим 2 на 2
- [ ] Деплой

---

## License

MIT
