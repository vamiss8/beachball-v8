# beachball-v8

[![ci](https://github.com/vamiss8/beachball-v8/actions/workflows/ci.yml/badge.svg)](https://github.com/vamiss8/beachball-v8/actions/workflows/ci.yml)

Two-player arcade volleyball in the browser · Аркадный волейбол в браузере на двоих

[English](#english) · [Русский](#russian)

---

# ENGLISH

A pet project: I'm building a clone of [beachball.online](https://beachball.online)
to figure out how realtime multiplayer games are actually put together.

The idea is simple. You open a link, send it to a friend, you play.

## How it works

The server decides everything. The client sends which keys are held down and
draws what comes back, with one deliberate exception: it also predicts its own
player, so the controls answer the keyboard without waiting for a round trip.
Every snapshot overrules that prediction.

```
browser                          server (Go)
  input ──── {"type":"input"} ───────►  room.Client
         60/sec, one per tick             │
                                          ▼
                                       game.World.Step()   60 times/sec
                                          │
  render ◄─── {"type":"state"} ──────────┘
         30/sec, every second tick
```

I started with physics on the client, using p2-es, and dropped it. Two browsers
drift apart from each other within seconds, and you can fix your own score
straight from the console. So I rewrote it in Go. The volleyball here is
arcade-y: the entire physics is gravity plus circle-versus-rectangle collision,
and a full engine is overkill for that. There's no decent p2 port for Go anyway.

The price is input lag, paid in two places. Other players are drawn a fraction
of a second in the past, interpolated between snapshots, so they move smoothly
even when packets arrive unevenly. Your own player is not: see below.

## Server packages

```
server/
├── cmd/server/          entrypoint: http, static files, graceful shutdown
├── cmd/loadtest/        simulated players that measure what a server holds
└── internal/
    ├── game/            the simulation itself, knows nothing about networking
    │   ├── config.go    every physics tunable in one place
    │   ├── types.go     Vec2, Side, Phase, Input
    │   ├── name.go      cleaning up player-supplied names
    │   ├── motion.go    the invisible half of a player's state
    │   ├── player.go    movement, jump, dash, block, spin
    │   ├── ball.go      gravity, integration, walls
    │   └── world.go     simulation step, collisions, scoring, rally phases
    ├── protocol/        every websocket message in one file
    └── room/            the match: simulation loop and snapshot broadcast
        ├── room.go      owns the state, talks to the outside only via channels
        ├── manager.go   the registry of live rooms, keyed by code
        ├── code.go      readable room codes
        └── client.go    one connection: readPump / writePump
```

`game` imports nothing from `room` or `protocol`. That's the whole point of the
split: the physics runs in ordinary unit tests, no sockets involved.

A room's state is touched only by its own goroutine, everything else talks to it
through channels. There are no mutexes on the game state. The manager is the one
exception — the map of rooms is not game state, so a plain mutex around it is
fine.

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

### Configuration

Flags win over environment variables, and the environment is what a container
uses, so an image runs with no arguments at all.

| Flag | Variable | Default | What it does |
| ---- | -------- | ------- | ------------ |
| `-addr` | `PORT` | `:8080` | listen address; `PORT` is a number, since that is how hosting platforms hand one over |
| `-static` | `STATIC_DIR` | `../client/dist` | directory with the built client |
| `-allowed-origins` | `ALLOWED_ORIGINS` | both dev spellings | comma separated origins allowed to open sockets, on top of the host we are served from; defaults to `http://localhost:5173` and `http://127.0.0.1:5173`, and set but empty means none, which is what the docker image uses |
| `-max-rooms` | `MAX_ROOMS` | `500` | most matches one process runs at once; set it to what your machine holds, see Performance |
| `-pprof` | `PPROF_ADDR` | off | serve the go profiler here, e.g. `localhost:6060`; never on the game port |

A socket is accepted when its `Origin` matches the host the page came from, so
`ALLOWED_ORIGINS` is usually only the vite dev server. It exists for proxies
that rewrite the `Host` header: most pass the public one through and the plain
comparison works, but one that does not would make every real player look like
a forgery.

## Docker

```bash
docker build -t beachball .
```

```bash
docker run --rm -p 8080:8080 beachball
```

Then http://localhost:8080. Nothing else needs installing — the image builds
the client with node and the server with go in separate stages, then throws
both toolchains away. What ships is a distroless image of about 15 MB: a static
binary, the built client, no shell and no package manager.

`/healthz` answers 200 for whatever the platform uses as a health check.

## Tests

```bash
cd server
go test ./...
```

## Performance

How many matches does one server hold? I built a load tester to find out
instead of guessing.

`cmd/loadtest` opens rooms two players at a time, the way invite links do, and
each bot behaves like a browser tab: one numbered input per tick and a ping
every second. It counts only what a player would notice. A level **holds**
when every player connects, nobody is dropped, the snapshot rate stays within
2% of the target, and 99 gaps in 100 between snapshots are no longer than two
intervals.

The server gets a fixed budget of 4 cores (`GOMAXPROCS=4`) and the bot runs on
the other 12 of the same machine: a Ryzen 7 5700X, Windows, amd64 builds. A CPU
profile taken mid-run showed the server at 387% of its 400%, so the ceiling
below belongs to the server, not to the bot.

| step | holds | does not hold |
| --- | --- | --- |
| before | 500 rooms | 750 |
| encode snapshots in one pass | 750 | 1000 |
| send a snapshot every second tick | **1000** | 1250 |
| decode input in one pass | 1000 | 1250 |

1000 rooms is 2000 players on 4 cores.

What the profiles showed, in the order they showed it:

1. **The room cap, not the CPU.** The first real run stopped at 500 rooms with
   the server nearly idle, because `MaxRooms` was a hardcoded 500. It is now
   `-max-rooms` / `MAX_ROOMS`.
2. **Every snapshot encoded twice.** `Encode` marshalled the world, then
   marshalled an envelope around those bytes, and `encoding/json` re-validates
   and compacts a `json.RawMessage` instead of copying it. That second pass was
   11.6% of all server CPU. Encoding in one pass took the snapshot benchmark
   from 8.2µs to 2.9µs.
3. **One socket write per player per tick.** By far the largest cost, 45% of
   CPU. No code makes a write cheaper, so snapshots now go out every second
   tick, 30 a second, while the simulation stays at 60. That halves the writes,
   the encoding and every player's download, 60 KB/s down to 30. Clients draw
   other players 100ms in the past and interpolate by tick number, so a two
   tick gap does not show: fed 30 Hz snapshots and sampled at 60 frames a
   second, the drawn ball strays at most 0.22 units from an even path.
4. **Every input decoded twice.** Inputs come every tick from every player,
   and each was decoded as an envelope and then decoded again. A fast path now
   reads the game client's own format in one pass, 2.5µs down to 1.6µs, and
   hands anything else to the general route. It did not move the ceiling: by
   then socket writes were the limit, and that is worth saying rather than
   crediting it with capacity it did not add.

Writes are still about 44% of the server's CPU at the ceiling. These numbers
come from Windows, where every write goes through `WSASend`; a Linux host
spends that time differently, and measuring there is the obvious next step.

### Reproducing it

```bash
cd server
GOMAXPROCS=4 go run ./cmd/server -max-rooms 5000 -pprof localhost:6060
```

In a second terminal:

```bash
cd server
go run ./cmd/loadtest -rooms 1000 -duration 20s
```

A CPU profile while it runs:

```bash
go tool pprof -top "http://localhost:6060/debug/pprof/profile?seconds=10"
```

Benchmarks for the two things a room spends its time on:

```bash
cd server
go test ./internal/game ./internal/protocol -run '^$' -bench . -benchmem
```

## Controls

| Key | Action |
| --- | ------ |
| `A` / `D` | move |
| `W` | jump, press again for a double jump with spin |
| `S` in the air | block: kills the ball and drops you down |
| `A` `A` / `D` `D` | tap twice to dash that way, two per airtime |

A spinning player smashes, a blocking one kills the ball. Catch it square on
your head while running and you carry it along instead of knocking it away.
The ball's gravity grows with every touch, so rallies can't go on forever.

## Rooms

Opening the site without a link puts you in a fresh room and writes its code
into the address bar, so the page you are already looking at *is* the invite.
Send it to someone and they land in your match; a third visitor watches.

Codes are four characters from an alphabet with no `0`/`O` or `1`/`I`/`L`, so
reading one out loud is unambiguous. A code that never existed still opens a
room under that name, which means a shared link survives a server restart.

A room closes itself a minute after the last person leaves, so a reload or a
short wait for your opponent costs nothing.

The bar also shows the round trip to the server, measured every couple of
seconds. Under 60ms a rally plays clean; past 150ms you start swinging at where
the ball used to be. The websocket protocol has ping frames of its own, but the
browser answers those itself and never surfaces them to javascript, so this
rides on the game protocol instead: the client sends a timestamp and the server
echoes it back without ever parsing it.

## The lobby

Nothing is simulated until both seats say go. You type a name, press ready, and
the match starts the moment the other side does the same. Skip the name and the
game calls you by your colour.

The same screen comes back when a match ends, so a rematch is one press each and
the score wipes itself. If your opponent disconnects mid-rally you drop back to
the lobby too — there is nobody to play against, and readiness is cleared so the
next match always starts on purpose. The half-played score goes with it: nobody
can finish that match, so the room you are left waiting in looks like one nobody
has played in yet.

Names are cut to 16 characters and stripped of control and invisible
formatting characters server-side, so a name can neither turn itself around
nor be made of nothing.
It is the only free text one player can put on another's screen, so none of it
is taken on trust.

## Predicting your own player

Waiting for the server before your own player moves is what made the controls
feel soft: every key press cost a full round trip before anything happened on
screen. So the client runs the server's player step itself, for its own player
only, and draws the result immediately.

The server still decides. Each snapshot carries the last input it applied to
you, the client throws away everything already accounted for, and replays only
what is left on top of the authoritative state. A disagreement smaller than a
player's width is absorbed by sliding the drawn position back over a few
frames; anything larger is a real disagreement and is shown as it is.

Two things make that replay line up:

- **One input per tick.** The client used to send only on a change, which was
  cheaper but left the server holding the same keys for however many ticks
  passed in between — a number the client could not know. Now it sends one
  numbered input per simulated tick and the server consumes exactly one per
  tick, so the same keys cover the same ticks on both sides. That also means
  the client steps its prediction on a fixed clock, or a 144hz screen would
  predict more than twice the ticks the server ran.
- **The numbers come from the server.** Speeds, gravity, dash strength and the
  double-tap window arrive with the welcome message instead of being written
  down twice. A client with its own copy would keep predicting with last
  week's jump height the moment one changed.

The client also needs the parts of a player that never appear on screen —
whether a dash is spent, whether the double jump is armed, how long ago a walk
key was tapped — so those travel in the snapshot as well. It is the one real
cost: the same data about your opponent is sent too, since everyone gets the
same snapshot. None of it is secret, it is all visible in how they move.

The javascript step is a hand-written copy of `player.go` and has to stay one.
I checked it by running the same input script through both and comparing every
field on every tick; they agreed on all 83 ticks, dash decay included.

## Rules

Before a serve the ball hangs in the air for a second so both players can get
into position. It lands on your half, the point goes to your opponent, and the
winner serves. First to 15.

## Roadmap

- [x] Server-side simulation, split into packages
- [x] Tests for the physics and the match rules
- [x] Client: connect to the socket, render, interpolate
- [x] Rooms by code and an invite link
- [x] Lobby, nicknames, ready checks
- [x] Client-side prediction of your own player
- [x] Load testing and profiling, capacity doubled
- [ ] Sprites, sound, hit effects
- [ ] 2v2 mode
- [ ] Deployment

---

# RUSSIAN

Пет-проект: пишу клон [beachball.online](https://beachball.online), чтобы
разобраться, как вообще делаются реалтайм-игры на несколько человек.

Идея простая: открыл ссылку, кинул её другу, играете.

## Как устроено

Всё решает сервер. Клиент шлёт, какие клавиши зажаты, и рисует то, что пришло
в ответ, — с одним осознанным исключением: своего игрока он ещё и предсказывает,
чтобы управление отвечало на клавиши, не дожидаясь круга до сервера. Любой
снапшот это предсказание перебивает.

```
браузер                          сервер (Go)
  ввод  ──── {"type":"input"} ───────►  room.Client
         60/сек, по одному на тик         │
                                          ▼
                                       game.World.Step()   60 раз/сек
                                          │
  рендер ◄─── {"type":"state"} ──────────┘
         30/сек, раз в два тика
```

Начинал я с физики на клиенте, на p2-es. Отказался: два браузера расходятся
между собой уже через несколько секунд, а поправить себе счёт можно прямо из
консоли. Переписал на свою физику в Go. Волейбол тут аркадный, вся физика —
это гравитация и столкновение круга с прямоугольником, полноценный движок для
такого избыточен. Плюс нормального порта p2 под Go всё равно нет.

Платим за это задержкой ввода, и платим в двух местах. Чужие игроки рисуются на
доли секунды в прошлом, с интерполяцией между снапшотами, поэтому движутся
плавно даже когда пакеты приходят неровно. Свой игрок — нет, см. ниже.

## Пакеты сервера

```
server/
├── cmd/server/          точка входа: http, статика, graceful shutdown
├── cmd/loadtest/        симулированные игроки, которые меряют ёмкость сервера
└── internal/
    ├── game/            сама симуляция, про сеть не знает
    │   ├── config.go    все настройки физики в одном месте
    │   ├── types.go     Vec2, Side, Phase, Input
    │   ├── name.go      чистка имён, присланных клиентом
    │   ├── player.go    движение, прыжок, даш, блок, вращение
    │   ├── ball.go      гравитация, интегрирование, стены
    │   └── world.go     шаг симуляции, коллизии, счёт, фазы раунда
    ├── protocol/        все сообщения WebSocket в одном файле
    └── room/            матч: цикл симуляции и рассылка снапшотов
        ├── room.go      владеет состоянием, наружу только через каналы
        ├── manager.go   реестр живых комнат по кодам
        ├── code.go      читаемые коды комнат
        └── client.go    одно соединение: readPump / writePump
```

`game` ничего не импортирует из `room` и `protocol`. Ради этого всё и
затевалось: физику можно гонять в обычных юнит-тестах, без сокетов.

Состояние комнаты трогает только её горутина, снаружи с ней общаются через
каналы. Мьютексов на игровом состоянии нет. Единственное исключение — менеджер:
карта комнат не игровое состояние, так что обычный мьютекс вокруг неё вполне
уместен.

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

### Настройки

Флаги перебивают переменные окружения, а окружением пользуется контейнер —
поэтому образ запускается вообще без аргументов.

| Флаг | Переменная | По умолчанию | Зачем |
| ---- | ---------- | ------------ | ----- |
| `-addr` | `PORT` | `:8080` | адрес прослушивания; в `PORT` лежит число, потому что именно так его выдают хостинги |
| `-static` | `STATIC_DIR` | `../client/dist` | папка со сборкой клиента |
| `-allowed-origins` | `ALLOWED_ORIGINS` | обе записи dev-адреса | origin'ы через запятую, которым разрешено открывать сокет, вдобавок к хосту, с которого отдана страница; по умолчанию `http://localhost:5173` и `http://127.0.0.1:5173`, а заданная пустой — никаких, так и сделано в docker-образе |
| `-max-rooms` | `MAX_ROOMS` | `500` | сколько матчей один процесс держит одновременно; ставь столько, сколько тянет машина, см. «Производительность» |
| `-pprof` | `PPROF_ADDR` | выкл. | отдавать профилировщик go на этом адресе, например `localhost:6060`; никогда не на игровом порту |

Сокет принимается, если его `Origin` совпадает с хостом страницы, так что
`ALLOWED_ORIGINS` обычно — это только dev-сервер vite. Переменная нужна для
прокси, переписывающих `Host`: большинство пробрасывает публичный, и обычного
сравнения хватает, но тот, который его меняет, превратил бы каждого живого
игрока в подделку.

## Docker

```bash
docker build -t beachball .
```

```bash
docker run --rm -p 8080:8080 beachball
```

Дальше http://localhost:8080. Ставить больше ничего не нужно: образ собирает
клиент через node и сервер через go отдельными стадиями, а потом выбрасывает
оба тулчейна. Наружу уезжает distroless-образ примерно на 15 МБ — статический
бинарник, собранный клиент, без шелла и пакетного менеджера.

`/healthz` отвечает 200 — на случай, если платформе нужен health check.

## Тесты

```bash
cd server
go test ./...
```

## Производительность

Сколько матчей держит один сервер? Вместо того чтобы гадать, я написал
нагрузочный тест.

`cmd/loadtest` открывает комнаты по два игрока, как это делают ссылки-
приглашения, и каждый бот ведёт себя как вкладка браузера: по одному
пронумерованному вводу на тик и пинг раз в секунду. Считается только то, что
заметил бы игрок. Уровень **держится**, если подключились все, никого не
выкинуло, частота снапшотов не ниже цели больше чем на 2%, и 99 промежутков из
100 между снапшотами не длиннее двух интервалов.

Серверу выдан фиксированный бюджет в 4 ядра (`GOMAXPROCS=4`), а бот работает на
оставшихся 12 той же машины: Ryzen 7 5700X, Windows, сборки под amd64. Профиль
процессора, снятый посреди прогона, показал сервер на 387% из его 400%, так что
потолок ниже — это потолок сервера, а не бота.

| шаг | держит | не держит |
| --- | --- | --- |
| исходно | 500 комнат | 750 |
| кодирование снапшота в один проход | 750 | 1000 |
| снапшот раз в два тика | **1000** | 1250 |
| разбор ввода в один проход | 1000 | 1250 |

1000 комнат — это 2000 игроков на 4 ядрах.

Что показали профили, в том порядке, в каком показали:

1. **Лимит комнат, а не процессор.** Первый настоящий прогон упёрся в 500
   комнат при почти простаивающем сервере: `MaxRooms` был захардкожен на 500.
   Теперь это `-max-rooms` / `MAX_ROOMS`.
2. **Каждый снапшот кодировался дважды.** `Encode` маршалил мир, а потом
   маршалил вокруг этих байтов конверт, и `encoding/json` не копирует
   `json.RawMessage`, а заново валидирует и сжимает его. Этот второй проход
   занимал 11.6% всего процессора сервера. Кодирование в один проход ускорило
   бенчмарк снапшота с 8.2 мкс до 2.9 мкс.
3. **Одна запись в сокет на игрока на тик.** Самая крупная статья, 45%
   процессора. Сделать запись дешевле кодом нельзя, поэтому снапшоты теперь
   уходят раз в два тика, 30 в секунду, а симуляция осталась на 60. Это вдвое
   режет записи, кодирование и загрузку у каждого игрока — с 60 КБ/с до 30.
   Чужих игроков клиент рисует на 100 мс в прошлом с интерполяцией по номеру
   тика, так что промежуток в два тика не виден: при снапшотах на 30 Гц и
   отрисовке на 60 кадрах мяч отклоняется от ровной траектории максимум на
   0.22 единицы.
4. **Каждый ввод разбирался дважды.** Ввод приходит на каждом тике от каждого
   игрока, и каждый разбирался как конверт, а потом ещё раз. Быстрый путь
   теперь читает формат нашего клиента в один проход, 1.6 мкс вместо 2.5, а
   всё остальное отдаёт общему пути. Потолок он не сдвинул: к тому моменту
   упирались уже в запись в сокеты, и это стоит сказать прямо, а не приписывать
   ему ёмкость, которой он не добавил.

Запись в сокеты по-прежнему около 44% процессора на потолке. Цифры сняты на
Windows, где каждая запись идёт через `WSASend`; на Linux это время
распределяется иначе, и замер там — очевидный следующий шаг.

### Как повторить

```bash
cd server
GOMAXPROCS=4 go run ./cmd/server -max-rooms 5000 -pprof localhost:6060
```

Во втором терминале:

```bash
cd server
go run ./cmd/loadtest -rooms 1000 -duration 20s
```

Профиль процессора во время прогона:

```bash
go tool pprof -top "http://localhost:6060/debug/pprof/profile?seconds=10"
```

Бенчмарки двух вещей, на которые комната тратит время:

```bash
cd server
go test ./internal/game ./internal/protocol -run '^$' -bench . -benchmem
```

## Управление

| Клавиша | Действие |
| ------- | -------- |
| `A` / `D` | движение |
| `W` | прыжок, второй раз — двойной, с вращением |
| `S` в воздухе | блок: гасит мяч и роняет тебя вниз |
| `A` `A` / `D` `D` | двойное нажатие — рывок в ту же сторону, два за полёт |

Крутящийся игрок бьёт смэш, блокирующий — гасит. Поймал мяч ровно на голову на
бегу — ведёшь его с собой, а не выбиваешь вперёд. Гравитация мяча растёт с
каждым касанием, так что бесконечных розыгрышей не бывает.

## Комнаты

Открыл сайт без ссылки — попал в свежую комнату, а её код тут же прописался в
адресной строке. То есть страница, на которую ты смотришь, и есть приглашение:
кинул ссылку — человек оказался в твоём матче. Третий зашедший смотрит со
стороны.

Код — четыре символа из алфавита без `0`/`O` и `1`/`I`/`L`, чтобы его можно было
продиктовать вслух и не переспрашивать. Код, которого никогда не существовало,
всё равно открывает комнату с этим именем — так ссылка переживает перезапуск
сервера.

Там же показывается пинг до сервера — замеряется каждые пару секунд. До 60 мс
розыгрыш идёт чисто, после 150 начинаешь бить туда, где мяч был. У протокола
websocket есть свои ping-фреймы, но браузер отвечает на них сам и в javascript
их не показывает, поэтому замер едет поверх игрового протокола: клиент шлёт
метку времени, а сервер возвращает её обратно, ни разу не заглянув внутрь.

Комната закрывается через минуту после ухода последнего игрока, так что
перезагрузка страницы или ожидание соперника ничего не стоят.

## Лобби

Пока оба места не скажут «готов», не считается ничего. Вводишь имя, жмёшь
готовность — и матч стартует в тот момент, когда то же самое сделает соперник.
Не ввёл имя — игра зовёт тебя по цвету.

Тот же экран возвращается после матча: реванш — это одно нажатие с каждой
стороны, счёт обнуляется сам. Если соперник отвалился посреди розыгрыша, ты тоже
попадаешь в лобби — играть не с кем, — и готовность сбрасывается, чтобы
следующий матч всегда начинался осознанно. Недоигранный счёт уходит вместе с
ней: доиграть тот матч всё равно некому, так что комната, в которой ты остался
ждать, выглядит как та, где ещё никто не играл.

Имя режется до 16 символов и чистится на сервере от управляющих и невидимых
символов форматирования, так что имя не может ни развернуться задом наперёд,
ни состоять из пустоты. Это
единственный свободный текст, который один игрок может показать другому, так что
на слово ему не верят.

## Предсказание своего игрока

Именно ожидание сервера делало управление ватным: каждое нажатие стоило
полного круга до сервера, прежде чем на экране что-то происходило. Поэтому
клиент сам прогоняет серверный шаг игрока — только для своего — и сразу рисует
результат.

Решает по-прежнему сервер. В каждом снапшоте приезжает номер последнего
применённого к тебе ввода, клиент выбрасывает всё уже учтённое и переигрывает
поверх авторитетного состояния только остаток. Расхождение меньше ширины игрока
всасывается: нарисованная позиция подъезжает к правильной за несколько кадров.
Всё, что больше, — настоящее расхождение, и оно показывается как есть.

Чтобы переигрывание сходилось, нужны две вещи:

- **Один ввод на тик.** Раньше клиент слал только при изменении — дешевле, но
  сервер держал те же клавиши неизвестное клиенту число тиков. Теперь на каждый
  симулированный тик уходит один пронумерованный ввод, а сервер съедает ровно
  один за тик, так что одни и те же клавиши покрывают одинаковое число тиков с
  обеих сторон. Отсюда же фиксированный шаг на клиенте: иначе экран на 144 Гц
  предсказал бы вдвое больше тиков, чем сервер отсимулировал.
- **Числа приезжают с сервера.** Скорости, гравитация, сила рывка и окно
  двойного нажатия приходят в welcome, а не записаны в двух местах. Клиент со
  своей копией продолжил бы предсказывать прошлогодней высотой прыжка, стоило
  бы её поменять.

Клиенту нужна ещё и та часть состояния игрока, которой не видно на экране:
потрачен ли рывок, взведён ли двойной прыжок, давно ли нажимали клавишу ходьбы.
Она тоже едет в снапшоте. Это единственная реальная цена: те же данные о
сопернике уходят всем, потому что снапшот общий. Секретного там ничего нет —
всё это и так видно по тому, как он двигается.

Шаг на javascript — рукописная копия `player.go`, и обязан ею оставаться. Я
проверил его, прогнав один и тот же сценарий ввода через обе реализации и
сравнив каждое поле на каждом тике: сошлись все 83 тика, включая затухание
рывка.

## Правила

Перед подачей мяч висит в воздухе секунду, чтобы оба успели встать. Упал на
половину — очко сопернику, подаёт выигравший. Играем до 15.

## Что дальше

- [x] Симуляция на сервере, разбитая на пакеты
- [x] Тесты физики и правил
- [x] Клиент: подключение к сокету, отрисовка, интерполяция
- [x] Комнаты по коду и ссылка-приглашение
- [x] Лобби, никнеймы, готовность
- [x] Предсказание своего игрока на клиенте
- [x] Нагрузочный тест и профилирование, ёмкость удвоена
- [ ] Спрайты, звук, эффекты ударов
- [ ] Режим 2 на 2
- [ ] Деплой

---

## License

MIT
