package game

import (
	"strings"
	"unicode"
)

// SanitizeName cleans up a name a client asked for. names are the only free
// text a player can put on someone else's screen, so nothing here trusts the
// input. control characters are dropped so a name cannot smuggle newlines
// into the canvas, and so are the invisible formatting ones: direction
// overrides that turn the text around, and zero-width characters that make a
// name out of nothing that still does not count as empty. the result is cut
// to MaxNameLength runes rather than bytes so a name of accented letters is
// not chopped mid character.
//
// an empty result stays empty. the client names that player by their colour
// instead, which keeps the notion of colours out of the simulation.
func SanitizeName(raw string) string {
	cleaned := strings.Map(func(r rune) rune {
		// Cc is the control set and Cf the invisible formatting one. Cf also
		// holds the joiner that glues some emoji together, and those fall
		// apart into their pieces here, which is a fair price in a name
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
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
