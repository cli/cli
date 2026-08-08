package sendtelemetry

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
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
					return config.NewBlankConfig(), nil
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
			server := httptest.NewServer(handler)
			defer server.Close()

			opts := &SendTelemetryOptions{
				TelemetryEndpointURL: server.URL,
				PayloadJSON:          mustMarshal(t, tt.payload),
			}

			err := runSendTelemetry(context.Background(), opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

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
