package develop

import (
	"bytes"
	"errors"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdDevelop(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		output     DevelopOptions
		wantStdout string
		wantStderr string
		wantErr    bool
		errMsg     string
	}{
		{
			name:    "no argument",
			input:   "",
			output:  DevelopOptions{},
			wantErr: true,
			errMsg:  "issue number or url is required",
		},
		{
			name:  "issue number",
			input: "1",
			output: DevelopOptions{
				IssueSelector: "1",
			},
		},
		{
			name:  "issue url",
			input: "https://github.com/cli/cli/issues/1",
			output: DevelopOptions{
				IssueSelector: "https://github.com/cli/cli/issues/1",
			},
		},
		{
			name:  "base flag",
			input: "1 --base feature",
			output: DevelopOptions{
				IssueSelector: "1",
				BaseBranch:    "feature",
			},
		},
		{
			name:  "checkout flag",
			input: "1 --checkout",
			output: DevelopOptions{
				IssueSelector: "1",
				Checkout:      true,
			},
		},
		{
			name:  "list flag",
			input: "1 --list",
			output: DevelopOptions{
				IssueSelector: "1",
				List:          true,
			},
		},
		{
			name:  "name flag",
			input: "1 --name feature",
			output: DevelopOptions{
				IssueSelector: "1",
				Name:          "feature",
			},
		},
		{
			name:  "issue-repo flag",
			input: "1 --issue-repo cli/cli",
			output: DevelopOptions{
				IssueSelector: "1",
			},
			wantStdout: "Flag --issue-repo has been deprecated, use `--repo` instead\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdOut, stdErr := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)
			var gotOpts *DevelopOptions
			cmd := NewCmdDevelop(f, func(opts *DevelopOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(stdOut)
			cmd.SetErr(stdErr)

			_, err = cmd.ExecuteC()
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.output.IssueSelector, gotOpts.IssueSelector)
			assert.Equal(t, tt.output.Name, gotOpts.Name)
			assert.Equal(t, tt.output.BaseBranch, gotOpts.BaseBranch)
			assert.Equal(t, tt.output.Checkout, gotOpts.Checkout)
			assert.Equal(t, tt.output.List, gotOpts.List)
			assert.Equal(t, tt.wantStdout, stdOut.String())
			assert.Equal(t, tt.wantStderr, stdErr.String())
		})
	}
}

func TestDevelopRun(t *testing.T) {
	featureEnabledPayload := `{"data":{"LinkedBranch":{"fields":[{"name":"id"},{"name":"ref"}]}}}`
	featureDisabledPayload := `{"data":{"LinkedBranch":null}}`

	tests := []struct {
		name           string
		opts           *DevelopOptions
		cmdStubs       func(*run.CommandStubber)
		runStubs       func(*run.CommandStubber)
		remotes        map[string]string
		httpStubs      func(*httpmock.Registry, *testing.T)
		expectedOut    string
		expectedErrOut string
		wantErr        string
		tty            bool
	}{
		{
			name: "returns an error when the feature is not supported by the API",
			opts: &DevelopOptions{
				IssueSelector: "42",
				List:          true,
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{"hasIssuesEnabled":true,"issue":{"id":"SOMEID","number":42}}}}`),
				)
				reg.Register(
					httpmock.GraphQL(`query LinkedBranchFeature\b`),
					httpmock.StringResponse(featureDisabledPayload),
				)
			},
			wantErr: "the `gh issue develop` command is not currently available",
		},
		{
			name: "list branches for an issue",
			opts: &DevelopOptions{
				IssueSelector: "42",
				List:          true,
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{"hasIssuesEnabled":true,"issue":{"id":"SOMEID","number":42}}}}`),
				)
				reg.Register(
					httpmock.GraphQL(`query LinkedBranchFeature\b`),
					httpmock.StringResponse(featureEnabledPayload),
				)
				reg.Register(
					httpmock.GraphQL(`query ListLinkedBranches\b`),
					httpmock.GraphQLQuery(`
		        {"data":{"repository":{"issue":{"linkedBranches":{"nodes":[{"ref":{"name":"foo","repository":{"url":"https://github.com/OWNER/REPO"}}},{"ref":{"name":"bar","repository":{"url":"https://github.com/OWNER/REPO"}}}]}}}}}
					`, func(query string, inputs map[string]interface{}) {
						assert.Equal(t, float64(42), inputs["number"])
						assert.Equal(t, "OWNER", inputs["owner"])
						assert.Equal(t, "REPO", inputs["name"])
					}))
			},
			expectedOut: "foo\thttps://github.com/OWNER/REPO/tree/foo\nbar\thttps://github.com/OWNER/REPO/tree/bar\n",
		},
		{
			name: "list branches for an issue in tty",
			opts: &DevelopOptions{
				IssueSelector: "42",
				List:          true,
			},
			tty: true,
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{"hasIssuesEnabled":true,"issue":{"id":"SOMEID","number":42}}}}`),
				)
				reg.Register(
					httpmock.GraphQL(`query LinkedBranchFeature\b`),
					httpmock.StringResponse(featureEnabledPayload),
				)
				reg.Register(
					httpmock.GraphQL(`query ListLinkedBranches\b`),
					httpmock.GraphQLQuery(`
		        {"data":{"repository":{"issue":{"linkedBranches":{"nodes":[{"ref":{"name":"foo","repository":{"url":"https://github.com/OWNER/REPO"}}},{"ref":{"name":"bar","repository":{"url":"https://github.com/OWNER/OTHER-REPO"}}}]}}}}}
					`, func(query string, inputs map[string]interface{}) {
						assert.Equal(t, float64(42), inputs["number"])
						assert.Equal(t, "OWNER", inputs["owner"])
						assert.Equal(t, "REPO", inputs["name"])
					}))
			},
			expectedOut: "\nShowing linked branches for OWNER/REPO#42\n\nfoo  https://github.com/OWNER/REPO/tree/foo\nbar  https://github.com/OWNER/OTHER-REPO/tree/bar\n",
		},
		{
			name: "list branches for an issue providing an issue url",
			opts: &DevelopOptions{
				IssueSelector: "https://github.com/cli/cli/issues/42",
				List:          true,
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{"hasIssuesEnabled":true,"issue":{"id":"SOMEID","number":42}}}}`),
				)
				reg.Register(
					httpmock.GraphQL(`query LinkedBranchFeature\b`),
					httpmock.StringResponse(featureEnabledPayload),
				)
				reg.Register(
					httpmock.GraphQL(`query ListLinkedBranches\b`),
					httpmock.GraphQLQuery(`
		        {"data":{"repository":{"issue":{"linkedBranches":{"nodes":[{"ref":{"name":"foo","repository":{"url":"https://github.com/OWNER/REPO"}}},{"ref":{"name":"bar","repository":{"url":"https://github.com/OWNER/OTHER-REPO"}}}]}}}}}
					`, func(query string, inputs map[string]interface{}) {
						assert.Equal(t, float64(42), inputs["number"])
						assert.Equal(t, "cli", inputs["owner"])
						assert.Equal(t, "cli", inputs["name"])
					}))
			},
			expectedOut: "foo\thttps://github.com/OWNER/REPO/tree/foo\nbar\thttps://github.com/OWNER/OTHER-REPO/tree/bar\n",
		},
		{
			name: "develop new branch",
			opts: &DevelopOptions{
				IssueSelector: "123",
			},
			remotes: map[string]string{
				"origin": "OWNER/REPO",
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query LinkedBranchFeature\b`),
					httpmock.StringResponse(featureEnabledPayload),
				)
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{"hasIssuesEnabled":true,"issue":{"id": "SOMEID","number":123,"title":"my issue"}}}}`),
				)
				reg.Register(
					httpmock.GraphQL(`query FindBaseOid\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"target":{"oid":"DEFAULTOID"}},"ref":{"target":{"oid":""}}}}}`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation CreateLinkedBranch\b`),
					httpmock.GraphQLMutation(`{"data":{"createLinkedBranch":{"linkedBranch":{"id":"2","ref":{"name":"my-issue-1"}}}}}`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, "DEFAULTOID", inputs["oid"])
							assert.Equal(t, "SOMEID", inputs["issueId"])
						}),
				)
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git fetch origin \+refs/heads/my-issue-1:refs/remotes/origin/my-issue-1`, 0, "")
			},
			expectedOut: "github.com/OWNER/REPO/tree/my-issue-1\n",
		},
		{
			name: "develop new branch with name and base specified",
			opts: &DevelopOptions{
				Name:          "my-branch",
				BaseBranch:    "main",
				IssueSelector: "123",
			},
			remotes: map[string]string{
				"origin": "OWNER/REPO",
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query LinkedBranchFeature\b`),
					httpmock.StringResponse(featureEnabledPayload),
				)
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{ "hasIssuesEnabled":true,"issue":{"id":"SOMEID","number":123,"title":"my issue"}}}}`),
				)
				reg.Register(
					httpmock.GraphQL(`query FindBaseOid\b`),
					httpmock.StringResponse(`{"data":{"repository":{"ref":{"target":{"oid":"OID"}}}}}`))
				reg.Register(
					httpmock.GraphQL(`mutation CreateLinkedBranch\b`),
					httpmock.GraphQLMutation(`{"data":{"createLinkedBranch":{"linkedBranch":{"id":"2","ref":{"name":"my-branch"}}}}}`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, "my-branch", inputs["name"])
							assert.Equal(t, "OID", inputs["oid"])
							assert.Equal(t, "SOMEID", inputs["issueId"])
						}),
				)
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git fetch origin \+refs/heads/my-branch:refs/remotes/origin/my-branch`, 0, "")
			},
			expectedOut: "github.com/OWNER/REPO/tree/my-branch\n",
		},
		{
			name: "develop new branch outside of local git repo",
			opts: &DevelopOptions{
				IssueSelector: "https://github.com/cli/cli/issues/123",
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query LinkedBranchFeature\b`),
					httpmock.StringResponse(featureEnabledPayload),
				)
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{"hasIssuesEnabled":true,"issue":{"id": "SOMEID","number":123,"title":"my issue"}}}}`),
				)
				reg.Register(
					httpmock.GraphQL(`query FindBaseOid\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"target":{"oid":"DEFAULTOID"}},"ref":{"target":{"oid":""}}}}}`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation CreateLinkedBranch\b`),
					httpmock.GraphQLMutation(`{"data":{"createLinkedBranch":{"linkedBranch":{"id":"2","ref":{"name":"my-issue-1"}}}}}`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, "DEFAULTOID", inputs["oid"])
							assert.Equal(t, "SOMEID", inputs["issueId"])
						}),
				)
			},
			expectedOut: "github.com/cli/cli/tree/my-issue-1\n",
		},
		{
			name: "develop new branch with checkout when local branch exists",
			opts: &DevelopOptions{
				Name:          "my-branch",
				IssueSelector: "123",
				Checkout:      true,
			},
			remotes: map[string]string{
				"origin": "OWNER/REPO",
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query LinkedBranchFeature\b`),
					httpmock.StringResponse(featureEnabledPayload),
				)
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{"hasIssuesEnabled":true,"issue":{"id": "SOMEID","number":123,"title":"my issue"}}}}`),
				)
				reg.Register(
					httpmock.GraphQL(`query FindBaseOid\b`),
					httpmock.StringResponse(`{"data":{"repository":{"ref":{"target":{"oid":"OID"}}}}}`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation CreateLinkedBranch\b`),
					httpmock.GraphQLMutation(`{"data":{"createLinkedBranch":{"linkedBranch":{"id":"2","ref":{"name":"my-branch"}}}}}`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, "my-branch", inputs["name"])
							assert.Equal(t, "OID", inputs["oid"])
							assert.Equal(t, "SOMEID", inputs["issueId"])
						}),
				)
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git fetch origin \+refs/heads/my-branch:refs/remotes/origin/my-branch`, 0, "")
				cs.Register(`git rev-parse --verify refs/heads/my-branch`, 0, "")
				cs.Register(`git checkout my-branch`, 0, "")
				cs.Register(`git pull --ff-only origin my-branch`, 0, "")
			},
			expectedOut: "github.com/OWNER/REPO/tree/my-branch\n",
		},
		{
			name: "develop new branch with checkout when local branch does not exist",
			opts: &DevelopOptions{
				Name:          "my-branch",
				IssueSelector: "123",
				Checkout:      true,
			},
			remotes: map[string]string{
				"origin": "OWNER/REPO",
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query LinkedBranchFeature\b`),
					httpmock.StringResponse(featureEnabledPayload),
				)
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{"hasIssuesEnabled":true,"issue":{"id": "SOMEID","number":123,"title":"my issue"}}}}`),
				)
				reg.Register(
					httpmock.GraphQL(`query FindBaseOid\b`),
					httpmock.StringResponse(`{"data":{"repository":{"ref":{"target":{"oid":"OID"}}}}}`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation CreateLinkedBranch\b`),
					httpmock.GraphQLMutation(`{"data":{"createLinkedBranch":{"linkedBranch":{"id":"2","ref":{"name":"my-branch"}}}}}`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, "my-branch", inputs["name"])
							assert.Equal(t, "OID", inputs["oid"])
							assert.Equal(t, "SOMEID", inputs["issueId"])
						}),
				)
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git fetch origin \+refs/heads/my-branch:refs/remotes/origin/my-branch`, 0, "")
				cs.Register(`git rev-parse --verify refs/heads/my-branch`, 1, "")
				cs.Register(`git checkout -b my-branch --track origin/my-branch`, 0, "")
			},
			expectedOut: "github.com/OWNER/REPO/tree/my-branch\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.opts

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg, t)
			}
			opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			ios.SetStdinTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)
			opts.IO = ios

			opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			}

			opts.Remotes = func() (context.Remotes, error) {
				if len(tt.remotes) == 0 {
					return nil, errors.New("no remotes")
				}
				var remotes context.Remotes
				for name, repo := range tt.remotes {
					r, err := ghrepo.FromFullName(repo)
					if err != nil {
						return remotes, err
					}
					remotes = append(remotes, &context.Remote{
						Remote: &git.Remote{Name: name},
						Repo:   r,
					})
				}
				return remotes, nil
			}

			opts.GitClient = &git.Client{
				GhPath:  "some/path/gh",
				GitPath: "some/path/git",
			}

			cmdStubs, cmdTeardown := run.Stub()
			defer cmdTeardown(t)
			if tt.runStubs != nil {
				tt.runStubs(cmdStubs)
			}

			err := developRun(opts)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedOut, stdout.String())
				assert.Equal(t, tt.expectedErrOut, stderr.String())
			}
		})
	}
}
