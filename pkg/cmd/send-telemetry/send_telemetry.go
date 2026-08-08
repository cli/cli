package sendtelemetry

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/cli/cli/v2/internal/barista/observability"
	"github.com/cli/cli/v2/internal/build"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

const defaultTelemetryEndpointURL = "https://cafe.github.com"

type SendTelemetryOptions struct {
	TelemetryEndpointURL string
	PayloadJSON          string
	HTTPUnixSocket       string
}

func NewCmdSendTelemetry(f *cmdutil.Factory) *cobra.Command {
	return newCmdSendTelemetry(f, nil)
}

func newCmdSendTelemetry(f *cmdutil.Factory, runF func(*SendTelemetryOptions) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "send-telemetry",
		Short:  "Send telemetry event to GitHub",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := f.Config()
			if err != nil {
				return err
			}

			payloadJSON, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("reading payload from stdin: %w", err)
			}
			if len(payloadJSON) == 0 {
				return fmt.Errorf("no payload provided on stdin")
			}

			opts := &SendTelemetryOptions{
				TelemetryEndpointURL: cmp.Or(os.Getenv("GH_TELEMETRY_ENDPOINT_URL"), defaultTelemetryEndpointURL),
				PayloadJSON:          string(payloadJSON),
				// This is a best effort to use a Unix Socket if configured. In most cases, if there is one configured
				// it will be at the global level. However, since the telemetry service is not related to a specific host, we can't
				// know that the socket we choose will work.
				HTTPUnixSocket: cfg.HTTPUnixSocket("").Value,
			}

			if runF != nil {
				return runF(opts)
			}

			return runSendTelemetry(cmd.Context(), opts)
		},
	}

	cmdutil.DisableAuthCheck(cmd)
	cmdutil.DisableTelemetry(cmd)

	return cmd
}

func runSendTelemetry(ctx context.Context, opts *SendTelemetryOptions) error {
	httpClient := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &userAgentTransport{
			base:      handleUnixDomainSocket(opts.HTTPUnixSocket),
			userAgent: fmt.Sprintf("GitHub CLI %s", build.Version),
		},
	}

	client := observability.NewTelemetryAPIProtobufClient(opts.TelemetryEndpointURL, httpClient)

	var payload telemetry.SendTelemetryPayload
	if err := json.Unmarshal([]byte(opts.PayloadJSON), &payload); err != nil {
		return fmt.Errorf("parsing payload JSON: %w", err)
	}

	if len(payload.Events) == 0 {
		return nil
	}

	events := make([]*observability.TelemetryEvent, len(payload.Events))
	for i, event := range payload.Events {
		events[i] = &observability.TelemetryEvent{
			App:        "github-cli",
			EventType:  event.Type,
			Dimensions: event.Dimensions,
			Measures:   event.Measures,
		}
	}

	_, err := client.RecordEvents(ctx, &observability.RecordEventsRequest{
		Events: events,
	})
	return err
}

type userAgentTransport struct {
	base      http.RoundTripper
	userAgent string
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", t.userAgent)
	return t.base.RoundTrip(req)
}

func handleUnixDomainSocket(socketPath string) http.RoundTripper {
	if socketPath == "" {
		return http.DefaultTransport
	}

	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
	}

	return &http.Transport{
		DialContext:       dialContext,
		DisableKeepAlives: true,
	}
}
