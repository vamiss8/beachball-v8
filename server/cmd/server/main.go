// Command server runs the beachball game server: it simulates the match,
// serves the built web client and bridges the two over websockets.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"beachball-v8/server/internal/room"

	"github.com/gorilla/websocket"
)

func main() {
	addr := flag.String("addr", ":8080", "host:port to listen on")
	static := flag.String("static", "../client/dist", "directory with the built web client")
	dev := flag.String("dev-origin", "http://localhost:5173", "extra origin allowed to connect, for the vite dev server")
	flag.Parse()

	// one room for now; matchmaking by room code comes next
	lobby := room.New("default")
	defer lobby.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsHandler(lobby, *dev))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.Handle("/", staticHandler(*static))

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("server listening on %s (static: %s)", *addr, *static)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	// wait for ctrl-c, then let in-flight requests finish
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

// wsHandler upgrades a request and hands the connection to the room.
func wsHandler(r *room.Room, devOrigin string) http.HandlerFunc {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     originChecker(devOrigin),
	}

	return func(w http.ResponseWriter, req *http.Request) {
		conn, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			// upgrade failures are per-request problems, never fatal:
			// one bad client must not take the whole server down
			log.Printf("upgrade failed for %s: %v", req.RemoteAddr, err)
			return
		}
		r.Serve(conn)
	}
}

// originChecker only accepts connections from the page we serve ourselves,
// plus the dev server origin. this is what stops a random website from
// opening sockets against this server in a visitor's browser.
func originChecker(devOrigin string) func(*http.Request) bool {
	return func(req *http.Request) bool {
		origin := req.Header.Get("Origin")
		if origin == "" {
			// non-browser client, nothing to forge
			return true
		}
		if devOrigin != "" && origin == devOrigin {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return u.Host == req.Host
	}
}

// staticHandler serves the built client, falling back to index.html so the
// game can be opened by a direct link to any path.
func staticHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
			http.Error(w, "client is not built yet: run `npm run build` in ./client", http.StatusNotFound)
			return
		}
		fs.ServeHTTP(w, r)
	})
}
