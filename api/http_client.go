package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/utils"
	"github.com/cli/go-gh"
	ghAPI "github.com/cli/go-gh/pkg/api"
)

var jsonTypeRE = regexp.MustCompile(`[/+]json($|;)`)

type tokenGetter interface {
	AuthToken(string) (string, string)
}

type HTTPClientOptions struct {
	AppVersion        string
	CacheTTL          time.Duration
	Config            tokenGetter
	EnableCache       bool
	Log               io.Writer
	LogColorize       bool
	SkipAcceptHeaders bool
}

func NewHTTPClient(opts HTTPClientOptions) (*http.Client, error) {
	// Provide invalid host, and token values so gh.HTTPClient will not automatically resolve them.
	// The real host and token are inserted at request time.
	clientOpts := ghAPI.ClientOptions{
		Host:         "none",
		AuthToken:    "none",
		LogIgnoreEnv: true,
	}

	if debugEnabled, debugValue := utils.IsDebugEnabled(); debugEnabled {
		clientOpts.Log = opts.Log
		clientOpts.LogColorize = opts.LogColorize
		clientOpts.LogVerboseHTTP = strings.Contains(debugValue, "api")
	}

	headers := map[string]string{
		userAgent: fmt.Sprintf("GitHub CLI %s", opts.AppVersion),
	}
	if opts.SkipAcceptHeaders {
		headers[accept] = ""
	}
	clientOpts.Headers = headers

	if opts.EnableCache {
		clientOpts.EnableCache = opts.EnableCache
		clientOpts.CacheTTL = opts.CacheTTL
	}

	client, err := gh.HTTPClient(&clientOpts)
	if err != nil {
		return nil, err
	}

	if opts.Config != nil {
		client.Transport = AddAuthTokenHeader(client.Transport, opts.Config)
	}

	client.Transport = SanitizeASCIIControlCharacters(client.Transport)

	return client, nil
}

func NewCachedHTTPClient(httpClient *http.Client, ttl time.Duration) *http.Client {
	newClient := *httpClient
	newClient.Transport = AddCacheTTLHeader(httpClient.Transport, ttl)
	return &newClient
}

// AddCacheTTLHeader adds an header to the request telling the cache that the request
// should be cached for a specified amount of time.
func AddCacheTTLHeader(rt http.RoundTripper, ttl time.Duration) http.RoundTripper {
	return &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
		// If the header is already set in the request, don't overwrite it.
		if req.Header.Get(cacheTTL) == "" {
			req.Header.Set(cacheTTL, ttl.String())
		}
		return rt.RoundTrip(req)
	}}
}

// AddAuthToken adds an authentication token header for the host specified by the request.
func AddAuthTokenHeader(rt http.RoundTripper, cfg tokenGetter) http.RoundTripper {
	return &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
		// If the header is already set in the request, don't overwrite it.
		if req.Header.Get(authorization) == "" {
			hostname := ghinstance.NormalizeHostname(getHost(req))
			if token, _ := cfg.AuthToken(hostname); token != "" {
				req.Header.Set(authorization, fmt.Sprintf("token %s", token))
			}
		}
		return rt.RoundTrip(req)
	}}
}

// ExtractHeader extracts a named header from any response received by this client and,
// if non-blank, saves it to dest.
func ExtractHeader(name string, dest *string) func(http.RoundTripper) http.RoundTripper {
	return func(tr http.RoundTripper) http.RoundTripper {
		return &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
			res, err := tr.RoundTrip(req)
			if err == nil {
				if value := res.Header.Get(name); value != "" {
					*dest = value
				}
			}
			return res, err
		}}
	}
}

// GitHub servers return non-printable characters as their unicode code point values.
// The values of \u0000 to \u001F represent C0 ASCII control characters and
// the values of \u0080 to \u009F represent C1 ASCII control characters. These control
// characters will be interpreted by the terminal, this behaviour can be used maliciously
// as an attack vector, especially the control character \u001B. This function escapes
// all non-printable characters between \u0000 and \u00FF so that the terminal will
// not interpret them.
func SanitizeASCIIControlCharacters(rt http.RoundTripper) http.RoundTripper {
	return &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
		res, err := rt.RoundTrip(req)
		if err != nil ||
			!jsonTypeRE.MatchString(res.Header.Get("Content-Type")) ||
			res.ContentLength == 0 {
			return res, err
		}
		var sanitized bytes.Buffer
		err = replaceControlCharacters(res.Body, &sanitized)
		if err != nil {
			err = fmt.Errorf("ascii control characters sanitization error: %w", err)
		}
		res.Body.Close()
		res.Body = io.NopCloser(&sanitized)
		res.ContentLength = int64(sanitized.Len())
		return res, err
	}}
}

// replaceControlCharacters is a sliding window alogorithm that
// detects C0 and C1 ASCII control sequences as they are read
// from r and replaces them with equivelent inert characters and
// writes them to w. Characters that are not part of a control
// sequence are written as is to w.
func replaceControlCharacters(r io.Reader, w io.Writer) error {
	// Byte representation of the string sequence "\u00" which is the prefix of
	// all C0 and C1 control sequeneces.
	find := []byte{92, 117, 48, 48}
	// Length of find sequence.
	size := 4
	// Create an index variable to match bytes.
	idx := 0
	// Used for reading single byte.
	b := make([]byte, 1)
	// Used for reading two bytes.
	c := make([]byte, 2)
	// Used to keep track of escape characters.
	var addBackslash bool
	// Used to keep track of when to read.
	var skipRead bool

	for {
		if !skipRead {
			if n, err := r.Read(b); err != nil {
				if !errors.Is(err, io.EOF) {
					return err
				}
				if n > 0 {
					if _, err = w.Write(b); err != nil {
						return err
					}
				}
				break
			}
		}
		skipRead = false

		// Match.
		if b[0] == find[idx] {
			idx++
			if idx == size {
				if _, err := r.Read(c); err != nil {
					return err
				}
				s := append(find, c[0], c[1])
				repl, found := mapControlCharacterToCaret(s)
				if found && addBackslash {
					repl = append([]byte{92}, repl...)
				}
				if _, err := w.Write(repl); err != nil {
					return err
				}
				idx = 0
				addBackslash = false
			}
			continue
		}

		// No match but with previous match.
		if idx != 0 {
			if _, err := w.Write(find[:idx]); err != nil {
				return err
			}
			if idx == 1 {
				addBackslash = !addBackslash
			}
			idx = 0
			skipRead = true
			continue
		}

		// No match and no previous match.
		if _, err := w.Write(b); err != nil {
			return err
		}
		idx = 0
		addBackslash = false
	}

	return nil
}

// mapControlCharacterToCaret maps C0 control sequences to caret notation and
// C1 control sequences to hex notation. C1 control sequences do
// not have caret notation representation.
func mapControlCharacterToCaret(b []byte) ([]byte, bool) {
	m := map[string]string{
		`\u0000`: `^@`,
		`\u0001`: `^A`,
		`\u0002`: `^B`,
		`\u0003`: `^C`,
		`\u0004`: `^D`,
		`\u0005`: `^E`,
		`\u0006`: `^F`,
		`\u0007`: `^G`,
		`\u0008`: `^H`,
		`\u0009`: `^I`,
		`\u000a`: `^J`,
		`\u000b`: `^K`,
		`\u000c`: `^L`,
		`\u000d`: `^M`,
		`\u000e`: `^N`,
		`\u000f`: `^O`,
		`\u0010`: `^P`,
		`\u0011`: `^Q`,
		`\u0012`: `^R`,
		`\u0013`: `^S`,
		`\u0014`: `^T`,
		`\u0015`: `^U`,
		`\u0016`: `^V`,
		`\u0017`: `^W`,
		`\u0018`: `^X`,
		`\u0019`: `^Y`,
		`\u001a`: `^Z`,
		`\u001b`: `^[`,
		`\u001c`: `^\\`,
		`\u001d`: `^]`,
		`\u001e`: `^^`,
		`\u001f`: `^_`,
		`\u0080`: `\\200`,
		`\u0081`: `\\201`,
		`\u0082`: `\\202`,
		`\u0083`: `\\203`,
		`\u0084`: `\\204`,
		`\u0085`: `\\205`,
		`\u0086`: `\\206`,
		`\u0087`: `\\207`,
		`\u0088`: `\\210`,
		`\u0089`: `\\211`,
		`\u008a`: `\\212`,
		`\u008b`: `\\213`,
		`\u008c`: `\\214`,
		`\u008d`: `\\215`,
		`\u008e`: `\\216`,
		`\u008f`: `\\217`,
		`\u0090`: `\\220`,
		`\u0091`: `\\221`,
		`\u0092`: `\\222`,
		`\u0093`: `\\223`,
		`\u0094`: `\\224`,
		`\u0095`: `\\225`,
		`\u0096`: `\\226`,
		`\u0097`: `\\227`,
		`\u0098`: `\\230`,
		`\u0099`: `\\231`,
		`\u009a`: `\\232`,
		`\u009b`: `\\233`,
		`\u009c`: `\\234`,
		`\u009d`: `\\235`,
		`\u009e`: `\\236`,
		`\u009f`: `\\237`,
	}
	if c, ok := m[strings.ToLower(string(b))]; ok {
		return []byte(c), true
	}
	return b, false
}

type funcTripper struct {
	roundTrip func(*http.Request) (*http.Response, error)
}

func (tr funcTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return tr.roundTrip(req)
}

func getHost(r *http.Request) string {
	if r.Host != "" {
		return r.Host
	}
	return r.URL.Hostname()
}
