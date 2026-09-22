package main

import (
	"log/slog"
	"net/http"
	"net/http/pprof"
	"time"
)

// servePprof exposes the go profiler on its own listener.
//
// it is never mounted on the public mux: a cpu profile blocks for as long as
// the caller asks and dumps the binary's internals, so on a real deployment it
// has to live on an address nobody outside can reach, like localhost. an empty
// address leaves it off, which is the default.
//
// the handlers are registered by hand on a private mux. importing the package
// also adds them to http.DefaultServeMux as a side effect, but that mux is
// never served here, so they stay unreachable from the game port.
func servePprof(addr string) {
	if addr == "" {
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("pprof listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// a profiler that cannot bind is not a reason to take the game down
			slog.Error("pprof stopped", "err", err)
		}
	}()
}
