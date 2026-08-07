package game

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNewWorldWaitsInTheLobby(t *testing.T) {
	w := NewWorld()

	if w.Phase != PhaseLobby {
		t.Fatalf("phase = %q, want %q", w.Phase, PhaseLobby)
	}

	// nothing should move while there is nobody to play
	before := w.Ball.Pos
	stepN(w, 120)
	if w.Ball.Pos != before {
		t.Fatalf("ball moved in the lobby: got %v, want %v", w.Ball.Pos, before)
	}
}

func TestOneReadyPlayerIsNotEnough(t *testing.T) {
	w := NewWorld()
	solo := w.AddPlayer("solo", SideLeft)
	solo.SetReady(true)

	stepN(w, 120)

	if w.Phase != PhaseLobby {
		t.Fatalf("phase = %q, want %q: a match needs an opponent", w.Phase, PhaseLobby)
	}
}

func TestMatchStartsWhenBothPlayersAreReady(t *testing.T) {
	w := NewWorld()
	left := w.AddPlayer("left", SideLeft)
	right := w.AddPlayer("right", SideRight)

	left.SetReady(true)
	stepN(w, 5)
	if w.Phase != PhaseLobby {
		t.Fatalf("phase = %q, want %q while the second player has not readied", w.Phase, PhaseLobby)
	}

	right.SetReady(true)
	w.Step()

	if w.Phase != PhaseServe {
		t.Fatalf("phase = %q, want %q once both are ready", w.Phase, PhaseServe)
	}
}

func TestLosingAPlayerMidMatchReturnsToTheLobby(t *testing.T) {
	w, _, _ := readyMatch()
	stepN(w, ServeHoldTicks+30) // well into open play

	w.RemovePlayer("right")

	if w.Phase != PhaseLobby {
		t.Fatalf("phase = %q, want %q after the opponent left", w.Phase, PhaseLobby)
	}
	// the one still here has to ask for the next match deliberately
	if w.Players["left"].Ready {
		t.Fatal("the remaining player was left marked ready")
	}
}

func TestAbandonedMatchLeavesACleanLobby(t *testing.T) {
	w, _, _ := readyMatch()
	stepN(w, ServeHoldTicks+30)

	// a match well under way, with the right player serving next
	w.Score[SideLeft] = 7
	w.Score[SideRight] = 5
	w.ServeSide = SideRight

	w.RemovePlayer("right") // and then they close the tab

	// that match can never be played out, so none of it may survive into the
	// lobby the player left behind is now sitting in
	if w.Score[SideLeft] != 0 || w.Score[SideRight] != 0 {
		t.Fatalf("score = %v, want a clean slate", w.Score)
	}
	if w.Winner != "" {
		t.Fatalf("winner = %q, want it cleared", w.Winner)
	}

	// down to the court itself: whoever walks in next must find the room
	// looking exactly like one nobody has played in yet
	fresh := NewWorld()
	if w.ServeSide != fresh.ServeSide {
		t.Fatalf("serveSide = %q, want %q as in a fresh lobby", w.ServeSide, fresh.ServeSide)
	}
	if w.Ball.Pos != fresh.Ball.Pos {
		t.Fatalf("ball parked at %v, want %v as in a fresh lobby", w.Ball.Pos, fresh.Ball.Pos)
	}
}

func TestFinishedMatchRestartsOnlyWhenBothReadyAgain(t *testing.T) {
	w, left, right := readyMatch()

	// hand the match to the left player outright
	w.Score[SideLeft] = PointsToWin - 1
	w.awardPoint(SideLeft)

	if w.Phase != PhaseFinished {
		t.Fatalf("phase = %q, want %q", w.Phase, PhaseFinished)
	}
	// readiness from the last match must not roll into a rematch
	if left.Ready || right.Ready {
		t.Fatal("players stayed ready across the end of the match")
	}

	stepN(w, 60)
	if w.Phase != PhaseFinished {
		t.Fatalf("phase = %q, want the result to stay on screen", w.Phase)
	}

	left.SetReady(true)
	right.SetReady(true)
	w.Step()

	if w.Phase != PhaseServe {
		t.Fatalf("phase = %q, want %q for the rematch", w.Phase, PhaseServe)
	}
	if w.Score[SideLeft] != 0 || w.Score[SideRight] != 0 {
		t.Fatalf("score = %v, want a clean slate", w.Score)
	}
	if w.Winner != "" {
		t.Fatalf("winner = %q, want it cleared", w.Winner)
	}
}

func TestSanitizeName(t *testing.T) {
	long := strings.Repeat("a", MaxNameLength+10)

	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"plain", "vamiss8", "vamiss8"},
		{"trimmed", "  spaced  ", "spaced"},
		{"newlines dropped", "two\nlines", "twolines"},
		{"tabs dropped", "a\tb", "ab"},
		{"empty stays empty", "   ", ""},
		{"cut to the limit", long, strings.Repeat("a", MaxNameLength)},
		{"no trailing space after the cut", strings.Repeat("b", MaxNameLength-1) + "  tail", strings.Repeat("b", MaxNameLength-1)},
	}

	for _, c := range cases {
		if got := SanitizeName(c.raw); got != c.want {
			t.Errorf("%s: SanitizeName(%q) = %q, want %q", c.name, c.raw, got, c.want)
		}
	}
}

func TestSanitizeNameCountsRunesNotBytes(t *testing.T) {
	// each of these is two bytes, so a byte-based cut would slice one in half
	// and leave broken utf-8 on someone else's screen
	name := strings.Repeat("я", MaxNameLength+4)

	got := SanitizeName(name)

	if runes := []rune(got); len(runes) != MaxNameLength {
		t.Fatalf("kept %d runes, want %d", len(runes), MaxNameLength)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("SanitizeName produced invalid utf-8: %q", got)
	}
}
