package httpmock

import (
	"fmt"
	"net/http"
	"sync"
)

type Registry struct {
	mu       sync.Mutex
	stubs    []*Stub
	Requests []*http.Request
}

func (r *Registry) Register(m Matcher, resp Responder) {
	r.stubs = append(r.stubs, &Stub{
		Matcher:   m,
		Responder: resp,
	})
}

type Testing interface {
	Errorf(string, ...interface{})
}

func (r *Registry) Verify(t Testing) {
	n := 0
	for _, s := range r.stubs {
		if !s.matched {
			n++
		}
	}
	if n > 0 {
		// NOTE: stubs offer no useful reflection, so we can't print details
		// about dead stubs and what they were trying to match
		t.Errorf("%d unmatched HTTP stubs", n)
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
		return nil, fmt.Errorf("no registered stubs matched %v", req)
	}

	r.Requests = append(r.Requests, req)
	r.mu.Unlock()

	return stub.Responder(req)
}
