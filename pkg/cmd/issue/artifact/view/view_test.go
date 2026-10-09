package view

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/artifact/client"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/jsonfieldstest"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViewJSONFields(t *testing.T) {
	jsonfieldstest.ExpectCommandToSupportJSONFields(t, NewCmdView, []string{
		"body",
		"bodyHtml",
		"createdAt",
		"creator",
		"description",
		"id",
		"name",
		"number",
		"type",
		"updatedAt",
		"updatedByActor",
		"versions",
	})
}

func TestNewCmdView(t *testing.T) {
	tests := []struct {
		name        string
		args        string
		ghRepo      string
		wantOpts    ViewOptions
		wantRepo    string
		wantErr     string
		wantFlagErr bool
	}{
		{
			name:     "an issue number and an artifact number",
			args:     "142 2",
			wantOpts: ViewOptions{IssueNumber: 142, ArtifactNumber: 2},
			wantRepo: "OWNER/REPO",
		},
		{
			name:     "an issue URL names the repository",
			args:     "https://github.com/monalisa/monas-cafe/issues/142 2",
			wantOpts: ViewOptions{IssueNumber: 142, ArtifactNumber: 2},
			wantRepo: "monalisa/monas-cafe",
		},
		{
			name:     "-R selects the repository",
			args:     "142 2 -R monalisa/monas-cafe",
			wantOpts: ViewOptions{IssueNumber: 142, ArtifactNumber: 2},
			wantRepo: "monalisa/monas-cafe",
		},
		{
			name:     "GH_REPO selects the repository",
			args:     "142 2",
			ghRepo:   "monalisa/monas-cafe",
			wantOpts: ViewOptions{IssueNumber: 142, ArtifactNumber: 2},
			wantRepo: "monalisa/monas-cafe",
		},
		{
			name:     "--version",
			args:     "142 2 --version 5",
			wantOpts: ViewOptions{IssueNumber: 142, ArtifactNumber: 2, Version: 5},
			wantRepo: "OWNER/REPO",
		},
		{
			name:        "--version 0",
			args:        "142 2 --version 0",
			wantErr:     "invalid version: 0",
			wantFlagErr: true,
		},
		{
			name:        "a negative --version",
			args:        "142 2 --version -1",
			wantErr:     "invalid version: -1",
			wantFlagErr: true,
		},
		{
			name:        "--version with --json",
			args:        "142 2 --version 5 --json body",
			wantErr:     "specify only one of `--version` or `--json`",
			wantFlagErr: true,
		},
		{
			name:        "--version with --web",
			args:        "142 3 --version 1 --web",
			wantErr:     "specify only one of `--version` or `--web`",
			wantFlagErr: true,
		},
		{
			name:     "--web",
			args:     "142 3 --web",
			wantOpts: ViewOptions{IssueNumber: 142, ArtifactNumber: 3, WebMode: true},
			wantRepo: "OWNER/REPO",
		},
		{
			name:     "-w",
			args:     "142 3 -w",
			wantOpts: ViewOptions{IssueNumber: 142, ArtifactNumber: 3, WebMode: true},
			wantRepo: "OWNER/REPO",
		},
		{
			name:    "--web with --json",
			args:    "142 3 --web --json body",
			wantErr: "cannot use `--web` with `--json`",
		},
		{
			name:    "--json with url, which isn't a field",
			args:    "142 2 --json number,url",
			wantErr: "Unknown JSON field: \"url\"\nAvailable fields:\n  body\n  bodyHtml\n  createdAt\n  creator\n  description\n  id\n  name\n  number\n  type\n  updatedAt\n  updatedByActor\n  versions",
		},
		{
			name:    "no artifact number",
			args:    "142",
			wantErr: "accepts 2 arg(s), received 1",
		},
		{
			name:        "an issue argument that isn't an issue",
			args:        "OAuth 2",
			wantErr:     `invalid issue format: "OAuth"`,
			wantFlagErr: true,
		},
		{
			name:        "an artifact number that isn't a number",
			args:        "142 OAuth",
			wantErr:     `invalid artifact number: "OAuth"`,
			wantFlagErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GH_REPO", tt.ghRepo)
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
			}

			var gotOpts *ViewOptions
			cmd := NewCmdView(f, func(opts *ViewOptions) error {
				gotOpts = opts
				return nil
			})
			// gh issue adds -R and its hook, which replaces f.BaseRepo before RunE.
			cmdutil.EnableRepoOverride(cmd, f)

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)
			cmd.SetArgs(argv)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				if tt.wantFlagErr {
					var flagErr *cmdutil.FlagError
					require.ErrorAs(t, err, &flagErr)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOpts.IssueNumber, gotOpts.IssueNumber)
			assert.Equal(t, tt.wantOpts.ArtifactNumber, gotOpts.ArtifactNumber)
			assert.Equal(t, tt.wantOpts.Version, gotOpts.Version)
			assert.Equal(t, tt.wantOpts.WebMode, gotOpts.WebMode)

			repo, err := gotOpts.BaseRepo()
			require.NoError(t, err)
			assert.Equal(t, tt.wantRepo, ghrepo.FullName(repo))
		})
	}
}

func TestViewRun(t *testing.T) {
	// Markdown rendering reads these, so the operator's settings can't change
	// the output.
	t.Setenv("GLAMOUR_STYLE", "")
	t.Setenv("GH_MDWIDTH", "")

	now := time.Date(2026, 9, 24, 23, 30, 0, 0, time.UTC)
	at := func(value string) *time.Time {
		ts, err := time.Parse(time.RFC3339, value)
		require.NoError(t, err)
		return &ts
	}
	copilot := &client.Actor{ID: 2, Login: "Copilot"}
	monalisa := &client.Actor{ID: 1, Login: "monalisa"}
	version := func(number int, name string, actor *client.Actor, savedAt string, body string) client.Version {
		return client.Version{Version: number, Name: name, Body: body, Actor: actor, CreatedAt: at(savedAt)}
	}

	// Artifact 2 on the spec's issue 142, with its 12 versions. Version 9
	// renamed it.
	const oauthPlanBody = "# OAuth callback plan\n\n1. Register the callback URL.\n2. Wire up `order-history` sync."
	oauthPlan := &client.ArtifactWithVersions{
		ID:             6626,
		Number:         2,
		Type:           "generic",
		Name:           "OAuth callback plan",
		Body:           oauthPlanBody,
		Creator:        copilot,
		UpdatedByActor: monalisa,
		CreatedAt:      at("2026-09-21T23:00:00Z"),
		UpdatedAt:      at("2026-09-24T21:00:00Z"),
		Versions: []client.Version{
			version(12, "OAuth callback plan", monalisa, "2026-09-24T21:00:00Z", oauthPlanBody),
			version(11, "OAuth callback plan", copilot, "2026-09-24T18:00:00Z", ""),
			version(10, "OAuth callback plan", copilot, "2026-09-24T16:00:00Z", ""),
			version(9, "OAuth callback plan", monalisa, "2026-09-23T21:00:00Z", ""),
			version(8, "OAuth plan", copilot, "2026-09-23T18:00:00Z", ""),
			version(7, "OAuth plan", copilot, "2026-09-23T08:00:00Z", ""),
			version(6, "OAuth plan", copilot, "2026-09-22T22:00:00Z", ""),
			version(5, "OAuth plan", monalisa, "2026-09-22T20:00:00Z", "# OAuth plan\n\n1. Register the callback URL."),
			version(4, "OAuth plan", copilot, "2026-09-22T10:00:00Z", ""),
			version(3, "OAuth plan", copilot, "2026-09-21T23:20:00Z", ""),
			version(2, "OAuth plan", copilot, "2026-09-21T23:10:00Z", ""),
			version(1, "OAuth plan", copilot, "2026-09-21T23:00:00Z", "# OAuth plan"),
		},
	}

	// Artifact 3 on the spec's issue 142, a link saved once.
	const runbookURL = "https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook"
	runbook := &client.ArtifactWithVersions{
		ID:             6627,
		Number:         3,
		Type:           "link",
		Name:           "Staging OAuth runbook",
		Body:           runbookURL,
		Creator:        copilot,
		UpdatedByActor: copilot,
		CreatedAt:      at("2026-09-23T22:00:00Z"),
		UpdatedAt:      at("2026-09-23T22:00:00Z"),
		Versions: []client.Version{
			version(1, "Staging OAuth runbook", copilot, "2026-09-23T22:00:00Z", runbookURL),
		},
	}

	// The same link after monalisa moved it to the production runbook.
	const productionRunbookURL = "https://github.com/monalisa/monas-cafe/wiki/Production-OAuth-runbook"
	movedRunbook := &client.ArtifactWithVersions{
		Number:         3,
		Type:           "link",
		Name:           "Production OAuth runbook",
		Body:           productionRunbookURL,
		Creator:        copilot,
		UpdatedByActor: monalisa,
		CreatedAt:      at("2026-09-23T22:00:00Z"),
		UpdatedAt:      at("2026-09-24T21:00:00Z"),
		Versions: []client.Version{
			version(2, "Production OAuth runbook", monalisa, "2026-09-24T21:00:00Z", productionRunbookURL),
			version(1, "Staging OAuth runbook", copilot, "2026-09-23T22:00:00Z", runbookURL),
		},
	}

	// A link whose users the API returned as null, and whose first version
	// has no time either. Its name needs cleaning up.
	ghosts := &client.ArtifactWithVersions{
		Number:    3,
		Type:      "link",
		Name:      "  Staging\tOAuth\n\n runbook ",
		Body:      runbookURL,
		CreatedAt: at("2026-09-23T22:00:00Z"),
		UpdatedAt: at("2026-09-24T21:00:00Z"),
		Versions: []client.Version{
			{Version: 2, Name: "Staging OAuth runbook", Body: runbookURL, CreatedAt: at("2026-09-24T21:00:00Z")},
			{Version: 1, Name: "Staging OAuth runbook", Body: runbookURL, Actor: copilot},
		},
	}

	// A link whose users and times the API returned as null, with no
	// versions.
	nulls := &client.ArtifactWithVersions{
		Number: 3, Type: "link", Name: "Staging OAuth runbook", Body: runbookURL,
	}

	link := func(body string) *client.ArtifactWithVersions {
		return &client.ArtifactWithVersions{
			Number: 3, Type: "link", Name: "Staging OAuth runbook", Body: body,
		}
	}

	jsonExporter := func(fields ...string) cmdutil.Exporter {
		e := cmdutil.NewJSONExporter()
		e.SetFields(fields)
		return e
	}

	// A pager command with an unterminated quote fails before any process
	// starts, so a row can see whether view started the pager.
	const brokenPager = "'"

	tests := []struct {
		name          string
		repo          ghrepo.Interface
		opts          ViewOptions
		tty           bool
		color         bool
		pager         string
		isPullRequest bool
		lookupErr     error
		artifact      *client.ArtifactWithVersions
		getErr        error
		wantCalls     []string
		wantStdout    string
		wantStderr    string
		wantBrowsed   string
		wantErr       string
	}{
		{
			name:      "a document in a terminal, with its edit history",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 2},
			tty:       true,
			artifact:  oauthPlan,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: heredoc.Doc(`
				OAuth callback plan monalisa/monas-cafe#142
				Artifact 2 • generic • Copilot created about 3 days ago • monalisa updated about 2 hours ago


				  # OAuth callback plan                                                       
				                                                                              
				  1. Register the callback URL.                                               
				  2. Wire up order-history sync.                                              


				Edit history
				12  monalisa  about 2 hours ago  Current
				11  Copilot   about 5 hours ago
				10  Copilot   about 7 hours ago
				9   monalisa  about 1 day ago  
				8   Copilot   about 1 day ago  
				7   Copilot   about 1 day ago  
				6   Copilot   about 2 days ago 
				5   monalisa  about 2 days ago 
				4   Copilot   about 2 days ago 
				3   Copilot   about 3 days ago 
				2   Copilot   about 3 days ago 
				1   Copilot   about 3 days ago 

				View this issue on GitHub: https://github.com/monalisa/monas-cafe/issues/142
			`),
		},
		{
			name:      "a link in a terminal shows its URL, and leaves out an update when it was never edited",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 3},
			tty:       true,
			artifact:  runbook,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantStdout: heredoc.Doc(`
				Staging OAuth runbook monalisa/monas-cafe#142
				Artifact 3 • link • Copilot created about 1 day ago

				  https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook

				Edit history
				1  Copilot  about 1 day ago  Current

				View this issue on GitHub: https://github.com/monalisa/monas-cafe/issues/142
			`),
		},
		{
			name:      "colors in a terminal",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 3},
			tty:       true,
			color:     true,
			artifact:  runbook,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantStdout: "\x1b[0;1;39mStaging OAuth runbook\x1b[0m monalisa/monas-cafe#142\n" +
				"\x1b[38;5;242mArtifact 3 • link • Copilot created about 1 day ago\x1b[0m\n" +
				"\n" +
				"  https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook\n" +
				"\n" +
				"\x1b[0;1;39mEdit history\x1b[0m\n" +
				"1  Copilot  \x1b[38;5;242mabout 1 day ago\x1b[0m  \x1b[0;1;36mCurrent\x1b[0m\n" +
				"\n" +
				"\x1b[38;5;242mView this issue on GitHub: https://github.com/monalisa/monas-cafe/issues/142\x1b[0m\n",
		},
		{
			name:      "--version shows an earlier version, marked Viewing",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 2, Version: 5},
			tty:       true,
			artifact:  oauthPlan,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: heredoc.Doc(`
				OAuth plan monalisa/monas-cafe#142
				Artifact 2 • generic • Version 5 • monalisa updated about 2 days ago


				  # OAuth plan                                                                
				                                                                              
				  1. Register the callback URL.                                               


				Edit history
				12  monalisa  about 2 hours ago  Current
				11  Copilot   about 5 hours ago
				10  Copilot   about 7 hours ago
				9   monalisa  about 1 day ago  
				8   Copilot   about 1 day ago  
				7   Copilot   about 1 day ago  
				6   Copilot   about 2 days ago 
				5   monalisa  about 2 days ago   Viewing
				4   Copilot   about 2 days ago 
				3   Copilot   about 3 days ago 
				2   Copilot   about 3 days ago 
				1   Copilot   about 3 days ago 

				View this issue on GitHub: https://github.com/monalisa/monas-cafe/issues/142
			`),
		},
		{
			name:      "--version 1 shows who created the artifact",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 3, Version: 1},
			tty:       true,
			artifact:  movedRunbook,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantStdout: heredoc.Doc(`
				Staging OAuth runbook monalisa/monas-cafe#142
				Artifact 3 • link • Version 1 • Copilot created about 1 day ago

				  https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook

				Edit history
				2  monalisa  about 2 hours ago  Current
				1  Copilot   about 1 day ago    Viewing

				View this issue on GitHub: https://github.com/monalisa/monas-cafe/issues/142
			`),
		},
		{
			name:      "--version with the current version's number shows the current version",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 3, Version: 2},
			tty:       true,
			artifact:  movedRunbook,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantStdout: heredoc.Doc(`
				Production OAuth runbook monalisa/monas-cafe#142
				Artifact 3 • link • Copilot created about 1 day ago • monalisa updated about 2 hours ago

				  https://github.com/monalisa/monas-cafe/wiki/Production-OAuth-runbook

				Edit history
				2  monalisa  about 2 hours ago  Current
				1  Copilot   about 1 day ago  

				View this issue on GitHub: https://github.com/monalisa/monas-cafe/issues/142
			`),
		},
		{
			name:      "--version the API didn't return",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 2, Version: 99},
			tty:       true,
			artifact:  oauthPlan,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantErr:   "version 99 not found for artifact 2",
		},
		{
			name:      "null users show as ghost when their time exists, and names are cleaned up",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 3},
			tty:       true,
			artifact:  ghosts,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantStdout: heredoc.Doc(`
				Staging OAuth runbook monalisa/monas-cafe#142
				Artifact 3 • link • ghost created about 1 day ago • ghost updated about 2 hours ago

				  https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook

				Edit history
				2  ghost    about 2 hours ago  Current
				1  Copilot                   

				View this issue on GitHub: https://github.com/monalisa/monas-cafe/issues/142
			`),
		},
		{
			name:      "null times leave out their parts, and no versions leave out the edit history",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 3},
			tty:       true,
			artifact:  nulls,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantStdout: heredoc.Doc(`
				Staging OAuth runbook monalisa/monas-cafe#142
				Artifact 3 • link

				  https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook

				View this issue on GitHub: https://github.com/monalisa/monas-cafe/issues/142
			`),
		},
		{
			name:      "--version on a version with a null time leaves out who saved it",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 3, Version: 1},
			tty:       true,
			artifact:  ghosts,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantStdout: heredoc.Doc(`
				Staging OAuth runbook monalisa/monas-cafe#142
				Artifact 3 • link • Version 1

				  https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook

				Edit history
				2  ghost    about 2 hours ago  Current
				1  Copilot                     Viewing

				View this issue on GitHub: https://github.com/monalisa/monas-cafe/issues/142
			`),
		},
		{
			name: "--version on a version with a null user shows ghost",
			opts: ViewOptions{IssueNumber: 142, ArtifactNumber: 3, Version: 2},
			tty:  true,
			artifact: &client.ArtifactWithVersions{
				Number: 3, Type: "link", Name: "Production OAuth runbook", Body: productionRunbookURL,
				Versions: []client.Version{
					version(3, "Production OAuth runbook", monalisa, "2026-09-24T21:00:00Z", productionRunbookURL),
					version(2, "Staging OAuth runbook", nil, "2026-09-24T18:00:00Z", runbookURL),
					version(1, "Staging OAuth runbook", copilot, "2026-09-23T22:00:00Z", runbookURL),
				},
			},
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantStdout: heredoc.Doc(`
				Staging OAuth runbook monalisa/monas-cafe#142
				Artifact 3 • link • Version 2 • ghost updated about 5 hours ago

				  https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook

				Edit history
				3  monalisa  about 2 hours ago  Current
				2  ghost     about 5 hours ago  Viewing
				1  Copilot   about 1 day ago  

				View this issue on GitHub: https://github.com/monalisa/monas-cafe/issues/142
			`),
		},
		{
			name:      "piped output",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 2},
			artifact:  oauthPlan,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: heredoc.Doc(`
				name:	OAuth callback plan
				number:	2
				version:	12
				type:	generic
				creator:	Copilot
				created:	2026-09-21T23:00:00Z
				updated-by:	monalisa
				updated:	2026-09-24T21:00:00Z
				--
				# OAuth callback plan

				1. Register the callback URL.
				2. Wire up ` + "`order-history`" + ` sync.
			`),
		},
		{
			name:      "piped output with --version",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 2, Version: 5},
			artifact:  oauthPlan,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: heredoc.Doc(`
				name:	OAuth plan
				number:	2
				version:	5
				type:	generic
				creator:	Copilot
				created:	2026-09-21T23:00:00Z
				updated-by:	monalisa
				updated:	2026-09-22T20:00:00Z
				--
				# OAuth plan

				1. Register the callback URL.
			`),
		},
		{
			name:      "piped output cleans up names and leaves null users empty",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 3},
			artifact:  ghosts,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantStdout: "name:\tStaging OAuth runbook\n" +
				"number:\t3\n" +
				"version:\t2\n" +
				"type:\tlink\n" +
				"creator:\t\n" +
				"created:\t2026-09-23T22:00:00Z\n" +
				"updated-by:\t\n" +
				"updated:\t2026-09-24T21:00:00Z\n" +
				"--\n" +
				"https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook\n",
		},
		{
			name:      "piped output leaves null times and a missing version empty",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 3},
			artifact:  nulls,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantStdout: "name:\tStaging OAuth runbook\n" +
				"number:\t3\n" +
				"version:\t\n" +
				"type:\tlink\n" +
				"creator:\t\n" +
				"created:\t\n" +
				"updated-by:\t\n" +
				"updated:\t\n" +
				"--\n" +
				"https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook\n",
		},
		{
			name:      "piped output with --version takes who saved it and when from that version",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 3, Version: 1},
			artifact:  ghosts,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantStdout: "name:\tStaging OAuth runbook\n" +
				"number:\t3\n" +
				"version:\t1\n" +
				"type:\tlink\n" +
				"creator:\t\n" +
				"created:\t2026-09-23T22:00:00Z\n" +
				"updated-by:\tCopilot\n" +
				"updated:\t\n" +
				"--\n" +
				"https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook\n",
		},
		{
			name:       "--json",
			opts:       ViewOptions{IssueNumber: 142, ArtifactNumber: 2, Exporter: jsonExporter("number", "name", "type")},
			tty:        true,
			artifact:   oauthPlan,
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: `{"name":"OAuth callback plan","number":2,"type":"generic"}` + "\n",
		},
		{
			name:      "--json with versions",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 3, Exporter: jsonExporter("versions")},
			artifact:  movedRunbook,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantStdout: `{"versions":[` +
				`{"actor":{"id":1,"login":"monalisa"},"body":"https://github.com/monalisa/monas-cafe/wiki/Production-OAuth-runbook","bodyHtml":"","createdAt":"2026-09-24T21:00:00Z","name":"Production OAuth runbook","version":2},` +
				`{"actor":{"id":2,"login":"Copilot"},"body":"https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook","bodyHtml":"","createdAt":"2026-09-23T22:00:00Z","name":"Staging OAuth runbook","version":1}` +
				`]}` + "\n",
		},
		{
			name:       "--json keeps nulls and exact names",
			opts:       ViewOptions{IssueNumber: 142, ArtifactNumber: 3, Exporter: jsonExporter("name", "creator", "versions")},
			artifact:   ghosts,
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantStdout: `{"creator":null,"name":"  Staging\tOAuth\n\n runbook ","versions":[{"actor":null,"body":"https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook","bodyHtml":"","createdAt":"2026-09-24T21:00:00Z","name":"Staging OAuth runbook","version":2},{"actor":{"id":2,"login":"Copilot"},"body":"https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook","bodyHtml":"","createdAt":null,"name":"Staging OAuth runbook","version":1}]}` + "\n",
		},
		{
			name:      "a pager that fails to start is reported, and the view still prints",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 3},
			tty:       true,
			pager:     brokenPager,
			artifact:  runbook,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantStdout: heredoc.Doc(`
				Staging OAuth runbook monalisa/monas-cafe#142
				Artifact 3 • link • Copilot created about 1 day ago

				  https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook

				Edit history
				1  Copilot  about 1 day ago  Current

				View this issue on GitHub: https://github.com/monalisa/monas-cafe/issues/142
			`),
			wantStderr: "error starting pager: EOF found when expecting closing quote\n",
		},
		{
			name:       "--json in a terminal doesn't start the pager",
			opts:       ViewOptions{IssueNumber: 142, ArtifactNumber: 3, Exporter: jsonExporter("number")},
			tty:        true,
			pager:      brokenPager,
			artifact:   runbook,
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantStdout: `{"number":3}` + "\n",
		},
		{
			name:        "--web opens a link's URL",
			opts:        ViewOptions{IssueNumber: 142, ArtifactNumber: 3, WebMode: true},
			tty:         true,
			artifact:    runbook,
			wantCalls:   []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantStderr:  "Opening https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook in your browser.\n",
			wantBrowsed: runbookURL,
		},
		{
			name:        "--web when piped opens the URL without a message",
			opts:        ViewOptions{IssueNumber: 142, ArtifactNumber: 3, WebMode: true},
			artifact:    runbook,
			wantCalls:   []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantBrowsed: runbookURL,
		},
		{
			name:        "--web shows the whole URL, trimmed",
			opts:        ViewOptions{IssueNumber: 142, ArtifactNumber: 3, WebMode: true},
			tty:         true,
			artifact:    link(" http://staging.monas-cafe.example:8080/oauth?step=callback#retry\n"),
			wantCalls:   []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantStderr:  "Opening http://staging.monas-cafe.example:8080/oauth?step=callback#retry in your browser.\n",
			wantBrowsed: "http://staging.monas-cafe.example:8080/oauth?step=callback#retry",
		},
		{
			name:      "--web refuses a document",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 2, WebMode: true},
			tty:       true,
			artifact:  oauthPlan,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantErr:   "artifact 2 is not a link; --web only opens link artifacts",
		},
		{
			name:      "--web refuses a link that isn't an http(s) URL",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 3, WebMode: true},
			tty:       true,
			artifact:  link("file:///etc/hosts"),
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantErr:   "artifact 3 doesn't link to an http(s) URL; --web only opens http(s) URLs",
		},
		{
			name: "a type gh doesn't know is refused",
			opts: ViewOptions{IssueNumber: 142, ArtifactNumber: 7},
			tty:  true,
			artifact: &client.ArtifactWithVersions{
				Number: 7, Type: "bad-type", Name: "Espresso checklist", Body: "Descale weekly.",
			},
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 7"},
			wantErr:   `artifact 7 has type "bad-type", which this version of gh doesn't support; upgrade gh`,
		},
		{
			name: "a type gh doesn't know is refused with --json too",
			opts: ViewOptions{IssueNumber: 142, ArtifactNumber: 7, Exporter: jsonExporter("number", "type")},
			artifact: &client.ArtifactWithVersions{
				Number: 7, Type: "bad-type", Name: "Espresso checklist", Body: "Descale weekly.",
			},
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 7"},
			wantErr:   `artifact 7 has type "bad-type", which this version of gh doesn't support; upgrade gh`,
		},
		{
			name:          "a pull request is refused before any artifact request",
			opts:          ViewOptions{IssueNumber: 158, ArtifactNumber: 2},
			tty:           true,
			isPullRequest: true,
			wantCalls:     []string{"IsPullRequest monalisa/monas-cafe#158"},
			wantErr:       "monalisa/monas-cafe#158 is a pull request; artifacts are only supported on issues",
		},
		{
			name:      "the lookup's error",
			opts:      ViewOptions{IssueNumber: 999, ArtifactNumber: 2},
			tty:       true,
			lookupErr: errors.New("GraphQL: Could not resolve to an issue or pull request with the number of 999. (repository.issue)"),
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#999"},
			wantErr:   "GraphQL: Could not resolve to an issue or pull request with the number of 999. (repository.issue)",
		},
		{
			name:    "GitHub Enterprise Server is refused before any request",
			repo:    ghrepo.NewWithHost("monalisa", "monas-cafe", "ghes.monas-cafe.example"),
			opts:    ViewOptions{IssueNumber: 142, ArtifactNumber: 2},
			tty:     true,
			wantErr: "issue artifacts are not supported on GitHub Enterprise Server",
		},
		{
			name:     "a ghe.com host is supported",
			repo:     ghrepo.NewWithHost("monalisa", "monas-cafe", "monas-cafe.ghe.com"),
			opts:     ViewOptions{IssueNumber: 142, ArtifactNumber: 3},
			tty:      true,
			artifact: runbook,
			wantCalls: []string{
				"IsPullRequest monas-cafe.ghe.com/monalisa/monas-cafe#142",
				"Get monas-cafe.ghe.com/monalisa/monas-cafe#142 artifact 3",
			},
			wantStdout: heredoc.Doc(`
				Staging OAuth runbook monalisa/monas-cafe#142
				Artifact 3 • link • Copilot created about 1 day ago

				  https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook

				Edit history
				1  Copilot  about 1 day ago  Current

				View this issue on GitHub: https://monas-cafe.ghe.com/monalisa/monas-cafe/issues/142
			`),
		},
		{
			name:      "an API error is returned as the API gives it",
			opts:      ViewOptions{IssueNumber: 142, ArtifactNumber: 9},
			tty:       true,
			getErr:    errors.New("HTTP 404: Not Found (https://api.github.com/repos/monalisa/monas-cafe/issues/142/artifacts/9)"),
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 9"},
			wantErr:   "HTTP 404: Not Found (https://api.github.com/repos/monalisa/monas-cafe/issues/142/artifacts/9)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)
			ios.SetColorEnabled(tt.color)
			ios.SetPager(tt.pager)

			repo := tt.repo
			if repo == nil {
				repo = ghrepo.New("monalisa", "monas-cafe")
			}

			var calls []string
			issueRef := func(r ghrepo.Interface, number int) string {
				if r.RepoHost() != "github.com" {
					return fmt.Sprintf("%s/%s#%d", r.RepoHost(), ghrepo.FullName(r), number)
				}
				return fmt.Sprintf("%s#%d", ghrepo.FullName(r), number)
			}
			mock := &client.ArtifactClientMock{
				IsPullRequestFunc: func(r ghrepo.Interface, number int) (bool, error) {
					calls = append(calls, "IsPullRequest "+issueRef(r, number))
					return tt.isPullRequest, tt.lookupErr
				},
				GetFunc: func(r ghrepo.Interface, issueNumber int, number int) (*client.ArtifactWithVersions, error) {
					calls = append(calls, fmt.Sprintf("Get %s artifact %d", issueRef(r, issueNumber), number))
					return tt.artifact, tt.getErr
				},
			}
			browser := &browser.Stub{}

			opts := tt.opts
			opts.IO = ios
			opts.BaseRepo = func() (ghrepo.Interface, error) { return repo, nil }
			opts.Client = func() (client.ArtifactClient, error) { return mock, nil }
			opts.Browser = browser
			opts.Now = func() time.Time { return now }

			err := viewRun(&opts)

			assert.Equal(t, tt.wantCalls, calls)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
			browser.Verify(t, tt.wantBrowsed)
		})
	}
}
