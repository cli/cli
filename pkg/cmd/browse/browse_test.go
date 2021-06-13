package browse

import (
	"net/http"
	"testing"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdBrowse(t *testing.T) {
	f := cmdutil.Factory{}
	var opts *BrowseOptions
	// pass a stub implementation of `runBrowse` for testing to avoid having real `runBrowse` called
	cmd := NewCmdBrowse(&f, func(o *BrowseOptions) error {
		opts = o
		return nil
	})

	cmd.SetArgs([]string{"--branch", "main"})
	_, err := cmd.ExecuteC()
	assert.NoError(t, err)

	assert.Equal(t, "main", opts.Branch)
	assert.Equal(t, "", opts.SelectorArg)
	assert.Equal(t, false, opts.ProjectsFlag)
	assert.Equal(t, false, opts.WikiFlag)
	assert.Equal(t, false, opts.SettingsFlag)
}

func Test_runBrowse(t *testing.T) {
	tests := []struct {
		name          string
		opts          BrowseOptions
		baseRepo      ghrepo.Interface
		defaultBranch string
		expectedURL   string
		wantsErr      bool
	}{
		{
			name: "no arguments",
			opts: BrowseOptions{
				SelectorArg: "",
			},
			baseRepo:      ghrepo.New("jessica", "cli"),
			defaultBranch: "trunk",
			expectedURL:   "https://github.com/jessica/cli/tree/trunk/",
		},
		{
			name:          "file argument",
			opts:          BrowseOptions{SelectorArg: "path/to/file.txt"},
			baseRepo:      ghrepo.New("ken", "cli"),
			defaultBranch: "main",
			expectedURL:   "https://github.com/ken/cli/tree/main/path/to/file.txt",
		},
		{
			name: "branch flag",
			opts: BrowseOptions{
				Branch: "trunk",
			},
			baseRepo:    ghrepo.New("thanh", "cli"),
			expectedURL: "https://github.com/thanh/cli/tree/trunk/",
		},
		{
			name: "settings flag",
			opts: BrowseOptions{
				SettingsFlag: true,
			},
			baseRepo:    ghrepo.New("bchadwic", "cli"),
			expectedURL: "https://github.com/bchadwic/cli/settings",
		},
		{
			name: "projects flag",
			opts: BrowseOptions{
				ProjectsFlag: true,
			},
			baseRepo:    ghrepo.New("bchadwic", "cli"),
			expectedURL: "https://github.com/bchadwic/cli/projects",
		},
		{
			name: "wiki flag",
			opts: BrowseOptions{
				WikiFlag: true,
			},
			baseRepo:    ghrepo.New("bchadwic", "cli"),
			expectedURL: "https://github.com/bchadwic/cli/wiki",
		},
		{
			name: "issue argument",
			opts: BrowseOptions{
				SelectorArg: "217",
			},
			baseRepo:    ghrepo.New("bchadwic", "cli"),
			expectedURL: "https://github.com/bchadwic/cli/issues/217",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, stdout, stderr := iostreams.Test()
			browser := cmdutil.TestBrowser{}

			reg := httpmock.Registry{}
			defer reg.Verify(t)
			if tt.defaultBranch != "" {
				reg.StubRepoInfoResponse(tt.baseRepo.RepoOwner(), tt.baseRepo.RepoName(), tt.defaultBranch)
			}

			opts := tt.opts
			opts.IO = io
			opts.BaseRepo = func() (ghrepo.Interface, error) {
				return tt.baseRepo, nil
			}
			opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: &reg}, nil
			}
			opts.Browser = &browser

			err := runBrowse(&opts)
			if tt.wantsErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, "", stdout.String())
			assert.Equal(t, "", stderr.String())
			browser.Verify(t, tt.expectedURL)
		})
	}
}

func TestFileArgParsing(t *testing.T) {
	tests := []struct {
		name           string
		arg            string
		errorExpected  bool
		fileArg        string
		lineArg        string
		stderrExpected string
	}{
		{
			name:          "non line number",
			arg:           "main.go",
			errorExpected: false,
			fileArg:       "main.go",
		},
		{
			name:          "line number",
			arg:           "main.go:32",
			errorExpected: false,
			fileArg:       "main.go",
			lineArg:       "32",
		},
		{
			name:           "non line number error",
			arg:            "ma:in.go",
			errorExpected:  true,
			stderrExpected: "invalid line number after colon\nUse 'gh browse --help' for more information about browse\n",
		},
	}
	for _, tt := range tests {
		arr, err := parseFileArg(tt.arg)
		if tt.errorExpected {
			assert.Equal(t, err.Error(), tt.stderrExpected)
		} else {
			assert.Equal(t, err, nil)
			if len(arr) > 1 {
				assert.Equal(t, tt.lineArg, arr[1])
			}
			assert.Equal(t, tt.fileArg, arr[0])
		}
	}
}
