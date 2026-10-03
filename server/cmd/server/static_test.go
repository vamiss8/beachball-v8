package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// builtClient lays out a directory the way `npm run build` leaves one: a page
// at the root and a hashed file under assets.
func builtClient(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"index.html":             "<!DOCTYPE html><canvas id=\"game\"></canvas>",
		"assets/index-abc123.js": "console.log('beachball')",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func get(t *testing.T, h http.Handler, path string) *http.Response {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Result()
}

func TestStaticFilesAreCachedByWhatTheyAre(t *testing.T) {
	h := staticHandler(builtClient(t))

	cases := []struct {
		name   string
		path   string
		status int
		cache  string
	}{
		// the page points at the hashed files, so a copy kept past a deploy
		// goes on asking for assets that are no longer there
		{"the page is always revalidated", "/", http.StatusOK, "no-cache"},
		// the hash in the name changes with the contents, so this url can
		// never come back with something else
		{"a hashed asset is kept for good", "/assets/index-abc123.js", http.StatusOK, "public, max-age=31536000, immutable"},
		// the one that matters after a deploy: an old page asks for an asset
		// the new build removed, and a miss remembered for a year would be
		// served long after. the header is set before the file is looked up,
		// so this only holds because net/http drops it from an error. pinned
		// here so that wrapping the file server cannot quietly undo it
		{"a missing asset is not remembered", "/assets/index-old999.js", http.StatusNotFound, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := get(t, h, tc.path)
			if resp.StatusCode != tc.status {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.status)
			}
			if got := resp.Header.Get("Cache-Control"); got != tc.cache {
				t.Fatalf("Cache-Control = %q, want %q", got, tc.cache)
			}
			if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
				t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
			}
		})
	}
}

func TestMissingBuildSaysHowToFixIt(t *testing.T) {
	h := staticHandler(filepath.Join(t.TempDir(), "never-built"))

	resp := get(t, h, "/")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	// a bare 404 on the front page says nothing about the cause. the path is
	// right and the server is up; the client simply has not been built
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "npm run build") {
		t.Fatalf("body = %q, want it to name the command that fixes this", body)
	}
}
