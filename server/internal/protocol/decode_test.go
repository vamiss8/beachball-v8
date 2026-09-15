package protocol

import (
	"encoding/json"
	"testing"

	"beachball-v8/server/internal/game"
)

// clientInput is an input exactly as the game's own client writes it
const clientInput = `{"type":"input","data":{"seq":42,"keys":{"left":false,"right":true,"jump":true,"block":false}}}`

// viaEnvelope is the general route: decode the envelope, then its data
func viaEnvelope(raw []byte) (Input, bool) {
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil || env.Type != TypeInput {
		return Input{}, false
	}
	var in Input
	if err := json.Unmarshal(env.Data, &in); err != nil {
		return Input{}, false
	}
	return in, true
}

func TestDecodeInputReadsWhatTheClientSends(t *testing.T) {
	in, ok := DecodeInput([]byte(clientInput))
	if !ok {
		t.Fatal("the client's own input was not taken by the fast path")
	}

	want := Input{Seq: 42, Keys: game.Input{Right: true, Jump: true}}
	if in != want {
		t.Fatalf("decoded %+v, want %+v", in, want)
	}

	// the fast path is only worth having if it can never disagree with the
	// route it replaces
	if slow, _ := viaEnvelope([]byte(clientInput)); slow != in {
		t.Fatalf("fast path gave %+v, envelope route gave %+v", in, slow)
	}
}

func TestDecodeInputDeclinesAnythingElse(t *testing.T) {
	cases := map[string]string{
		"keys in another order": `{"data":{"seq":1,"keys":{}},"type":"input"}`,
		"a lobby message":       `{"type":"lobby","data":{"name":"x","ready":true}}`,
		"a ping":                `{"type":"ping","data":{"sentAt":1}}`,
		"type overridden later": `{"type":"input","type":"lobby","data":{"seq":1}}`,
		"truncated":             `{"type":"input","data":{"seq":`,
		"not json at all":       `{"type":"input", garbage`,
		"empty":                 ``,
	}

	for name, raw := range cases {
		if in, ok := DecodeInput([]byte(raw)); ok {
			t.Errorf("%s: fast path accepted it as %+v", name, in)
		}
	}

	// declining is not rejecting: the reordered input is still valid, and the
	// general route has to be the one that reads it
	if _, ok := viaEnvelope([]byte(cases["keys in another order"])); !ok {
		t.Fatal("a reordered input is not readable by the general route either")
	}
}

// BenchmarkDecodeInput and BenchmarkDecodeInputViaEnvelope are a pair: the
// message every player sends every tick, read the fast way and the old way.
func BenchmarkDecodeInput(b *testing.B) {
	raw := []byte(clientInput)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, ok := DecodeInput(raw); !ok {
			b.Fatal("fast path declined the client input")
		}
	}
}

func BenchmarkDecodeInputViaEnvelope(b *testing.B) {
	raw := []byte(clientInput)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, ok := viaEnvelope(raw); !ok {
			b.Fatal("envelope route declined the client input")
		}
	}
}
