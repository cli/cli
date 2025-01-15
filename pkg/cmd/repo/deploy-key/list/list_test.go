package list

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func TestListRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       ListOptions
		isTTY      bool
		httpStubs  func(t *testing.T, reg *httpmock.Registry)
		wantStdout string
		wantStderr string
		wantErr    bool
	}{
		{
			name:  "list tty",
			isTTY: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				createdAt := time.Now().Add(time.Duration(-24) * time.Hour)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/keys"),
					httpmock.StringResponse(fmt.Sprintf(`[
						{
							"id": 1234,
							"key": "ssh-rsa AAAABbBB123",
							"title": "Mac",
							"created_at": "%[1]s",
							"read_only": true
						},
						{
							"id": 5678,
							"key": "ssh-rsa EEEEEEEK247",
							"title": "hubot@Windows",
							"created_at": "%[1]s",
							"read_only": false
						}
					]`, createdAt.Format(time.RFC3339))),
				)
			},
			wantStdout: heredoc.Doc(`
				ID    TITLE          TYPE        KEY                  CREATED AT
				1234  Mac            read-only   ssh-rsa AAAABbBB123  about 1 day ago
				5678  hubot@Windows  read-write  ssh-rsa EEEEEEEK247  about 1 day ago
			`),
			wantStderr: "",
		},
		{
			name:  "list non-tty",
			isTTY: false,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				createdAt, _ := time.Parse(time.RFC3339, "2020-08-31T15:44:24+02:00")
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/keys"),
					httpmock.StringResponse(fmt.Sprintf(`[
						{
							"id": 1234,
							"key": "ssh-rsa AAAABbBB123",
							"title": "Mac",
							"created_at": "%[1]s",
							"read_only": false
						},
						{
							"id": 5678,
							"key": "ssh-rsa EEEEEEEK247",
							"title": "hubot@Windows",
							"created_at": "%[1]s",
							"read_only": true
						}
					]`, createdAt.Format(time.RFC3339))),
				)
			},
			wantStdout: heredoc.Doc(`
				1234	Mac	read-write	ssh-rsa AAAABbBB123	2020-08-31T15:44:24+02:00
				5678	hubot@Windows	read-only	ssh-rsa EEEEEEEK247	2020-08-31T15:44:24+02:00
			`),
			wantStderr: "",
		},
		{
			name:  "no keys",
			isTTY: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/keys"),
					httpmock.StringResponse(`[]`))
			},
			wantStdout: "",
			wantStderr: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}

			opts := tt.opts
			opts.IO = ios
			opts.BaseRepo = func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil }
			opts.HTTPClient = func() (*http.Client, error) { return &http.Client{Transport: reg}, nil }

			err := listRun(&opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("listRun() return error: %v", err)
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
