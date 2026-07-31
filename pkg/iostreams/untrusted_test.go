package iostreams

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const esc = "\x1b"

func TestUntrusted_String_sanitizes(t *testing.T) {
	u := NewUntrusted("hello" + esc + "[31mRED" + esc + "[0m")
	assert.NotContains(t, u.String(), esc)
}

// The property that drove the design: fmt reflection must not leak the raw
// bytes through any verb. Because Untrusted implements Stringer, %s, %v, and the
// Print family all route through String() and sanitize.
func TestUntrusted_fmt_paths_never_leak(t *testing.T) {
	u := NewUntrusted("x" + esc + "]0;title" + esc + "\\")
	cases := map[string]string{
		"%s":     fmt.Sprintf("%s", u),
		"%v":     fmt.Sprintf("%v", u),
		"Sprint": fmt.Sprint(u),
		"woven":  fmt.Sprintf("by %s here", u),
	}
	for name, out := range cases {
		t.Run(name, func(t *testing.T) {
			assert.NotContains(t, out, esc)
		})
	}
}

func TestUntrusted_Raw_returnsExactBytes(t *testing.T) {
	payload := "x" + esc + "[1mbold"
	u := NewUntrusted(payload)
	assert.Equal(t, payload, u.Raw())
	assert.Equal(t, payload, string(u.RawBytes()))
}

func TestUntrustedBytes_roundTrip(t *testing.T) {
	u := NewUntrustedBytes([]byte("plain text"))
	assert.Equal(t, "plain text", u.String())
}

func TestStripControl_dropsC0KeepsWhitespace(t *testing.T) {
	assert.Equal(t, "abc\td\ne", stripControl("a\x1bb\x07c\td\ne"))
}

// The showcase property: an Untrusted struct field is populated by
// json.Unmarshal with provenance intact, so printing it later sanitizes even
// though the bytes arrived through a JSON decode.
func TestUntrusted_survivesJSONDecode(t *testing.T) {
	var entry struct {
		Content Untrusted `json:"content"`
	}
	payload := `{"content":"log\u001b[31mline"}`
	require.NoError(t, json.Unmarshal([]byte(payload), &entry))
	assert.Equal(t, "log\x1b[31mline", entry.Content.Raw())
	assert.NotContains(t, entry.Content.String(), esc)
}

func TestUntrusted_jsonRoundTrip(t *testing.T) {
	u := NewUntrusted("x\x1b[0m")
	b, err := json.Marshal(u)
	require.NoError(t, err)

	var back Untrusted
	require.NoError(t, json.Unmarshal(b, &back))
	assert.Equal(t, u.Raw(), back.Raw())
}

func TestUntrusted_Empty(t *testing.T) {
	assert.True(t, NewUntrusted("").Empty())
	assert.False(t, NewUntrusted("x").Empty())
}
