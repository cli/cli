package add

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func Test_addRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       AddOptions
		isTTY      bool
		stdin      string
		httpStubs  func(t *testing.T, reg *httpmock.Registry)
		wantStdout string
		wantStderr string
		wantErr    bool
	}{
		{
			name:  "add from stdin",
			isTTY: true,
			opts: AddOptions{
				KeyFile:    "-",
				Title:      "my sacred key",
				AllowWrite: false,
			},
			stdin: "PUBKEY\n",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/keys"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						if title := payload["title"].(string); title != "my sacred key" {
							t.Errorf("POST title %q, want %q", title, "my sacred key")
						}
						if key := payload["key"].(string); key != "PUBKEY\n" {
							t.Errorf("POST key %q, want %q", key, "PUBKEY\n")
						}
						if isReadOnly := payload["read_only"].(bool); !isReadOnly {
							t.Errorf("POST read_only %v, want %v", isReadOnly, true)
						}
					}))
			},
			wantStdout: "✓ Deploy key added to OWNER/REPO\n",
			wantStderr: "",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, stdin, stdout, stderr := iostreams.Test()
			stdin.WriteString(tt.stdin)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}

			opts := tt.opts
			opts.IO = ios
			opts.BaseRepo = func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil }
			opts.HTTPClient = func() (*http.Client, error) { return &http.Client{Transport: reg}, nil }

			err := addRun(&opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("addRun() return error: %v", err)
				return
			}

			if stdout.String() != tt.wantStdout {
				t.Errorf("wants stdout %q, got %q", tt.wantStdout, stdout.String())
			}
			if stderr.String() != tt.wantStderr {
				t.Errorf("wants stderr %q, got %q", tt.wantStderr, stderr.String())
			}
		})
	}
}
