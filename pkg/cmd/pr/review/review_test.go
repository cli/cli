package review

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewCmdReview(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "my-body.md")
	err := ioutil.WriteFile(tmpFile, []byte("a body from file"), 0600)
	require.NoError(t, err)

	tests := []struct {
		name    string
		args    string
		stdin   string
		isTTY   bool
		want    ReviewOptions
		wantErr string
	}{
		{
			name:  "number argument",
			args:  "123",
			isTTY: true,
			want: ReviewOptions{
				SelectorArg: "123",
				ReviewType:  0,
				Body:        "",
			},
		},
		{
			name:  "no argument",
			args:  "",
			isTTY: true,
			want: ReviewOptions{
				SelectorArg: "",
				ReviewType:  0,
				Body:        "",
			},
		},
		{
			name:  "body from stdin",
			args:  "123 --request-changes --body-file -",
			stdin: "this is on standard input",
			isTTY: true,
			want: ReviewOptions{
				SelectorArg: "123",
				ReviewType:  1,
				Body:        "this is on standard input",
			},
		},
		{
			name:  "body from file",
			args:  fmt.Sprintf("123 --request-changes --body-file '%s'", tmpFile),
			isTTY: true,
			want: ReviewOptions{
				SelectorArg: "123",
				ReviewType:  1,
				Body:        "a body from file",
			},
		},
		{
			name:    "no argument with --repo override",
			args:    "-R owner/repo",
			isTTY:   true,
			wantErr: "argument required when using the --repo flag",
		},
		{
			name:    "no arguments in non-interactive mode",
			args:    "",
			isTTY:   false,
			wantErr: "--approve, --request-changes, or --comment required when not running interactively",
		},
		{
			name:    "mutually exclusive review types",
			args:    `--approve --comment -b hello`,
			isTTY:   true,
			wantErr: "need exactly one of --approve, --request-changes, or --comment",
		},
		{
			name:    "comment without body",
			args:    `--comment`,
			isTTY:   true,
			wantErr: "body cannot be blank for comment review",
		},
		{
			name:    "request changes without body",
			args:    `--request-changes`,
			isTTY:   true,
			wantErr: "body cannot be blank for request-changes review",
		},
		{
			name:    "only body argument",
			args:    `-b hello`,
			isTTY:   true,
			wantErr: "--body unsupported without --approve, --request-changes, or --comment",
		},
		{
			name:    "body and body-file flags",
			args:    "--body 'test' --body-file 'test-file.txt'",
			isTTY:   true,
			wantErr: "specify only one of `--body` or `--body-file`",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, stdin, _, _ := iostreams.Test()
			io.SetStdoutTTY(tt.isTTY)
			io.SetStdinTTY(tt.isTTY)
			io.SetStderrTTY(tt.isTTY)

			if tt.stdin != "" {
				_, _ = stdin.WriteString(tt.stdin)
			}

			f := &cmdutil.Factory{
				IOStreams: io,
			}

			var opts *ReviewOptions
			cmd := NewCmdReview(f, func(o *ReviewOptions) error {
				opts = o
				return nil
			})
			cmd.PersistentFlags().StringP("repo", "R", "", "")

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)
			cmd.SetArgs(argv)

			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(ioutil.Discard)
			cmd.SetErr(ioutil.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.want.SelectorArg, opts.SelectorArg)
			assert.Equal(t, tt.want.Body, opts.Body)
		})
	}
}

func runCommand(rt http.RoundTripper, remotes context.Remotes, isTTY bool, cli string) (*test.CmdOut, error) {
	io, _, stdout, stderr := iostreams.Test()
	io.SetStdoutTTY(isTTY)
	io.SetStdinTTY(isTTY)
	io.SetStderrTTY(isTTY)

	factory := &cmdutil.Factory{
		IOStreams: io,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		Config: func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
		Remotes: func() (context.Remotes, error) {
			if remotes == nil {
				return context.Remotes{
					{
						Remote: &git.Remote{Name: "origin"},
						Repo:   ghrepo.New("OWNER", "REPO"),
					},
				}, nil
			}

			return remotes, nil
		},
		Branch: func() (string, error) {
			return "feature", nil
		},
	}

	cmd := NewCmdReview(factory, nil)

	argv, err := shlex.Split(cli)
	if err != nil {
		return nil, err
	}
	cmd.SetArgs(argv)

	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(ioutil.Discard)
	cmd.SetErr(ioutil.Discard)

	_, err = cmd.ExecuteC()
	return &test.CmdOut{
		OutBuf: stdout,
		ErrBuf: stderr,
	}, err
}

func TestPRReview_url_arg(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query PullRequestByNumber\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": { "pullRequest": {
				"id": "foobar123",
				"number": 123,
				"headRefName": "feature",
				"headRepositoryOwner": {
					"login": "hubot"
				},
				"headRepository": {
					"name": "REPO",
					"defaultBranchRef": {
						"name": "master"
					}
				},
				"isCrossRepository": false,
				"maintainerCanModify": false
			} } } }`),
	)
	http.Register(
		httpmock.GraphQL(`mutation PullRequestReviewAdd\b`),
		httpmock.GraphQLMutation(`{"data": {} }`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["pullRequestId"], "foobar123")
				assert.Equal(t, inputs["event"], "APPROVE")
				assert.Equal(t, inputs["body"], "")
			}),
	)

	output, err := runCommand(http, nil, true, "--approve https://github.com/OWNER/REPO/pull/123")
	if err != nil {
		t.Fatalf("error running pr review: %s", err)
	}

	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(), "Approved pull request #123")
}

func TestPRReview_number_arg(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query PullRequestByNumber\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": { "pullRequest": {
				"id": "foobar123",
				"number": 123,
				"headRefName": "feature",
				"headRepositoryOwner": {
					"login": "hubot"
				},
				"headRepository": {
					"name": "REPO",
					"defaultBranchRef": {
						"name": "master"
					}
				},
				"isCrossRepository": false,
				"maintainerCanModify": false
			} } } } `),
	)
	http.Register(
		httpmock.GraphQL(`mutation PullRequestReviewAdd`),
		httpmock.GraphQLMutation(`{"data": {} }`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["pullRequestId"], "foobar123")
				assert.Equal(t, inputs["event"], "APPROVE")
				assert.Equal(t, inputs["body"], "")
			}),
	)

	output, err := runCommand(http, nil, true, "--approve 123")
	if err != nil {
		t.Fatalf("error running pr review: %s", err)
	}

	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(), "Approved pull request #123")
}

func TestPRReview_no_arg(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": { "pullRequests": { "nodes": [
				{ "url": "https://github.com/OWNER/REPO/pull/123",
				  "number": 123,
				  "id": "foobar123",
				  "headRefName": "feature",
					"baseRefName": "master" }
			] } } } }`),
	)
	http.Register(
		httpmock.GraphQL(`mutation PullRequestReviewAdd\b`),
		httpmock.GraphQLMutation(`{"data": {} }`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["pullRequestId"], "foobar123")
				assert.Equal(t, inputs["event"], "COMMENT")
				assert.Equal(t, inputs["body"], "cool story")
			}),
	)

	output, err := runCommand(http, nil, true, `--comment -b "cool story"`)
	if err != nil {
		t.Fatalf("error running pr review: %s", err)
	}

	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(), "Reviewed pull request #123")
}

func TestPRReview(t *testing.T) {
	type c struct {
		Cmd           string
		ExpectedEvent string
		ExpectedBody  string
	}
	cases := []c{
		{`--request-changes -b"bad"`, "REQUEST_CHANGES", "bad"},
		{`--approve`, "APPROVE", ""},
		{`--approve -b"hot damn"`, "APPROVE", "hot damn"},
		{`--comment --body "i dunno"`, "COMMENT", "i dunno"},
	}

	for _, kase := range cases {
		t.Run(kase.Cmd, func(t *testing.T) {
			http := &httpmock.Registry{}
			defer http.Verify(t)

			http.Register(
				httpmock.GraphQL(`query PullRequestForBranch\b`),
				httpmock.StringResponse(`
				{ "data": { "repository": { "pullRequests": { "nodes": [
					{ "url": "https://github.com/OWNER/REPO/pull/123",
					"id": "foobar123",
					"headRefName": "feature",
						"baseRefName": "master" }
				] } } } }`),
			)
			http.Register(
				httpmock.GraphQL(`mutation PullRequestReviewAdd\b`),
				httpmock.GraphQLMutation(`{"data": {} }`,
					func(inputs map[string]interface{}) {
						assert.Equal(t, inputs["event"], kase.ExpectedEvent)
						assert.Equal(t, inputs["body"], kase.ExpectedBody)
					}),
			)

			_, err := runCommand(http, nil, false, kase.Cmd)
			if err != nil {
				t.Fatalf("got unexpected error running %s: %s", kase.Cmd, err)
			}
		})
	}
}

func TestPRReview_nontty(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": { "pullRequests": { "nodes": [
				{ "url": "https://github.com/OWNER/REPO/pull/123",
				  "number": 123,
				  "id": "foobar123",
				  "headRefName": "feature",
					"baseRefName": "master" }
			] } } } }`),
	)
	http.Register(
		httpmock.GraphQL(`mutation PullRequestReviewAdd\b`),
		httpmock.GraphQLMutation(`{"data": {} }`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["event"], "COMMENT")
				assert.Equal(t, inputs["body"], "cool")
			}),
	)

	output, err := runCommand(http, nil, false, "-c -bcool")
	if err != nil {
		t.Fatalf("unexpected error running command: %s", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPRReview_interactive(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": { "pullRequests": { "nodes": [
				{ "url": "https://github.com/OWNER/REPO/pull/123",
				  "number": 123,
				  "id": "foobar123",
				  "headRefName": "feature",
					"baseRefName": "master" }
			] } } } }`),
	)
	http.Register(
		httpmock.GraphQL(`mutation PullRequestReviewAdd\b`),
		httpmock.GraphQLMutation(`{"data": {} }`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["event"], "APPROVE")
				assert.Equal(t, inputs["body"], "cool story")
			}),
	)

	as, teardown := prompt.InitAskStubber()
	defer teardown()

	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "reviewType",
			Value: "Approve",
		},
	})
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "body",
			Value: "cool story",
		},
	})
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "confirm",
			Value: true,
		},
	})

	output, err := runCommand(http, nil, true, "")
	if err != nil {
		t.Fatalf("got unexpected error running pr review: %s", err)
	}

	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(), "Approved pull request #123")

	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.String(),
		"Got:",
		"cool.*story")
}

func TestPRReview_interactive_no_body(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": { "pullRequests": { "nodes": [
				{ "url": "https://github.com/OWNER/REPO/pull/123",
				  "id": "foobar123",
				  "headRefName": "feature",
					"baseRefName": "master" }
			] } } } }`),
	)

	as, teardown := prompt.InitAskStubber()
	defer teardown()

	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "reviewType",
			Value: "Request changes",
		},
	})
	as.Stub([]*prompt.QuestionStub{
		{
			Name:    "body",
			Default: true,
		},
	})
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "confirm",
			Value: true,
		},
	})

	_, err := runCommand(http, nil, true, "")
	assert.EqualError(t, err, "this type of review cannot be blank")
}

func TestPRReview_interactive_blank_approve(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": { "pullRequests": { "nodes": [
				{ "url": "https://github.com/OWNER/REPO/pull/123",
				  "number": 123,
				  "id": "foobar123",
				  "headRefName": "feature",
					"baseRefName": "master" }
			] } } } }`),
	)
	http.Register(
		httpmock.GraphQL(`mutation PullRequestReviewAdd\b`),
		httpmock.GraphQLMutation(`{"data": {} }`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["event"], "APPROVE")
				assert.Equal(t, inputs["body"], "")
			}),
	)

	as, teardown := prompt.InitAskStubber()
	defer teardown()

	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "reviewType",
			Value: "Approve",
		},
	})
	as.Stub([]*prompt.QuestionStub{
		{
			Name:    "body",
			Default: true,
		},
	})
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "confirm",
			Value: true,
		},
	})

	output, err := runCommand(http, nil, true, "")
	if err != nil {
		t.Fatalf("got unexpected error running pr review: %s", err)
	}

	unexpect := regexp.MustCompile("Got:")
	if unexpect.MatchString(output.String()) {
		t.Errorf("did not expect to see body printed in %s", output.String())
	}

	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(), "Approved pull request #123")
}
