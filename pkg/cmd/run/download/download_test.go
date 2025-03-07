package download

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/safepaths"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
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

type run struct {
	id            string
	testArtifacts []testArtifact
}

type testArtifact struct {
	artifact shared.Artifact
	files    []string
}

type fakePlatform struct {
	runs []run
}

func (f *fakePlatform) List(runID string) ([]shared.Artifact, error) {
	runIds := map[string]struct{}{}
	if runID != "" {
		runIds[runID] = struct{}{}
	} else {
		for _, run := range f.runs {
			runIds[run.id] = struct{}{}
		}
	}

	var artifacts []shared.Artifact
	for _, run := range f.runs {
		// Skip over any runs that we aren't looking for
		if _, ok := runIds[run.id]; !ok {
			continue
		}

		// Grab the artifacts of everything else
		for _, testArtifact := range run.testArtifacts {
			artifacts = append(artifacts, testArtifact.artifact)
		}
	}

	return artifacts, nil
}

func (f *fakePlatform) Download(url string, dir safepaths.Absolute) error {
	if err := os.MkdirAll(dir.String(), 0755); err != nil {
		return err
	}
	// Now to be consistent, we find the artifact with the provided URL.
	// It's a bit janky to iterate the runs, to find the right artifact
	// rather than keying directly to it, but it allows the setup of the
	// fake platform to be declarative rather than imperative.
	// Think fakePlatform { artifacts: ... } rather than fakePlatform.makeArtifactAvailable()
	for _, run := range f.runs {
		for _, testArtifact := range run.testArtifacts {
			if testArtifact.artifact.DownloadURL == url {
				for _, file := range testArtifact.files {
					path := filepath.Join(dir.String(), file)
					return os.WriteFile(path, []byte{}, 0600)
				}
			}
		}
	}

	return errors.New("no artifact matches the provided URL")
}

func Test_runDownload(t *testing.T) {
	tests := []struct {
		name          string
		opts          DownloadOptions
		platform      *fakePlatform
		promptStubs   func(*prompter.MockPrompter)
		expectedFiles []string
		wantErr       string
	}{
		{
			name: "download non-expired to relative directory",
			opts: DownloadOptions{
				RunID:          "2345",
				DestinationDir: "./tmp",
			},
			platform: &fakePlatform{
				runs: []run{
					{
						id: "2345",
						testArtifacts: []testArtifact{
							{
								artifact: shared.Artifact{
									Name:        "artifact-1",
									DownloadURL: "http://download.com/artifact1.zip",
									Expired:     false,
								},
								files: []string{
									"artifact-1-file",
								},
							},
							{
								artifact: shared.Artifact{
									Name:        "expired-artifact",
									DownloadURL: "http://download.com/expired.zip",
									Expired:     true,
								},
								files: []string{
									"expired",
								},
							},
							{
								artifact: shared.Artifact{
									Name:        "artifact-2",
									DownloadURL: "http://download.com/artifact2.zip",
									Expired:     false,
								},
								files: []string{
									"artifact-2-file",
								},
							},
						},
					},
				},
			},
			expectedFiles: []string{
				filepath.Join("artifact-1", "artifact-1-file"),
				filepath.Join("artifact-2", "artifact-2-file"),
			},
		},
		{
			name: "download non-expired to absolute directory",
			opts: DownloadOptions{
				RunID:          "2345",
				DestinationDir: "/tmp",
			},
			platform: &fakePlatform{
				runs: []run{
					{
						id: "2345",
						testArtifacts: []testArtifact{
							{
								artifact: shared.Artifact{
									Name:        "artifact-1",
									DownloadURL: "http://download.com/artifact1.zip",
									Expired:     false,
								},
								files: []string{
									"artifact-1-file",
								},
							},
							{
								artifact: shared.Artifact{
									Name:        "expired-artifact",
									DownloadURL: "http://download.com/expired.zip",
									Expired:     true,
								},
								files: []string{
									"expired",
								},
							},
							{
								artifact: shared.Artifact{
									Name:        "artifact-2",
									DownloadURL: "http://download.com/artifact2.zip",
									Expired:     false,
								},
								files: []string{
									"artifact-2-file",
								},
							},
						},
					},
				},
			},
			expectedFiles: []string{
				filepath.Join("artifact-1", "artifact-1-file"),
				filepath.Join("artifact-2", "artifact-2-file"),
			},
		},
		{
			name: "all artifacts are expired",
			opts: DownloadOptions{
				RunID: "2345",
			},
			platform: &fakePlatform{
				runs: []run{
					{
						id: "2345",
						testArtifacts: []testArtifact{
							{
								artifact: shared.Artifact{
									Name:        "artifact-1",
									DownloadURL: "http://download.com/artifact1.zip",
									Expired:     true,
								},
								files: []string{
									"artifact-1-file",
								},
							},
							{
								artifact: shared.Artifact{
									Name:        "artifact-2",
									DownloadURL: "http://download.com/artifact2.zip",
									Expired:     true,
								},
								files: []string{
									"artifact-2-file",
								},
							},
						},
					},
				},
			},
			expectedFiles: []string{},
			wantErr:       "no valid artifacts found to download",
		},
		{
			name: "no name matches",
			opts: DownloadOptions{
				RunID: "2345",
				Names: []string{"artifact-3"},
			},
			platform: &fakePlatform{
				runs: []run{
					{
						id: "2345",
						testArtifacts: []testArtifact{
							{
								artifact: shared.Artifact{
									Name:        "artifact-1",
									DownloadURL: "http://download.com/artifact1.zip",
									Expired:     false,
								},
								files: []string{
									"artifact-1-file",
								},
							},
							{
								artifact: shared.Artifact{
									Name:        "artifact-2",
									DownloadURL: "http://download.com/artifact2.zip",
									Expired:     false,
								},
								files: []string{
									"artifact-2-file",
								},
							},
						},
					},
				},
			},
			expectedFiles: []string{},
			wantErr:       "no artifact matches any of the names or patterns provided",
		},
		{
			name: "pattern matches",
			opts: DownloadOptions{
				RunID:        "2345",
				FilePatterns: []string{"artifact-*"},
			},
			platform: &fakePlatform{
				runs: []run{
					{
						id: "2345",
						testArtifacts: []testArtifact{
							{
								artifact: shared.Artifact{
									Name:        "artifact-1",
									DownloadURL: "http://download.com/artifact1.zip",
									Expired:     false,
								},
								files: []string{
									"artifact-1-file",
								},
							},
							{
								artifact: shared.Artifact{
									Name:        "non-artifact-2",
									DownloadURL: "http://download.com/non-artifact-2.zip",
									Expired:     false,
								},
								files: []string{
									"non-artifact-2-file",
								},
							},
							{
								artifact: shared.Artifact{
									Name:        "artifact-3",
									DownloadURL: "http://download.com/artifact3.zip",
									Expired:     false,
								},
								files: []string{
									"artifact-3-file",
								},
							},
						},
					},
				},
			},
			expectedFiles: []string{
				filepath.Join("artifact-1", "artifact-1-file"),
				filepath.Join("artifact-3", "artifact-3-file"),
			},
		},
		{
			name: "no pattern matches",
			opts: DownloadOptions{
				RunID:        "2345",
				FilePatterns: []string{"artifiction-*"},
			},
			platform: &fakePlatform{
				runs: []run{
					{
						id: "2345",
						testArtifacts: []testArtifact{
							{
								artifact: shared.Artifact{
									Name:        "artifact-1",
									DownloadURL: "http://download.com/artifact1.zip",
									Expired:     false,
								},
								files: []string{
									"artifact-1-file",
								},
							},
							{
								artifact: shared.Artifact{
									Name:        "artifact-2",
									DownloadURL: "http://download.com/artifact2.zip",
									Expired:     false,
								},
								files: []string{
									"artifact-2-file",
								},
							},
						},
					},
				},
			},
			expectedFiles: []string{},
			wantErr:       "no artifact matches any of the names or patterns provided",
		},
		{
			name: "want specific single artifact",
			opts: DownloadOptions{
				RunID: "2345",
				Names: []string{"non-artifact-2"},
			},
			platform: &fakePlatform{
				runs: []run{
					{
						id: "2345",
						testArtifacts: []testArtifact{
							{
								artifact: shared.Artifact{
									Name:        "artifact-1",
									DownloadURL: "http://download.com/artifact1.zip",
									Expired:     false,
								},
								files: []string{
									"artifact-1-file",
								},
							},
							{
								artifact: shared.Artifact{
									Name:        "non-artifact-2",
									DownloadURL: "http://download.com/non-artifact-2.zip",
									Expired:     false,
								},
								files: []string{
									"non-artifact-2-file",
								},
							},
							{
								artifact: shared.Artifact{
									Name:        "artifact-3",
									DownloadURL: "http://download.com/artifact3.zip",
									Expired:     false,
								},
								files: []string{
									"artifact-3-file",
								},
							},
						},
					},
				},
			},
			expectedFiles: []string{
				filepath.Join("non-artifact-2-file"),
			},
		},
		{
			name: "want specific multiple artifacts",
			opts: DownloadOptions{
				RunID: "2345",
				Names: []string{"artifact-1", "artifact-3"},
			},
			platform: &fakePlatform{
				runs: []run{
					{
						id: "2345",
						testArtifacts: []testArtifact{
							{
								artifact: shared.Artifact{
									Name:        "artifact-1",
									DownloadURL: "http://download.com/artifact1.zip",
									Expired:     false,
								},
								files: []string{
									"artifact-1-file",
								},
							},
							{
								artifact: shared.Artifact{
									Name:        "non-artifact-2",
									DownloadURL: "http://download.com/non-artifact-2.zip",
									Expired:     false,
								},
								files: []string{
									"non-artifact-2-file",
								},
							},
							{
								artifact: shared.Artifact{
									Name:        "artifact-3",
									DownloadURL: "http://download.com/artifact3.zip",
									Expired:     false,
								},
								files: []string{
									"artifact-3-file",
								},
							},
						},
					},
				},
			},
			expectedFiles: []string{
				filepath.Join("artifact-1", "artifact-1-file"),
				filepath.Join("artifact-3", "artifact-3-file"),
			},
		},
		{
			name: "avoid redownloading files of the same name",
			opts: DownloadOptions{
				RunID: "2345",
			},
			platform: &fakePlatform{
				runs: []run{
					{
						id: "2345",
						testArtifacts: []testArtifact{
							{
								artifact: shared.Artifact{
									Name:        "artifact-1",
									DownloadURL: "http://download.com/artifact1.zip",
									Expired:     false,
								},
								files: []string{
									"artifact-1-file",
								},
							},
							{
								artifact: shared.Artifact{
									Name:        "artifact-1",
									DownloadURL: "http://download.com/artifact2.zip",
									Expired:     false,
								},
								files: []string{
									"artifact-2-file",
								},
							},
						},
					},
				},
			},
			expectedFiles: []string{
				filepath.Join("artifact-1", "artifact-1-file"),
			},
		},
		{
			name: "prompt to select artifact",
			opts: DownloadOptions{
				RunID:    "",
				DoPrompt: true,
				Names:    []string(nil),
			},
			platform: &fakePlatform{
				runs: []run{
					{
						id: "2345",
						testArtifacts: []testArtifact{
							{
								artifact: shared.Artifact{
									Name:        "artifact-1",
									DownloadURL: "http://download.com/artifact1.zip",
									Expired:     false,
								},
								files: []string{
									"artifact-1-file",
								},
							},
							{
								artifact: shared.Artifact{
									Name:        "expired-artifact",
									DownloadURL: "http://download.com/expired.zip",
									Expired:     true,
								},
								files: []string{
									"expired",
								},
							},
						},
					},
					{
						id: "6789",
						testArtifacts: []testArtifact{
							{
								artifact: shared.Artifact{
									Name:        "artifact-2",
									DownloadURL: "http://download.com/artifact2.zip",
									Expired:     false,
								},
								files: []string{
									"artifact-2-file",
								},
							},
						},
					},
				},
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterMultiSelect("Select artifacts to download:", nil, []string{"artifact-1", "artifact-2"},
					func(_ string, _, opts []string) ([]int, error) {
						for i, o := range opts {
							if o == "artifact-2" {
								return []int{i}, nil
							}
						}
						return nil, fmt.Errorf("no artifact-2 found in %v", opts)
					})
			},
			expectedFiles: []string{
				filepath.Join("artifact-2-file"),
			},
		},
		{
			name: "handling artifact name with path traversal exploit",
			opts: DownloadOptions{
				RunID: "2345",
			},
			platform: &fakePlatform{
				runs: []run{
					{
						id: "2345",
						testArtifacts: []testArtifact{
							{
								artifact: shared.Artifact{
									Name:        "..",
									DownloadURL: "http://download.com/artifact1.zip",
									Expired:     false,
								},
								files: []string{
									"etc/passwd",
								},
							},
						},
					},
				},
			},
			expectedFiles: []string{},
			wantErr:       "error downloading ..: would result in path traversal",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &tt.opts
			if opts.DestinationDir == "" {
				opts.DestinationDir = t.TempDir()
			} else {
				opts.DestinationDir = filepath.Join(t.TempDir(), opts.DestinationDir)
			}

			ios, _, stdout, stderr := iostreams.Test()
			opts.IO = ios
			opts.Platform = tt.platform

			pm := prompter.NewMockPrompter(t)
			opts.Prompter = pm
			if tt.promptStubs != nil {
				tt.promptStubs(pm)
			}

			err := runDownload(opts)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			// Check that the exact number of files exist
			require.Equal(t, len(tt.expectedFiles), countFilesInDirRecursively(t, opts.DestinationDir))

			// Then check that the exact files are correct
			for _, name := range tt.expectedFiles {
				require.FileExists(t, filepath.Join(opts.DestinationDir, name))
			}

			assert.Equal(t, "", stdout.String())
			assert.Equal(t, "", stderr.String())
		})
	}
}

func countFilesInDirRecursively(t *testing.T, dir string) int {
	t.Helper()

	count := 0
	require.NoError(t, filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		require.NoError(t, err)
		if !info.IsDir() {
			count++
		}
		return nil
	}))

	return count
}
