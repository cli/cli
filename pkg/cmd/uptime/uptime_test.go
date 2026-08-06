package uptime

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixture builds a githubstatus summary.json body for the given overall
// indicator/description and component name->status pairs.
func fixture(indicator, description string, comps [][2]string) string {
	body := map[string]interface{}{
		"status": map[string]string{"indicator": indicator, "description": description},
	}
	list := make([]map[string]interface{}, 0, len(comps))
	for _, c := range comps {
		list = append(list, map[string]interface{}{"name": c[0], "status": c[1], "group": false})
	}
	body["components"] = list
	b, _ := json.Marshal(body)
	return string(b)
}

func TestNewCmdUptime(t *testing.T) {
	tests := []struct {
		name    string
		cli     string
		wantErr string
		want    UptimeOptions
	}{
		{
			name: "no args",
			cli:  "",
			want: UptimeOptions{Interval: 30 * time.Second},
		},
		{
			name: "component + json",
			cli:  "--component Actions --json status",
			want: UptimeOptions{Component: "Actions", Interval: 30 * time.Second},
		},
		{
			name: "watch with interval",
			cli:  "--watch --interval 45s",
			want: UptimeOptions{Watch: true, Interval: 45 * time.Second},
		},
		{
			name:    "interval too small",
			cli:     "--watch --interval 1s",
			wantErr: "--interval must be at least 5s to respect the status page",
		},
		{
			name:    "rejects positional args",
			cli:     "Actions",
			wantErr: `unknown command "Actions" for "uptime"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios}

			var gotOpts *UptimeOptions
			cmd := NewCmdUptime(f, func(o *UptimeOptions) error {
				gotOpts = o
				return nil
			})

			argv, err := shlex.Split(tt.cli)
			require.NoError(t, err)
			cmd.SetArgs(argv)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want.Component, gotOpts.Component)
			assert.Equal(t, tt.want.Watch, gotOpts.Watch)
			assert.Equal(t, tt.want.Interval, gotOpts.Interval)
		})
	}
}

func runWith(t *testing.T, opts *UptimeOptions, body string) (stdout string, err error) {
	t.Helper()
	reg := &httpmock.Registry{}
	reg.Register(
		httpmock.REST(http.MethodGet, "api/v2/summary.json"),
		httpmock.StringResponse(body),
	)
	ios, _, stdoutBuf, _ := iostreams.Test()
	opts.IO = ios
	opts.HTTPClient = func() *http.Client {
		return &http.Client{Transport: reg}
	}
	err = uptimeRun(opts)
	return stdoutBuf.String(), err
}

func TestUptimeRun_AllOperational(t *testing.T) {
	body := fixture("none", "All Systems Operational", [][2]string{
		{"Git Operations", "operational"},
		{"Actions", "operational"},
	})
	out, err := runWith(t, &UptimeOptions{Interval: 30 * time.Second}, body)
	require.NoError(t, err)
	assert.Contains(t, out, "All Systems Operational")
	assert.Contains(t, out, "Git Operations")
	assert.Contains(t, out, "Actions")
}

func TestUptimeRun_ExitCodeWhenDegraded(t *testing.T) {
	body := fixture("major", "Partial System Outage", [][2]string{
		{"Actions", "major_outage"},
		{"Git Operations", "operational"},
	})
	// Without --exit-code: reports but returns nil.
	out, err := runWith(t, &UptimeOptions{Interval: 30 * time.Second}, body)
	require.NoError(t, err)
	assert.Contains(t, out, "Partial System Outage")

	// With --exit-code: SilentError (exit 1) because the platform is degraded.
	_, err = runWith(t, &UptimeOptions{Interval: 30 * time.Second, ExitCode: true}, body)
	assert.ErrorIs(t, err, cmdutil.SilentError)
}

func TestUptimeRun_ComponentExitCode(t *testing.T) {
	body := fixture("major", "Partial System Outage", [][2]string{
		{"Actions", "major_outage"},
		{"Git Operations", "operational"},
	})
	// A degraded component we don't care about must not trip --exit-code when we
	// scope to a healthy one.
	_, err := runWith(t, &UptimeOptions{Component: "Git Operations", Interval: 30 * time.Second, ExitCode: true}, body)
	require.NoError(t, err, "scoped to an operational component, exit code should be clean")

	// Scoped to the outaged component, --exit-code must fail.
	_, err = runWith(t, &UptimeOptions{Component: "Actions", Interval: 30 * time.Second, ExitCode: true}, body)
	assert.ErrorIs(t, err, cmdutil.SilentError)
}

func TestUptimeRun_UnknownComponent(t *testing.T) {
	body := fixture("none", "All Systems Operational", [][2]string{
		{"Actions", "operational"},
	})
	_, err := runWith(t, &UptimeOptions{Component: "Nope", Interval: 30 * time.Second}, body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such GitHub component")
}

func TestUptimeRun_JSONExport(t *testing.T) {
	body := fixture("major", "Partial System Outage", [][2]string{
		{"Actions", "major_outage"},
		{"Git Operations", "operational"},
	})
	exporter := cmdutil.NewJSONExporter()
	exporter.SetFields([]string{"operational", "status", "component"})
	out, err := runWith(t, &UptimeOptions{Component: "Actions", Interval: 30 * time.Second, Exporter: exporter}, body)
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, false, got["operational"])
	assert.Equal(t, "major_outage", got["status"])
	assert.Equal(t, "Actions", got["component"])
}

func TestLeafComponents_DropsNonServiceRows(t *testing.T) {
	s := &summary{}
	s.Components = []component{
		{Name: "Actions", Status: "operational"},
		{Name: "Visit www.githubstatus.com for more information", Status: "operational"},
		{Name: "Some Group", Status: "operational", Group: true},
	}
	got := leafComponents(s)
	require.Len(t, got, 1)
	assert.Equal(t, "Actions", got[0].Name)
}

func TestTargetOperational(t *testing.T) {
	s := &summary{Components: []component{
		{Name: "Actions", Status: "major_outage"},
		{Name: "Git Operations", Status: "operational"},
	}}
	s.Status.Indicator = "major"

	assert.False(t, targetOperational(s, ""), "platform indicator major => not operational")
	assert.False(t, targetOperational(s, "Actions"))
	assert.True(t, targetOperational(s, "git operations"), "component match is case-insensitive")
	assert.False(t, targetOperational(s, "Missing"), "unknown component is treated as not operational")
}

// ensure the fields list advertised to --json stays in sync with what we emit.
func TestUptimeFieldsCoverExport(t *testing.T) {
	shape := exportShape(&summary{}, "Actions")
	for _, f := range []string{"operational", "status", "component", "indicator", "description", "components"} {
		_, ok := shape[f]
		assert.True(t, ok, "export shape missing advertised field %q", f)
		assert.Contains(t, strings.Join(uptimeFields, ","), f)
	}
}
