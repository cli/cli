package grpc

import (
	"crypto/tls"
	"errors"
	"fmt"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/jmhodges/clock"
	"github.com/letsencrypt/boulder/cmd"
	bcreds "github.com/letsencrypt/boulder/grpc/creds"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"

	// 'grpc/health' is imported for its init function, which causes clients to
	// rely on the Health Service for load-balancing.
	// 'grpc/internal/resolver/dns' is imported for its init function, which
	// registers the SRV resolver.
	_ "github.com/letsencrypt/boulder/grpc/internal/resolver/dns"
	"google.golang.org/grpc/balancer/roundrobin"
	_ "google.golang.org/grpc/health"
)

// ClientSetup creates a gRPC TransportCredentials that presents
// a client certificate and validates the server certificate based
// on the provided *tls.Config.
// It dials the remote service and returns a grpc.ClientConn if successful.
func ClientSetup(c *cmd.GRPCClientConfig, tlsConfig *tls.Config, statsRegistry prometheus.Registerer, clk clock.Clock) (*grpc.ClientConn, error) {
	if c == nil {
		return nil, errors.New("nil gRPC client config provided: JSON config is probably missing a fooService section")
	}
	if tlsConfig == nil {
		return nil, errNilTLS
	}

	metrics, err := newClientMetrics(statsRegistry)
	if err != nil {
		return nil, err
	}

	cmi := clientMetadataInterceptor{c.Timeout.Duration, metrics, clk, !c.NoWaitForReady}

	unaryInterceptors := []grpc.UnaryClientInterceptor{
		cmi.Unary,
		cmi.metrics.grpcMetrics.UnaryClientInterceptor(),
		otelgrpc.UnaryClientInterceptor(),
	}

	streamInterceptors := []grpc.StreamClientInterceptor{
		cmi.Stream,
		cmi.metrics.grpcMetrics.StreamClientInterceptor(),
		otelgrpc.StreamClientInterceptor(),
	}

	target, hostOverride, err := c.MakeTargetAndHostOverride()
	if err != nil {
		return nil, err
	}

	creds := bcreds.NewClientCredentials(tlsConfig.RootCAs, tlsConfig.Certificates, hostOverride)
	return grpc.Dial(
		target,
		grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"loadBalancingConfig": [{"%s":{}}]}`, roundrobin.Name)),
		grpc.WithTransportCredentials(creds),
		grpc.WithChainUnaryInterceptor(unaryInterceptors...),
		grpc.WithChainStreamInterceptor(streamInterceptors...),
	)
}

// clientMetrics is a struct type used to return registered metrics from
// `NewClientMetrics`
type clientMetrics struct {
	grpcMetrics *grpc_prometheus.ClientMetrics
	// inFlightRPCs is a labelled gauge that slices by service/method the number
	// of outstanding/in-flight RPCs.
	inFlightRPCs *prometheus.GaugeVec
}

// newClientMetrics constructs a *grpc_prometheus.ClientMetrics, registered with
// the given registry, with timing histogram enabled. It must be called a
// maximum of once per registry, or there will be conflicting names.
func newClientMetrics(stats prometheus.Registerer) (clientMetrics, error) {
	// Create the grpc prometheus client metrics instance and register it
	grpcMetrics := grpc_prometheus.NewClientMetrics()
	grpcMetrics.EnableClientHandlingTimeHistogram()
	err := stats.Register(grpcMetrics)
	if err != nil {
		are := prometheus.AlreadyRegisteredError{}
		if errors.As(err, &are) {
			grpcMetrics = are.ExistingCollector.(*grpc_prometheus.ClientMetrics)
		} else {
			return clientMetrics{}, err
		}
	}

	// Create a gauge to track in-flight RPCs and register it.
	inFlightGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "grpc_in_flight",
		Help: "Number of in-flight (sent, not yet completed) RPCs",
	}, []string{"method", "service"})
	err = stats.Register(inFlightGauge)
	if err != nil {
		are := prometheus.AlreadyRegisteredError{}
		if errors.As(err, &are) {
			inFlightGauge = are.ExistingCollector.(*prometheus.GaugeVec)
		} else {
			return clientMetrics{}, err
		}
	}

	return clientMetrics{
		grpcMetrics:  grpcMetrics,
		inFlightRPCs: inFlightGauge,
	}, nil
}
