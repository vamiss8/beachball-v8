package room

import (
	"crypto/rand"
	"math/big"
	"strings"
)

// CodeAlphabet leaves out the characters people mix up when a code is read
// aloud or copied off a screen: 0/O, 1/I/L.
const CodeAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// CodeLength is short enough to type from a screenshot and still leaves
// roughly a million combinations, so nobody stumbles into someone else's game.
const CodeLength = 4

// NewCode returns a random room code. crypto/rand rather than math/rand
// because a predictable code is an invitation to join a stranger's match.
func NewCode() (string, error) {
	limit := big.NewInt(int64(len(CodeAlphabet)))

	out := make([]byte, CodeLength)
	for i := range out {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		out[i] = CodeAlphabet[n.Int64()]
	}
	return string(out), nil
}

// NormalizeCode upper-cases a code that came in from a url and reports whether
// it is one we could have issued. anything else is rejected rather than
// quietly accepted, otherwise a script could fill the room map with junk keys.
func NormalizeCode(raw string) (string, bool) {
	if len(raw) != CodeLength {
		return "", false
	}

	code := strings.ToUpper(raw)
	for _, c := range code {
		if !strings.ContainsRune(CodeAlphabet, c) {
			return "", false
		}
	}
	return code, true
}
