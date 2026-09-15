// Command loadtest drives a beachball server with simulated players and
// reports whether it kept up.
//
// every room gets two players who ready up and then play the way a browser
// does: one numbered input per tick and a ping every second. what comes back
// is judged by the only measure a player would notice, which is whether
// snapshots keep arriving sixty times a second, evenly, without anyone being
// dropped.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"time"
)

func main() {
	target := flag.String("url", "ws://localhost:8080/ws", "websocket endpoint of the server under test")
	rooms := flag.Int("rooms", 50, "rooms to open, two players each")
	duration := flag.Duration("duration", 20*time.Second, "how long to measure once every room is up")
	spawnRate := flag.Int("spawn-rate", 100, "rooms opened per second while ramping up")
	warmup := flag.Duration("warmup", 2*time.Second, "settling time between the last room opening and measuring")
	flag.Parse()

	if *rooms < 1 || *spawnRate < 1 {
		fmt.Fprintln(os.Stderr, "loadtest: -rooms and -spawn-rate must be positive")
		os.Exit(2)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// the window is fixed up front: ramp, then warm up, then measure. bots
	// read it without coordinating, and no sample from a half-built fleet
	// ever lands in the result
	ramp := time.Duration(*rooms) * time.Second / time.Duration(*spawnRate)
	start := time.Now()
	win := window{from: start.Add(ramp + *warmup)}
	win.to = win.from.Add(*duration)

	m := &metrics{
		gaps: newHistogram(100*time.Microsecond, 500*time.Millisecond),
		rtts: newHistogram(50*time.Microsecond, 500*time.Millisecond),
	}

	fmt.Printf("opening %d rooms (%d players) over %v, measuring for %v\n",
		*rooms, *rooms*2, ramp.Round(time.Millisecond), *duration)

	var wg sync.WaitGroup
	spawn := time.NewTicker(time.Second / time.Duration(*spawnRate))
	defer spawn.Stop()

spawning:
	for i := 0; i < *rooms; i++ {
		select {
		case <-ctx.Done():
			break spawning
		case <-spawn.C:
		}

		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			openRoom(ctx, *target, i, m, win)
		}(i)
	}

	wg.Wait()
	report(m, *rooms, *duration)
}

// openRoom seats a host, then sends the guest after them into the same code,
// exactly as a shared invite link would.
func openRoom(ctx context.Context, target string, i int, m *metrics, win window) {
	joined := make(chan string, 1)

	host := &bot{url: target, name: fmt.Sprintf("host%d", i), m: m, win: win}
	guest := &bot{url: target, name: fmt.Sprintf("guest%d", i), m: m, win: win}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		host.run(ctx, "", joined)
	}()

	if code, ok := <-joined; ok {
		guest.run(ctx, code, nil)
	} else {
		// no room to follow into, so the guest never got a chance to connect
		m.connectFail.Add(1)
	}
	wg.Wait()
}

func report(m *metrics, rooms int, duration time.Duration) {
	players := int64(rooms * 2)
	connected := m.connected.Load()
	seconds := duration.Seconds()

	wantRate := m.snapshotRate.Load()
	if wantRate == 0 {
		// a server from before snapshots were decoupled sends one per tick
		wantRate = tickRate
	}
	interval := time.Second / time.Duration(wantRate)

	var rate, down float64
	if connected > 0 {
		rate = float64(m.snapshots.Load()) / float64(connected) / seconds
		down = float64(m.bytes.Load()) / float64(connected) / seconds / 1024
	}

	// two decimals: a loopback round trip is tens of microseconds, and one
	// decimal would print it as zero
	ms := func(d time.Duration) string {
		return fmt.Sprintf("%.2fms", float64(d)/float64(time.Millisecond))
	}

	fmt.Println()
	fmt.Printf("connected       %d/%d\n", connected, players)
	fmt.Printf("connect failed  %d\n", m.connectFail.Load())
	fmt.Printf("dropped         %d\n", m.dropped.Load())
	fmt.Printf("snapshot rate   %.2f/s per player (target %d)\n", rate, wantRate)
	fmt.Printf("snapshot gap    p50 %s  p99 %s  p99.9 %s  max %s\n",
		ms(m.gaps.percentile(0.50)), ms(m.gaps.percentile(0.99)),
		ms(m.gaps.percentile(0.999)), ms(time.Duration(m.gaps.max.Load())))
	fmt.Printf("ping rtt        p50 %s  p99 %s  max %s\n",
		ms(m.rtts.percentile(0.50)), ms(m.rtts.percentile(0.99)), ms(time.Duration(m.rtts.max.Load())))
	fmt.Printf("downstream      %.1f KB/s per player\n", down)

	// held means a player could not tell this server from an idle one: the
	// snapshot rate within two percent of target, nobody dropped, nobody
	// turned away at the door, and ninety nine gaps in a hundred no longer
	// than two ticks. a rate that averages out fine can still hide the
	// stutter a player actually sees, which is what the last check is for
	jitterOK := m.gaps.percentile(0.99) <= 2*interval
	held := connected == players && m.dropped.Load() == 0 && rate >= float64(wantRate)*0.98 && jitterOK
	verdict := "HELD"
	if !held {
		verdict = "DID NOT HOLD"
	}
	fmt.Printf("\n%s at %d rooms\n", verdict, rooms)
}
