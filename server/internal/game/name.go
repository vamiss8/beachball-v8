package game

import (
	"strings"
	"unicode"
)

// SanitizeName cleans up a name a client asked for. names are the only free
// text a player can put on someone else's screen, so nothing here trusts the
// input: control characters are dropped so a name cannot smuggle newlines or
// direction overrides into the canvas, and the result is cut to MaxNameLength
// runes rather than bytes so a name of accented letters is not chopped mid
// character.
//
// an empty result stays empty. the client names that player by their colour
// instead, which keeps the notion of colours out of the simulation.
func SanitizeName(raw string) string {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, raw)

	runes := []rune(strings.TrimSpace(cleaned))
	if len(runes) > MaxNameLength {
		runes = runes[:MaxNameLength]
	}

	// trimmed again: cutting a long name can leave a trailing space behind
	return strings.TrimSpace(string(runes))
}
