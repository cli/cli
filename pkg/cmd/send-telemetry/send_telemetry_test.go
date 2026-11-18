package sendtelemetry

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/barista/observability"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTelemetryAPI struct {
	request *observability.RecordEventsRequest
	err     error
}

func (m *mockTelemetryAPI) RecordEvents(_ context.Context, req *observability.RecordEventsRequest) (*observability.RecordEventsResponse, error) {
	m.request = req
	return &observability.RecordEventsResponse{}, m.err
}

func TestNewCmdSendTelemetry(t *testing.T) {
	tests := []struct {
		name     string
		stdin    string
		env      map[string]string
		wantOpts SendTelemetryOptions
		wantErr  string
	}{
		{
			name:  "reads payload from stdin",
			stdin: `{"events":[{"type":"usage","dimensions":{"command":"gh pr list"}}]}`,
			wantOpts: SendTelemetryOptions{
				TelemetryEndpointURL: defaultTelemetryEndpointURL,
				PayloadJSON:          `{"events":[{"type":"usage","dimensions":{"command":"gh pr list"}}]}`,
			},
		},
		{
			name:  "uses GH_TELEMETRY_ENDPOINT_URL env var",
			stdin: `{"events":[]}`,
			env:   map[string]string{"GH_TELEMETRY_ENDPOINT_URL": "https://custom.endpoint"},
			wantOpts: SendTelemetryOptions{
				TelemetryEndpointURL: "https://custom.endpoint",
				PayloadJSON:          `{"events":[]}`,
			},
		},
		{
			name:  "defaults endpoint when env var not set",
			stdin: `{}`,
			wantOpts: SendTelemetryOptions{
				TelemetryEndpointURL: defaultTelemetryEndpointURL,
				PayloadJSON:          `{}`,
			},
		},
		{
			name:    "errors on empty stdin",
			stdin:   "",
			wantErr: "no payload provided on stdin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
				Config: func() (gh.Config, error) {
					return config.NewMockConfig(), nil
				},
			}

			var gotOpts *SendTelemetryOptions
			cmd := newCmdSendTelemetry(f, func(opts *SendTelemetryOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs([]string{})
			cmd.SetIn(strings.NewReader(tt.stdin))
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err := cmd.ExecuteC()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, gotOpts)
			assert.Equal(t, tt.wantOpts.TelemetryEndpointURL, gotOpts.TelemetryEndpointURL)
			assert.Equal(t, tt.wantOpts.PayloadJSON, gotOpts.PayloadJSON)
		})
	}
}

func TestRunSendTelemetry(t *testing.T) {
	tests := []struct {
		name       string
		payload    telemetry.SendTelemetryPayload
		unixSocket bool
		https      bool
		serverErr  error
		wantErr    bool
		assertFunc func(t *testing.T, req *observability.RecordEventsRequest)
	}{
		{
			name: "posts single event to endpoint",
			payload: telemetry.SendTelemetryPayload{
				Events: []telemetry.PayloadEvent{
					{
						Type: "command_invocation",
						Dimensions: map[string]string{
							"command":   "gh pr create",
							"device_id": "abc123",
							"os":        "darwin",
						},
						Measures: map[string]int64{"duration_ms": 150},
					},
				},
			},
			assertFunc: func(t *testing.T, req *observability.RecordEventsRequest) {
				t.Helper()
				require.Len(t, req.Events, 1)
				event := req.Events[0]
				assert.Equal(t, "github-cli", event.App)
				assert.Equal(t, "command_invocation", event.EventType)
				assert.Equal(t, "gh pr create", event.Dimensions["command"])
				assert.Equal(t, "abc123", event.Dimensions["device_id"])
				assert.Equal(t, "darwin", event.Dimensions["os"])
			},
		},
		{
			name: "posts multiple events in single batch request",
			payload: telemetry.SendTelemetryPayload{
				Events: []telemetry.PayloadEvent{
					{Type: "event1", Dimensions: map[string]string{"a": "1"}},
					{Type: "event2", Dimensions: map[string]string{"b": "2"}},
				},
			},
			assertFunc: func(t *testing.T, req *observability.RecordEventsRequest) {
				t.Helper()
				require.Len(t, req.Events, 2)
				assert.Equal(t, "1", req.Events[0].Dimensions["a"])
				assert.Equal(t, "2", req.Events[1].Dimensions["b"])
				assert.Equal(t, "github-cli", req.Events[0].App)
				assert.Equal(t, "event1", req.Events[0].EventType)
				assert.Equal(t, "github-cli", req.Events[1].App)
				assert.Equal(t, "event2", req.Events[1].EventType)
			},
		},
		{
			name:       "posts over a Unix socket",
			unixSocket: true,
			payload: telemetry.SendTelemetryPayload{
				Events: []telemetry.PayloadEvent{{Type: "test"}},
			},
		},
		{
			name:       "HTTPS over a Unix socket retains TLS verification and CA policy",
			unixSocket: true,
			https:      true,
			wantErr:    true,
			payload: telemetry.SendTelemetryPayload{
				Events: []telemetry.PayloadEvent{{Type: "test"}},
			},
		},
		{
			name: "empty events list produces no request",
			payload: telemetry.SendTelemetryPayload{
				Events: []telemetry.PayloadEvent{},
			},
			assertFunc: func(t *testing.T, req *observability.RecordEventsRequest) {
				t.Helper()
				assert.Nil(t, req)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockTelemetryAPI{err: tt.serverErr}
			handler := observability.NewTelemetryAPIServer(mock)
			server := httptest.NewUnstartedServer(handler)
			defer server.Close()
			var socketPath string
			if tt.unixSocket {
				if runtime.GOOS == "windows" {
					t.Skip("Unix-socket fixture requires a Unix platform")
				}
				dir, err := os.MkdirTemp("", "gh-telemetry-")
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, os.Remove(dir)) })
				socketPath = filepath.Join(dir, "socket")
				listener, err := net.Listen("unix", socketPath)
				require.NoError(t, err)
				require.NoError(t, server.Listener.Close())
				server.Listener = listener
			}
			if tt.https {
				server.StartTLS()
				rt, err := handleUnixDomainSocket(socketPath)
				require.NoError(t, err)
				transport, ok := rt.(*http.Transport)
				require.True(t, ok)
				require.NotNil(t, transport.TLSClientConfig)
				assert.Nil(t, transport.TLSClientConfig.RootCAs)
				require.NotNil(t, transport.TLSClientConfig.VerifyConnection)
				// emSign ECC Root CA - G3: https://repository.emsign.com/certs/emSignECCRootCAG3.crt
				spki, err := hex.DecodeString("3076301006072a8648ce3d020106052b810400220362000423a50cb82d12f528f3b1b2dde20212809e395f494d9fc925345974ecbb061ce7c072afe8ae2fe141548714a84ab2e87c82e65b6ab5dcb375ce8b06d08623bf46d58e0f3f04f4d71c927ef6a563c2f55f8e2e4fa11819022b320a82647d1693d1")
				require.NoError(t, err)
				require.ErrorContains(t, transport.TLSClientConfig.VerifyConnection(tls.ConnectionState{
					VerifiedChains: [][]*x509.Certificate{{{RawSubjectPublicKeyInfo: spki}}},
				}), "distrusted CA")
			} else {
				server.Start()
			}

			opts := &SendTelemetryOptions{
				TelemetryEndpointURL: server.URL,
				PayloadJSON:          mustMarshal(t, tt.payload),
				HTTPUnixSocket:       socketPath,
			}
			if tt.unixSocket {
				opts.TelemetryEndpointURL = "http://telemetry.invalid"
				if tt.https {
					opts.TelemetryEndpointURL = "https://telemetry.invalid"
				}
			}

			err := runSendTelemetry(context.Background(), opts)
			if tt.wantErr {
				require.Error(t, err)
				if tt.https {
					require.ErrorContains(t, err, "x509:")
				}
				return
			}
			require.NoError(t, err)
			if tt.unixSocket {
				require.NotNil(t, mock.request)
			}

			if tt.assertFunc != nil {
				tt.assertFunc(t, mock.request)
			}
		})
	}
}

func TestRunSendTelemetryInvalidPayload(t *testing.T) {
	err := runSendTelemetry(context.Background(), &SendTelemetryOptions{
		TelemetryEndpointURL: "http://localhost:0",
		PayloadJSON:          "not-json",
	})
	require.Error(t, err)
}

func TestRunSendTelemetryServerError(t *testing.T) {
	mock := &mockTelemetryAPI{err: assert.AnError}
	handler := observability.NewTelemetryAPIServer(mock)
	server := httptest.NewServer(handler)
	defer server.Close()

	err := runSendTelemetry(context.Background(), &SendTelemetryOptions{
		TelemetryEndpointURL: server.URL,
		PayloadJSON:          `{"events":[{"type":"test","dimensions":{"a":"1"}}]}`,
	})
	require.Error(t, err)
}

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return string(data)
}
