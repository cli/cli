package list

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
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
			name:       "list tty",
			opts:       ListOptions{HTTPClient: mockGPGResponse},
			isTTY:      true,
			wantStdout: "johnny@test.com      ABCDEF12345...  xJMEW...oofoo  Created Ju...  Expires 20...\nmonalisa@github.com  1234567890A...  xJMEW...arbar  Created Ja...  Never expires\n",
			wantStderr: "",
		},
		{
			name:       "list non-tty",
			opts:       ListOptions{HTTPClient: mockGPGResponse},
			isTTY:      false,
			wantStdout: "johnny@test.com\tABCDEF1234567890\txJMEWfoofoofoo\t2020-06-11T15:44:24+01:00\t2099-01-01T15:44:24+01:00\nmonalisa@github.com\t1234567890ABCDEF\txJMEWbarbarbar\t2021-01-11T15:44:24+01:00\t0001-01-01T00:00:00Z\n",
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

func mockGPGResponse() (*http.Client, error) {
	ca1, _ := time.Parse(time.RFC3339, "2020-06-11T15:44:24+01:00")
	ea1, _ := time.Parse(time.RFC3339, "2099-01-01T15:44:24+01:00")
	ca2, _ := time.Parse(time.RFC3339, "2021-01-11T15:44:24+01:00")
	ea2 := time.Time{}
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
		]`, ca1.Format(time.RFC3339),
			ea1.Format(time.RFC3339),
			ca2.Format(time.RFC3339),
			ea2.Format(time.RFC3339))),
	)
	return &http.Client{Transport: reg}, nil
}
