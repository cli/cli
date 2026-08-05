package githubrest

import (
	"io"
	"net/http"
	"regexp"

	"github.com/cli/go-gh/v2/pkg/asciisanitizer"
	"golang.org/x/text/transform"
)

// jsonContentTypeRE matches the JSON content types worth sanitizing. Responses
// that are not JSON are left alone, so asset and log downloads pass through
// untouched; the sanitizer rejects invalid UTF-8 and would fail on them.
var jsonContentTypeRE = regexp.MustCompile(`[/+]json($|;)`) // sanitizingTransport neutralizes ASCII control characters in JSON response
// bodies, so that content the API returns cannot be interpreted by the terminal
// it is eventually printed to. \x1B and \x9B are the dangerous ones.
//
// This behaviour used to come from go-gh: api.Client.REST built a go-gh REST
// client around the caller's transport, and go-gh wrapped it in the same
// transform. Applying it here makes it a property of the Client rather than of
// however the caller assembled its *http.Client, so a transport built elsewhere
// cannot silently drop it. That matters for tests in particular, which pass a
// bare httpmock registry and would otherwise exercise a weaker stack than
// production runs.
//
// Wrapping is idempotent, so the copy go-gh installs inside NewHTTPClient does
// no harm: it replaces control characters with printable stand-ins, which a
// second pass then leaves alone.
type sanitizingTransport struct {
	rt http.RoundTripper
}

func (t sanitizingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.rt.RoundTrip(req)
	if err != nil || resp == nil {
		return resp, err
	}
	if resp.Body == nil || !jsonContentTypeRE.MatchString(resp.Header.Get("Content-Type")) {
		return resp, nil
	}
	resp.Body = struct {
		io.Reader
		io.Closer
	}{
		Reader: transform.NewReader(resp.Body, &asciisanitizer.Sanitizer{JSON: true}),
		Closer: resp.Body,
	}
	return resp, nil
}

// withResponseSanitizer returns a shallow copy of httpClient whose transport
// sanitizes JSON response bodies. The copy avoids mutating the single
// *http.Client the CLI shares across commands.
func withResponseSanitizer(httpClient *http.Client) *http.Client {
	copied := &http.Client{}
	if httpClient != nil {
		c := *httpClient
		copied = &c
	}
	rt := copied.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	copied.Transport = sanitizingTransport{rt: rt}
	return copied
}
