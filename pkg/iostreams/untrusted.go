package iostreams

import (
	"encoding/json"
	"strings"

	"github.com/cli/go-gh/v2/pkg/asciisanitizer"
	"golang.org/x/text/transform"
)

// Untrusted wraps string content the application did not author: HTTP response
// bodies, file contents fetched from a remote, anything that originates outside
// the CLI. The raw bytes are unexported so the only ways out are the methods
// below.
//
// Untrusted satisfies fmt.Stringer, and String sanitizes, so any fmt print path
// (Fprint, Fprintf with %s or %v, Sprint) renders the content with ANSI escape
// sequences neutralized. The only way to reach the raw bytes is Raw, which is
// deliberately easy to grep for and is intended for non-terminal uses such as
// hashing, writing to a file, or piping to another program.
type Untrusted struct {
	raw string
}

// NewUntrusted labels a string as untrusted external content.
func NewUntrusted(s string) Untrusted {
	return Untrusted{raw: s}
}

// NewUntrustedBytes labels a byte slice as untrusted external content.
func NewUntrustedBytes(b []byte) Untrusted {
	return Untrusted{raw: string(b)}
}

// String returns the content with ANSI escape sequences neutralized. It is
// called automatically by the fmt package, so printing an Untrusted value is
// safe by default on every fmt path.
func (u Untrusted) String() string {
	sanitized, _, err := transform.String(&asciisanitizer.Sanitizer{}, u.raw)
	if err != nil {
		return stripControl(u.raw)
	}
	return sanitized
}

// Raw returns the unsanitized content. It is the explicit, greppable opt-out
// for non-terminal uses (hashing, writing to disk, piping). Never pass the
// result to a terminal writer.
func (u Untrusted) Raw() string {
	return u.raw
}

// Empty reports whether the content is empty, for callers that branch on
// presence without needing the bytes.
func (u Untrusted) Empty() bool {
	return u.raw == ""
}

// UnmarshalJSON lets a struct field typed as Untrusted be populated directly by
// json.Unmarshal, so provenance is preserved across a JSON decode. This is what
// lets a decoded field (e.g. a streamed log line) stay labeled when
// the surrounding response was never sanitized by the JSON transport.
func (u *Untrusted) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	u.raw = s
	return nil
}

// MarshalJSON emits the raw content as a JSON string so a value round-trips
// faithfully through encode/decode.
func (u Untrusted) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.raw)
}

// RawBytes is Raw as a byte slice, for callers that need []byte (hashing, file
// writes). Same terminal caveat as Raw.
func (u Untrusted) RawBytes() []byte {
	return []byte(u.raw)
}

// stripControl is a defensive fallback used only if the sanitizing transform
// errors, which the asciisanitizer does not do in practice. It drops C0 control
// bytes other than tab, newline, and carriage return so the result can never
// carry an escape sequence.
func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return -1
		}
		return r
	}, s)
}
