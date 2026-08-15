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
	"strings"
	"syscall"
	"time"

	"beachball-v8/server/internal/room"

	"github.com/gorilla/websocket"
)

// devOrigin is the vite dev server, allowed by default so `npm run dev` works
// against a locally running server without any extra configuration.
const devOrigin = "http://localhost:5173"

func main() {
	// defaults come from the environment so a container needs no arguments,
	// and flags still win when one is passed
	addr := flag.String("addr", defaultAddr(), "host:port to listen on ($PORT)")
	static := flag.String("static", envOr("STATIC_DIR", "../client/dist"), "directory with the built web client ($STATIC_DIR)")
	origins := flag.String("allowed-origins", envOr("ALLOWED_ORIGINS", devOrigin), "comma separated origins allowed to open sockets, on top of the host we are served from ($ALLOWED_ORIGINS)")
	flag.Parse()

	rooms := room.NewManager()
	defer rooms.CloseAll()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsHandler(rooms, parseOrigins(*origins)))
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

	// said once at startup rather than left to be discovered by whoever opens
	// the page first and gets a 404 with no idea the path is simply wrong
	if _, err := os.Stat(*static); errors.Is(err, os.ErrNotExist) {
		log.Printf("warning: static dir %q does not exist, the client will 404 until it is built", *static)
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

// wsHandler upgrades a request and hands the connection to the room named in
// the query string. no code means "open me a fresh room".
func wsHandler(rooms *room.Manager, allowed map[string]bool) http.HandlerFunc {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     originChecker(allowed),
	}

	return func(w http.ResponseWriter, req *http.Request) {
		// resolved before the upgrade, so a bad code gets a plain http error
		// the browser can actually show instead of an instant socket close
		rm, err := rooms.Join(req.URL.Query().Get("room"))
		if err != nil {
			status := http.StatusServiceUnavailable
			if errors.Is(err, room.ErrBadCode) {
				status = http.StatusBadRequest
			}
			log.Printf("join failed for %s: %v", req.RemoteAddr, err)
			http.Error(w, err.Error(), status)
			return
		}

		conn, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			// upgrade failures are per-request problems, never fatal:
			// one bad client must not take the whole server down
			log.Printf("upgrade failed for %s: %v", req.RemoteAddr, err)
			return
		}
		rm.Serve(conn)
	}
}

// originChecker only accepts connections from the page we serve ourselves,
// plus any origin listed explicitly. this is what stops a random website from
// opening sockets against this server in a visitor's browser.
//
// the explicit list exists for deployments behind a reverse proxy: most
// proxies pass the public Host through and the comparison below just works,
// but one that rewrites it would leave every real player looking like a
// forgery, and the list is the way out of that.
func originChecker(allowed map[string]bool) func(*http.Request) bool {
	return func(req *http.Request) bool {
		origin := req.Header.Get("Origin")
		if origin == "" {
			// non-browser client, nothing to forge
			return true
		}
		if allowed[origin] {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return u.Host == req.Host
	}
}

// parseOrigins turns a comma separated list into a set. blanks are skipped, so
// a trailing comma or an empty variable is harmless rather than a rule that
// matches an empty origin.
func parseOrigins(raw string) map[string]bool {
	allowed := make(map[string]bool)
	for _, origin := range strings.Split(raw, ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			allowed[origin] = true
		}
	}
	return allowed
}

// defaultAddr reads the port a platform assigned us. paas hosts hand it over
// in $PORT and expect the app to listen there, not on a port of its choosing.
func defaultAddr() string {
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	return ":8080"
}

// envOr reads a setting from the environment, falling back to def.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// staticHandler serves the built client. no single-page fallback is needed:
// a room lives in the query string, so every link people share is still the
// site root and the file server can answer it directly.
func staticHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
			http.Error(w, "client is not built yet: run `npm run build` in ./client", http.StatusNotFound)
			return
		}
		// go sniffs a content type from the bytes when the extension does not
		// give one away, and a browser doing its own sniffing on top can talk
		// itself into running a file as a script. the type we send is the one
		// that counts
		w.Header().Set("X-Content-Type-Options", "nosniff")

		setCacheControl(w, r.URL.Path)
		fs.ServeHTTP(w, r)
	})
}

// setCacheControl decides how long a file may be reused.
//
// vite writes a hash of the contents into every asset filename, so one of
// those urls can never come back with something different and may be kept for
// good. index.html is the opposite: its name never changes and it points at
// those hashes, so a cached copy would go on asking for asset files that the
// next deploy has already removed.
func setCacheControl(w http.ResponseWriter, path string) {
	if strings.HasPrefix(path, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}

	// not "do not store": the browser may keep it, it just has to ask whether
	// it is still current, which a 304 answers without resending the page
	w.Header().Set("Cache-Control", "no-cache")
}
