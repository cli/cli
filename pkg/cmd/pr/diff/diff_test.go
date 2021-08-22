package diff

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/test"
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
				UseColor:    "auto",
			},
		},
		{
			name:  "no argument",
			args:  "",
			isTTY: true,
			want: DiffOptions{
				SelectorArg: "",
				UseColor:    "auto",
			},
		},
		{
			name:  "no color when redirected",
			args:  "",
			isTTY: false,
			want: DiffOptions{
				SelectorArg: "",
				UseColor:    "never",
			},
		},
		{
			name:    "no argument with --repo override",
			args:    "-R owner/repo",
			isTTY:   true,
			wantErr: "argument required when using the --repo flag",
		},
		{
			name:    "invalid --color argument",
			args:    "--color doublerainbow",
			isTTY:   true,
			wantErr: `did not understand color: "doublerainbow". Expected one of always, never, or auto`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			io.SetStdoutTTY(tt.isTTY)
			io.SetStdinTTY(tt.isTTY)
			io.SetStderrTTY(tt.isTTY)

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
	}

	cmd := NewCmdDiff(factory, nil)

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

func TestPRDiff_notty(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.RunCommandFinder("", &api.PullRequest{Number: 123}, ghrepo.New("OWNER", "REPO"))

	http.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/pulls/123"),
		httpmock.StringResponse(testDiff))

	output, err := runCommand(http, nil, false, "")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if diff := cmp.Diff(testDiff, output.String()); diff != "" {
		t.Errorf("command output did not match:\n%s", diff)
	}
}

func TestPRDiff_tty(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.RunCommandFinder("123", &api.PullRequest{Number: 123}, ghrepo.New("OWNER", "REPO"))

	http.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/pulls/123"),
		httpmock.StringResponse(testDiff),
	)

	output, err := runCommand(http, nil, true, "123")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	assert.Contains(t, output.String(), "\x1b[32m+site: bin/gh\x1b[m")
}

const testDiff = `diff --git a/.github/workflows/releases.yml b/.github/workflows/releases.yml
index 73974448..b7fc0154 100644
--- a/.github/workflows/releases.yml
+++ b/.github/workflows/releases.yml
@@ -44,6 +44,11 @@ jobs:
           token: ${{secrets.SITE_GITHUB_TOKEN}}
       - name: Publish documentation site
         if: "!contains(github.ref, '-')" # skip prereleases
+        env:
+          GIT_COMMITTER_NAME: cli automation
+          GIT_AUTHOR_NAME: cli automation
+          GIT_COMMITTER_EMAIL: noreply@github.com
+          GIT_AUTHOR_EMAIL: noreply@github.com
         run: make site-publish
       - name: Move project cards
         if: "!contains(github.ref, '-')" # skip prereleases
diff --git a/Makefile b/Makefile
index f2b4805c..3d7bd0f9 100644
--- a/Makefile
+++ b/Makefile
@@ -22,8 +22,8 @@ test:
 	go test ./...
 .PHONY: test
 
-site:
-	git clone https://github.com/github/cli.github.com.git "$@"
+site: bin/gh
+	bin/gh repo clone github/cli.github.com "$@"
 
 site-docs: site
 	git -C site pull
`
