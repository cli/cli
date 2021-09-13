package browse

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdBrowse(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		factory  func(*cmdutil.Factory) *cmdutil.Factory
		wants    BrowseOptions
		wantsErr bool
	}{
		{
			name:     "no arguments",
			cli:      "",
			wantsErr: false,
		},
		{
			name: "settings flag",
			cli:  "--settings",
			wants: BrowseOptions{
				SettingsFlag: true,
			},
			wantsErr: false,
		},
		{
			name: "projects flag",
			cli:  "--projects",
			wants: BrowseOptions{
				ProjectsFlag: true,
			},
			wantsErr: false,
		},
		{
			name: "wiki flag",
			cli:  "--wiki",
			wants: BrowseOptions{
				WikiFlag: true,
			},
			wantsErr: false,
		},
		{
			name: "no browser flag",
			cli:  "--no-browser",
			wants: BrowseOptions{
				NoBrowserFlag: true,
			},
			wantsErr: false,
		},
		{
			name: "branch flag",
			cli:  "--branch main",
			wants: BrowseOptions{
				Branch: "main",
			},
			wantsErr: false,
		},
		{
			name:     "branch flag without a branch name",
			cli:      "--branch",
			wantsErr: true,
		},
		{
			name: "combination: settings projects",
			cli:  "--settings --projects",
			wants: BrowseOptions{
				SettingsFlag: true,
				ProjectsFlag: true,
			},
			wantsErr: true,
		},
		{
			name: "combination: projects wiki",
			cli:  "--projects --wiki",
			wants: BrowseOptions{
				ProjectsFlag: true,
				WikiFlag:     true,
			},
			wantsErr: true,
		},
		{
			name: "passed argument",
			cli:  "main.go",
			wants: BrowseOptions{
				SelectorArg: "main.go",
			},
			wantsErr: false,
		},
		{
			name:     "passed two arguments",
			cli:      "main.go main.go",
			wantsErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := cmdutil.Factory{}
			var opts *BrowseOptions
			cmd := NewCmdBrowse(&f, func(o *BrowseOptions) error {
				opts = o
				return nil
			})
			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)
			cmd.SetArgs(argv)
			_, err = cmd.ExecuteC()

			if tt.wantsErr {
				assert.Error(t, err)
				return
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wants.Branch, opts.Branch)
			assert.Equal(t, tt.wants.SelectorArg, opts.SelectorArg)
			assert.Equal(t, tt.wants.ProjectsFlag, opts.ProjectsFlag)
			assert.Equal(t, tt.wants.WikiFlag, opts.WikiFlag)
			assert.Equal(t, tt.wants.NoBrowserFlag, opts.NoBrowserFlag)
			assert.Equal(t, tt.wants.SettingsFlag, opts.SettingsFlag)
		})
	}
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
			baseRepo:    ghrepo.New("jlsestak", "cli"),
			expectedURL: "https://github.com/jlsestak/cli",
		},
		{
			name: "settings flag",
			opts: BrowseOptions{
				SettingsFlag: true,
			},
			baseRepo:    ghrepo.New("bchadwic", "ObscuredByClouds"),
			expectedURL: "https://github.com/bchadwic/ObscuredByClouds/settings",
		},
		{
			name: "projects flag",
			opts: BrowseOptions{
				ProjectsFlag: true,
			},
			baseRepo:    ghrepo.New("ttran112", "7ate9"),
			expectedURL: "https://github.com/ttran112/7ate9/projects",
		},
		{
			name: "wiki flag",
			opts: BrowseOptions{
				WikiFlag: true,
			},
			baseRepo:    ghrepo.New("ravocean", "ThreatLevelMidnight"),
			expectedURL: "https://github.com/ravocean/ThreatLevelMidnight/wiki",
		},
		{
			name:          "file argument",
			opts:          BrowseOptions{SelectorArg: "path/to/file.txt"},
			baseRepo:      ghrepo.New("ken", "mrprofessor"),
			defaultBranch: "main",
			expectedURL:   "https://github.com/ken/mrprofessor/tree/main/path/to/file.txt",
		},
		{
			name: "issue argument",
			opts: BrowseOptions{
				SelectorArg: "217",
			},
			baseRepo:    ghrepo.New("kevin", "MinTy"),
			expectedURL: "https://github.com/kevin/MinTy/issues/217",
		},
		{
			name: "branch flag",
			opts: BrowseOptions{
				Branch: "trunk",
			},
			baseRepo:    ghrepo.New("jlsestak", "CouldNotThinkOfARepoName"),
			expectedURL: "https://github.com/jlsestak/CouldNotThinkOfARepoName/tree/trunk/",
		},
		{
			name: "branch flag with file",
			opts: BrowseOptions{
				Branch:      "trunk",
				SelectorArg: "main.go",
			},
			baseRepo:    ghrepo.New("bchadwic", "LedZeppelinIV"),
			expectedURL: "https://github.com/bchadwic/LedZeppelinIV/tree/trunk/main.go",
		},
		{
			name: "file with line number",
			opts: BrowseOptions{
				SelectorArg: "path/to/file.txt:32",
			},
			baseRepo:      ghrepo.New("ravocean", "angur"),
			defaultBranch: "trunk",
			expectedURL:   "https://github.com/ravocean/angur/tree/trunk/path/to/file.txt#L32",
		},
		{
			name: "file with line range",
			opts: BrowseOptions{
				SelectorArg: "path/to/file.txt:32-40",
			},
			baseRepo:      ghrepo.New("ravocean", "angur"),
			defaultBranch: "trunk",
			expectedURL:   "https://github.com/ravocean/angur/tree/trunk/path/to/file.txt#L32-L40",
		},
		{
			name: "file with invalid line number",
			opts: BrowseOptions{
				SelectorArg: "path/to/file.txt:32:32",
			},
			baseRepo: ghrepo.New("ttran112", "ttrain211"),
			wantsErr: true,
		},
		{
			name: "file with invalid line range",
			opts: BrowseOptions{
				SelectorArg: "path/to/file.txt:32-abc",
			},
			baseRepo: ghrepo.New("ttran112", "ttrain211"),
			wantsErr: true,
		},
		{
			name: "branch with issue number",
			opts: BrowseOptions{
				SelectorArg: "217",
				Branch:      "trunk",
			},
			baseRepo:    ghrepo.New("ken", "grc"),
			wantsErr:    false,
			expectedURL: "https://github.com/ken/grc/issues/217",
		},
		{
			name: "opening branch file with line number",
			opts: BrowseOptions{
				Branch:      "first-browse-pull",
				SelectorArg: "browse.go:32",
			},
			baseRepo:    ghrepo.New("github", "ThankYouGitHub"),
			wantsErr:    false,
			expectedURL: "https://github.com/github/ThankYouGitHub/tree/first-browse-pull/browse.go#L32",
		},
		{
			name: "no browser with branch file and line number",
			opts: BrowseOptions{
				Branch:        "3-0-stable",
				SelectorArg:   "init.rb:6",
				NoBrowserFlag: true,
			},
			baseRepo:    ghrepo.New("mislav", "will_paginate"),
			wantsErr:    false,
			expectedURL: "https://github.com/mislav/will_paginate/tree/3-0-stable/init.rb#L6",
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

			if opts.NoBrowserFlag {
				assert.Equal(t, fmt.Sprintf("%s\n", tt.expectedURL), stdout.String())
				assert.Equal(t, "", stderr.String())
				browser.Verify(t, "")
			} else {
				assert.Equal(t, "", stdout.String())
				assert.Equal(t, "", stderr.String())
				browser.Verify(t, tt.expectedURL)
			}
		})
	}
}

func Test_parseFileArg(t *testing.T) {
	tests := []struct {
		name            string
		arg             string
		errorExpected   bool
		expectedFileArg string
		stderrExpected  string
	}{
		{
			name:            "non line number",
			arg:             "main.go",
			errorExpected:   false,
			expectedFileArg: "main.go",
		},
		{
			name:            "line number",
			arg:             "main.go:32",
			errorExpected:   false,
			expectedFileArg: "main.go#L32",
		},
		{
			name:           "non line number error",
			arg:            "ma:in.go",
			errorExpected:  true,
			stderrExpected: "invalid line number after colon\nUse 'gh browse --help' for more information about browse\n",
		},
	}
	for _, tt := range tests {
		fileArg, err := parseFileArg(tt.arg)
		if tt.errorExpected {
			assert.Equal(t, err.Error(), tt.stderrExpected)
		} else {
			assert.Equal(t, err, nil)
			assert.Equal(t, tt.expectedFileArg, fileArg)
		}
	}
}
