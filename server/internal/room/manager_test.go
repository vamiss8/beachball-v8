package room

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestJoinOpensAFreshRoomForAnEmptyCode(t *testing.T) {
	m := NewManager()
	defer m.CloseAll()

	first, err := m.Join("")
	if err != nil {
		t.Fatalf("Join(\"\"): %v", err)
	}
	second, err := m.Join("")
	if err != nil {
		t.Fatalf("Join(\"\"): %v", err)
	}

	if first == second {
		t.Fatal("two players arriving without a link landed in the same room")
	}
	if m.Count() != 2 {
		t.Fatalf("live rooms = %d, want 2", m.Count())
	}
}

func TestJoinReusesARoomByItsCode(t *testing.T) {
	m := NewManager()
	defer m.CloseAll()

	host, err := m.Join("")
	if err != nil {
		t.Fatalf("Join(\"\"): %v", err)
	}

	// the friend follows the invite link, which carries the same code in a
	// url that chat apps are free to lower-case on the way
	guest, err := m.Join(strings.ToLower(host.ID))
	if err != nil {
		t.Fatalf("Join(%q): %v", host.ID, err)
	}

	if guest != host {
		t.Fatal("the invite link opened a different room")
	}
	if m.Count() != 1 {
		t.Fatalf("live rooms = %d, want 1", m.Count())
	}
}

func TestJoinRejectsCodesThatCouldNotHaveBeenIssued(t *testing.T) {
	m := NewManager()
	defer m.CloseAll()

	for _, bad := range []string{"ABC", "ABCDE", "ABC0", "../.", "%%%%"} {
		if _, err := m.Join(bad); !errors.Is(err, ErrBadCode) {
			t.Errorf("Join(%q) error = %v, want ErrBadCode", bad, err)
		}
	}

	if m.Count() != 0 {
		t.Fatalf("live rooms = %d, want 0: junk codes must not allocate", m.Count())
	}
}

func TestForgetDropsARoomFromTheRegistry(t *testing.T) {
	m := NewManager()
	defer m.CloseAll()

	first, err := m.Join("")
	if err != nil {
		t.Fatalf("Join(\"\"): %v", err)
	}
	code := first.ID

	// this is what a room calls on its way out after sitting empty
	m.forget(code)
	first.Close()

	if m.Count() != 0 {
		t.Fatalf("live rooms = %d, want 0", m.Count())
	}

	// the code is free again, so an old link opens a new match rather than
	// failing outright
	revived, err := m.Join(code)
	if err != nil {
		t.Fatalf("Join(%q): %v", code, err)
	}
	if revived == first {
		t.Fatal("expected a new room behind the reused code")
	}
}

func TestRoomClosesOnlyAfterTheGracePeriod(t *testing.T) {
	// driven by hand, with no goroutine behind it, so the clock is ours
	r := &Room{ID: "TEST"}

	if r.idleTooLong() {
		t.Fatal("a room that just went empty must not close straight away")
	}
	if r.emptySince.IsZero() {
		t.Fatal("the empty timer never started")
	}
	if r.idleTooLong() {
		t.Fatal("closed inside the grace period")
	}

	// pretend everyone left a while ago
	r.emptySince = time.Now().Add(-EmptyRoomTTL - time.Second)
	if !r.idleTooLong() {
		t.Fatal("room stayed open past its grace period")
	}
}
