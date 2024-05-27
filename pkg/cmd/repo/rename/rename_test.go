package rename

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdRename(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		output  RenameOptions
		errMsg  string
		tty     bool
		wantErr bool
	}{
		{
			name:    "no arguments no tty",
			input:   "",
			errMsg:  "new name argument required when not running interactively",
			wantErr: true,
		},
		{
			name:  "one argument no tty confirmed",
			input: "REPO --yes",
			output: RenameOptions{
				newRepoSelector: "REPO",
			},
		},
		{
			name:    "one argument no tty",
			input:   "REPO",
			errMsg:  "--yes required when passing a single argument",
			wantErr: true,
		},
		{
			name:  "one argument tty confirmed",
			input: "REPO --yes",
			tty:   true,
			output: RenameOptions{
				newRepoSelector: "REPO",
			},
		},
		{
			name:  "one argument tty",
			input: "REPO",
			tty:   true,
			output: RenameOptions{
				newRepoSelector: "REPO",
				DoConfirm:       true,
			},
		},
		{
			name:  "full flag argument",
			input: "--repo OWNER/REPO NEW_REPO",
			output: RenameOptions{
				newRepoSelector: "NEW_REPO",
			},
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)
			var gotOpts *RenameOptions
			cmd := NewCmdRename(f, func(opts *RenameOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.output.newRepoSelector, gotOpts.newRepoSelector)
		})
	}
}

func TestRenameRun(t *testing.T) {
	testCases := []struct {
		name        string
		opts        RenameOptions
		httpStubs   func(*httpmock.Registry)
		execStubs   func(*run.CommandStubber)
		promptStubs func(*prompter.MockPrompter)
		wantOut     string
		tty         bool
	}{
		{
			name:    "none argument",
			wantOut: "✓ Renamed repository OWNER/NEW_REPO\n✓ Updated the \"origin\" remote\n",
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterInput("Rename OWNER/REPO to:", func(_, _ string) (string, error) {
					return "NEW_REPO", nil
				})
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(200, `{"name":"NEW_REPO","owner":{"login":"OWNER"}}`))
			},
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git remote set-url origin https://github.com/OWNER/NEW_REPO.git`, 0, "")
			},
			tty: true,
		},
		{
			name: "repo override",
			opts: RenameOptions{
				HasRepoOverride: true,
			},
			wantOut: "✓ Renamed repository OWNER/NEW_REPO\n",
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterInput("Rename OWNER/REPO to:", func(_, _ string) (string, error) {
					return "NEW_REPO", nil
				})
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(200, `{"name":"NEW_REPO","owner":{"login":"OWNER"}}`))
			},
			tty: true,
		},
		{
			name: "owner repo change name argument tty",
			opts: RenameOptions{
				newRepoSelector: "NEW_REPO",
			},
			wantOut: "✓ Renamed repository OWNER/NEW_REPO\n✓ Updated the \"origin\" remote\n",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(200, `{"name":"NEW_REPO","owner":{"login":"OWNER"}}`))
			},
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git remote set-url origin https://github.com/OWNER/NEW_REPO.git`, 0, "")
			},
			tty: true,
		},
		{
			name: "owner repo change name argument no tty",
			opts: RenameOptions{
				newRepoSelector: "NEW_REPO",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(200, `{"name":"NEW_REPO","owner":{"login":"OWNER"}}`))
			},
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git remote set-url origin https://github.com/OWNER/NEW_REPO.git`, 0, "")
			},
		},
		{
			name: "confirmation with yes",
			tty:  true,
			opts: RenameOptions{
				newRepoSelector: "NEW_REPO",
				DoConfirm:       true,
			},
			wantOut: "✓ Renamed repository OWNER/NEW_REPO\n✓ Updated the \"origin\" remote\n",
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterConfirm("Rename OWNER/REPO to NEW_REPO?", func(_ string, _ bool) (bool, error) {
					return true, nil
				})
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(200, `{"name":"NEW_REPO","owner":{"login":"OWNER"}}`))
			},
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git remote set-url origin https://github.com/OWNER/NEW_REPO.git`, 0, "")
			},
		},

		{
			name: "confirmation with no",
			tty:  true,
			opts: RenameOptions{
				newRepoSelector: "NEW_REPO",
				DoConfirm:       true,
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterConfirm("Rename OWNER/REPO to NEW_REPO?", func(_ string, _ bool) (bool, error) {
					return false, nil
				})
			},
			wantOut: "",
		},
	}

	for _, tt := range testCases {
		pm := prompter.NewMockPrompter(t)
		tt.opts.Prompter = pm
		if tt.promptStubs != nil {
			tt.promptStubs(pm)
		}

		repo, _ := ghrepo.FromFullName("OWNER/REPO")
		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			return repo, nil
		}

		tt.opts.Config = func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		}

		tt.opts.Remotes = func() (context.Remotes, error) {
			return []*context.Remote{
				{
					Remote: &git.Remote{Name: "origin"},
					Repo:   repo,
				},
			}, nil
		}

		cs, restoreRun := run.Stub()
		defer restoreRun(t)
		if tt.execStubs != nil {
			tt.execStubs(cs)
		}

		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		}
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		ios, _, stdout, _ := iostreams.Test()
		ios.SetStdinTTY(tt.tty)
		ios.SetStdoutTTY(tt.tty)
		tt.opts.IO = ios

		tt.opts.GitClient = &git.Client{GitPath: "some/path/git"}

		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)
			err := renameRun(&tt.opts)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantOut, stdout.String())
		})
	}
}
