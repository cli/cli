package download

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
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
			name:  "version argument",
			args:  "v1.2.3",
			isTTY: true,
			want: DownloadOptions{
				TagName:      "v1.2.3",
				FilePatterns: []string(nil),
				Destination:  ".",
				Concurrency:  5,
			},
		},
		{
			name:  "version and file pattern",
			args:  "v1.2.3 -p *.tgz",
			isTTY: true,
			want: DownloadOptions{
				TagName:      "v1.2.3",
				FilePatterns: []string{"*.tgz"},
				Destination:  ".",
				Concurrency:  5,
			},
		},
		{
			name:  "multiple file patterns",
			args:  "v1.2.3 -p 1 -p 2,3",
			isTTY: true,
			want: DownloadOptions{
				TagName:      "v1.2.3",
				FilePatterns: []string{"1", "2,3"},
				Destination:  ".",
				Concurrency:  5,
			},
		},
		{
			name:  "version and destination",
			args:  "v1.2.3 -D tmp/assets",
			isTTY: true,
			want: DownloadOptions{
				TagName:      "v1.2.3",
				FilePatterns: []string(nil),
				Destination:  "tmp/assets",
				Concurrency:  5,
			},
		},
		{
			name:  "download latest",
			args:  "-p *",
			isTTY: true,
			want: DownloadOptions{
				TagName:      "",
				FilePatterns: []string{"*"},
				Destination:  ".",
				Concurrency:  5,
			},
		},
		{
			name:    "no arguments",
			args:    "",
			isTTY:   true,
			wantErr: "the '--pattern' flag is required when downloading the latest release",
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

			assert.Equal(t, tt.want.TagName, opts.TagName)
			assert.Equal(t, tt.want.FilePatterns, opts.FilePatterns)
			assert.Equal(t, tt.want.Destination, opts.Destination)
			assert.Equal(t, tt.want.Concurrency, opts.Concurrency)
		})
	}
}

func Test_downloadRun(t *testing.T) {
	tests := []struct {
		name       string
		isTTY      bool
		opts       DownloadOptions
		wantErr    string
		wantStdout string
		wantStderr string
		wantFiles  []string
	}{
		{
			name:  "download all assets",
			isTTY: true,
			opts: DownloadOptions{
				TagName:     "v1.2.3",
				Destination: ".",
				Concurrency: 2,
			},
			wantStdout: ``,
			wantStderr: ``,
			wantFiles: []string{
				"linux.tgz",
				"windows-32bit.zip",
				"windows-64bit.zip",
			},
		},
		{
			name:  "download assets matching pattern into destination directory",
			isTTY: true,
			opts: DownloadOptions{
				TagName:      "v1.2.3",
				FilePatterns: []string{"windows-*.zip"},
				Destination:  "tmp/assets",
				Concurrency:  2,
			},
			wantStdout: ``,
			wantStderr: ``,
			wantFiles: []string{
				"tmp/assets/windows-32bit.zip",
				"tmp/assets/windows-64bit.zip",
			},
		},
		{
			name:  "no match for pattern",
			isTTY: true,
			opts: DownloadOptions{
				TagName:      "v1.2.3",
				FilePatterns: []string{"linux*.zip"},
				Destination:  ".",
				Concurrency:  2,
			},
			wantStdout: ``,
			wantStderr: ``,
			wantErr:    "no assets match the file pattern",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tt.opts.Destination = filepath.Join(tempDir, tt.opts.Destination)

			io, _, stdout, stderr := iostreams.Test()
			io.SetStdoutTTY(tt.isTTY)
			io.SetStdinTTY(tt.isTTY)
			io.SetStderrTTY(tt.isTTY)

			fakeHTTP := &httpmock.Registry{}
			fakeHTTP.Register(httpmock.REST("GET", "repos/OWNER/REPO/releases/tags/v1.2.3"), httpmock.StringResponse(`{
				"assets": [
					{ "name": "windows-32bit.zip", "size": 12,
					  "url": "https://api.github.com/assets/1234" },
					{ "name": "windows-64bit.zip", "size": 34,
					  "url": "https://api.github.com/assets/3456" },
					{ "name": "linux.tgz", "size": 56,
					  "url": "https://api.github.com/assets/5678" }
				]
			}`))
			fakeHTTP.Register(httpmock.REST("GET", "assets/1234"), httpmock.StringResponse(`1234`))
			fakeHTTP.Register(httpmock.REST("GET", "assets/3456"), httpmock.StringResponse(`3456`))
			fakeHTTP.Register(httpmock.REST("GET", "assets/5678"), httpmock.StringResponse(`5678`))

			tt.opts.IO = io
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: fakeHTTP}, nil
			}
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("OWNER/REPO")
			}

			err := downloadRun(&tt.opts)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, "application/octet-stream", fakeHTTP.Requests[1].Header.Get("Accept"))

			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())

			downloadedFiles, err := listFiles(tempDir)
			require.NoError(t, err)
			assert.Equal(t, tt.wantFiles, downloadedFiles)
		})
	}
}

func listFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(p string, f os.FileInfo, err error) error {
		if !f.IsDir() {
			rp, err := filepath.Rel(dir, p)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rp))
		}
		return err
	})
	return files, err
}
