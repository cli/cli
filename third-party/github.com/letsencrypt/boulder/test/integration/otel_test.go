//go:build integration

package integration

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eggsampler/acme/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/letsencrypt/boulder/cmd"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/test"
)

// TraceResponse is the list of traces returned from Jaeger's trace search API
// We always search for a single trace by ID, so this should be length 1.
// This is a specialization of Jaeger's structuredResponse type which
// uses []interface{} upstream.
type TraceResponse struct {
	Data []Trace
}

// Trace represents a single trace in Jaeger's API
// See https://pkg.go.dev/github.com/jaegertracing/jaeger/model/json#Trace
type Trace struct {
	TraceID   string
	Spans     []Span
	Processes map[string]struct {
		ServiceName string
	}
	Warnings []string
}

// Span represents a single span in Jaeger's API
// See https://pkg.go.dev/github.com/jaegertracing/jaeger/model/json#Span
type Span struct {
	SpanID        string
	OperationName string
	Warnings      []string
	ProcessID     string
	References    []struct {
		RefType string
		TraceID string
		SpanID  string
	}
}

func getTraceFromJaeger(t *testing.T, traceID trace.TraceID) Trace {
	t.Helper()
	traceURL := "http://bjaeger:16686/api/traces/" + traceID.String()
	resp, err := http.Get(traceURL)
	test.AssertNotError(t, err, "failed to trace from jaeger: "+traceID.String())
	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("jaeger returned 404 for trace %s", traceID)
	}
	test.AssertEquals(t, resp.StatusCode, http.StatusOK)

	body, err := io.ReadAll(resp.Body)
	test.AssertNotError(t, err, "failed to read trace body")

	var parsed TraceResponse
	err = json.Unmarshal(body, &parsed)
	test.AssertNotError(t, err, "failed to decode traces body")

	if len(parsed.Data) != 1 {
		t.Fatalf("expected to get exactly one trace from jaeger for %s: %v", traceID, parsed)
	}

	return parsed.Data[0]
}

type expectedSpans struct {
	Operation string
	Service   string
	Children  []expectedSpans
}

// isParent returns true if the given span has a parent of ParentID
// The empty string means no ParentID
func isParent(parentID string, span Span) bool {
	if len(span.References) == 0 {
		return parentID == ""
	}
	for _, ref := range span.References {
		// In OpenTelemetry, CHILD_OF is the only reference, but Jaeger supports other systems.
		if ref.RefType == "CHILD_OF" {
			return ref.SpanID == parentID
		}
	}
	return false
}

func missingChildren(trace Trace, spanID string, children []expectedSpans) bool {
	for _, child := range children {
		if !findSpans(trace, spanID, child) {
			// Missing Child
			return true
		}
	}
	return false
}

// findSpans checks if the expectedSpan and its expected children are found in trace
func findSpans(trace Trace, parentSpan string, expectedSpan expectedSpans) bool {
	for _, span := range trace.Spans {
		if !isParent(parentSpan, span) {
			continue
		}
		if trace.Processes[span.ProcessID].ServiceName != expectedSpan.Service {
			continue
		}
		if span.OperationName != expectedSpan.Operation {
			continue
		}
		if missingChildren(trace, span.SpanID, expectedSpan.Children) {
			continue
		}

		// This span has the correct parent, service, operation, and children
		return true
	}
	fmt.Printf("did not find span %s::%s with parent '%s'\n", expectedSpan.Service, expectedSpan.Operation, parentSpan)
	return false
}

// ContextInjectingRoundTripper holds a context that is added to every request
// sent through this RoundTripper, propagating the OpenTelemetry trace through
// the requests made with it.
//
// This is useful for tracing HTTP clients which don't pass through a context,
// notably including the eggsampler ACME client used in this test.
//
// This test uses a trace started in the test to connect all the outgoing
// requests into a trace that is retrieved from Jaeger's API to make assertions
// about the spans from Boulder.
type ContextInjectingRoundTripper struct {
	ctx context.Context
}

// RoundTrip implements http.RoundTripper, injecting c.ctx and the OpenTelemetry
// propagation headers into the request. This ensures all requests are traced.
func (c *ContextInjectingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	// RoundTrip is not permitted to modify the request, so we clone with this context
	r := request.Clone(c.ctx)
	// Inject the otel propagation headers
	otel.GetTextMapPropagator().Inject(c.ctx, propagation.HeaderCarrier(r.Header))
	return http.DefaultTransport.RoundTrip(r)
}

// rpcSpan is a helper for constructing an RPC span where we have both a client and server rpc operation
func rpcSpan(op, client, server string, children ...expectedSpans) expectedSpans {
	return expectedSpans{
		Operation: op,
		Service:   client,
		Children: []expectedSpans{
			{
				Operation: op,
				Service:   server,
				Children:  children,
			},
		},
	}
}

func httpSpan(endpoint string, children ...expectedSpans) expectedSpans {
	return expectedSpans{
		Operation: endpoint,
		Service:   "boulder-wfe2",
		Children: append(children,
			rpcSpan("nonce.NonceService/Nonce", "boulder-wfe2", "nonce-service"),
			rpcSpan("nonce.NonceService/Redeem", "boulder-wfe2", "nonce-service"),
		),
	}
}

func redisPipelineSpan(op, service string, children ...expectedSpans) expectedSpans {
	return expectedSpans{
		Operation: "redis.pipeline " + op,
		Service:   service,
		Children:  children,
	}
}

// TestTraces tests that all the expected spans are present and properly connected
func TestTraces(t *testing.T) {
	t.Parallel()
	if !strings.Contains(os.Getenv("BOULDER_CONFIG_DIR"), "test/config-next") {
		t.Skip("OpenTelemetry is only configured in config-next")
	}

	traceID := traceIssuingTestCert(t)

	wfe := "boulder-wfe2"
	ra := "boulder-ra"
	ca := "boulder-ca"

	// A very stripped-down version of the expected call graph of a full issuance
	// flow: just enough to ensure that our otel tracing is working without
	// asserting too much about the exact set of RPCs we use under the hood.
	expectedSpans := expectedSpans{
		Operation: "TraceTest",
		Service:   "integration.test",
		Children: []expectedSpans{
			{Operation: "/directory", Service: wfe},
			{Operation: "/acme/new-nonce", Service: wfe, Children: []expectedSpans{
				rpcSpan("nonce.NonceService/Nonce", wfe, "nonce-service")}},
			httpSpan("/acme/new-acct",
				redisPipelineSpan("get", wfe)),
			httpSpan("/acme/new-order"),
			httpSpan("/acme/authz/"),
			httpSpan("/acme/chall/"),
			httpSpan("/acme/finalize/",
				rpcSpan("ra.RegistrationAuthority/FinalizeOrder", wfe, ra,
					rpcSpan("ca.CertificateAuthority/IssueCertificate", ra, ca))),
		},
	}

	// Retry checking for spans. Span submission is batched asynchronously, so we
	// may have to wait for the DefaultScheduleDelay (5 seconds) for results to
	// be available. Rather than always waiting, we retry a few times.
	// Empirically, this test passes on the second or third try.
	var trace Trace
	found := false
	const retries = 10
	for range retries {
		trace := getTraceFromJaeger(t, traceID)
		if findSpans(trace, "", expectedSpans) {
			found = true
			break
		}
		time.Sleep(sdktrace.DefaultScheduleDelay / 5 * time.Millisecond)
	}
	test.Assert(t, found, fmt.Sprintf("Failed to find expected spans in Jaeger for trace %s", traceID))

	test.AssertEquals(t, len(trace.Warnings), 0)
	for _, span := range trace.Spans {
		for _, warning := range span.Warnings {
			if strings.Contains(warning, "clock skew adjustment disabled; not applying calculated delta") {
				continue
			}
			t.Errorf("Span %s (%s) warning: %v", span.SpanID, span.OperationName, warning)
		}
	}
}

func traceIssuingTestCert(t *testing.T) trace.TraceID {
	// Configure this integration test to trace to jaeger:4317 like Boulder will
	shutdown := cmd.NewOpenTelemetry(cmd.OpenTelemetryConfig{
		Endpoint:    "bjaeger:4317",
		SampleRatio: 1,
	}, blog.Get())
	defer shutdown(context.Background())

	tracer := otel.GetTracerProvider().Tracer("TraceTest")
	ctx, span := tracer.Start(context.Background(), "TraceTest")
	defer span.End()

	// Provide an HTTP client with otel spans.
	// The acme client doesn't pass contexts through, so we inject one.
	option := acme.WithHTTPClient(&http.Client{
		Timeout:   60 * time.Second,
		Transport: &ContextInjectingRoundTripper{ctx},
	})

	c, err := acme.NewClient("http://boulder.service.consul:4001/directory", option)
	test.AssertNotError(t, err, "acme.NewClient failed")

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "Generating ECDSA key failed")

	account, err := c.NewAccount(privKey, false, true)
	test.AssertNotError(t, err, "newAccount failed")

	_, err = authAndIssue(&client{account, c}, nil, []acme.Identifier{{Type: "dns", Value: random_domain()}}, true, "")
	test.AssertNotError(t, err, "authAndIssue failed")

	return span.SpanContext().TraceID()
}
