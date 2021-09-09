package add

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func Test_addRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       AddOptions
		isTTY      bool
		wantStdout string
		wantStderr string
		wantErr    bool
	}{
		{
			name: "add correctly",
			opts: AddOptions{
				HTTPClient: func() (*http.Client, error) {
					reg := httpmock.Registry{}
					reg.Register(
						httpmock.REST("POST", "repos/OWNER/REPO/keys"),
						httpmock.StringResponse(`{}`))

					return &http.Client{Transport: &reg}, nil
				},
				KeyFile: "-",
				Title:   "my sacred key",
			},
			wantStdout: "✓ Public key added to your repository\n",
			wantStderr: "",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, stdin, stdout, stderr := iostreams.Test()
			io.SetStdinTTY(false)
			io.SetStdoutTTY(true)
			io.SetStderrTTY(true)

			stdin.WriteString("PUBKEY")

			opts := tt.opts
			opts.IO = io
			opts.Config = func() (config.Config, error) { return config.NewBlankConfig(), nil }
			opts.BaseRepo = func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil }

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
