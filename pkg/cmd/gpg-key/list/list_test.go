package list

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
)

func TestListRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       ListOptions
		isTTY      bool
		wantStdout string
		wantStderr string
		wantErr    bool
	}{
		{
			name: "list tty",
			opts: ListOptions{
				HTTPClient: func() (*http.Client, error) {
					expiresAt, _ := time.Parse(time.RFC3339, "2021-06-11T15:44:24+01:00")
					reg := &httpmock.Registry{}
					reg.Register(
						httpmock.REST("GET", "user/gpg_keys"),
						httpmock.StringResponse(fmt.Sprintf(`[
							{
								"id": 1234,
								"key_id": "ABCDEF1234567890",
								"public_key": "xJMEWfoofoofoo",
								"expires_at": "%[1]s"
							},
							{
								"id": 5678,
								"key_id": "1234567890ABCDEF",
								"public_key": "xJMEWbarbarbar",
								"expires_at": "%[1]s"
							}
						]`, expiresAt.Format(time.RFC3339))),
					)
					return &http.Client{Transport: reg}, nil
				},
			},
			isTTY: true,
			wantStdout: heredoc.Doc(`
				ABCDEF1234567890  2021-06-11T15:44:24+01:00  xJMEWfoofoofoo
				1234567890ABCDEF  2021-06-11T15:44:24+01:00  xJMEWbarbarbar
			`),
			wantStderr: "",
		},
		{
			name: "list non-tty",
			opts: ListOptions{
				HTTPClient: func() (*http.Client, error) {
					expiresAt, _ := time.Parse(time.RFC3339, "2021-06-11T15:44:24+01:00")
					reg := &httpmock.Registry{}
					reg.Register(
						httpmock.REST("GET", "user/gpg_keys"),
						httpmock.StringResponse(fmt.Sprintf(`[
							{
								"id": 1234,
								"key_id": "ABCDEF1234567890",
								"public_key": "xJMEWfoofoofoo",
								"expires_at": "%[1]s"
							},
							{
								"id": 5678,
								"key_id": "1234567890ABCDEF",
								"public_key": "xJMEWbarbarbar",
								"expires_at": "%[1]s"
							}
						]`, expiresAt.Format(time.RFC3339))),
					)
					return &http.Client{Transport: reg}, nil
				},
			},
			isTTY: false,
			wantStdout: heredoc.Doc(`
				ABCDEF1234567890	2021-06-11T15:44:24+01:00	xJMEWfoofoofoo
				1234567890ABCDEF	2021-06-11T15:44:24+01:00	xJMEWbarbarbar
			`),
			wantStderr: "",
		},
		{
			name: "no keys",
			opts: ListOptions{
				HTTPClient: func() (*http.Client, error) {
					reg := &httpmock.Registry{}
					reg.Register(
						httpmock.REST("GET", "user/gpg_keys"),
						httpmock.StringResponse(`[]`),
					)
					return &http.Client{Transport: reg}, nil
				},
			},
			wantStdout: "",
			wantStderr: "No GPG keys present in GitHub account.\n",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, stdout, stderr := iostreams.Test()
			io.SetStdoutTTY(tt.isTTY)
			io.SetStdinTTY(tt.isTTY)
			io.SetStderrTTY(tt.isTTY)

			opts := tt.opts
			opts.IO = io
			opts.Config = func() (config.Config, error) { return config.NewBlankConfig(), nil }

			err := listRun(&opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("linRun() return error: %v", err)
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
