package api

import (
	"io"
	"net/http"
	"regexp"

	"github.com/cli/cli/v2/internal/asciisanitizer"
	"golang.org/x/text/transform"
)

var jsonTypeRE = regexp.MustCompile(`[/+]json($|;)`)

// GitHub servers do not sanitize their API output for terminal display
// and leave in unescaped ASCII control characters.
// These control characters will be interpreted by the terminal, this behaviour can be
// used maliciously as an attack vector, especially the control characters \u001B and \u009B.
// This function wraps JSON response bodies in a ReadCloser that transforms C0 and C1
// control characters to their caret notations respectively so that the terminal will not
// interpret them.
func AddASCIISanitizer(rt http.RoundTripper) http.RoundTripper {
	return &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
		res, err := rt.RoundTrip(req)
		if err != nil || !jsonTypeRE.MatchString(res.Header.Get("Content-Type")) {
			return res, err
		}
		res.Body = sanitizedReadCloser(res.Body)
		return res, err
	}}
}

func sanitizedReadCloser(rc io.ReadCloser) io.ReadCloser {
	return struct {
		io.Reader
		io.Closer
	}{
		Reader: transform.NewReader(rc, &asciisanitizer.Sanitizer{}),
		Closer: rc,
	}
}
