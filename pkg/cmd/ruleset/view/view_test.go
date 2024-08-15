package view

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/ruleset/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
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
			name:  "no arguments",
			args:  "",
			isTTY: true,
			want: ViewOptions{
				ID:              "",
				WebMode:         false,
				IncludeParents:  true,
				InteractiveMode: true,
				Organization:    "",
			},
		},
		{
			name:  "only ID",
			args:  "3",
			isTTY: true,
			want: ViewOptions{
				ID:              "3",
				WebMode:         false,
				IncludeParents:  true,
				InteractiveMode: false,
				Organization:    "",
			},
		},
		{
			name:  "org",
			args:  "--org \"my-org\"",
			isTTY: true,
			want: ViewOptions{
				ID:              "",
				WebMode:         false,
				IncludeParents:  true,
				InteractiveMode: true,
				Organization:    "my-org",
			},
		},
		{
			name:  "web mode",
			args:  "--web",
			isTTY: true,
			want: ViewOptions{
				ID:              "",
				WebMode:         true,
				IncludeParents:  true,
				InteractiveMode: true,
				Organization:    "",
			},
		},
		{
			name:  "parents",
			args:  "--parents=false",
			isTTY: true,
			want: ViewOptions{
				ID:              "",
				WebMode:         false,
				IncludeParents:  false,
				InteractiveMode: true,
				Organization:    "",
			},
		},
		{
			name:    "repo and org specified",
			args:    "--org \"my-org\" -R \"owner/repo\"",
			isTTY:   true,
			wantErr: "only one of --repo and --org may be specified",
		},
		{
			name:    "invalid ID",
			args:    "1.5",
			isTTY:   true,
			wantErr: "invalid value for ruleset ID: 1.5 is not an integer",
		},
		{
			name:    "ID not provided and not TTY",
			args:    "",
			isTTY:   false,
			wantErr: "a ruleset ID must be provided when not running interactively",
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
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.want.ID, opts.ID)
			assert.Equal(t, tt.want.WebMode, opts.WebMode)
			assert.Equal(t, tt.want.IncludeParents, opts.IncludeParents)
			assert.Equal(t, tt.want.InteractiveMode, opts.InteractiveMode)
			assert.Equal(t, tt.want.Organization, opts.Organization)
		})
	}
}

func Test_viewRun(t *testing.T) {
	repoRulesetStdout := heredoc.Doc(`

	Test Ruleset
	ID: 42
	Source: my-owner/repo-name (Repository)
	Enforcement: Active
	You can bypass: pull requests only
	
	Bypass List
	- OrganizationAdmin (ID: 1), mode: always
	- RepositoryRole (ID: 5), mode: always
	
	Conditions
	- ref_name: [exclude: []] [include: [~ALL]] 
	
	Rules
	- commit_author_email_pattern: [name: ] [negate: false] [operator: ends_with] [pattern: @example.com] 
	- commit_message_pattern: [name: ] [negate: false] [operator: contains] [pattern: asdf] 
	- creation
	`)

	tests := []struct {
		name          string
		isTTY         bool
		opts          ViewOptions
		httpStubs     func(*httpmock.Registry)
		prompterStubs func(*prompter.MockPrompter)
		wantErr       string
		wantStdout    string
		wantStderr    string
		wantBrowse    string
	}{
		{
			name:  "view repo ruleset",
			isTTY: true,
			opts: ViewOptions{
				ID: "42",
			},
			wantStdout: repoRulesetStdout,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/my-owner/repo-name/rulesets/42"),
					httpmock.FileResponse("./fixtures/rulesetViewRepo.json"),
				)
			},
			wantStderr: "",
			wantBrowse: "",
		},
		{
			name:  "view org ruleset",
			isTTY: true,
			opts: ViewOptions{
				ID:           "74",
				Organization: "my-owner",
			},
			wantStdout: heredoc.Doc(`

			My Org Ruleset
			ID: 74
			Source: my-owner (Organization)
			Enforcement: Evaluate Mode (not enforced)
			
			Bypass List
			This ruleset cannot be bypassed
			
			Conditions
			- ref_name: [exclude: []] [include: [~ALL]] 
			- repository_name: [exclude: []] [include: [~ALL]] [protected: true] 
			
			Rules
			- commit_author_email_pattern: [name: ] [negate: false] [operator: ends_with] [pattern: @example.com] 
			- commit_message_pattern: [name: ] [negate: false] [operator: contains] [pattern: asdf] 
			- creation
			`),
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "orgs/my-owner/rulesets/74"),
					httpmock.FileResponse("./fixtures/rulesetViewOrg.json"),
				)
			},
			wantStderr: "",
			wantBrowse: "",
		},
		{
			name:  "interactive mode, repo, no rulesets found",
			isTTY: true,
			opts: ViewOptions{
				InteractiveMode: true,
			},
			wantStdout: "",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepoRulesetList\b`),
					httpmock.JSONResponse(shared.RulesetList{
						TotalCount: 0,
						Rulesets:   []shared.RulesetGraphQL{},
					}),
				)
			},
			wantErr:    "no rulesets found in my-owner/repo-name",
			wantStderr: "",
			wantBrowse: "",
		},
		{
			name:  "interactive mode, org, no rulesets found",
			isTTY: true,
			opts: ViewOptions{
				InteractiveMode: true,
				Organization:    "my-owner",
			},
			wantStdout: "",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query OrgRulesetList\b`),
					httpmock.JSONResponse(shared.RulesetList{
						TotalCount: 0,
						Rulesets:   []shared.RulesetGraphQL{},
					}),
				)
			},
			wantErr:    "no rulesets found in my-owner",
			wantStderr: "",
			wantBrowse: "",
		},
		{
			name:  "interactive mode, prompter",
			isTTY: true,
			opts: ViewOptions{
				InteractiveMode: true,
			},
			wantStdout: repoRulesetStdout,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepoRulesetList\b`),
					httpmock.FileResponse("./fixtures/rulesetViewMultiple.json"),
				)
				reg.Register(
					httpmock.REST("GET", "repos/my-owner/repo-name/rulesets/42"),
					httpmock.FileResponse("./fixtures/rulesetViewRepo.json"),
				)
			},
			prompterStubs: func(pm *prompter.MockPrompter) {
				const repoRuleset = "42: Test Ruleset | active | contains 3 rules | configured in my-owner/repo-name (repo)"
				pm.RegisterSelect("Which ruleset would you like to view?",
					[]string{
						"74: My Org Ruleset | evaluate | contains 3 rules | configured in my-owner (org)",
						repoRuleset,
					},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, repoRuleset)
					})
			},
		},
		{
			name:  "web mode, TTY, repo",
			isTTY: true,
			opts: ViewOptions{
				ID:      "42",
				WebMode: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/my-owner/repo-name/rulesets/42"),
					httpmock.FileResponse("./fixtures/rulesetViewRepo.json"),
				)
			},
			wantStdout: "Opening https://github.com/my-owner/repo-name/rules/42 in your browser.\n",
			wantStderr: "",
			wantBrowse: "https://github.com/my-owner/repo-name/rules/42",
		},
		{
			name:  "web mode, non-TTY, repo",
			isTTY: false,
			opts: ViewOptions{
				ID:      "42",
				WebMode: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/my-owner/repo-name/rulesets/42"),
					httpmock.FileResponse("./fixtures/rulesetViewRepo.json"),
				)
			},
			wantStdout: "",
			wantStderr: "",
			wantBrowse: "https://github.com/my-owner/repo-name/rules/42",
		},
		{
			name:  "web mode, TTY, org",
			isTTY: true,
			opts: ViewOptions{
				ID:           "74",
				Organization: "my-owner",
				WebMode:      true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "orgs/my-owner/rulesets/74"),
					httpmock.FileResponse("./fixtures/rulesetViewOrg.json"),
				)
			},
			wantStdout: "Opening https://github.com/organizations/my-owner/settings/rules/74 in your browser.\n",
			wantStderr: "",
			wantBrowse: "https://github.com/organizations/my-owner/settings/rules/74",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			pm := prompter.NewMockPrompter(t)
			if tt.prompterStubs != nil {
				tt.prompterStubs(pm)
			}
			tt.opts.Prompter = pm

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}

			tt.opts.IO = ios
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}

			// only set this if org is not set, because the repo isn't needed if --org is provided and
			// leaving it undefined will catch potential errors
			if tt.opts.Organization == "" {
				tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("my-owner/repo-name")
				}
			}

			browser := &browser.Stub{}
			tt.opts.Browser = browser

			err := viewRun(&tt.opts)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			if tt.wantBrowse != "" {
				browser.Verify(t, tt.wantBrowse)
			}

			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
