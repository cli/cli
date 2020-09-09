package view

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewCmdView(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		isTTY   bool
		want    ViewOptions
		wantErr string
	}{
		{
			name:  "version argument",
			args:  "v1.2.3",
			isTTY: true,
			want: ViewOptions{
				TagName: "v1.2.3",
				WebMode: false,
			},
		},
		{
			name:  "no arguments",
			args:  "",
			isTTY: true,
			want: ViewOptions{
				TagName: "",
				WebMode: false,
			},
		},
		{
			name:  "web mode",
			args:  "-w",
			isTTY: true,
			want: ViewOptions{
				TagName: "",
				WebMode: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			io.SetStdoutTTY(tt.isTTY)
			io.SetStdinTTY(tt.isTTY)
			io.SetStderrTTY(tt.isTTY)

			f := &cmdutil.Factory{
				IOStreams: io,
			}

			var opts *ViewOptions
			cmd := NewCmdView(f, func(o *ViewOptions) error {
				opts = o
				return nil
			})
			cmd.PersistentFlags().StringP("repo", "R", "", "")

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)
			cmd.SetArgs(argv)

			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(ioutil.Discard)
			cmd.SetErr(ioutil.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.want.TagName, opts.TagName)
			assert.Equal(t, tt.want.WebMode, opts.WebMode)
		})
	}
}

func Test_viewRun(t *testing.T) {
	oneHourAgo := time.Now().Add(time.Duration(-24) * time.Hour)
	frozenTime, err := time.Parse(time.RFC3339, "2020-08-31T15:44:24+02:00")
	require.NoError(t, err)

	tests := []struct {
		name       string
		isTTY      bool
		releasedAt time.Time
		opts       ViewOptions
		wantErr    string
		wantStdout string
		wantStderr string
	}{
		{
			name:       "view specific release",
			isTTY:      true,
			releasedAt: oneHourAgo,
			opts: ViewOptions{
				TagName: "v1.2.3",
			},
			wantStdout: heredoc.Doc(`
				v1.2.3
				MonaLisa released this about 1 day ago
				
				                                                                              
				  • Fixed bugs                                                                
				
				
				Assets
				windows.zip  12 B
				linux.tgz    34 B
				
				View on GitHub: https://github.com/OWNER/REPO/releases/tags/v1.2.3
			`),
			wantStderr: ``,
		},
		{
			name:       "view latest release",
			isTTY:      true,
			releasedAt: oneHourAgo,
			opts: ViewOptions{
				TagName: "",
			},
			wantStdout: heredoc.Doc(`
				v1.2.3
				MonaLisa released this about 1 day ago
				
				                                                                              
				  • Fixed bugs                                                                
				
				
				Assets
				windows.zip  12 B
				linux.tgz    34 B
				
				View on GitHub: https://github.com/OWNER/REPO/releases/tags/v1.2.3
			`),
			wantStderr: ``,
		},
		{
			name:       "view machine-readable",
			isTTY:      false,
			releasedAt: frozenTime,
			opts: ViewOptions{
				TagName: "v1.2.3",
			},
			wantStdout: heredoc.Doc(`
				title:	
				tag:	v1.2.3
				draft:	false
				prerelease:	false
				author:	MonaLisa
				created:	2020-08-31T15:44:24+02:00
				published:	2020-08-31T15:44:24+02:00
				url:	https://github.com/OWNER/REPO/releases/tags/v1.2.3
				asset:	windows.zip
				asset:	linux.tgz
				--
				* Fixed bugs
			`),
			wantStderr: ``,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, stdout, stderr := iostreams.Test()
			io.SetStdoutTTY(tt.isTTY)
			io.SetStdinTTY(tt.isTTY)
			io.SetStderrTTY(tt.isTTY)

			path := "repos/OWNER/REPO/releases/tags/v1.2.3"
			if tt.opts.TagName == "" {
				path = "repos/OWNER/REPO/releases/latest"
			}

			fakeHTTP := &httpmock.Registry{}
			fakeHTTP.Register(httpmock.REST("GET", path), httpmock.StringResponse(fmt.Sprintf(`{
				"tag_name": "v1.2.3",
				"draft": false,
				"author": { "login": "MonaLisa" },
				"body": "* Fixed bugs\n",
				"created_at": "%[1]s",
				"published_at": "%[1]s",
				"html_url": "https://github.com/OWNER/REPO/releases/tags/v1.2.3",
				"assets": [
					{ "name": "windows.zip", "size": 12 },
					{ "name": "linux.tgz", "size": 34 }
				]
			}`, tt.releasedAt.Format(time.RFC3339))))

			tt.opts.IO = io
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: fakeHTTP}, nil
			}
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("OWNER/REPO")
			}

			err := viewRun(&tt.opts)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}

func Test_humanFileSize(t *testing.T) {
	tests := []struct {
		name string
		size int64
		want string
	}{
		{
			name: "min bytes",
			size: 1,
			want: "1 B",
		},
		{
			name: "max bytes",
			size: 1023,
			want: "1023 B",
		},
		{
			name: "min kibibytes",
			size: 1024,
			want: "1.00 KiB",
		},
		{
			name: "max kibibytes",
			size: 1024*1024 - 1,
			want: "1023.99 KiB",
		},
		{
			name: "min mibibytes",
			size: 1024 * 1024,
			want: "1.00 MiB",
		},
		{
			name: "fractional mibibytes",
			size: 1024*1024*12 + 1024*350,
			want: "12.34 MiB",
		},
		{
			name: "max mibibytes",
			size: 1024*1024*1024 - 1,
			want: "1023.99 MiB",
		},
		{
			name: "min gibibytes",
			size: 1024 * 1024 * 1024,
			want: "1.00 GiB",
		},
		{
			name: "fractional gibibytes",
			size: 1024 * 1024 * 1024 * 1.5,
			want: "1.50 GiB",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := humanFileSize(tt.size); got != tt.want {
				t.Errorf("humanFileSize() = %v, want %v", got, tt.want)
			}
		})
	}
}
