package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc/filters"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"

	"github.com/letsencrypt/boulder/cmd"
	bcreds "github.com/letsencrypt/boulder/grpc/creds"
	blog "github.com/letsencrypt/boulder/log"
)

// CodedError is a alias required to appease go vet
var CodedError = status.Errorf

var errNilTLS = errors.New("boulder/grpc: received nil tls.Config")

// checker is an interface for checking the health of a grpc service
// implementation.
type checker interface {
	// Health returns nil if the service is healthy, or an error if it is not.
	// If the passed context is canceled, it should return immediately with an
	// error.
	Health(context.Context) error
}

// service represents a single gRPC service that can be registered with a gRPC
// server.
type service struct {
	desc *grpc.ServiceDesc
	impl any
}

// serverBuilder implements a builder pattern for constructing new gRPC servers
// and registering gRPC services on those servers.
type serverBuilder struct {
	cfg           *cmd.GRPCServerConfig
	services      map[string]service
	healthSrv     *health.Server
	checkInterval time.Duration
	logger        blog.Logger
	err           error
}

// NewServer returns an object which can be used to build gRPC servers. It takes
// the server's configuration to perform initialization and a logger for deep
// health checks.
func NewServer(c *cmd.GRPCServerConfig, logger blog.Logger) *serverBuilder {
	return &serverBuilder{cfg: c, services: make(map[string]service), logger: logger}
}

// WithCheckInterval sets the interval at which the server will check the health
// of its registered services. If this is not called, a default interval of 5
// seconds will be used.
func (sb *serverBuilder) WithCheckInterval(i time.Duration) *serverBuilder {
	sb.checkInterval = i
	return sb
}

// Add registers a new service (consisting of its description and its
// implementation) to the set of services which will be exposed by this server.
// It returns the modified-in-place serverBuilder so that calls can be chained.
// If there is an error adding this service, it will be exposed when .Build() is
// called.
func (sb *serverBuilder) Add(desc *grpc.ServiceDesc, impl any) *serverBuilder {
	if _, found := sb.services[desc.ServiceName]; found {
		// We've already registered a service with this same name, error out.
		sb.err = fmt.Errorf("attempted double-registration of gRPC service %q", desc.ServiceName)
		return sb
	}
	sb.services[desc.ServiceName] = service{desc: desc, impl: impl}
	return sb
}

// Build creates a gRPC server that uses the provided *tls.Config and exposes
// all of the services added to the builder. It also exposes a health check
// service. It returns one functions, start(), which should be used to start
// the server. It spawns a goroutine which will listen for OS signals and
// gracefully stop the server if one is caught, causing the start() function to
// exit.
func (sb *serverBuilder) Build(tlsConfig *tls.Config, statsRegistry prometheus.Registerer, clk clock.Clock) (func() error, error) {
	// Register the health service with the server.
	sb.healthSrv = health.NewServer()
	sb.Add(&healthpb.Health_ServiceDesc, sb.healthSrv)

	// Check to see if any of the calls to .Add() resulted in an error.
	if sb.err != nil {
		return nil, sb.err
	}

	// Ensure that every configured service also got added.
	var registeredServices []string
	for r := range sb.services {
		registeredServices = append(registeredServices, r)
	}
	for serviceName := range sb.cfg.Services {
		_, ok := sb.services[serviceName]
		if !ok {
			return nil, fmt.Errorf("gRPC service %q in config does not match any service: %s", serviceName, strings.Join(registeredServices, ", "))
		}
	}

	if tlsConfig == nil {
		return nil, errNilTLS
	}

	// Collect all names which should be allowed to connect to the server at all.
	// This is the names which are allowlisted at the server level, plus the union
	// of all names which are allowlisted for any individual service.
	acceptedSANs := make(map[string]struct{})
	var acceptedSANsSlice []string
	for _, service := range sb.cfg.Services {
		for _, name := range service.ClientNames {
			acceptedSANs[name] = struct{}{}
			if !slices.Contains(acceptedSANsSlice, name) {
				acceptedSANsSlice = append(acceptedSANsSlice, name)
			}
		}
	}

	// Ensure that the health service has the same ClientNames as the other
	// services, so that health checks can be performed by clients which are
	// allowed to connect to the server.
	sb.cfg.Services[healthpb.Health_ServiceDesc.ServiceName].ClientNames = acceptedSANsSlice

	creds, err := bcreds.NewServerCredentials(tlsConfig, acceptedSANs)
	if err != nil {
		return nil, err
	}

	// Set up all of our interceptors which handle metrics, traces, error
	// propagation, and more.
	metrics, err := newServerMetrics(statsRegistry)
	if err != nil {
		return nil, err
	}

	var ai serverInterceptor
	if len(sb.cfg.Services) > 0 {
		ai = newServiceAuthChecker(sb.cfg)
	} else {
		ai = &noopServerInterceptor{}
	}

	mi := newServerMetadataInterceptor(metrics, clk)

	unaryInterceptors := []grpc.UnaryServerInterceptor{
		mi.metrics.grpcMetrics.UnaryServerInterceptor(),
		ai.Unary,
		mi.Unary,
	}

	streamInterceptors := []grpc.StreamServerInterceptor{
		mi.metrics.grpcMetrics.StreamServerInterceptor(),
		ai.Stream,
		mi.Stream,
	}

	options := []grpc.ServerOption{
		grpc.Creds(creds),
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
		grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithFilter(filters.Not(filters.HealthCheck())))),
	}
	if sb.cfg.MaxConnectionAge.Duration > 0 {
		options = append(options,
			grpc.KeepaliveParams(keepalive.ServerParameters{
				MaxConnectionAge: sb.cfg.MaxConnectionAge.Duration,
			}))
	}

	// Create the server itself and register all of our services on it.
	server := grpc.NewServer(options...)
	for _, service := range sb.services {
		server.RegisterService(service.desc, service.impl)
	}

	if sb.cfg.Address == "" {
		return nil, errors.New("GRPC listen address not configured")
	}
	sb.logger.Infof("grpc listening on %s", sb.cfg.Address)

	// Finally return the functions which will start and stop the server.
	listener, err := net.Listen("tcp", sb.cfg.Address)
	if err != nil {
		return nil, err
	}

	start := func() error {
		return server.Serve(listener)
	}

	// Initialize long-running health checks of all services which implement the
	// checker interface.
	if sb.checkInterval <= 0 {
		sb.checkInterval = 5 * time.Second
	}
	healthCtx, stopHealthChecks := context.WithCancel(context.Background())
	for _, s := range sb.services {
		check, ok := s.impl.(checker)
		if !ok {
			continue
		}
		sb.initLongRunningCheck(healthCtx, s.desc.ServiceName, check.Health)
	}

	// Start a goroutine which listens for a termination signal, and then
	// gracefully stops the gRPC server. This in turn causes the start() function
	// to exit, allowing its caller (generally a main() function) to exit.
	go cmd.CatchSignals(func() {
		stopHealthChecks()
		sb.healthSrv.Shutdown()
		server.GracefulStop()
	})

	return start, nil
}

// initLongRunningCheck initializes a goroutine which will periodically check
// the health of the provided service and update the health server accordingly.
//
// TODO(#8255): Remove the service parameter and instead rely on transitioning
// the overall health of the server (e.g. "") instead of individual services.
func (sb *serverBuilder) initLongRunningCheck(shutdownCtx context.Context, service string, checkImpl func(context.Context) error) {
	// Set the initial health status for the service.
	sb.healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	sb.healthSrv.SetServingStatus(service, healthpb.HealthCheckResponse_NOT_SERVING)

	// check is a helper function that checks the health of the service and, if
	// necessary, updates its status in the health server.
	checkAndMaybeUpdate := func(checkCtx context.Context, last healthpb.HealthCheckResponse_ServingStatus) healthpb.HealthCheckResponse_ServingStatus {
		// Make a context with a timeout at 90% of the interval.
		checkImplCtx, cancel := context.WithTimeout(checkCtx, sb.checkInterval*9/10)
		defer cancel()

		var next healthpb.HealthCheckResponse_ServingStatus
		err := checkImpl(checkImplCtx)
		if err != nil {
			next = healthpb.HealthCheckResponse_NOT_SERVING
		} else {
			next = healthpb.HealthCheckResponse_SERVING
		}

		if last == next {
			// No change in health status.
			return next
		}

		if next != healthpb.HealthCheckResponse_SERVING {
			sb.logger.Errf("transitioning overall health from %q to %q, due to: %s", last, next, err)
			sb.logger.Errf("transitioning health of %q from %q to %q, due to: %s", service, last, next, err)
		} else {
			sb.logger.Infof("transitioning overall health from %q to %q", last, next)
			sb.logger.Infof("transitioning health of %q from %q to %q", service, last, next)
		}
		sb.healthSrv.SetServingStatus("", next)
		sb.healthSrv.SetServingStatus(service, next)
		return next
	}

	go func() {
		ticker := time.NewTicker(sb.checkInterval)
		defer ticker.Stop()

		// Assume the service is not healthy to start.
		last := healthpb.HealthCheckResponse_NOT_SERVING

		// Check immediately, and then at the specified interval.
		last = checkAndMaybeUpdate(shutdownCtx, last)
		for {
			select {
			case <-shutdownCtx.Done():
				// The server is shutting down.
				return
			case <-ticker.C:
				last = checkAndMaybeUpdate(shutdownCtx, last)
			}
		}
	}()
}

// serverMetrics is a struct type used to return a few registered metrics from
// `newServerMetrics`
type serverMetrics struct {
	grpcMetrics *grpc_prometheus.ServerMetrics
	rpcLag      prometheus.Histogram
}

// newServerMetrics registers metrics with a registry. It constructs and
// registers a *grpc_prometheus.ServerMetrics with timing histogram enabled as
// well as a prometheus Histogram for RPC latency. If called more than once on a
// single registry, it will gracefully avoid registering duplicate metrics.
func newServerMetrics(stats prometheus.Registerer) (serverMetrics, error) {
	// Create the grpc prometheus server metrics instance and register it
	grpcMetrics := grpc_prometheus.NewServerMetrics(
		grpc_prometheus.WithServerHandlingTimeHistogram(
			grpc_prometheus.WithHistogramBuckets([]float64{.01, .025, .05, .1, .5, 1, 2.5, 5, 10, 45, 90}),
		),
	)
	err := stats.Register(grpcMetrics)
	if err != nil {
		are := prometheus.AlreadyRegisteredError{}
		if errors.As(err, &are) {
			grpcMetrics = are.ExistingCollector.(*grpc_prometheus.ServerMetrics)
		} else {
			return serverMetrics{}, err
		}
	}

	// rpcLag is a prometheus histogram tracking the difference between the time
	// the client sent an RPC and the time the server received it. Create and
	// register it.
	rpcLag := prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name: "grpc_lag",
			Help: "Delta between client RPC send time and server RPC receipt time",
		})
	err = stats.Register(rpcLag)
	if err != nil {
		are := prometheus.AlreadyRegisteredError{}
		if errors.As(err, &are) {
			rpcLag = are.ExistingCollector.(prometheus.Histogram)
		} else {
			return serverMetrics{}, err
		}
	}

	return serverMetrics{
		grpcMetrics: grpcMetrics,
		rpcLag:      rpcLag,
	}, nil
}
