package httpmock

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Replace http.Client transport layer with registry so all requests get
// recorded.
func ReplaceTripper(client *http.Client, reg *Registry) {
	client.Transport = reg
}

type Registry struct {
	mu       sync.Mutex
	stubs    []*Stub
	Requests []*http.Request
}

func (r *Registry) Register(m Matcher, resp Responder) {
	r.stubs = append(r.stubs, &Stub{
		Stack:     string(debug.Stack()),
		Matcher:   m,
		Responder: resp,
	})
}

func (r *Registry) Exclude(t *testing.T, m Matcher) {
	excludedStub := &Stub{
		Matcher: m,
		Responder: func(req *http.Request) (*http.Response, error) {
			assert.FailNowf(t, "Exclude error", "API called when excluded: %v", req.URL)
			return nil, nil
		},
		exclude: true,
	}
	r.stubs = append(r.stubs, excludedStub)
}

type Testing interface {
	Errorf(string, ...interface{})
	Helper()
}

func (r *Registry) Verify(t Testing) {
	var unmatchedStubStacks []string
	for _, s := range r.stubs {
		if !s.matched && !s.exclude {
			unmatchedStubStacks = append(unmatchedStubStacks, s.Stack)
		}
	}
	if len(unmatchedStubStacks) > 0 {
		t.Helper()
		stacks := strings.Builder{}
		for i, stack := range unmatchedStubStacks {
			stacks.WriteString(fmt.Sprintf("Stub %d:\n", i+1))
			stacks.WriteString(fmt.Sprintf("\t%s", stack))
			if stack != unmatchedStubStacks[len(unmatchedStubStacks)-1] {
				stacks.WriteString("\n")
			}
		}
		// about dead stubs and what they were trying to match
		t.Errorf("%d HTTP stubs unmatched, stacks:\n%s", len(unmatchedStubStacks), stacks.String())
	}
}

// RoundTrip satisfies http.RoundTripper
func (r *Registry) RoundTrip(req *http.Request) (*http.Response, error) {
	var stub *Stub

	r.mu.Lock()
	for _, s := range r.stubs {
		if s.matched || !s.Matcher(req) {
			continue
		}
		// TODO: reinstate this check once the legacy layer has been cleaned up
		// if stub != nil {
		// 	r.mu.Unlock()
		// 	return nil, fmt.Errorf("more than 1 stub matched %v", req)
		// }
		stub = s
		break // TODO: remove
	}

	if stub != nil {
		stub.matched = true
	}

	if stub == nil {
		r.mu.Unlock()
		return nil, fmt.Errorf("no registered HTTP stubs matched %v", req)
	}

	r.Requests = append(r.Requests, req)
	r.mu.Unlock()

	return stub.Responder(req)
}
