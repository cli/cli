package iostreams

import "io"

// IOStreams is a minimal stub mirroring the real package's writer fields so the
// query can match Out / ErrOut / ContentOut by qualified name in tests.
type IOStreams struct {
	Out        io.Writer
	ErrOut     io.Writer
	ContentOut io.Writer
}

// Untrusted is a minimal stub of the real provenance type so fixtures can mint
// and unwrap external content and the query can match String / Raw by qualified
// name.
type Untrusted struct {
	raw string
}

func NewUntrusted(s string) Untrusted { return Untrusted{raw: s} }

func NewUntrustedBytes(b []byte) Untrusted { return Untrusted{raw: string(b)} }

func (u Untrusted) String() string { return sanitizeStub(u.raw) }

func (u Untrusted) Raw() string { return u.raw }

func sanitizeStub(s string) string { return s }

