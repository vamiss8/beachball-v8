package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"beachball-v8/server/internal/protocol"
	"beachball-v8/server/internal/room"

	"github.com/gorilla/websocket"
)

// testServer runs the real /ws handler behind a real listener, wired the way
// main wires it minus the static files, and returns the base http url.
func testServer(t *testing.T, maxRooms int) string {
	t.Helper()

	rooms := room.NewManager(maxRooms)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsHandler(rooms, parseOrigins("")))

	srv := httptest.NewServer(mux)
	t.Cleanup(func() {
		rooms.CloseAll()
		srv.Close()
	})
	return srv.URL
}

// dial opens a game socket the way the browser does, optionally into a room
// and from a given page. the http response is returned too, since that is
// where a refused handshake says why.
func dial(t *testing.T, base, code, origin string) (*websocket.Conn, *http.Response, error) {
	t.Helper()

	url := "ws" + strings.TrimPrefix(base, "http") + "/ws"
	if code != "" {
		url += "?room=" + code
	}
	header := http.Header{}
	if origin != "" {
		header.Set("Origin", origin)
	}

	conn, resp, err := websocket.DefaultDialer.Dial(url, header)
	if conn != nil {
		t.Cleanup(func() { conn.Close() })
	}
	return conn, resp, err
}

// join dials and reads the welcome, failing the test if either goes wrong.
func join(t *testing.T, base, code string) protocol.Welcome {
	t.Helper()

	conn, resp, err := dial(t, base, code, "")
	if err != nil {
		if resp != nil {
			t.Fatalf("join %q: refused with %s", code, resp.Status)
		}
		t.Fatalf("join %q: %v", code, err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var msg struct {
		Type string           `json:"type"`
		Data protocol.Welcome `json:"data"`
	}
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("join %q: reading welcome: %v", code, err)
	}
	if msg.Type != protocol.TypeWelcome {
		t.Fatalf("join %q: first message is %q, want %q", code, msg.Type, protocol.TypeWelcome)
	}
	return msg.Data
}

func TestHandshakesTheServerRefuses(t *testing.T) {
	base := testServer(t, room.DefaultMaxRooms)

	cases := []struct {
		name   string
		code   string
		origin string
		want   int
	}{
		{name: "a code no server could have issued", code: "ABC0", want: http.StatusBadRequest},
		{name: "a page on some other site", origin: "https://elsewhere.example", want: http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, resp, err := dial(t, base, tc.code, tc.origin)
			if err == nil {
				t.Fatal("handshake went through, want it refused")
			}
			if resp == nil {
				t.Fatalf("no http response to read the refusal from: %v", err)
			}
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

func TestRefusedRequestsDoNotUseUpRooms(t *testing.T) {
	// a cap of one: if anything refused below still opened a room on its way
	// out, the real player at the end would find the server full
	base := testServer(t, 1)

	// a plain page load of /ws, which is what a crawler or a stray
	// script sends, and a websocket from a page on someone else's site
	resp, err := http.Get(base + "/ws")
	if err != nil {
		t.Fatalf("plain get: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("plain get: status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if _, _, err := dial(t, base, "", "https://elsewhere.example"); err == nil {
		t.Fatal("foreign origin: handshake went through, want it refused")
	}

	if w := join(t, base, ""); w.RoomID == "" {
		t.Fatal("welcome named no room")
	}
}
