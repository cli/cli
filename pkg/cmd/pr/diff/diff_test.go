package diff

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/go-cmp/cmp"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewCmdDiff(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		isTTY   bool
		want    DiffOptions
		wantErr string
	}{
		{
			name:  "number argument",
			args:  "123",
			isTTY: true,
			want: DiffOptions{
				SelectorArg: "123",
				UseColor:    true,
			},
		},
		{
			name:  "no argument",
			args:  "",
			isTTY: true,
			want: DiffOptions{
				SelectorArg: "",
				UseColor:    true,
			},
		},
		{
			name:  "no color when redirected",
			args:  "",
			isTTY: false,
			want: DiffOptions{
				SelectorArg: "",
				UseColor:    false,
			},
		},
		{
			name:  "force color",
			args:  "--color always",
			isTTY: false,
			want: DiffOptions{
				SelectorArg: "",
				UseColor:    true,
			},
		},
		{
			name:  "disable color",
			args:  "--color never",
			isTTY: true,
			want: DiffOptions{
				SelectorArg: "",
				UseColor:    false,
			},
		},
		{
			name:    "no argument with --repo override",
			args:    "-R owner/repo",
			isTTY:   true,
			wantErr: "argument required when using the `--repo` flag",
		},
		{
			name:    "invalid --color argument",
			args:    "--color doublerainbow",
			isTTY:   true,
			wantErr: "the value for `--color` must be one of \"auto\", \"always\", or \"never\"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			io.SetStdoutTTY(tt.isTTY)
			io.SetStdinTTY(tt.isTTY)
			io.SetStderrTTY(tt.isTTY)
			io.SetColorEnabled(tt.isTTY)

			f := &cmdutil.Factory{
				IOStreams: io,
			}

			var opts *DiffOptions
			cmd := NewCmdDiff(f, func(o *DiffOptions) error {
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
			assert.Equal(t, tt.want.UseColor, opts.UseColor)
		})
	}
}

func Test_diffRun(t *testing.T) {
	pr := &api.PullRequest{Number: 123}

	tests := []struct {
		name       string
		opts       DiffOptions
		rawDiff    string
		wantAccept string
		wantStdout string
	}{
		{
			name: "no color",
			opts: DiffOptions{
				SelectorArg: "123",
				UseColor:    false,
				Patch:       false,
			},
			rawDiff:    fmt.Sprintf(testDiff, "", "", "", ""),
			wantAccept: "application/vnd.github.v3.diff",
			wantStdout: fmt.Sprintf(testDiff, "", "", "", ""),
		},
		{
			name: "with color",
			opts: DiffOptions{
				SelectorArg: "123",
				UseColor:    true,
				Patch:       false,
			},
			rawDiff:    fmt.Sprintf(testDiff, "", "", "", ""),
			wantAccept: "application/vnd.github.v3.diff",
			wantStdout: fmt.Sprintf(testDiff, "\x1b[m", "\x1b[1;38m", "\x1b[32m", "\x1b[31m"),
		},
		{
			name: "patch format",
			opts: DiffOptions{
				SelectorArg: "123",
				UseColor:    false,
				Patch:       true,
			},
			rawDiff:    fmt.Sprintf(testDiff, "", "", "", ""),
			wantAccept: "application/vnd.github.v3.patch",
			wantStdout: fmt.Sprintf(testDiff, "", "", "", ""),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpReg := &httpmock.Registry{}
			defer httpReg.Verify(t)

			var gotAccept string
			httpReg.Register(
				httpmock.REST("GET", "repos/OWNER/REPO/pulls/123"),
				func(req *http.Request) (*http.Response, error) {
					gotAccept = req.Header.Get("Accept")
					return &http.Response{
						StatusCode: 200,
						Request:    req,
						Body:       ioutil.NopCloser(strings.NewReader(tt.rawDiff)),
					}, nil
				})

			opts := tt.opts
			opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: httpReg}, nil
			}

			io, _, stdout, stderr := iostreams.Test()
			opts.IO = io

			finder := shared.NewMockFinder("123", pr, ghrepo.New("OWNER", "REPO"))
			finder.ExpectFields([]string{"number"})
			opts.Finder = finder

			if err := diffRun(&opts); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if diff := cmp.Diff(tt.wantStdout, stdout.String()); diff != "" {
				t.Errorf("command output did not match:\n%s", diff)
			}
			if stderr.String() != "" {
				t.Errorf("unexpected stderr output: %s", stderr.String())
			}
			if gotAccept != tt.wantAccept {
				t.Errorf("unexpected Accept header: %s", gotAccept)
			}
		})
	}
}

const testDiff = `%[2]sdiff --git a/.github/workflows/releases.yml b/.github/workflows/releases.yml%[1]s
%[2]sindex 73974448..b7fc0154 100644%[1]s
%[2]s--- a/.github/workflows/releases.yml%[1]s
%[2]s+++ b/.github/workflows/releases.yml%[1]s
@@ -44,6 +44,11 @@ jobs:
           token: ${{secrets.SITE_GITHUB_TOKEN}}
       - name: Publish documentation site
         if: "!contains(github.ref, '-')" # skip prereleases
%[3]s+        env:%[1]s
%[3]s+          GIT_COMMITTER_NAME: cli automation%[1]s
%[3]s+          GIT_AUTHOR_NAME: cli automation%[1]s
%[3]s+          GIT_COMMITTER_EMAIL: noreply@github.com%[1]s
%[3]s+          GIT_AUTHOR_EMAIL: noreply@github.com%[1]s
         run: make site-publish
       - name: Move project cards
         if: "!contains(github.ref, '-')" # skip prereleases
%[2]sdiff --git a/Makefile b/Makefile%[1]s
%[2]sindex f2b4805c..3d7bd0f9 100644%[1]s
%[2]s--- a/Makefile%[1]s
%[2]s+++ b/Makefile%[1]s
@@ -22,8 +22,8 @@ test:
 	go test ./...
 .PHONY: test
 
%[4]s-site:%[1]s
%[4]s-	git clone https://github.com/github/cli.github.com.git "$@"%[1]s
%[3]s+site: bin/gh%[1]s
%[3]s+	bin/gh repo clone github/cli.github.com "$@"%[1]s
 
 site-docs: site
 	git -C site pull
`

func Test_colorDiffLines(t *testing.T) {
	inputs := []struct {
		input, output string
	}{
		{
			input:  "",
			output: "",
		},
		{
			input:  "\n",
			output: "\n",
		},
		{
			input:  "foo\nbar\nbaz\n",
			output: "foo\nbar\nbaz\n",
		},
		{
			input:  "foo\nbar\nbaz",
			output: "foo\nbar\nbaz\n",
		},
		{
			input: fmt.Sprintf("+foo\n-b%sr\n+++ baz\n", strings.Repeat("a", 2*lineBufferSize)),
			output: fmt.Sprintf(
				"%[4]s+foo%[2]s\n%[5]s-b%[1]sr%[2]s\n%[3]s+++ baz%[2]s\n",
				strings.Repeat("a", 2*lineBufferSize),
				"\x1b[m",
				"\x1b[1;38m",
				"\x1b[32m",
				"\x1b[31m",
			),
		},
	}
	for _, tt := range inputs {
		buf := bytes.Buffer{}
		if err := colorDiffLines(&buf, strings.NewReader(tt.input)); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if got := buf.String(); got != tt.output {
			t.Errorf("expected: %q, got: %q", tt.output, got)
		}
	}
}
