package download

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewCmdDownload(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		isTTY   bool
		want    DownloadOptions
		wantErr string
	}{
		{
			name:    "empty",
			args:    "",
			isTTY:   true,
			wantErr: "either run ID or `--pattern` is required",
		},
		{
			name:  "with run ID",
			args:  "2345",
			isTTY: true,
			want: DownloadOptions{
				RunID:          "2345",
				FilePatterns:   []string(nil),
				DestinationDir: ".",
			},
		},
		{
			name:  "to destination",
			args:  "2345 -D tmp/dest",
			isTTY: true,
			want: DownloadOptions{
				RunID:          "2345",
				FilePatterns:   []string(nil),
				DestinationDir: "tmp/dest",
			},
		},
		{
			name:  "repo level with patterns",
			args:  "-p one -p two",
			isTTY: true,
			want: DownloadOptions{
				RunID:          "",
				FilePatterns:   []string{"one", "two"},
				DestinationDir: ".",
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
				HttpClient: func() (*http.Client, error) {
					return nil, nil
				},
				BaseRepo: func() (ghrepo.Interface, error) {
					return nil, nil
				},
			}

			var opts *DownloadOptions
			cmd := NewCmdDownload(f, func(o *DownloadOptions) error {
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

			assert.Equal(t, tt.want.RunID, opts.RunID)
			assert.Equal(t, tt.want.FilePatterns, opts.FilePatterns)
			assert.Equal(t, tt.want.DestinationDir, opts.DestinationDir)
		})
	}
}

func Test_runDownload(t *testing.T) {
	tests := []struct {
		name           string
		opts           DownloadOptions
		artifacts      []Artifact
		wantDownloaded []string
		wantErr        string
	}{
		{
			name: "download non-expired",
			opts: DownloadOptions{
				RunID:          "2345",
				DestinationDir: ".",
				FilePatterns:   []string(nil),
			},
			artifacts: []Artifact{
				{
					Name:        "artifact-1",
					DownloadURL: "http://download.com/artifact-1.zip",
					Expired:     false,
				},
				{
					Name:        "expired-artifact",
					DownloadURL: "http://download.com/expired.zip",
					Expired:     true,
				},
				{
					Name:        "artifact-2",
					DownloadURL: "http://download.com/artifact-2.zip",
					Expired:     false,
				},
			},
			wantDownloaded: []string{
				"http://download.com/artifact-1.zip",
				"http://download.com/artifact-2.zip",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &tt.opts
			io, _, stdout, stderr := iostreams.Test()
			opts.IO = io
			platform := &stubPlatform{listResults: tt.artifacts}
			opts.Platform = platform

			err := runDownload(opts)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantDownloaded, platform.downloaded)
			assert.Equal(t, "", stdout.String())
			assert.Equal(t, "", stderr.String())
		})
	}
}

type stubPlatform struct {
	listResults []Artifact
	downloaded  []string
}

func (p *stubPlatform) List(runID string) ([]Artifact, error) {
	return p.listResults, nil
}

func (p *stubPlatform) Download(url string, dir string) error {
	p.downloaded = append(p.downloaded, url)
	return nil
}
