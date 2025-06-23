package grpc

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/letsencrypt/boulder/cmd"
	berrors "github.com/letsencrypt/boulder/errors"
)

const (
	returnOverhead         = 20 * time.Millisecond
	meaningfulWorkOverhead = 100 * time.Millisecond
	clientRequestTimeKey   = "client-request-time"
)

type serverInterceptor interface {
	Unary(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error)
	Stream(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error
}

// noopServerInterceptor provides no-op interceptors. It can be substituted for
// an interceptor that has been disabled.
type noopServerInterceptor struct{}

// Unary is a gRPC unary interceptor.
func (n *noopServerInterceptor) Unary(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	return handler(ctx, req)
}

// Stream is a gRPC stream interceptor.
func (n *noopServerInterceptor) Stream(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	return handler(srv, ss)
}

// Ensure noopServerInterceptor matches the serverInterceptor interface.
var _ serverInterceptor = &noopServerInterceptor{}

type clientInterceptor interface {
	Unary(ctx context.Context, method string, req interface{}, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error
	Stream(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error)
}

// serverMetadataInterceptor is a gRPC interceptor that adds Prometheus
// metrics to requests handled by a gRPC server, and wraps Boulder-specific
// errors for transmission in a grpc/metadata trailer (see bcodes.go).
type serverMetadataInterceptor struct {
	metrics serverMetrics
	clk     clock.Clock
}

func newServerMetadataInterceptor(metrics serverMetrics, clk clock.Clock) serverMetadataInterceptor {
	return serverMetadataInterceptor{
		metrics: metrics,
		clk:     clk,
	}
}

// Unary implements the grpc.UnaryServerInterceptor interface.
func (smi *serverMetadataInterceptor) Unary(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler) (interface{}, error) {
	if info == nil {
		return nil, berrors.InternalServerError("passed nil *grpc.UnaryServerInfo")
	}

	// Extract the grpc metadata from the context. If the context has
	// a `clientRequestTimeKey` field, and it has a value, then observe the RPC
	// latency with Prometheus.
	if md, ok := metadata.FromIncomingContext(ctx); ok && len(md[clientRequestTimeKey]) > 0 {
		err := smi.observeLatency(md[clientRequestTimeKey][0])
		if err != nil {
			return nil, err
		}
	}

	// Shave 20 milliseconds off the deadline to ensure that if the RPC server times
	// out any sub-calls it makes (like DNS lookups, or onwards RPCs), it has a
	// chance to report that timeout to the client. This allows for more specific
	// errors, e.g "the VA timed out looking up CAA for example.com" (when called
	// from RA.NewCertificate, which was called from WFE.NewCertificate), as
	// opposed to "RA.NewCertificate timed out" (causing a 500).
	// Once we've shaved the deadline, we ensure we have we have at least another
	// 100ms left to do work; otherwise we abort early.
	deadline, ok := ctx.Deadline()
	// Should never happen: there was no deadline.
	if !ok {
		deadline = time.Now().Add(100 * time.Second)
	}
	deadline = deadline.Add(-returnOverhead)
	remaining := time.Until(deadline)
	if remaining < meaningfulWorkOverhead {
		return nil, status.Errorf(codes.DeadlineExceeded, "not enough time left on clock: %s", remaining)
	}

	localCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	resp, err := handler(localCtx, req)
	if err != nil {
		err = wrapError(localCtx, err)
	}
	return resp, err
}

// interceptedServerStream wraps an existing server stream, but replaces its
// context with its own.
type interceptedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context implements part of the grpc.ServerStream interface.
func (iss interceptedServerStream) Context() context.Context {
	return iss.ctx
}

// Stream implements the grpc.StreamServerInterceptor interface.
func (smi *serverMetadataInterceptor) Stream(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler) error {
	ctx := ss.Context()

	// Extract the grpc metadata from the context. If the context has
	// a `clientRequestTimeKey` field, and it has a value, then observe the RPC
	// latency with Prometheus.
	if md, ok := metadata.FromIncomingContext(ctx); ok && len(md[clientRequestTimeKey]) > 0 {
		err := smi.observeLatency(md[clientRequestTimeKey][0])
		if err != nil {
			return err
		}
	}

	// Shave 20 milliseconds off the deadline to ensure that if the RPC server times
	// out any sub-calls it makes (like DNS lookups, or onwards RPCs), it has a
	// chance to report that timeout to the client. This allows for more specific
	// errors, e.g "the VA timed out looking up CAA for example.com" (when called
	// from RA.NewCertificate, which was called from WFE.NewCertificate), as
	// opposed to "RA.NewCertificate timed out" (causing a 500).
	// Once we've shaved the deadline, we ensure we have we have at least another
	// 100ms left to do work; otherwise we abort early.
	deadline, ok := ctx.Deadline()
	// Should never happen: there was no deadline.
	if !ok {
		deadline = time.Now().Add(100 * time.Second)
	}
	deadline = deadline.Add(-returnOverhead)
	remaining := time.Until(deadline)
	if remaining < meaningfulWorkOverhead {
		return status.Errorf(codes.DeadlineExceeded, "not enough time left on clock: %s", remaining)
	}

	// Server stream interceptors are synchronous (they return their error, if
	// any, when the stream is done) so defer cancel() is safe here.
	localCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	err := handler(srv, interceptedServerStream{ss, localCtx})
	if err != nil {
		err = wrapError(localCtx, err)
	}
	return err
}

// splitMethodName is borrowed directly from
// `grpc-ecosystem/go-grpc-prometheus/util.go` and is used to extract the
// service and method name from the `method` argument to
// a `UnaryClientInterceptor`.
func splitMethodName(fullMethodName string) (string, string) {
	fullMethodName = strings.TrimPrefix(fullMethodName, "/") // remove leading slash
	if i := strings.Index(fullMethodName, "/"); i >= 0 {
		return fullMethodName[:i], fullMethodName[i+1:]
	}
	return "unknown", "unknown"
}

// observeLatency is called with the `clientRequestTimeKey` value from
// a request's gRPC metadata. This string value is converted to a timestamp and
// used to calculate the latency between send and receive time. The latency is
// published to the server interceptor's rpcLag prometheus histogram. An error
// is returned if the `clientReqTime` string is not a valid timestamp.
func (smi *serverMetadataInterceptor) observeLatency(clientReqTime string) error {
	// Convert the metadata request time into an int64
	reqTimeUnixNanos, err := strconv.ParseInt(clientReqTime, 10, 64)
	if err != nil {
		return berrors.InternalServerError("grpc metadata had illegal %s value: %q - %s",
			clientRequestTimeKey, clientReqTime, err)
	}
	// Calculate the elapsed time since the client sent the RPC
	reqTime := time.Unix(0, reqTimeUnixNanos)
	elapsed := smi.clk.Since(reqTime)
	// Publish an RPC latency observation to the histogram
	smi.metrics.rpcLag.Observe(elapsed.Seconds())
	return nil
}

// Ensure serverMetadataInterceptor matches the serverInterceptor interface.
var _ serverInterceptor = (*serverMetadataInterceptor)(nil)

// clientMetadataInterceptor is a gRPC interceptor that adds Prometheus
// metrics to sent requests, and disables FailFast. We disable FailFast because
// non-FailFast mode is most similar to the old AMQP RPC layer: If a client
// makes a request while all backends are briefly down (e.g. for a restart), the
// request doesn't necessarily fail. A backend can service the request if it
// comes back up within the timeout. Under gRPC the same effect is achieved by
// retries up to the Context deadline.
type clientMetadataInterceptor struct {
	timeout time.Duration
	metrics clientMetrics
	clk     clock.Clock

	waitForReady bool
}

// Unary implements the grpc.UnaryClientInterceptor interface.
func (cmi *clientMetadataInterceptor) Unary(
	ctx context.Context,
	fullMethod string,
	req,
	reply interface{},
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption) error {
	// This should not occur but fail fast with a clear error if it does (e.g.
	// because of buggy unit test code) instead of a generic nil panic later!
	if cmi.metrics.inFlightRPCs == nil {
		return berrors.InternalServerError("clientInterceptor has nil inFlightRPCs gauge")
	}

	// Ensure that the context has a deadline set.
	localCtx, cancel := context.WithTimeout(ctx, cmi.timeout)
	defer cancel()

	// Convert the current unix nano timestamp to a string for embedding in the grpc metadata
	nowTS := strconv.FormatInt(cmi.clk.Now().UnixNano(), 10)
	// Create a grpc/metadata.Metadata instance for the request metadata.
	// Initialize it with the request time.
	reqMD := metadata.New(map[string]string{clientRequestTimeKey: nowTS})
	// Configure the localCtx with the metadata so it gets sent along in the request
	localCtx = metadata.NewOutgoingContext(localCtx, reqMD)

	// Disable fail-fast so RPCs will retry until deadline, even if all backends
	// are down.
	opts = append(opts, grpc.WaitForReady(cmi.waitForReady))

	// Create a grpc/metadata.Metadata instance for a grpc.Trailer.
	respMD := metadata.New(nil)
	// Configure a grpc Trailer with respMD. This allows us to wrap error
	// types in the server interceptor later on.
	opts = append(opts, grpc.Trailer(&respMD))

	// Split the method and service name from the fullMethod.
	// UnaryClientInterceptor's receive a `method` arg of the form
	// "/ServiceName/MethodName"
	service, method := splitMethodName(fullMethod)
	// Slice the inFlightRPC inc/dec calls by method and service
	labels := prometheus.Labels{
		"method":  method,
		"service": service,
	}
	// Increment the inFlightRPCs gauge for this method/service
	cmi.metrics.inFlightRPCs.With(labels).Inc()
	// And defer decrementing it when we're done
	defer cmi.metrics.inFlightRPCs.With(labels).Dec()

	// Handle the RPC
	begin := cmi.clk.Now()
	err := invoker(localCtx, fullMethod, req, reply, cc, opts...)
	if err != nil {
		err = unwrapError(err, respMD)
		if status.Code(err) == codes.DeadlineExceeded {
			return deadlineDetails{
				service: service,
				method:  method,
				latency: cmi.clk.Since(begin),
			}
		}
	}
	return err
}

// interceptedClientStream wraps an existing client stream, and calls finish
// when the stream ends or any operation on it fails.
type interceptedClientStream struct {
	grpc.ClientStream
	finish func(error) error
}

// Header implements part of the grpc.ClientStream interface.
func (ics interceptedClientStream) Header() (metadata.MD, error) {
	md, err := ics.ClientStream.Header()
	if err != nil {
		err = ics.finish(err)
	}
	return md, err
}

// SendMsg implements part of the grpc.ClientStream interface.
func (ics interceptedClientStream) SendMsg(m interface{}) error {
	err := ics.ClientStream.SendMsg(m)
	if err != nil {
		err = ics.finish(err)
	}
	return err
}

// RecvMsg implements part of the grpc.ClientStream interface.
func (ics interceptedClientStream) RecvMsg(m interface{}) error {
	err := ics.ClientStream.RecvMsg(m)
	if err != nil {
		err = ics.finish(err)
	}
	return err
}

// CloseSend implements part of the grpc.ClientStream interface.
func (ics interceptedClientStream) CloseSend() error {
	err := ics.ClientStream.CloseSend()
	if err != nil {
		err = ics.finish(err)
	}
	return err
}

// Stream implements the grpc.StreamClientInterceptor interface.
func (cmi *clientMetadataInterceptor) Stream(
	ctx context.Context,
	desc *grpc.StreamDesc,
	cc *grpc.ClientConn,
	fullMethod string,
	streamer grpc.Streamer,
	opts ...grpc.CallOption) (grpc.ClientStream, error) {
	// This should not occur but fail fast with a clear error if it does (e.g.
	// because of buggy unit test code) instead of a generic nil panic later!
	if cmi.metrics.inFlightRPCs == nil {
		return nil, berrors.InternalServerError("clientInterceptor has nil inFlightRPCs gauge")
	}

	// We don't defer cancel() here, because this function is going to return
	// immediately. Instead we store it in the interceptedClientStream.
	localCtx, cancel := context.WithTimeout(ctx, cmi.timeout)

	// Convert the current unix nano timestamp to a string for embedding in the grpc metadata
	nowTS := strconv.FormatInt(cmi.clk.Now().UnixNano(), 10)
	// Create a grpc/metadata.Metadata instance for the request metadata.
	// Initialize it with the request time.
	reqMD := metadata.New(map[string]string{clientRequestTimeKey: nowTS})
	// Configure the localCtx with the metadata so it gets sent along in the request
	localCtx = metadata.NewOutgoingContext(localCtx, reqMD)

	// Disable fail-fast so RPCs will retry until deadline, even if all backends
	// are down.
	opts = append(opts, grpc.WaitForReady(cmi.waitForReady))

	// Create a grpc/metadata.Metadata instance for a grpc.Trailer.
	respMD := metadata.New(nil)
	// Configure a grpc Trailer with respMD. This allows us to wrap error
	// types in the server interceptor later on.
	opts = append(opts, grpc.Trailer(&respMD))

	// Split the method and service name from the fullMethod.
	// UnaryClientInterceptor's receive a `method` arg of the form
	// "/ServiceName/MethodName"
	service, method := splitMethodName(fullMethod)
	// Slice the inFlightRPC inc/dec calls by method and service
	labels := prometheus.Labels{
		"method":  method,
		"service": service,
	}
	// Increment the inFlightRPCs gauge for this method/service
	cmi.metrics.inFlightRPCs.With(labels).Inc()
	begin := cmi.clk.Now()

	// Cancel the local context and decrement the metric when we're done. Also
	// transform the error into a more usable form, if necessary.
	finish := func(err error) error {
		cancel()
		cmi.metrics.inFlightRPCs.With(labels).Dec()
		if err != nil {
			err = unwrapError(err, respMD)
			if status.Code(err) == codes.DeadlineExceeded {
				return deadlineDetails{
					service: service,
					method:  method,
					latency: cmi.clk.Since(begin),
				}
			}
		}
		return err
	}

	// Handle the RPC
	cs, err := streamer(localCtx, desc, cc, fullMethod, opts...)
	ics := interceptedClientStream{cs, finish}
	return ics, err
}

var _ clientInterceptor = (*clientMetadataInterceptor)(nil)

// deadlineDetails is an error type that we use in place of gRPC's
// DeadlineExceeded errors in order to add more detail for debugging.
type deadlineDetails struct {
	service string
	method  string
	latency time.Duration
}

func (dd deadlineDetails) Error() string {
	return fmt.Sprintf("%s.%s timed out after %d ms",
		dd.service, dd.method, int64(dd.latency/time.Millisecond))
}

// authInterceptor provides two server interceptors (Unary and Stream) which can
// check that every request for a given gRPC service is being made over an mTLS
// connection from a client which is allow-listed for that particular service.
type authInterceptor struct {
	// serviceClientNames is a map of gRPC service names (e.g. "ca.CertificateAuthority")
	// to allowed client certificate SANs (e.g. "ra.boulder") which are allowed to
	// make RPCs to that service. The set of client names is implemented as a map
	// of names to empty structs for easy lookup.
	serviceClientNames map[string]map[string]struct{}
}

// newServiceAuthChecker takes a GRPCServerConfig and uses its Service stanzas
// to construct a serviceAuthChecker which enforces the service/client mappings
// contained in the config.
func newServiceAuthChecker(c *cmd.GRPCServerConfig) *authInterceptor {
	names := make(map[string]map[string]struct{})
	for serviceName, service := range c.Services {
		names[serviceName] = make(map[string]struct{})
		for _, clientName := range service.ClientNames {
			names[serviceName][clientName] = struct{}{}
		}
	}
	return &authInterceptor{names}
}

// Unary is a gRPC unary interceptor.
func (ac *authInterceptor) Unary(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	err := ac.checkContextAuth(ctx, info.FullMethod)
	if err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

// Stream is a gRPC stream interceptor.
func (ac *authInterceptor) Stream(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	err := ac.checkContextAuth(ss.Context(), info.FullMethod)
	if err != nil {
		return err
	}
	return handler(srv, ss)
}

// checkContextAuth does most of the heavy lifting. It extracts TLS information
// from the incoming context, gets the set of DNS names contained in the client
// mTLS cert, and returns nil if at least one of those names appears in the set
// of allowed client names for given service (or if the set of allowed client
// names is empty).
func (ac *authInterceptor) checkContextAuth(ctx context.Context, fullMethod string) error {
	serviceName, _ := splitMethodName(fullMethod)

	allowedClientNames, ok := ac.serviceClientNames[serviceName]
	if !ok || len(allowedClientNames) == 0 {
		return fmt.Errorf("service %q has no allowed client names", serviceName)
	}

	p, ok := peer.FromContext(ctx)
	if !ok {
		return fmt.Errorf("unable to fetch peer info from grpc context")
	}

	if p.AuthInfo == nil {
		return fmt.Errorf("grpc connection appears to be plaintext")
	}

	tlsAuth, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return fmt.Errorf("connection is not TLS authed")
	}

	if len(tlsAuth.State.VerifiedChains) == 0 || len(tlsAuth.State.VerifiedChains[0]) == 0 {
		return fmt.Errorf("connection auth not verified")
	}

	cert := tlsAuth.State.VerifiedChains[0][0]

	for _, clientName := range cert.DNSNames {
		_, ok := allowedClientNames[clientName]
		if ok {
			return nil
		}
	}

	return fmt.Errorf(
		"client names %v are not authorized for service %q (%v)",
		cert.DNSNames, serviceName, allowedClientNames)
}

// Ensure authInterceptor matches the serverInterceptor interface.
var _ serverInterceptor = (*authInterceptor)(nil)
