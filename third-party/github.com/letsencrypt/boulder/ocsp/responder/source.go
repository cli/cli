package responder

import (
	"context"

	"golang.org/x/crypto/ocsp"
)

// Response is a wrapper around the standard library's *ocsp.Response, but it
// also carries with it the raw bytes of the encoded response.
type Response struct {
	*ocsp.Response
	Raw []byte
}

// Source represents the logical source of OCSP responses, i.e.,
// the logic that actually chooses a response based on a request.
type Source interface {
	Response(context.Context, *ocsp.Request) (*Response, error)
}
