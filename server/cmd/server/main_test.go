package main

import (
	"net/http/httptest"
	"os"
	"testing"
)

func TestOriginChecker(t *testing.T) {
	allowed := parseOrigins("https://beachball.example.com")

	cases := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		{
			name:   "no origin at all",
			origin: "",
			host:   "beachball.example.com",
			// not a browser, so there is no session to ride on and nothing to forge
			want: true,
		},
		{
			name:   "served from the same host",
			origin: "https://beachball.example.com",
			host:   "beachball.example.com",
			want:   true,
		},
		{
			name:   "listed explicitly, host rewritten by a proxy",
			origin: "https://beachball.example.com",
			host:   "10.0.0.7:8080",
			want:   true,
		},
		{
			name:   "someone else's site",
			origin: "https://evil.example.com",
			host:   "beachball.example.com",
			want:   false,
		},
		{
			name:   "lookalike host",
			origin: "https://beachball.example.com.evil.test",
			host:   "beachball.example.com",
			want:   false,
		},
		{
			name:   "unparseable origin",
			origin: "://nonsense",
			host:   "beachball.example.com",
			want:   false,
		},
		{
			name:   "local dev over plain http",
			origin: "http://localhost:8080",
			host:   "localhost:8080",
			want:   true,
		},
	}

	check := originChecker(allowed)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/ws", nil)
			req.Host = c.host
			if c.origin != "" {
				req.Header.Set("Origin", c.origin)
			}

			if got := check(req); got != c.want {
				t.Errorf("origin %q against host %q = %v, want %v", c.origin, c.host, got, c.want)
			}
		})
	}
}

func TestParseOrigins(t *testing.T) {
	// a trailing comma and stray spaces are what a hand-written env var
	// actually looks like, and neither may turn into a rule matching ""
	allowed := parseOrigins(" https://a.example.com , https://b.example.com ,")

	if len(allowed) != 2 {
		t.Fatalf("parsed %d origins, want 2: %v", len(allowed), allowed)
	}
	if !allowed["https://a.example.com"] || !allowed["https://b.example.com"] {
		t.Fatalf("origins were not trimmed: %v", allowed)
	}
	if allowed[""] {
		t.Fatal("an empty origin became an allow rule")
	}

	if empty := parseOrigins(""); len(empty) != 0 {
		t.Fatalf("empty input parsed to %v, want nothing", empty)
	}
}

func TestDefaultAddrFollowsThePlatformPort(t *testing.T) {
	t.Setenv("PORT", "3000")
	if got := defaultAddr(); got != ":3000" {
		t.Fatalf("defaultAddr() = %q, want %q", got, ":3000")
	}

	t.Setenv("PORT", "")
	if got := defaultAddr(); got != ":8080" {
		t.Fatalf("defaultAddr() without $PORT = %q, want %q", got, ":8080")
	}
}

func TestEmptyAllowedOriginsSwitchesTheDevDefaultsOff(t *testing.T) {
	// what the docker image sets: there is no vite dev server in production,
	// so a page on someone's localhost:5173 has no business opening sockets
	t.Setenv("ALLOWED_ORIGINS", "")
	if got := parseOrigins(lookupEnvOr("ALLOWED_ORIGINS", devOrigin)); len(got) != 0 {
		t.Fatalf("allowed = %v, want nothing beyond our own host", got)
	}
}

func TestUnsetAllowedOriginsKeepsTheDevDefaults(t *testing.T) {
	// t.Setenv first so the variable is restored afterwards, then removed
	// for real: unset and empty are exactly the difference under test
	t.Setenv("ALLOWED_ORIGINS", "")
	os.Unsetenv("ALLOWED_ORIGINS")

	got := parseOrigins(lookupEnvOr("ALLOWED_ORIGINS", devOrigin))
	if !got["http://localhost:5173"] || !got["http://127.0.0.1:5173"] {
		t.Fatalf("allowed = %v, want both dev spellings", got)
	}
}

func TestDevOriginAllowsBothLoopbackSpellings(t *testing.T) {
	// vite prints and listens on 127.0.0.1, but people type localhost; the
	// page sends whichever its address bar shows, and either must connect
	allowed := parseOrigins(devOrigin)

	for _, origin := range []string{"http://localhost:5173", "http://127.0.0.1:5173"} {
		if !allowed[origin] {
			t.Errorf("dev origin %q is not allowed by default", origin)
		}
	}
}
