package room

import (
	"strings"
	"testing"
)

func TestNewCodeUsesTheSafeAlphabet(t *testing.T) {
	for i := 0; i < 100; i++ {
		code, err := NewCode()
		if err != nil {
			t.Fatalf("NewCode: %v", err)
		}
		if len(code) != CodeLength {
			t.Fatalf("code %q has length %d, want %d", code, len(code), CodeLength)
		}
		for _, c := range code {
			if !strings.ContainsRune(CodeAlphabet, c) {
				t.Fatalf("code %q contains %q, which is not in the alphabet", code, c)
			}
		}
	}
}

func TestNormalizeCode(t *testing.T) {
	cases := []struct {
		raw  string
		want string
		ok   bool
	}{
		{"ABCD", "ABCD", true},
		{"abcd", "ABCD", true}, // links get lower-cased by chat apps
		{"", "", false},
		{"ABC", "", false},   // too short
		{"ABCDE", "", false}, // too long
		{"ABC0", "", false},  // 0 is not in the alphabet
		{"ABCI", "", false},  // neither is I
		{"AB CD", "", false},
		{"../..", "", false},
	}

	for _, c := range cases {
		got, ok := NormalizeCode(c.raw)
		if ok != c.ok || got != c.want {
			t.Errorf("NormalizeCode(%q) = %q, %v; want %q, %v", c.raw, got, ok, c.want, c.ok)
		}
	}
}
