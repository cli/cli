package web

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	blog "github.com/letsencrypt/boulder/log"
)

// RequestEvent is a structured record of the metadata we care about for a
// single web request. It is generated when a request is received, passed to
// the request handler which can populate its fields as appropriate, and then
// logged when the request completes.
type RequestEvent struct {
	// These fields are not rendered in JSON; instead, they are rendered
	// whitespace-separated ahead of the JSON. This saves bytes in the logs since
	// we don't have to include field names, quotes, or commas -- all of these
	// fields are known to not include whitespace.
	Method    string  `json:"-"`
	Endpoint  string  `json:"-"`
	Requester int64   `json:"-"`
	Code      int     `json:"-"`
	Latency   float64 `json:"-"`
	RealIP    string  `json:"-"`

	Slug           string   `json:",omitempty"`
	InternalErrors []string `json:",omitempty"`
	Error          string   `json:",omitempty"`
	UserAgent      string   `json:"ua,omitempty"`
	// Origin is sent by the browser from XHR-based clients.
	Origin string                 `json:",omitempty"`
	Extra  map[string]interface{} `json:",omitempty"`

	// For endpoints that create objects, the ID of the newly created object.
	Created string `json:",omitempty"`

	// For challenge and authorization GETs and POSTs:
	// the status of the authorization at the time the request began.
	Status string `json:",omitempty"`
	// The DNS name, if there is a single relevant name, for instance
	// in an authorization or challenge request.
	DNSName string `json:",omitempty"`
	// The set of DNS names, if there are potentially multiple relevant
	// names, for instance in a new-order, finalize, or revoke request.
	DNSNames []string `json:",omitempty"`

	// For challenge POSTs, the challenge type.
	ChallengeType string `json:",omitempty"`

	// suppressed controls whether this event will be logged when the request
	// completes. If true, no log line will be emitted. Can only be set by
	// calling .Suppress(); automatically unset by adding an internal error.
	suppressed bool `json:"-"`
}

// AddError formats the given message with the given args and appends it to the
// list of internal errors that have occurred as part of handling this event.
// If the RequestEvent has been suppressed, this un-suppresses it.
func (e *RequestEvent) AddError(msg string, args ...interface{}) {
	e.InternalErrors = append(e.InternalErrors, fmt.Sprintf(msg, args...))
	e.suppressed = false
}

// Suppress causes the RequestEvent to not be logged at all when the request
// is complete. This is a no-op if an internal error has been added to the event
// (logging errors takes precedence over suppressing output).
func (e *RequestEvent) Suppress() {
	if len(e.InternalErrors) == 0 {
		e.suppressed = true
	}
}

type WFEHandlerFunc func(context.Context, *RequestEvent, http.ResponseWriter, *http.Request)

func (f WFEHandlerFunc) ServeHTTP(e *RequestEvent, w http.ResponseWriter, r *http.Request) {
	f(r.Context(), e, w, r)
}

type wfeHandler interface {
	ServeHTTP(e *RequestEvent, w http.ResponseWriter, r *http.Request)
}

type TopHandler struct {
	wfe wfeHandler
	log blog.Logger
}

func NewTopHandler(log blog.Logger, wfe wfeHandler) *TopHandler {
	return &TopHandler{
		wfe: wfe,
		log: log,
	}
}

// responseWriterWithStatus satisfies http.ResponseWriter, but keeps track of the
// status code for logging.
type responseWriterWithStatus struct {
	http.ResponseWriter
	code int
}

// WriteHeader stores a status code for generating stats.
func (r *responseWriterWithStatus) WriteHeader(code int) {
	r.code = code
	r.ResponseWriter.WriteHeader(code)
}

func (th *TopHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check that this header is well-formed, since we assume it is when logging.
	realIP := r.Header.Get("X-Real-IP")
	if net.ParseIP(realIP) == nil {
		realIP = "0.0.0.0"
	}

	logEvent := &RequestEvent{
		RealIP:    realIP,
		Method:    r.Method,
		UserAgent: r.Header.Get("User-Agent"),
		Origin:    r.Header.Get("Origin"),
		Extra:     make(map[string]interface{}),
	}
	// We specifically override the default r.Context() because we would prefer
	// for clients to not be able to cancel our operations in arbitrary places.
	// Instead we start a new context, and apply timeouts in our various RPCs.
	ctx := context.WithoutCancel(r.Context())
	r = r.WithContext(ctx)

	// Some clients will send a HTTP Host header that includes the default port
	// for the scheme that they are using. Previously when we were fronted by
	// Akamai they would rewrite the header and strip out the unnecessary port,
	// now that they are not in our request path we need to strip these ports out
	// ourselves.
	//
	// The main reason we want to strip these ports out is so that when this header
	// is sent to the /directory endpoint we don't reply with directory URLs that
	// also contain these ports.
	//
	// We unconditionally strip :443 even when r.TLS is nil because the WFE2
	// may be deployed HTTP-only behind another service that terminates HTTPS on
	// its behalf.
	r.Host = strings.TrimSuffix(r.Host, ":443")
	r.Host = strings.TrimSuffix(r.Host, ":80")

	begin := time.Now()
	rwws := &responseWriterWithStatus{w, 0}
	defer func() {
		logEvent.Code = rwws.code
		if logEvent.Code == 0 {
			// If we haven't explicitly set a status code golang will set it
			// to 200 itself when writing to the wire
			logEvent.Code = http.StatusOK
		}
		logEvent.Latency = time.Since(begin).Seconds()
		th.logEvent(logEvent)
	}()
	th.wfe.ServeHTTP(logEvent, rwws, r)
}

func (th *TopHandler) logEvent(logEvent *RequestEvent) {
	if logEvent.suppressed {
		return
	}
	var msg string
	jsonEvent, err := json.Marshal(logEvent)
	if err != nil {
		th.log.AuditErrf("failed to marshal logEvent - %s - %#v", msg, err)
		return
	}
	th.log.Infof("%s %s %d %d %d %s JSON=%s",
		logEvent.Method, logEvent.Endpoint, logEvent.Requester, logEvent.Code,
		int(logEvent.Latency*1000), logEvent.RealIP, jsonEvent)
}

// GetClientAddr returns a comma-separated list of HTTP clients involved in
// making this request, starting with the original requester and ending with the
// remote end of our TCP connection (which is typically our own proxy).
func GetClientAddr(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff + "," + r.RemoteAddr
	}
	return r.RemoteAddr
}

func KeyTypeToString(pub crypto.PublicKey) string {
	switch pk := pub.(type) {
	case *rsa.PublicKey:
		return fmt.Sprintf("RSA %d", pk.N.BitLen())
	case *ecdsa.PublicKey:
		return fmt.Sprintf("ECDSA %s", pk.Params().Name)
	}
	return "unknown"
}
