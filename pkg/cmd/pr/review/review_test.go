package review

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewCmdReview(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "my-body.md")
	err := os.WriteFile(tmpFile, []byte("a body from file"), 0600)
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
			ios, stdin, _, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			if tt.stdin != "" {
				_, _ = stdin.WriteString(tt.stdin)
			}

			f := &cmdutil.Factory{
				IOStreams: ios,
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
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

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

func runCommand(rt http.RoundTripper, prompter prompter.Prompter, remotes context.Remotes, isTTY bool, cli string) (*test.CmdOut, error) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(isTTY)
	ios.SetStdinTTY(isTTY)
	ios.SetStderrTTY(isTTY)

	factory := &cmdutil.Factory{
		IOStreams: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		Config: func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		},
		Prompter: prompter,
	}

	cmd := NewCmdReview(factory, nil)

	argv, err := shlex.Split(cli)
	if err != nil {
		return nil, err
	}
	cmd.SetArgs(argv)

	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	_, err = cmd.ExecuteC()
	return &test.CmdOut{
		OutBuf: stdout,
		ErrBuf: stderr,
	}, err
}

func TestPRReview(t *testing.T) {
	tests := []struct {
		args      string
		wantEvent string
		wantBody  string
	}{
		{
			args:      `--request-changes -b"bad"`,
			wantEvent: "REQUEST_CHANGES",
			wantBody:  "bad",
		},
		{
			args:      `--approve`,
			wantEvent: "APPROVE",
			wantBody:  "",
		},
		{
			args:      `--approve -b"hot damn"`,
			wantEvent: "APPROVE",
			wantBody:  "hot damn",
		},
		{
			args:      `--comment --body "i dunno"`,
			wantEvent: "COMMENT",
			wantBody:  "i dunno",
		},
	}

	for _, tt := range tests {
		t.Run(tt.args, func(t *testing.T) {
			http := &httpmock.Registry{}
			defer http.Verify(t)

			shared.RunCommandFinder("", &api.PullRequest{ID: "THE-ID"}, ghrepo.New("OWNER", "REPO"))

			http.Register(
				httpmock.GraphQL(`mutation PullRequestReviewAdd\b`),
				httpmock.GraphQLMutation(`{"data": {} }`,
					func(inputs map[string]interface{}) {
						assert.Equal(t, map[string]interface{}{
							"pullRequestId": "THE-ID",
							"event":         tt.wantEvent,
							"body":          tt.wantBody,
						}, inputs)
					}),
			)

			output, err := runCommand(http, nil, nil, false, tt.args)
			assert.NoError(t, err)
			assert.Equal(t, "", output.String())
			assert.Equal(t, "", output.Stderr())
		})
	}
}

func TestPRReview_interactive(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.RunCommandFinder("", &api.PullRequest{ID: "THE-ID", Number: 123}, ghrepo.New("OWNER", "REPO"))

	http.Register(
		httpmock.GraphQL(`mutation PullRequestReviewAdd\b`),
		httpmock.GraphQLMutation(`{"data": {} }`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["event"], "APPROVE")
				assert.Equal(t, inputs["body"], "cool story")
			}),
	)

	pm := &prompter.PrompterMock{
		SelectFunc:         func(_, _ string, _ []string) (int, error) { return 1, nil },
		MarkdownEditorFunc: func(_, _ string, _ bool) (string, error) { return "cool story", nil },
		ConfirmFunc:        func(_ string, _ bool) (bool, error) { return true, nil },
	}

	output, err := runCommand(http, pm, nil, true, "")
	assert.NoError(t, err)
	assert.Equal(t, heredoc.Doc(`
		Got:

		  cool story                                                                  
		
	`), output.String())
	assert.Equal(t, "✓ Approved pull request #123\n", output.Stderr())
}

func TestPRReview_interactive_no_body(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.RunCommandFinder("", &api.PullRequest{ID: "THE-ID", Number: 123}, ghrepo.New("OWNER", "REPO"))

	pm := &prompter.PrompterMock{
		SelectFunc:         func(_, _ string, _ []string) (int, error) { return 2, nil },
		MarkdownEditorFunc: func(_, _ string, _ bool) (string, error) { return "", nil },
	}

	_, err := runCommand(http, pm, nil, true, "")
	assert.EqualError(t, err, "this type of review cannot be blank")
}

func TestPRReview_interactive_blank_approve(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.RunCommandFinder("", &api.PullRequest{ID: "THE-ID", Number: 123}, ghrepo.New("OWNER", "REPO"))

	http.Register(
		httpmock.GraphQL(`mutation PullRequestReviewAdd\b`),
		httpmock.GraphQLMutation(`{"data": {} }`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["event"], "APPROVE")
				assert.Equal(t, inputs["body"], "")
			}),
	)

	pm := &prompter.PrompterMock{
		SelectFunc:         func(_, _ string, _ []string) (int, error) { return 1, nil },
		MarkdownEditorFunc: func(_, defVal string, _ bool) (string, error) { return defVal, nil },
		ConfirmFunc:        func(_ string, _ bool) (bool, error) { return true, nil },
	}

	output, err := runCommand(http, pm, nil, true, "")
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "✓ Approved pull request #123\n", output.Stderr())
}
