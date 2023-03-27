package create

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdCreate(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "my-body.md")
	err := os.WriteFile(tmpFile, []byte("a body from file"), 0600)
	require.NoError(t, err)

	tests := []struct {
		name      string
		tty       bool
		stdin     string
		cli       string
		wantsErr  bool
		wantsOpts CreateOptions
	}{
		{
			name:     "empty non-tty",
			tty:      false,
			cli:      "",
			wantsErr: true,
		},
		{
			name:     "only title non-tty",
			tty:      false,
			cli:      "--title mytitle",
			wantsErr: true,
		},
		{
			name:     "minimum non-tty",
			tty:      false,
			cli:      "--title mytitle --body ''",
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:               "mytitle",
				TitleProvided:       true,
				Body:                "",
				BodyProvided:        true,
				Autofill:            false,
				RecoverFile:         "",
				WebMode:             false,
				IsDraft:             false,
				BaseBranch:          "",
				HeadBranch:          "",
				MaintainerCanModify: true,
			},
		},
		{
			name:     "empty tty",
			tty:      true,
			cli:      "",
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:               "",
				TitleProvided:       false,
				Body:                "",
				BodyProvided:        false,
				Autofill:            false,
				RecoverFile:         "",
				WebMode:             false,
				IsDraft:             false,
				BaseBranch:          "",
				HeadBranch:          "",
				MaintainerCanModify: true,
			},
		},
		{
			name:     "body from stdin",
			tty:      false,
			stdin:    "this is on standard input",
			cli:      "-t mytitle -F -",
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:               "mytitle",
				TitleProvided:       true,
				Body:                "this is on standard input",
				BodyProvided:        true,
				Autofill:            false,
				RecoverFile:         "",
				WebMode:             false,
				IsDraft:             false,
				BaseBranch:          "",
				HeadBranch:          "",
				MaintainerCanModify: true,
			},
		},
		{
			name:     "body from file",
			tty:      false,
			cli:      fmt.Sprintf("-t mytitle -F '%s'", tmpFile),
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:               "mytitle",
				TitleProvided:       true,
				Body:                "a body from file",
				BodyProvided:        true,
				Autofill:            false,
				RecoverFile:         "",
				WebMode:             false,
				IsDraft:             false,
				BaseBranch:          "",
				HeadBranch:          "",
				MaintainerCanModify: true,
			},
		},
		{
			name:     "template from file name tty",
			tty:      true,
			cli:      "-t mytitle --template bug_fix.md",
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:               "mytitle",
				TitleProvided:       true,
				Body:                "",
				BodyProvided:        false,
				Autofill:            false,
				RecoverFile:         "",
				WebMode:             false,
				IsDraft:             false,
				BaseBranch:          "",
				HeadBranch:          "",
				MaintainerCanModify: true,
				Template:            "bug_fix.md",
			},
		},
		{
			name:     "template from file name non-tty",
			tty:      false,
			cli:      "-t mytitle --template bug_fix.md",
			wantsErr: true,
		},
		{
			name:     "template and body",
			tty:      false,
			cli:      `-t mytitle --template bug_fix.md --body "pr body"`,
			wantsErr: true,
		},
		{
			name:     "template and body file",
			tty:      false,
			cli:      "-t mytitle --template bug_fix.md --body-file body_file.md",
			wantsErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, stdin, stdout, stderr := iostreams.Test()
			if tt.stdin != "" {
				_, _ = stdin.WriteString(tt.stdin)
			} else if tt.tty {
				ios.SetStdinTTY(true)
				ios.SetStdoutTTY(true)
			}

			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			var opts *CreateOptions
			cmd := NewCmdCreate(f, func(o *CreateOptions) error {
				opts = o
				return nil
			})

			args, err := shlex.Split(tt.cli)
			require.NoError(t, err)
			cmd.SetArgs(args)
			cmd.SetOut(stderr)
			cmd.SetErr(stderr)
			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, "", stdout.String())
			assert.Equal(t, "", stderr.String())

			assert.Equal(t, tt.wantsOpts.Body, opts.Body)
			assert.Equal(t, tt.wantsOpts.BodyProvided, opts.BodyProvided)
			assert.Equal(t, tt.wantsOpts.Title, opts.Title)
			assert.Equal(t, tt.wantsOpts.TitleProvided, opts.TitleProvided)
			assert.Equal(t, tt.wantsOpts.Autofill, opts.Autofill)
			assert.Equal(t, tt.wantsOpts.WebMode, opts.WebMode)
			assert.Equal(t, tt.wantsOpts.RecoverFile, opts.RecoverFile)
			assert.Equal(t, tt.wantsOpts.IsDraft, opts.IsDraft)
			assert.Equal(t, tt.wantsOpts.MaintainerCanModify, opts.MaintainerCanModify)
			assert.Equal(t, tt.wantsOpts.BaseBranch, opts.BaseBranch)
			assert.Equal(t, tt.wantsOpts.HeadBranch, opts.HeadBranch)
			assert.Equal(t, tt.wantsOpts.Template, opts.Template)
		})
	}
}

func Test_createRun(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(*CreateOptions, *testing.T) func()
		cmdStubs       func(*run.CommandStubber)
		promptStubs    func(*prompter.PrompterMock)
		httpStubs      func(*httpmock.Registry, *testing.T)
		expectedOut    string
		expectedErrOut string
		expectedBrowse string
		wantErr        string
		tty            bool
	}{
		{
			name: "nontty web",
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.WebMode = true
				opts.HeadBranch = "feature"
				return func() {}
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git( .+)? log( .+)? origin/master\.\.\.feature`, 0, "")
			},
			expectedBrowse: "https://github.com/OWNER/REPO/compare/master...feature?body=&expand=1",
		},
		{
			name: "nontty",
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "createPullRequest": { "pullRequest": {
						"URL": "https://github.com/OWNER/REPO/pull/12"
					} } } }`,
						func(input map[string]interface{}) {
							assert.Equal(t, "REPOID", input["repositoryId"])
							assert.Equal(t, "my title", input["title"])
							assert.Equal(t, "my body", input["body"])
							assert.Equal(t, "master", input["baseRefName"])
							assert.Equal(t, "feature", input["headRefName"])
						}))
			},
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "my title"
				opts.Body = "my body"
				opts.HeadBranch = "feature"
				return func() {}
			},
			expectedOut: "https://github.com/OWNER/REPO/pull/12\n",
		},
		{
			name: "survey",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "my title"
				opts.Body = "my body"
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.StubRepoResponse("OWNER", "REPO")
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data": {"viewer": {"login": "OWNER"} } }`))
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
						{ "data": { "createPullRequest": { "pullRequest": {
							"URL": "https://github.com/OWNER/REPO/pull/12"
						} } } }`, func(input map[string]interface{}) {
						assert.Equal(t, "REPOID", input["repositoryId"].(string))
						assert.Equal(t, "my title", input["title"].(string))
						assert.Equal(t, "my body", input["body"].(string))
						assert.Equal(t, "master", input["baseRefName"].(string))
						assert.Equal(t, "feature", input["headRefName"].(string))
						assert.Equal(t, false, input["draft"].(bool))
					}))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --get-regexp.+branch\\\.feature\\\.`, 0, "")
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 0, "")
				cs.Register(`git push --set-upstream origin HEAD:feature`, 0, "")
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					if p == "Where should we push the 'feature' branch?" {
						return 0, nil
					} else {
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for feature into master in OWNER/REPO\n\n",
		},
		{
			name: "project v2",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "my title"
				opts.Body = "my body"
				opts.Projects = []string{"RoadmapV2"}
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.StubRepoResponse("OWNER", "REPO")
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data": {"viewer": {"login": "OWNER"} } }`))
				mockRetrieveProjects(t, reg)
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
						{ "data": { "createPullRequest": { "pullRequest": {
							"id": "PullRequest#1",
							"URL": "https://github.com/OWNER/REPO/pull/12"
						} } } }
						`, func(input map[string]interface{}) {
						assert.Equal(t, "REPOID", input["repositoryId"].(string))
						assert.Equal(t, "my title", input["title"].(string))
						assert.Equal(t, "my body", input["body"].(string))
						assert.Equal(t, "master", input["baseRefName"].(string))
						assert.Equal(t, "feature", input["headRefName"].(string))
						assert.Equal(t, false, input["draft"].(bool))
					}))
				reg.Register(
					httpmock.GraphQL(`mutation UpdateProjectV2Items\b`),
					httpmock.GraphQLQuery(`
						{ "data": { "add_000": { "item": {
							"id": "1"
						} } } }
						`, func(mutations string, inputs map[string]interface{}) {
						variables, err := json.Marshal(inputs)
						assert.NoError(t, err)
						expectedMutations := "mutation UpdateProjectV2Items($input_000: AddProjectV2ItemByIdInput!) {add_000: addProjectV2ItemById(input: $input_000) { item { id } }}"
						expectedVariables := `{"input_000":{"contentId":"PullRequest#1","projectId":"ROADMAPV2ID"}}`
						assert.Equal(t, expectedMutations, mutations)
						assert.Equal(t, expectedVariables, string(variables))
					}))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --get-regexp.+branch\\\.feature\\\.`, 0, "")
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 0, "")
				cs.Register(`git push --set-upstream origin HEAD:feature`, 0, "")
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					if p == "Where should we push the 'feature' branch?" {
						return 0, nil
					} else {
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for feature into master in OWNER/REPO\n\n",
		},
		{
			name: "no maintainer modify",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "my title"
				opts.Body = "my body"
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.StubRepoResponse("OWNER", "REPO")
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data": {"viewer": {"login": "OWNER"} } }`))
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
						{ "data": { "createPullRequest": { "pullRequest": {
							"URL": "https://github.com/OWNER/REPO/pull/12"
						} } } }
						`, func(input map[string]interface{}) {
						assert.Equal(t, false, input["maintainerCanModify"].(bool))
						assert.Equal(t, "REPOID", input["repositoryId"].(string))
						assert.Equal(t, "my title", input["title"].(string))
						assert.Equal(t, "my body", input["body"].(string))
						assert.Equal(t, "master", input["baseRefName"].(string))
						assert.Equal(t, "feature", input["headRefName"].(string))
					}))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --get-regexp.+branch\\\.feature\\\.`, 0, "")
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 0, "")
				cs.Register(`git push --set-upstream origin HEAD:feature`, 0, "")
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					if p == "Where should we push the 'feature' branch?" {
						return 0, nil
					} else {
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for feature into master in OWNER/REPO\n\n",
		},
		{
			name: "create fork",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "title"
				opts.Body = "body"
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.StubRepoResponse("OWNER", "REPO")
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data": {"viewer": {"login": "monalisa"} } }`))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/forks"),
					httpmock.StatusStringResponse(201, `
						{ "node_id": "NODEID",
						  "name": "REPO",
						  "owner": {"login": "monalisa"}
						}`))
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
						{ "data": { "createPullRequest": { "pullRequest": {
							"URL": "https://github.com/OWNER/REPO/pull/12"
						}}}}`, func(input map[string]interface{}) {
						assert.Equal(t, "REPOID", input["repositoryId"].(string))
						assert.Equal(t, "master", input["baseRefName"].(string))
						assert.Equal(t, "monalisa:feature", input["headRefName"].(string))
					}))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --get-regexp.+branch\\\.feature\\\.`, 0, "")
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 0, "")
				cs.Register(`git remote add fork https://github.com/monalisa/REPO.git`, 0, "")
				cs.Register(`git push --set-upstream fork HEAD:feature`, 0, "")
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					if p == "Where should we push the 'feature' branch?" {
						return prompter.IndexFor(opts, "Create a fork of OWNER/REPO")
					} else {
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for monalisa:feature into master in OWNER/REPO\n\n",
		},
		{
			name: "pushed to non base repo",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "title"
				opts.Body = "body"
				opts.Remotes = func() (context.Remotes, error) {
					return context.Remotes{
						{
							Remote: &git.Remote{
								Name:     "upstream",
								Resolved: "base",
							},
							Repo: ghrepo.New("OWNER", "REPO"),
						},
						{
							Remote: &git.Remote{
								Name:     "origin",
								Resolved: "base",
							},
							Repo: ghrepo.New("monalisa", "REPO"),
						},
					}, nil
				}
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
						{ "data": { "createPullRequest": { "pullRequest": {
							"URL": "https://github.com/OWNER/REPO/pull/12"
						} } } }`, func(input map[string]interface{}) {
						assert.Equal(t, "REPOID", input["repositoryId"].(string))
						assert.Equal(t, "master", input["baseRefName"].(string))
						assert.Equal(t, "monalisa:feature", input["headRefName"].(string))
					}))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --get-regexp \^branch\\\.feature\\\.`, 1, "") // determineTrackingBranch
				cs.Register("git show-ref --verify", 0, heredoc.Doc(`
		deadbeef HEAD
		deadb00f refs/remotes/upstream/feature
		deadbeef refs/remotes/origin/feature`)) // determineTrackingBranch
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for monalisa:feature into master in OWNER/REPO\n\n",
		},
		{
			name: "pushed to different branch name",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "title"
				opts.Body = "body"
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
		`, func(input map[string]interface{}) {
						assert.Equal(t, "REPOID", input["repositoryId"].(string))
						assert.Equal(t, "master", input["baseRefName"].(string))
						assert.Equal(t, "my-feat2", input["headRefName"].(string))
					}))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --get-regexp \^branch\\\.feature\\\.`, 0, heredoc.Doc(`
		branch.feature.remote origin
		branch.feature.merge refs/heads/my-feat2
	`)) // determineTrackingBranch
				cs.Register("git show-ref --verify", 0, heredoc.Doc(`
		deadbeef HEAD
		deadbeef refs/remotes/origin/my-feat2
	`)) // determineTrackingBranch
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for my-feat2 into master in OWNER/REPO\n\n",
		},
		{
			name: "non legacy template",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.Title = "my title"
				opts.HeadBranch = "feature"
				opts.RootDirOverride = "./fixtures/repoWithNonLegacyPRTemplates"
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query PullRequestTemplates\b`),
					httpmock.StringResponse(`
				{ "data": { "repository": { "pullRequestTemplates": [
					{ "filename": "template1",
					  "body": "this is a bug" },
					{ "filename": "template2",
					  "body": "this is a enhancement" }
				] } } }`))
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
			{ "data": { "createPullRequest": { "pullRequest": {
				"URL": "https://github.com/OWNER/REPO/pull/12"
			} } } }
			`, func(input map[string]interface{}) {
						assert.Equal(t, "my title", input["title"].(string))
						assert.Equal(t, "- commit 1\n- commit 0\n\nthis is a bug", input["body"].(string))
					}))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git( .+)? log( .+)? origin/master\.\.\.feature`, 0, "1234567890,commit 0\n2345678901,commit 1")
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.MarkdownEditorFunc = func(p, d string, ba bool) (string, error) {
					if p == "Body" {
						return d, nil
					} else {
						return "", prompter.NoSuchPromptErr(p)
					}
				}
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					switch p {
					case "What's next?":
						return 0, nil
					case "Choose a template":
						return prompter.IndexFor(opts, "template1")
					default:
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for feature into master in OWNER/REPO\n\n",
		},
		{
			name: "metadata",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.Title = "TITLE"
				opts.BodyProvided = true
				opts.Body = "BODY"
				opts.HeadBranch = "feature"
				opts.Assignees = []string{"monalisa"}
				opts.Labels = []string{"bug", "todo"}
				opts.Projects = []string{"roadmap"}
				opts.Reviewers = []string{"hubot", "monalisa", "/core", "/robots"}
				opts.Milestone = "big one.oh"
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryResolveMetadataIDs\b`),
					httpmock.StringResponse(`
				{ "data": {
					"u000": { "login": "MonaLisa", "id": "MONAID" },
					"u001": { "login": "hubot", "id": "HUBOTID" },
					"repository": {
						"l000": { "name": "bug", "id": "BUGID" },
						"l001": { "name": "TODO", "id": "TODOID" }
					},
					"organization": {
						"t000": { "slug": "core", "id": "COREID" },
						"t001": { "slug": "robots", "id": "ROBOTID" }
					}
				} }
				`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryMilestoneList\b`),
					httpmock.StringResponse(`
				{ "data": { "repository": { "milestones": {
					"nodes": [
						{ "title": "GA", "id": "GAID" },
						{ "title": "Big One.oh", "id": "BIGONEID" }
					],
					"pageInfo": { "hasNextPage": false }
				} } } }
				`))
				mockRetrieveProjects(t, reg)
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
				{ "data": { "createPullRequest": { "pullRequest": {
					"id": "NEWPULLID",
					"URL": "https://github.com/OWNER/REPO/pull/12"
				} } } }
			`, func(inputs map[string]interface{}) {
						assert.Equal(t, "TITLE", inputs["title"])
						assert.Equal(t, "BODY", inputs["body"])
						if v, ok := inputs["assigneeIds"]; ok {
							t.Errorf("did not expect assigneeIds: %v", v)
						}
						if v, ok := inputs["userIds"]; ok {
							t.Errorf("did not expect userIds: %v", v)
						}
					}))
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreateMetadata\b`),
					httpmock.GraphQLMutation(`
				{ "data": { "updatePullRequest": {
					"clientMutationId": ""
				} } }
			`, func(inputs map[string]interface{}) {
						assert.Equal(t, "NEWPULLID", inputs["pullRequestId"])
						assert.Equal(t, []interface{}{"MONAID"}, inputs["assigneeIds"])
						assert.Equal(t, []interface{}{"BUGID", "TODOID"}, inputs["labelIds"])
						assert.Equal(t, []interface{}{"ROADMAPID"}, inputs["projectIds"])
						assert.Equal(t, "BIGONEID", inputs["milestoneId"])
					}))
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreateRequestReviews\b`),
					httpmock.GraphQLMutation(`
				{ "data": { "requestReviews": {
					"clientMutationId": ""
				} } }
			`, func(inputs map[string]interface{}) {
						assert.Equal(t, "NEWPULLID", inputs["pullRequestId"])
						assert.Equal(t, []interface{}{"HUBOTID", "MONAID"}, inputs["userIds"])
						assert.Equal(t, []interface{}{"COREID", "ROBOTID"}, inputs["teamIds"])
						assert.Equal(t, true, inputs["union"])
					}))
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for feature into master in OWNER/REPO\n\n",
		},
		{
			name: "already exists",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "title"
				opts.Body = "body"
				opts.HeadBranch = "feature"
				opts.Finder = shared.NewMockFinder("feature", &api.PullRequest{URL: "https://github.com/OWNER/REPO/pull/123"}, ghrepo.New("OWNER", "REPO"))
				return func() {}
			},
			wantErr: "a pull request for branch \"feature\" into branch \"master\" already exists:\nhttps://github.com/OWNER/REPO/pull/123",
		},
		{
			name: "web",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.WebMode = true
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.StubRepoResponse("OWNER", "REPO")
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data": {"viewer": {"login": "OWNER"} } }`))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --get-regexp.+branch\\\.feature\\\.`, 0, "")
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 0, "")
				cs.Register(`git( .+)? log( .+)? origin/master\.\.\.feature`, 0, "")
				cs.Register(`git push --set-upstream origin HEAD:feature`, 0, "")
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					if p == "Where should we push the 'feature' branch?" {
						return 0, nil
					} else {
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			expectedErrOut: "Opening github.com/OWNER/REPO/compare/master...feature in your browser.\n",
			expectedBrowse: "https://github.com/OWNER/REPO/compare/master...feature?body=&expand=1",
		},
		{
			name: "web project",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.WebMode = true
				opts.Projects = []string{"Triage"}
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.StubRepoResponse("OWNER", "REPO")
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data": {"viewer": {"login": "OWNER"} } }`))
				mockRetrieveProjects(t, reg)
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --get-regexp.+branch\\\.feature\\\.`, 0, "")
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 0, "")
				cs.Register(`git( .+)? log( .+)? origin/master\.\.\.feature`, 0, "")
				cs.Register(`git push --set-upstream origin HEAD:feature`, 0, "")

			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					if p == "Where should we push the 'feature' branch?" {
						return 0, nil
					} else {
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			expectedErrOut: "Opening github.com/OWNER/REPO/compare/master...feature in your browser.\n",
			expectedBrowse: "https://github.com/OWNER/REPO/compare/master...feature?body=&expand=1&projects=ORG%2F1",
		},
		{
			name: "draft",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.Title = "my title"
				opts.HeadBranch = "feature"
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query PullRequestTemplates\b`),
					httpmock.StringResponse(`
			{ "data": { "repository": { "pullRequestTemplates": [
				{ "filename": "template1",
				  "body": "this is a bug" },
				{ "filename": "template2",
				  "body": "this is a enhancement" }
			] } } }`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
		`, func(input map[string]interface{}) {
						assert.Equal(t, true, input["draft"].(bool))
					}))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git -c log.ShowSignature=false log --pretty=format:%H,%s --cherry origin/master...feature`, 0, "")
				cs.Register(`git rev-parse --show-toplevel`, 0, "")
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.MarkdownEditorFunc = func(p, d string, ba bool) (string, error) {
					if p == "Body" {
						return d, nil
					} else {
						return "", prompter.NoSuchPromptErr(p)
					}
				}
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					switch p {
					case "What's next?":
						return prompter.IndexFor(opts, "Submit as draft")
					case "Choose a template":
						return 0, nil
					default:
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for feature into master in OWNER/REPO\n\n",
		},
		{
			name: "recover",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryResolveMetadataIDs\b`),
					httpmock.StringResponse(`
		{ "data": {
			"u000": { "login": "jillValentine", "id": "JILLID" },
			"repository": {},
			"organization": {}
		} }
		`))
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreateRequestReviews\b`),
					httpmock.GraphQLMutation(`
		{ "data": { "requestReviews": {
			"clientMutationId": ""
		} } }
	`, func(inputs map[string]interface{}) {
						assert.Equal(t, []interface{}{"JILLID"}, inputs["userIds"])
					}))
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
		`, func(input map[string]interface{}) {
						assert.Equal(t, "recovered title", input["title"].(string))
						assert.Equal(t, "recovered body", input["body"].(string))
					}))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git( .+)? log( .+)? origin/master\.\.\.feature`, 0, "")
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.InputFunc = func(p, d string) (string, error) {
					if p == "Title" {
						return d, nil
					} else {
						return "", prompter.NoSuchPromptErr(p)
					}
				}
				pm.MarkdownEditorFunc = func(p, d string, ba bool) (string, error) {
					if p == "Body" {
						return d, nil
					} else {
						return "", prompter.NoSuchPromptErr(p)
					}
				}
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					if p == "What's next?" {
						return 0, nil
					} else {
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			setup: func(opts *CreateOptions, t *testing.T) func() {
				tmpfile, err := os.CreateTemp(t.TempDir(), "testrecover*")
				assert.NoError(t, err)
				state := shared.IssueMetadataState{
					Title:     "recovered title",
					Body:      "recovered body",
					Reviewers: []string{"jillValentine"},
				}
				data, err := json.Marshal(state)
				assert.NoError(t, err)
				_, err = tmpfile.Write(data)
				assert.NoError(t, err)

				opts.RecoverFile = tmpfile.Name()
				opts.HeadBranch = "feature"
				return func() { tmpfile.Close() }
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for feature into master in OWNER/REPO\n\n",
		},
		{
			name: "web long URL",
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git( .+)? log( .+)? origin/master\.\.\.feature`, 0, "")
			},
			setup: func(opts *CreateOptions, t *testing.T) func() {
				longBody := make([]byte, 9216)
				opts.Body = string(longBody)
				opts.BodyProvided = true
				opts.WebMode = true
				opts.HeadBranch = "feature"
				return func() {}
			},
			wantErr: "cannot open in browser: maximum URL length exceeded",
		},
		{
			name: "no local git repo",
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.Title = "My PR"
				opts.TitleProvided = true
				opts.Body = ""
				opts.BodyProvided = true
				opts.HeadBranch = "feature"
				opts.RepoOverride = "OWNER/REPO"
				opts.Remotes = func() (context.Remotes, error) {
					return nil, errors.New("not a git repository")
				}
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.StringResponse(`
						{ "data": { "createPullRequest": { "pullRequest": {
							"URL": "https://github.com/OWNER/REPO/pull/12"
						} } } }
					`))
			},
			expectedOut: "https://github.com/OWNER/REPO/pull/12\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			branch := "feature"

			reg := &httpmock.Registry{}
			reg.StubRepoInfoResponse("OWNER", "REPO", "master")
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg, t)
			}

			pm := &prompter.PrompterMock{}

			if tt.promptStubs != nil {
				tt.promptStubs(pm)
			}

			cs, cmdTeardown := run.Stub()
			defer cmdTeardown(t)
			cs.Register(`git status --porcelain`, 0, "")

			if tt.cmdStubs != nil {
				tt.cmdStubs(cs)
			}

			opts := CreateOptions{}
			opts.Prompter = pm

			ios, _, stdout, stderr := iostreams.Test()
			// TODO do i need to bother with this
			ios.SetStdoutTTY(tt.tty)
			ios.SetStdinTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)
			browser := &browser.Stub{}
			opts.IO = ios
			opts.Browser = browser
			opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			opts.Config = func() (config.Config, error) {
				return config.NewBlankConfig(), nil
			}
			opts.Remotes = func() (context.Remotes, error) {
				return context.Remotes{
					{
						Remote: &git.Remote{
							Name:     "origin",
							Resolved: "base",
						},
						Repo: ghrepo.New("OWNER", "REPO"),
					},
				}, nil
			}
			opts.Branch = func() (string, error) {
				return branch, nil
			}
			opts.Finder = shared.NewMockFinder(branch, nil, nil)
			opts.GitClient = &git.Client{
				GhPath:  "some/path/gh",
				GitPath: "some/path/git",
			}
			cleanSetup := func() {}
			if tt.setup != nil {
				cleanSetup = tt.setup(&opts, t)
			}
			defer cleanSetup()

			err := createRun(&opts)
			output := &test.CmdOut{
				OutBuf:     stdout,
				ErrBuf:     stderr,
				BrowsedURL: browser.BrowsedURL(),
			}
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedOut, output.String())
				assert.Equal(t, tt.expectedErrOut, output.Stderr())
				assert.Equal(t, tt.expectedBrowse, output.BrowsedURL)
			}
		})
	}
}

func Test_determineTrackingBranch(t *testing.T) {
	tests := []struct {
		name     string
		cmdStubs func(*run.CommandStubber)
		remotes  context.Remotes
		assert   func(ref *git.TrackingRef, t *testing.T)
	}{
		{
			name: "empty",
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --get-regexp.+branch\\\.feature\\\.`, 0, "")
				cs.Register(`git show-ref --verify -- HEAD`, 0, "abc HEAD")
			},
			assert: func(ref *git.TrackingRef, t *testing.T) {
				assert.Nil(t, ref)
			},
		},
		{
			name: "no match",
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --get-regexp.+branch\\\.feature\\\.`, 0, "")
				cs.Register("git show-ref --verify -- HEAD refs/remotes/origin/feature refs/remotes/upstream/feature", 0, "abc HEAD\nbca refs/remotes/origin/feature")
			},
			remotes: context.Remotes{
				&context.Remote{
					Remote: &git.Remote{Name: "origin"},
					Repo:   ghrepo.New("hubot", "Spoon-Knife"),
				},
				&context.Remote{
					Remote: &git.Remote{Name: "upstream"},
					Repo:   ghrepo.New("octocat", "Spoon-Knife"),
				},
			},
			assert: func(ref *git.TrackingRef, t *testing.T) {
				assert.Nil(t, ref)
			},
		},
		{
			name: "match",
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --get-regexp.+branch\\\.feature\\\.`, 0, "")
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature refs/remotes/upstream/feature$`, 0, heredoc.Doc(`
		deadbeef HEAD
		deadb00f refs/remotes/origin/feature
		deadbeef refs/remotes/upstream/feature
	`))
			},
			remotes: context.Remotes{
				&context.Remote{
					Remote: &git.Remote{Name: "origin"},
					Repo:   ghrepo.New("hubot", "Spoon-Knife"),
				},
				&context.Remote{
					Remote: &git.Remote{Name: "upstream"},
					Repo:   ghrepo.New("octocat", "Spoon-Knife"),
				},
			},
			assert: func(ref *git.TrackingRef, t *testing.T) {
				assert.Equal(t, "upstream", ref.RemoteName)
				assert.Equal(t, "feature", ref.BranchName)
			},
		},
		{
			name: "respect tracking config",
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --get-regexp.+branch\\\.feature\\\.`, 0, heredoc.Doc(`
		branch.feature.remote origin
		branch.feature.merge refs/heads/great-feat
	`))
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/great-feat refs/remotes/origin/feature$`, 0, heredoc.Doc(`
		deadbeef HEAD
		deadb00f refs/remotes/origin/feature
	`))
			},
			remotes: context.Remotes{
				&context.Remote{
					Remote: &git.Remote{Name: "origin"},
					Repo:   ghrepo.New("hubot", "Spoon-Knife"),
				},
			},
			assert: func(ref *git.TrackingRef, t *testing.T) {
				assert.Nil(t, ref)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs, cmdTeardown := run.Stub()
			defer cmdTeardown(t)

			tt.cmdStubs(cs)

			gitClient := &git.Client{
				GhPath:  "some/path/gh",
				GitPath: "some/path/git",
			}
			ref := determineTrackingBranch(gitClient, tt.remotes, "feature")
			tt.assert(ref, t)
		})
	}
}

func Test_generateCompareURL(t *testing.T) {
	tests := []struct {
		name    string
		ctx     CreateContext
		state   shared.IssueMetadataState
		want    string
		wantErr bool
	}{
		{
			name: "basic",
			ctx: CreateContext{
				BaseRepo:        api.InitRepoHostname(&api.Repository{Name: "REPO", Owner: api.RepositoryOwner{Login: "OWNER"}}, "github.com"),
				BaseBranch:      "main",
				HeadBranchLabel: "feature",
			},
			want:    "https://github.com/OWNER/REPO/compare/main...feature?body=&expand=1",
			wantErr: false,
		},
		{
			name: "with labels",
			ctx: CreateContext{
				BaseRepo:        api.InitRepoHostname(&api.Repository{Name: "REPO", Owner: api.RepositoryOwner{Login: "OWNER"}}, "github.com"),
				BaseBranch:      "a",
				HeadBranchLabel: "b",
			},
			state: shared.IssueMetadataState{
				Labels: []string{"one", "two three"},
			},
			want:    "https://github.com/OWNER/REPO/compare/a...b?body=&expand=1&labels=one%2Ctwo+three",
			wantErr: false,
		},
		{
			name: "'/'s in branch names/labels are percent-encoded",
			ctx: CreateContext{
				BaseRepo:        api.InitRepoHostname(&api.Repository{Name: "REPO", Owner: api.RepositoryOwner{Login: "OWNER"}}, "github.com"),
				BaseBranch:      "main/trunk",
				HeadBranchLabel: "owner:feature",
			},
			want:    "https://github.com/OWNER/REPO/compare/main%2Ftrunk...owner:feature?body=&expand=1",
			wantErr: false,
		},
		{
			name: "Any of !'(),; but none of $&+=@ and : in branch names/labels are percent-encoded ",
			/*
					- Technically, per section 3.3 of RFC 3986, none of !$&'()*+,;= (sub-delims) and :[]@ (part of gen-delims) in path segments are optionally percent-encoded, but url.PathEscape percent-encodes !'(),; anyway
					- !$&'()+,;=@ is a valid Git branch name—essentially RFC 3986 sub-delims without * and gen-delims without :/?#[]
					- : is GitHub separator between a fork name and a branch name
				    - See https://github.com/golang/go/issues/27559.
			*/
			ctx: CreateContext{
				BaseRepo:        api.InitRepoHostname(&api.Repository{Name: "REPO", Owner: api.RepositoryOwner{Login: "OWNER"}}, "github.com"),
				BaseBranch:      "main/trunk",
				HeadBranchLabel: "owner:!$&'()+,;=@",
			},
			want:    "https://github.com/OWNER/REPO/compare/main%2Ftrunk...owner:%21$&%27%28%29+%2C%3B=@?body=&expand=1",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := generateCompareURL(tt.ctx, tt.state)
			if (err != nil) != tt.wantErr {
				t.Errorf("generateCompareURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("generateCompareURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func mockRetrieveProjects(_ *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`query RepositoryProjectList\b`),
		httpmock.StringResponse(`
				{ "data": { "repository": { "projects": {
					"nodes": [
						{ "name": "Cleanup", "id": "CLEANUPID", "resourcePath": "/OWNER/REPO/projects/1" },
						{ "name": "Roadmap", "id": "ROADMAPID", "resourcePath": "/OWNER/REPO/projects/2" }
					],
					"pageInfo": { "hasNextPage": false }
				} } } }
				`))
	reg.Register(
		httpmock.GraphQL(`query RepositoryProjectV2List\b`),
		httpmock.StringResponse(`
				{ "data": { "repository": { "projectsV2": {
					"nodes": [
						{ "title": "CleanupV2", "id": "CLEANUPV2ID", "resourcePath": "/OWNER/REPO/projects/3" },
						{ "title": "RoadmapV2", "id": "ROADMAPV2ID", "resourcePath": "/OWNER/REPO/projects/4" }
					],
					"pageInfo": { "hasNextPage": false }
				} } } }
				`))
	reg.Register(
		httpmock.GraphQL(`query OrganizationProjectList\b`),
		httpmock.StringResponse(`
				{ "data": { "organization": { "projects": {
					"nodes": [
						{ "name": "Triage", "id": "TRIAGEID", "resourcePath": "/orgs/ORG/projects/1" }
					],
					"pageInfo": { "hasNextPage": false }
				} } } }
				`))
	reg.Register(
		httpmock.GraphQL(`query OrganizationProjectV2List\b`),
		httpmock.StringResponse(`
				{ "data": { "organization": { "projectsV2": {
					"nodes": [
						{ "title": "TriageV2", "id": "TRIAGEV2ID", "resourcePath": "/orgs/ORG/projects/2" }
					],
					"pageInfo": { "hasNextPage": false }
				} } } }
				`))
	reg.Register(
		httpmock.GraphQL(`query UserProjectV2List\b`),
		httpmock.StringResponse(`
				{ "data": { "viewer": { "projectsV2": {
					"nodes": [
						{ "title": "MonalisaV2", "id": "MONALISAV2ID", "resourcePath": "/user/MONALISA/projects/2" }
					],
					"pageInfo": { "hasNextPage": false }
				} } } }
				`))
}

// TODO interactive metadata tests once: 1) we have test utils for Prompter and 2) metadata questions use Prompter
