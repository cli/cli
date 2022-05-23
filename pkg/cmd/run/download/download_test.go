package download

import (
	"bytes"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
			name:  "empty",
			args:  "",
			isTTY: true,
			want: DownloadOptions{
				RunID:          "",
				DoPrompt:       true,
				Names:          []string(nil),
				DestinationDir: ".",
			},
		},
		{
			name:  "with run ID",
			args:  "2345",
			isTTY: true,
			want: DownloadOptions{
				RunID:          "2345",
				DoPrompt:       false,
				Names:          []string(nil),
				DestinationDir: ".",
			},
		},
		{
			name:  "to destination",
			args:  "2345 -D tmp/dest",
			isTTY: true,
			want: DownloadOptions{
				RunID:          "2345",
				DoPrompt:       false,
				Names:          []string(nil),
				DestinationDir: "tmp/dest",
			},
		},
		{
			name:  "repo level with names",
			args:  "-n one -n two",
			isTTY: true,
			want: DownloadOptions{
				RunID:          "",
				DoPrompt:       false,
				Names:          []string{"one", "two"},
				DestinationDir: ".",
			},
		},
		{
			name:  "repo level with patterns",
			args:  "-p o*e -p tw*",
			isTTY: true,
			want: DownloadOptions{
				RunID:          "",
				DoPrompt:       false,
				FilePatterns:   []string{"o*e", "tw*"},
				DestinationDir: ".",
			},
		},
		{
			name:  "repo level with names and patterns",
			args:  "-p o*e -p tw* -n three -n four",
			isTTY: true,
			want: DownloadOptions{
				RunID:          "",
				DoPrompt:       false,
				Names:          []string{"three", "four"},
				FilePatterns:   []string{"o*e", "tw*"},
				DestinationDir: ".",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			f := &cmdutil.Factory{
				IOStreams: ios,
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
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.want.RunID, opts.RunID)
			assert.Equal(t, tt.want.Names, opts.Names)
			assert.Equal(t, tt.want.FilePatterns, opts.FilePatterns)
			assert.Equal(t, tt.want.DestinationDir, opts.DestinationDir)
			assert.Equal(t, tt.want.DoPrompt, opts.DoPrompt)
		})
	}
}

func Test_runDownload(t *testing.T) {
	tests := []struct {
		name       string
		opts       DownloadOptions
		mockAPI    func(*mockPlatform)
		mockPrompt func(*mockPrompter)
		wantErr    string
	}{
		{
			name: "download non-expired",
			opts: DownloadOptions{
				RunID:          "2345",
				DestinationDir: "./tmp",
				Names:          []string(nil),
			},
			mockAPI: func(p *mockPlatform) {
				p.On("List", "2345").Return([]shared.Artifact{
					{
						Name:        "artifact-1",
						DownloadURL: "http://download.com/artifact1.zip",
						Expired:     false,
					},
					{
						Name:        "expired-artifact",
						DownloadURL: "http://download.com/expired.zip",
						Expired:     true,
					},
					{
						Name:        "artifact-2",
						DownloadURL: "http://download.com/artifact2.zip",
						Expired:     false,
					},
				}, nil)
				p.On("Download", "http://download.com/artifact1.zip", filepath.FromSlash("tmp/artifact-1")).Return(nil)
				p.On("Download", "http://download.com/artifact2.zip", filepath.FromSlash("tmp/artifact-2")).Return(nil)
			},
		},
		{
			name: "no valid artifacts",
			opts: DownloadOptions{
				RunID:          "2345",
				DestinationDir: ".",
				Names:          []string(nil),
			},
			mockAPI: func(p *mockPlatform) {
				p.On("List", "2345").Return([]shared.Artifact{
					{
						Name:        "artifact-1",
						DownloadURL: "http://download.com/artifact1.zip",
						Expired:     true,
					},
					{
						Name:        "artifact-2",
						DownloadURL: "http://download.com/artifact2.zip",
						Expired:     true,
					},
				}, nil)
			},
			wantErr: "no valid artifacts found to download",
		},
		{
			name: "no name matches",
			opts: DownloadOptions{
				RunID:          "2345",
				DestinationDir: ".",
				Names:          []string{"artifact-3"},
			},
			mockAPI: func(p *mockPlatform) {
				p.On("List", "2345").Return([]shared.Artifact{
					{
						Name:        "artifact-1",
						DownloadURL: "http://download.com/artifact1.zip",
						Expired:     false,
					},
					{
						Name:        "artifact-2",
						DownloadURL: "http://download.com/artifact2.zip",
						Expired:     false,
					},
				}, nil)
			},
			wantErr: "no artifact matches any of the names or patterns provided",
		},
		{
			name: "no pattern matches",
			opts: DownloadOptions{
				RunID:          "2345",
				DestinationDir: ".",
				FilePatterns:   []string{"artifiction-*"},
			},
			mockAPI: func(p *mockPlatform) {
				p.On("List", "2345").Return([]shared.Artifact{
					{
						Name:        "artifact-1",
						DownloadURL: "http://download.com/artifact1.zip",
						Expired:     false,
					},
					{
						Name:        "artifact-2",
						DownloadURL: "http://download.com/artifact2.zip",
						Expired:     false,
					},
				}, nil)
			},
			wantErr: "no artifact matches any of the names or patterns provided",
		},
		{
			name: "prompt to select artifact",
			opts: DownloadOptions{
				RunID:          "",
				DoPrompt:       true,
				DestinationDir: ".",
				Names:          []string(nil),
			},
			mockAPI: func(p *mockPlatform) {
				p.On("List", "").Return([]shared.Artifact{
					{
						Name:        "artifact-1",
						DownloadURL: "http://download.com/artifact1.zip",
						Expired:     false,
					},
					{
						Name:        "expired-artifact",
						DownloadURL: "http://download.com/expired.zip",
						Expired:     true,
					},
					{
						Name:        "artifact-2",
						DownloadURL: "http://download.com/artifact2.zip",
						Expired:     false,
					},
					{
						Name:        "artifact-2",
						DownloadURL: "http://download.com/artifact2.also.zip",
						Expired:     false,
					},
				}, nil)
				p.On("Download", "http://download.com/artifact2.zip", ".").Return(nil)
			},
			mockPrompt: func(p *mockPrompter) {
				p.On("Prompt", "Select artifacts to download:", []string{"artifact-1", "artifact-2"}, mock.AnythingOfType("*[]string")).
					Run(func(args mock.Arguments) {
						result := args.Get(2).(*[]string)
						*result = []string{"artifact-2"}
					}).
					Return(nil)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &tt.opts
			ios, _, stdout, stderr := iostreams.Test()
			opts.IO = ios
			opts.Platform = newMockPlatform(t, tt.mockAPI)
			opts.Prompter = newMockPrompter(t, tt.mockPrompt)

			err := runDownload(opts)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, "", stdout.String())
			assert.Equal(t, "", stderr.String())
		})
	}
}

type mockPlatform struct {
	mock.Mock
}

func newMockPlatform(t *testing.T, config func(*mockPlatform)) *mockPlatform {
	m := &mockPlatform{}
	m.Test(t)
	t.Cleanup(func() {
		m.AssertExpectations(t)
	})
	if config != nil {
		config(m)
	}
	return m
}

func (p *mockPlatform) List(runID string) ([]shared.Artifact, error) {
	args := p.Called(runID)
	return args.Get(0).([]shared.Artifact), args.Error(1)
}

func (p *mockPlatform) Download(url string, dir string) error {
	args := p.Called(url, dir)
	return args.Error(0)
}

type mockPrompter struct {
	mock.Mock
}

func newMockPrompter(t *testing.T, config func(*mockPrompter)) *mockPrompter {
	m := &mockPrompter{}
	m.Test(t)
	t.Cleanup(func() {
		m.AssertExpectations(t)
	})
	if config != nil {
		config(m)
	}
	return m
}

func (p *mockPrompter) Prompt(msg string, opts []string, res interface{}) error {
	args := p.Called(msg, opts, res)
	return args.Error(0)
}
