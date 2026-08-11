package extension

import (
	"io"
	"net/http"
)

const ssoHeader = "X-GitHub-SSO"

// WithSAMLFallback returns a copy of authClient that retries GET requests through
// plainClient when SAML enforcement rejects the token. Extension repositories are
// often public but owned by an org the user has no reason to authorize, and a
// successful anonymous retry proves the repository is public. See
// https://github.com/cli/cli/issues/6675.
//
// This is only sound because anonymous access cannot observe private data, so
// preferring the anonymous response can never widen what the user sees. Do not
// reuse it for endpoints whose response body differs by authentication level.
//
// onRecover, if non-nil, is called each time a rejected request is recovered.
func WithSAMLFallback(authClient, plainClient *http.Client, onRecover func()) *http.Client {
	fallbackClient := *authClient
	fallbackClient.Transport = samlFallbackTransport{
		authed:    authClient.Transport,
		plain:     plainClient.Transport,
		onRecover: onRecover,
	}
	return &fallbackClient
}

type samlFallbackTransport struct {
	authed    http.RoundTripper
	plain     http.RoundTripper
	onRecover func()
}

func (t samlFallbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// The authenticated chain sets Authorization on req in place, so a clone taken
	// afterwards would resend the token.
	var pristine *http.Request
	if isRetryable(req) {
		pristine = req.Clone(req.Context())
	}

	res, err := t.authed.RoundTrip(req)
	if err != nil || pristine == nil || !isSAMLEnforced(res) {
		return res, err
	}

	// Only 4xx and 5xx are failures, because a release asset download answers
	// with a 302 to a storage host that the outer http.Client then follows.
	plainRes, plainErr := t.plain.RoundTrip(pristine)
	if plainErr != nil || plainRes.StatusCode >= http.StatusBadRequest {
		if plainRes != nil {
			_, _ = io.Copy(io.Discard, plainRes.Body)
			plainRes.Body.Close()
		}
		// Returning the authenticated response keeps existing errors for private repos.
		return res, err
	}

	_, _ = io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if t.onRecover != nil {
		t.onRecover()
	}
	return plainRes, nil
}

func isSAMLEnforced(res *http.Response) bool {
	return res != nil && res.StatusCode == http.StatusForbidden && res.Header.Get(ssoHeader) != ""
}

// isRetryable restricts replays to bodyless GETs, which covers every extension
// request while guaranteeing a mutating request is never resent.
func isRetryable(req *http.Request) bool {
	return req.Method == http.MethodGet && req.Body == nil
}
