package list

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/artifact/client"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name        string
		args        string
		ghRepo      string
		wantOpts    ListOptions
		wantRepo    string
		wantFields  []string
		wantErr     string
		wantFlagErr bool
	}{
		{
			name:     "an issue number",
			args:     "142",
			wantOpts: ListOptions{IssueNumber: 142},
			wantRepo: "OWNER/REPO",
		},
		{
			name:     "an issue URL names the repository",
			args:     "https://github.com/monalisa/monas-cafe/issues/142",
			wantOpts: ListOptions{IssueNumber: 142},
			wantRepo: "monalisa/monas-cafe",
		},
		{
			name:     "-R selects the repository",
			args:     "142 -R monalisa/monas-cafe",
			wantOpts: ListOptions{IssueNumber: 142},
			wantRepo: "monalisa/monas-cafe",
		},
		{
			name:     "GH_REPO selects the repository",
			args:     "142",
			ghRepo:   "monalisa/monas-cafe",
			wantOpts: ListOptions{IssueNumber: 142},
			wantRepo: "monalisa/monas-cafe",
		},
		{
			name:     "an issue URL wins over GH_REPO",
			args:     "https://github.com/monalisa/monas-cafe/issues/142",
			ghRepo:   "OWNER/REPO",
			wantOpts: ListOptions{IssueNumber: 142},
			wantRepo: "monalisa/monas-cafe",
		},
		{
			name:     "an issue URL wins over -R",
			args:     "https://github.com/monalisa/monas-cafe/issues/142 -R OWNER/REPO",
			wantOpts: ListOptions{IssueNumber: 142},
			wantRepo: "monalisa/monas-cafe",
		},
		{
			name:     "--limit",
			args:     "142 --limit 2",
			wantOpts: ListOptions{IssueNumber: 142, Limit: 2},
			wantRepo: "OWNER/REPO",
		},
		{
			name:     "-L",
			args:     "142 -L 2",
			wantOpts: ListOptions{IssueNumber: 142, Limit: 2},
			wantRepo: "OWNER/REPO",
		},
		{
			name:        "--limit 0",
			args:        "142 --limit 0",
			wantErr:     "invalid limit: 0",
			wantFlagErr: true,
		},
		{
			name:        "a negative --limit",
			args:        "142 --limit -1",
			wantErr:     "invalid limit: -1",
			wantFlagErr: true,
		},
		{
			name:     "--type generic",
			args:     "142 --type generic",
			wantOpts: ListOptions{IssueNumber: 142, Type: "generic"},
			wantRepo: "OWNER/REPO",
		},
		{
			name:     "--type plan",
			args:     "142 --type plan",
			wantOpts: ListOptions{IssueNumber: 142, Type: "plan"},
			wantRepo: "OWNER/REPO",
		},
		{
			name:     "--type link",
			args:     "142 --type link",
			wantOpts: ListOptions{IssueNumber: 142, Type: "link"},
			wantRepo: "OWNER/REPO",
		},
		{
			name:    "an unknown --type",
			args:    "142 --type docs",
			wantErr: `invalid argument "docs" for "--type" flag: valid values are {generic|plan|link}`,
		},
		{
			name:       "--json with every field",
			args:       "142 --json body,bodyHtml,createdAt,creator,description,id,name,number,type,updatedAt,updatedByActor",
			wantOpts:   ListOptions{IssueNumber: 142},
			wantRepo:   "OWNER/REPO",
			wantFields: []string{"body", "bodyHtml", "createdAt", "creator", "description", "id", "name", "number", "type", "updatedAt", "updatedByActor"},
		},
		{
			name:    "--json with url, which isn't a field",
			args:    "142 --json number,url",
			wantErr: "Unknown JSON field: \"url\"\nAvailable fields:\n  body\n  bodyHtml\n  createdAt\n  creator\n  description\n  id\n  name\n  number\n  type\n  updatedAt\n  updatedByActor",
		},
		{
			name:    "no issue",
			args:    "",
			wantErr: "accepts 1 arg(s), received 0",
		},
		{
			name:    "an argument that isn't an issue",
			args:    "OAuth",
			wantErr: `invalid issue format: "OAuth"`,
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

			var gotOpts *ListOptions
			cmd := NewCmdList(f, func(opts *ListOptions) error {
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
			assert.Equal(t, tt.wantOpts.Limit, gotOpts.Limit)
			assert.Equal(t, tt.wantOpts.Type, gotOpts.Type)

			repo, err := gotOpts.BaseRepo()
			require.NoError(t, err)
			assert.Equal(t, tt.wantRepo, ghrepo.FullName(repo))

			if tt.wantFields == nil {
				assert.Nil(t, gotOpts.Exporter)
			} else {
				require.NotNil(t, gotOpts.Exporter)
				assert.Equal(t, tt.wantFields, gotOpts.Exporter.Fields())
			}
		})
	}
}

func TestListRun(t *testing.T) {
	now := time.Date(2026, 9, 24, 23, 0, 0, 0, time.UTC)
	hoursAgo := func(h int) *time.Time {
		t := now.Add(-time.Duration(h) * time.Hour)
		return &t
	}

	// The artifacts on the spec's issue 142.
	oauthPlan := client.Artifact{ID: 6626, Number: 2, Type: "generic", Name: "OAuth callback plan", UpdatedAt: hoursAgo(2)}
	runbook := client.Artifact{ID: 6627, Number: 3, Type: "link", Name: "Staging OAuth runbook", UpdatedAt: hoursAgo(24)}
	feedback := client.Artifact{ID: 6629, Number: 5, Type: "generic", Name: "Barista feedback notes", UpdatedAt: hoursAgo(72)}
	// The documents on the spec's issue 137.
	approvedPlan := client.Artifact{ID: 6640, Number: 1, Type: "plan", Name: "Approved plan", UpdatedAt: hoursAgo(5)}
	research := client.Artifact{ID: 6641, Number: 2, Type: "generic", Name: "OAuth provider research", UpdatedAt: hoursAgo(1)}

	jsonExporter := func(fields ...string) cmdutil.Exporter {
		e := cmdutil.NewJSONExporter()
		e.SetFields(fields)
		return e
	}

	// A pager command with an unterminated quote fails before any process
	// starts, so a row can see whether list started the pager.
	const brokenPager = "'"

	tests := []struct {
		name          string
		repo          ghrepo.Interface
		opts          ListOptions
		tty           bool
		pager         string
		isPullRequest bool
		lookupErr     error
		artifacts     map[string][]client.Artifact
		listErrs      map[string]error
		wantCalls     []string
		wantStdout    string
		wantStderr    string
		wantErr       string
		wantNoResults bool
	}{
		{
			name:      "a table in a terminal",
			opts:      ListOptions{IssueNumber: 142},
			tty:       true,
			artifacts: map[string][]client.Artifact{"": {oauthPlan, runbook, feedback}},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`List monalisa/monas-cafe#142 type="" limit=0`,
			},
			wantStdout: heredoc.Doc(`

				Showing 3 artifacts on monalisa/monas-cafe#142

				NUMBER  NAME                    TYPE     UPDATED
				2       OAuth callback plan     generic  about 2 hours ago
				3       Staging OAuth runbook   link     about 1 day ago
				5       Barista feedback notes  generic  about 3 days ago
			`),
		},
		{
			name:      "one artifact",
			opts:      ListOptions{IssueNumber: 128},
			tty:       true,
			artifacts: map[string][]client.Artifact{"": {runbook}},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#128",
				`List monalisa/monas-cafe#128 type="" limit=0`,
			},
			wantStdout: heredoc.Doc(`

				Showing 1 artifact on monalisa/monas-cafe#128

				NUMBER  NAME                   TYPE  UPDATED
				3       Staging OAuth runbook  link  about 1 day ago
			`),
		},
		{
			name:      "--limit asks for that many",
			opts:      ListOptions{IssueNumber: 142, Limit: 2},
			tty:       true,
			artifacts: map[string][]client.Artifact{"": {oauthPlan, runbook}},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`List monalisa/monas-cafe#142 type="" limit=2`,
			},
			wantStdout: heredoc.Doc(`

				Showing 2 artifacts on monalisa/monas-cafe#142

				NUMBER  NAME                   TYPE     UPDATED
				2       OAuth callback plan    generic  about 2 hours ago
				3       Staging OAuth runbook  link     about 1 day ago
			`),
		},
		{
			name:      "--type link",
			opts:      ListOptions{IssueNumber: 142, Type: "link"},
			tty:       true,
			artifacts: map[string][]client.Artifact{"link": {runbook}},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`List monalisa/monas-cafe#142 type="link" limit=0`,
			},
			wantStdout: heredoc.Doc(`

				Showing 1 artifact on monalisa/monas-cafe#142 that matches your search

				NUMBER  NAME                   TYPE  UPDATED
				3       Staging OAuth runbook  link  about 1 day ago
			`),
		},
		{
			name: "--type generic lists plans too, which keep their type",
			opts: ListOptions{IssueNumber: 137, Type: "generic"},
			tty:  true,
			artifacts: map[string][]client.Artifact{
				"generic": {research},
				"plan":    {approvedPlan},
			},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#137",
				`List monalisa/monas-cafe#137 type="generic" limit=0`,
				`List monalisa/monas-cafe#137 type="plan" limit=0`,
			},
			wantStdout: heredoc.Doc(`

				Showing 2 artifacts on monalisa/monas-cafe#137 that match your search

				NUMBER  NAME                     TYPE     UPDATED
				1       Approved plan            plan     about 5 hours ago
				2       OAuth provider research  generic  about 1 hour ago
			`),
		},
		{
			name: "--type plan lists generic artifacts too",
			opts: ListOptions{IssueNumber: 137, Type: "plan"},
			tty:  true,
			artifacts: map[string][]client.Artifact{
				"generic": {research},
				"plan":    {approvedPlan},
			},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#137",
				`List monalisa/monas-cafe#137 type="generic" limit=0`,
				`List monalisa/monas-cafe#137 type="plan" limit=0`,
			},
			wantStdout: heredoc.Doc(`

				Showing 2 artifacts on monalisa/monas-cafe#137 that match your search

				NUMBER  NAME                     TYPE     UPDATED
				1       Approved plan            plan     about 5 hours ago
				2       OAuth provider research  generic  about 1 hour ago
			`),
		},
		{
			name: "--limit keeps the lowest numbers of both document types",
			opts: ListOptions{IssueNumber: 137, Type: "generic", Limit: 2},
			artifacts: map[string][]client.Artifact{
				"generic": {
					{Number: 2, Type: "generic", Name: "OAuth provider research"},
					{Number: 5, Type: "generic", Name: "Menu notes"},
				},
				"plan": {
					{Number: 1, Type: "plan", Name: "Approved plan"},
					{Number: 4, Type: "plan", Name: "Revised plan"},
				},
			},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#137",
				`List monalisa/monas-cafe#137 type="generic" limit=2`,
				`List monalisa/monas-cafe#137 type="plan" limit=2`,
			},
			wantStdout: "1\tApproved plan\tplan\t\n2\tOAuth provider research\tgeneric\t\n",
		},
		{
			name:       "piped output",
			opts:       ListOptions{IssueNumber: 142},
			artifacts:  map[string][]client.Artifact{"": {oauthPlan, runbook, feedback}},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", `List monalisa/monas-cafe#142 type="" limit=0`},
			wantStdout: "2\tOAuth callback plan\tgeneric\t2026-09-24T21:00:00Z\n3\tStaging OAuth runbook\tlink\t2026-09-23T23:00:00Z\n5\tBarista feedback notes\tgeneric\t2026-09-21T23:00:00Z\n",
		},
		{
			name: "names are cleaned up in a terminal",
			opts: ListOptions{IssueNumber: 142},
			tty:  true,
			artifacts: map[string][]client.Artifact{"": {
				{Number: 5, Type: "generic", Name: "  Barista\tfeedback\n\n notes ", UpdatedAt: hoursAgo(72)},
			}},
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", `List monalisa/monas-cafe#142 type="" limit=0`},
			wantStdout: heredoc.Doc(`

				Showing 1 artifact on monalisa/monas-cafe#142

				NUMBER  NAME                    TYPE     UPDATED
				5       Barista feedback notes  generic  about 3 days ago
			`),
		},
		{
			name: "names are cleaned up when piped",
			opts: ListOptions{IssueNumber: 142},
			artifacts: map[string][]client.Artifact{"": {
				{Number: 5, Type: "generic", Name: "  Barista\tfeedback\n\n notes ", UpdatedAt: hoursAgo(72)},
			}},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", `List monalisa/monas-cafe#142 type="" limit=0`},
			wantStdout: "5\tBarista feedback notes\tgeneric\t2026-09-21T23:00:00Z\n",
		},
		{
			name: "a null update time leaves the cell empty in a terminal",
			opts: ListOptions{IssueNumber: 142},
			tty:  true,
			artifacts: map[string][]client.Artifact{"": {
				oauthPlan,
				{Number: 3, Type: "link", Name: "Staging OAuth runbook"},
			}},
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", `List monalisa/monas-cafe#142 type="" limit=0`},
			wantStdout: "\nShowing 2 artifacts on monalisa/monas-cafe#142\n\n" +
				"NUMBER  NAME                   TYPE     UPDATED\n" +
				"2       OAuth callback plan    generic  about 2 hours ago\n" +
				"3       Staging OAuth runbook  link     \n",
		},
		{
			name: "a null update time leaves the field empty when piped",
			opts: ListOptions{IssueNumber: 142},
			artifacts: map[string][]client.Artifact{"": {
				{Number: 3, Type: "link", Name: "Staging OAuth runbook"},
			}},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", `List monalisa/monas-cafe#142 type="" limit=0`},
			wantStdout: "3\tStaging OAuth runbook\tlink\t\n",
		},
		{
			name: "a stored type gh doesn't know is listed as stored",
			opts: ListOptions{IssueNumber: 142},
			tty:  true,
			artifacts: map[string][]client.Artifact{"": {
				oauthPlan,
				{Number: 7, Type: "bad-type", Name: "Espresso checklist", UpdatedAt: hoursAgo(2)},
			}},
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", `List monalisa/monas-cafe#142 type="" limit=0`},
			wantStdout: heredoc.Doc(`

				Showing 2 artifacts on monalisa/monas-cafe#142

				NUMBER  NAME                 TYPE      UPDATED
				2       OAuth callback plan  generic   about 2 hours ago
				7       Espresso checklist   bad-type  about 2 hours ago
			`),
		},
		{
			name:       "--json",
			opts:       ListOptions{IssueNumber: 142, Exporter: jsonExporter("number", "name", "type")},
			tty:        true,
			artifacts:  map[string][]client.Artifact{"": {oauthPlan, runbook, feedback}},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", `List monalisa/monas-cafe#142 type="" limit=0`},
			wantStdout: `[{"name":"OAuth callback plan","number":2,"type":"generic"},{"name":"Staging OAuth runbook","number":3,"type":"link"},{"name":"Barista feedback notes","number":5,"type":"generic"}]` + "\n",
		},
		{
			name: "--json keeps nulls and exact names",
			opts: ListOptions{IssueNumber: 142, Exporter: jsonExporter("name", "creator", "updatedAt")},
			artifacts: map[string][]client.Artifact{"": {
				{Number: 5, Type: "generic", Name: "  Barista\tfeedback notes "},
			}},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", `List monalisa/monas-cafe#142 type="" limit=0`},
			wantStdout: `[{"creator":null,"name":"  Barista\tfeedback notes ","updatedAt":null}]` + "\n",
		},
		{
			name:       "--json with no artifacts prints an empty array",
			opts:       ListOptions{IssueNumber: 161, Exporter: jsonExporter("number")},
			tty:        true,
			artifacts:  map[string][]client.Artifact{"": {}},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#161", `List monalisa/monas-cafe#161 type="" limit=0`},
			wantStdout: "[]\n",
		},
		{
			name:      "a pager that fails to start is reported, and the table still prints",
			opts:      ListOptions{IssueNumber: 128},
			tty:       true,
			pager:     brokenPager,
			artifacts: map[string][]client.Artifact{"": {runbook}},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#128",
				`List monalisa/monas-cafe#128 type="" limit=0`,
			},
			wantStdout: heredoc.Doc(`

				Showing 1 artifact on monalisa/monas-cafe#128

				NUMBER  NAME                   TYPE  UPDATED
				3       Staging OAuth runbook  link  about 1 day ago
			`),
			wantStderr: "failed to start pager: EOF found when expecting closing quote\n",
		},
		{
			name:       "--json in a terminal doesn't start the pager",
			opts:       ListOptions{IssueNumber: 128, Exporter: jsonExporter("number")},
			tty:        true,
			pager:      brokenPager,
			artifacts:  map[string][]client.Artifact{"": {runbook}},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#128", `List monalisa/monas-cafe#128 type="" limit=0`},
			wantStdout: `[{"number":3}]` + "\n",
		},
		{
			name:          "no artifacts",
			opts:          ListOptions{IssueNumber: 161},
			tty:           true,
			artifacts:     map[string][]client.Artifact{"": {}},
			wantCalls:     []string{"IsPullRequest monalisa/monas-cafe#161", `List monalisa/monas-cafe#161 type="" limit=0`},
			wantErr:       "no artifacts found on monalisa/monas-cafe#161",
			wantNoResults: true,
		},
		{
			name:      "no artifacts match --type",
			opts:      ListOptions{IssueNumber: 128, Type: "generic"},
			tty:       true,
			artifacts: map[string][]client.Artifact{"generic": {}, "plan": {}},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#128",
				`List monalisa/monas-cafe#128 type="generic" limit=0`,
				`List monalisa/monas-cafe#128 type="plan" limit=0`,
			},
			wantErr:       "no artifacts match your search on monalisa/monas-cafe#128",
			wantNoResults: true,
		},
		{
			name:          "a pull request is refused before any artifact request",
			opts:          ListOptions{IssueNumber: 158},
			tty:           true,
			isPullRequest: true,
			wantCalls:     []string{"IsPullRequest monalisa/monas-cafe#158"},
			wantErr:       "monalisa/monas-cafe#158 is a pull request; artifacts are only supported on issues",
		},
		{
			name:      "the lookup's error",
			opts:      ListOptions{IssueNumber: 999},
			tty:       true,
			lookupErr: errors.New("GraphQL: Could not resolve to an issue or pull request with the number of 999. (repository.issue)"),
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#999"},
			wantErr:   "GraphQL: Could not resolve to an issue or pull request with the number of 999. (repository.issue)",
		},
		{
			name:    "GitHub Enterprise Server is refused before any request",
			repo:    ghrepo.NewWithHost("monalisa", "monas-cafe", "ghes.monas-cafe.example"),
			opts:    ListOptions{IssueNumber: 142},
			tty:     true,
			wantErr: "issue artifacts are not supported on GitHub Enterprise Server",
		},
		{
			name:      "a ghe.com host is supported",
			repo:      ghrepo.NewWithHost("monalisa", "monas-cafe", "monas-cafe.ghe.com"),
			opts:      ListOptions{IssueNumber: 142},
			artifacts: map[string][]client.Artifact{"": {runbook}},
			wantCalls: []string{
				"IsPullRequest monas-cafe.ghe.com/monalisa/monas-cafe#142",
				`List monas-cafe.ghe.com/monalisa/monas-cafe#142 type="" limit=0`,
			},
			wantStdout: "3\tStaging OAuth runbook\tlink\t2026-09-23T23:00:00Z\n",
		},
		{
			name: "an API error is returned as the API gives it",
			repo: ghrepo.NewWithHost("monalisa", "monas-cafe", "monas-cafe.ghe.com"),
			opts: ListOptions{IssueNumber: 142},
			tty:  true,
			listErrs: map[string]error{
				"": errors.New("HTTP 503: Artifact storage is not available (https://api.monas-cafe.ghe.com/repos/monalisa/monas-cafe/issues/142/artifacts?per_page=100)"),
			},
			wantCalls: []string{
				"IsPullRequest monas-cafe.ghe.com/monalisa/monas-cafe#142",
				`List monas-cafe.ghe.com/monalisa/monas-cafe#142 type="" limit=0`,
			},
			wantErr: "HTTP 503: Artifact storage is not available (https://api.monas-cafe.ghe.com/repos/monalisa/monas-cafe/issues/142/artifacts?per_page=100)",
		},
		{
			name:      "an API error listing the second document type",
			opts:      ListOptions{IssueNumber: 137, Type: "generic"},
			tty:       true,
			artifacts: map[string][]client.Artifact{"generic": {research}},
			listErrs:  map[string]error{"plan": errors.New("HTTP 404: Not Found (https://api.github.com/repos/monalisa/monas-cafe/issues/137/artifacts?per_page=100&type=plan)")},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#137",
				`List monalisa/monas-cafe#137 type="generic" limit=0`,
				`List monalisa/monas-cafe#137 type="plan" limit=0`,
			},
			wantErr: "HTTP 404: Not Found (https://api.github.com/repos/monalisa/monas-cafe/issues/137/artifacts?per_page=100&type=plan)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)
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
				ListFunc: func(r ghrepo.Interface, issueNumber int, artifactType string, limit int) ([]client.Artifact, error) {
					calls = append(calls, fmt.Sprintf("List %s type=%q limit=%d", issueRef(r, issueNumber), artifactType, limit))
					return tt.artifacts[artifactType], tt.listErrs[artifactType]
				},
			}

			opts := tt.opts
			opts.IO = ios
			opts.BaseRepo = func() (ghrepo.Interface, error) { return repo, nil }
			opts.Client = func() (client.ArtifactClient, error) { return mock, nil }
			opts.Now = func() time.Time { return now }

			err := listRun(&opts)

			assert.Equal(t, tt.wantCalls, calls)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				if tt.wantNoResults {
					var noResults cmdutil.NoResultsError
					require.ErrorAs(t, err, &noResults)
				}
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
