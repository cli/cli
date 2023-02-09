package list

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func Test_listRun(t *testing.T) {
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
			opts: ListOptions{HTTPClient: func() (*http.Client, error) {
				createdAt := time.Now().Add(time.Duration(-24) * time.Hour)
				expiresAt, _ := time.Parse(time.RFC3339, "2099-01-01T15:44:24+01:00")
				noExpires := time.Time{}
				reg := &httpmock.Registry{}
				reg.Register(
					httpmock.REST("GET", "user/gpg_keys"),
					httpmock.StringResponse(fmt.Sprintf(`[
						{
							"id": 1234,
							"key_id": "ABCDEF1234567890",
							"public_key": "xJMEWfoofoofoo",
							"emails": [{"email": "johnny@test.com"}],
							"created_at": "%[1]s",
							"expires_at": "%[2]s"
						},
						{
							"id": 5678,
							"key_id": "1234567890ABCDEF",
							"public_key": "xJMEWbarbarbar",
							"emails": [{"email": "monalisa@github.com"}],
							"created_at": "%[1]s",
							"expires_at": "%[3]s"
						}
					]`, createdAt.Format(time.RFC3339),
						expiresAt.Format(time.RFC3339),
						noExpires.Format(time.RFC3339))),
				)
				return &http.Client{Transport: reg}, nil
			}},
			isTTY: true,
			wantStdout: heredoc.Doc(`
				EMAIL                KEY ID            PUBLIC KEY      ADDED  EXPIRES
				johnny@test.com      ABCDEF1234567890  xJMEWfoofoofoo  1d     2099-01-01
				monalisa@github.com  1234567890ABCDEF  xJMEWbarbarbar  1d     Never
			`),
			wantStderr: "",
		},
		{
			name: "list non-tty",
			opts: ListOptions{HTTPClient: func() (*http.Client, error) {
				createdAt1, _ := time.Parse(time.RFC3339, "2020-06-11T15:44:24+01:00")
				expiresAt, _ := time.Parse(time.RFC3339, "2099-01-01T15:44:24+01:00")
				createdAt2, _ := time.Parse(time.RFC3339, "2021-01-11T15:44:24+01:00")
				noExpires := time.Time{}
				reg := &httpmock.Registry{}
				reg.Register(
					httpmock.REST("GET", "user/gpg_keys"),
					httpmock.StringResponse(fmt.Sprintf(`[
						{
							"id": 1234,
							"key_id": "ABCDEF1234567890",
							"public_key": "xJMEWfoofoofoo",
							"emails": [{"email": "johnny@test.com"}],
							"created_at": "%[1]s",
							"expires_at": "%[2]s"
						},
						{
							"id": 5678,
							"key_id": "1234567890ABCDEF",
							"public_key": "xJMEWbarbarbar",
							"emails": [{"email": "monalisa@github.com"}],
							"created_at": "%[3]s",
							"expires_at": "%[4]s"
						}
					]`, createdAt1.Format(time.RFC3339),
						expiresAt.Format(time.RFC3339),
						createdAt2.Format(time.RFC3339),
						noExpires.Format(time.RFC3339))),
				)
				return &http.Client{Transport: reg}, nil
			}},
			isTTY: false,
			wantStdout: heredoc.Doc(`
				johnny@test.com	ABCDEF1234567890	xJMEWfoofoofoo	2020-06-11T15:44:24+01:00	2099-01-01T15:44:24+01:00
				monalisa@github.com	1234567890ABCDEF	xJMEWbarbarbar	2021-01-11T15:44:24+01:00	0001-01-01T00:00:00Z
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
			wantStderr: "",
			wantErr:    true,
			isTTY:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)
			opts := tt.opts
			opts.IO = ios
			opts.Config = func() (config.Config, error) { return config.NewBlankConfig(), nil }
			err := listRun(&opts)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
