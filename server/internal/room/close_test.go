package room

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
)

// serverConn opens one real websocket and returns the server's end of it, for
// tests that need a working *websocket.Conn behind a Client without bringing
// up a room or a manager.
func serverConn(t *testing.T) *websocket.Conn {
	t.Helper()

	accepted := make(chan *websocket.Conn, 1)
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if conn, err := upgrader.Upgrade(w, r, nil); err == nil {
			accepted <- conn
		}
	}))
	t.Cleanup(srv.Close)

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	return <-accepted
}

func TestCloseIsSafeFromEveryGoroutineAtOnce(t *testing.T) {
	// one connection is enough: closing it again only returns an error, and
	// what is under test is the channel, which every round makes fresh
	conn := serverConn(t)

	// the three callers in real life: the read pump and the write pump on
	// their way out, and the room dropping a client that fell behind. they
	// are released together so they reach close at the same moment, and the
	// window between checking and closing is so narrow that it takes a few
	// thousand rounds to hit it reliably
	const callers = 3
	for i := 0; i < 5000; i++ {
		c := &Client{conn: conn, closed: make(chan struct{})}

		start := make(chan struct{})
		var wg sync.WaitGroup
		for g := 0; g < callers; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				c.close()
			}()
		}
		close(start)
		wg.Wait()
	}
}
