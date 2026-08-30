package diff

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
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
			name: "name only",
			args: "--name-only",
			want: DiffOptions{
				NameOnly: true,
			},
		},
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
			name:  "exclude single pattern",
			args:  "--exclude '*.yml'",
			isTTY: true,
			want: DiffOptions{
				SelectorArg: "",
				UseColor:    true,
				Exclude:     []string{"*.yml"},
			},
		},
		{
			name:  "exclude multiple patterns",
			args:  "--exclude '*.yml' --exclude Makefile",
			isTTY: true,
			want: DiffOptions{
				SelectorArg: "",
				UseColor:    true,
				Exclude:     []string{"*.yml", "Makefile"},
			},
		},
		{
			name:    "invalid --color argument",
			args:    "--color doublerainbow",
			isTTY:   true,
			wantErr: "invalid argument \"doublerainbow\" for \"--color\" flag: valid values are {always|never|auto}",
		},
		{
			name:  "web mode",
			args:  "123 --web",
			isTTY: true,
			want: DiffOptions{
				SelectorArg: "123",
				UseColor:    true,
				BrowserMode: true,
			},
		},
		{
			name:  "allow escape sequences",
			args:  "--allow-escape-sequences",
			isTTY: true,
			want: DiffOptions{
				SelectorArg:          "",
				UseColor:             true,
				AllowEscapeSequences: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)
			ios.SetColorEnabled(tt.isTTY)

			f := &cmdutil.Factory{
				IOStreams: ios,
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
			assert.Equal(t, tt.want.UseColor, opts.UseColor)
			assert.Equal(t, tt.want.BrowserMode, opts.BrowserMode)
			assert.Equal(t, tt.want.Exclude, opts.Exclude)
			assert.Equal(t, tt.want.AllowEscapeSequences, opts.AllowEscapeSequences)
		})
	}
}

func Test_diffRun(t *testing.T) {
	pr := &api.PullRequest{Number: 123, URL: "https://github.com/OWNER/REPO/pull/123"}

	tests := []struct {
		name           string
		opts           DiffOptions
		notTTY         bool
		wantFields     []string
		wantStdout     string
		wantStderr     string
		wantErr        string
		wantBrowsedURL string
		httpStubs      func(*httpmock.Registry)
	}{
		{
			name: "no color",
			opts: DiffOptions{
				SelectorArg: "123",
				UseColor:    false,
				Patch:       false,
			},
			wantFields: []string{"number"},
			wantStdout: fmt.Sprintf(testDiff, "", "", "", ""),
			httpStubs: func(reg *httpmock.Registry) {
				stubDiffRequest(reg, "application/vnd.github.v3.diff", fmt.Sprintf(testDiff, "", "", "", ""))
			},
		},
		{
			name: "with color",
			opts: DiffOptions{
				SelectorArg: "123",
				UseColor:    true,
				Patch:       false,
			},
			wantFields: []string{"number"},
			wantStdout: fmt.Sprintf(testDiff, "\x1b[m", "\x1b[1;37m", "\x1b[32m", "\x1b[31m"),
			httpStubs: func(reg *httpmock.Registry) {
				stubDiffRequest(reg, "application/vnd.github.v3.diff", fmt.Sprintf(testDiff, "", "", "", ""))
			},
		},
		{
			name: "patch format",
			opts: DiffOptions{
				SelectorArg: "123",
				UseColor:    false,
				Patch:       true,
			},
			wantFields: []string{"number"},
			wantStdout: fmt.Sprintf(testDiff, "", "", "", ""),
			httpStubs: func(reg *httpmock.Registry) {
				stubDiffRequest(reg, "application/vnd.github.v3.patch", fmt.Sprintf(testDiff, "", "", "", ""))
			},
		},
		{
			name: "name only",
			opts: DiffOptions{
				SelectorArg: "123",
				UseColor:    false,
				Patch:       false,
				NameOnly:    true,
			},
			wantFields: []string{"number"},
			wantStdout: ".github/workflows/releases.yml\nMakefile\n",
			httpStubs: func(reg *httpmock.Registry) {
				stubDiffRequest(reg, "application/vnd.github.v3.diff", fmt.Sprintf(testDiff, "", "", "", ""))
			},
		},
		{
			name: "exclude yml files",
			opts: DiffOptions{
				SelectorArg: "123",
				UseColor:    false,
				Exclude:     []string{"*.yml"},
			},
			wantFields: []string{"number"},
			wantStdout: `diff --git a/Makefile b/Makefile
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
`,
			httpStubs: func(reg *httpmock.Registry) {
				stubDiffRequest(reg, "application/vnd.github.v3.diff", fmt.Sprintf(testDiff, "", "", "", ""))
			},
		},
		{
			name: "name only with exclude",
			opts: DiffOptions{
				SelectorArg: "123",
				UseColor:    false,
				NameOnly:    true,
				Exclude:     []string{"*.yml"},
			},
			wantFields: []string{"number"},
			wantStdout: "Makefile\n",
			httpStubs: func(reg *httpmock.Registry) {
				stubDiffRequest(reg, "application/vnd.github.v3.diff", fmt.Sprintf(testDiff, "", "", "", ""))
			},
		},
		{
			name: "web mode",
			opts: DiffOptions{
				SelectorArg: "123",
				BrowserMode: true,
			},
			wantFields:     []string{"url"},
			wantStderr:     "Opening https://github.com/OWNER/REPO/pull/123/files in your browser.\n",
			wantBrowsedURL: "https://github.com/OWNER/REPO/pull/123/files",
		},
		{
			name: "neutralizes escape sequences by default",
			opts: DiffOptions{
				SelectorArg: "123",
				UseColor:    false,
			},
			wantFields: []string{"number"},
			wantStdout: "diff --git a/f b/f\n+ hello ^[[m world\n",
			httpStubs: func(reg *httpmock.Registry) {
				stubDiffRequest(reg, "application/vnd.github.v3.diff", "diff --git a/f b/f\n+ hello \x1b[m world\n")
			},
		},
		{
			name: "passes escape sequences through with --allow-escape-sequences",
			opts: DiffOptions{
				SelectorArg:          "123",
				UseColor:             false,
				AllowEscapeSequences: true,
			},
			wantFields: []string{"number"},
			wantStdout: "diff --git a/f b/f\n+ hello \x1b[m world\n",
			httpStubs: func(reg *httpmock.Registry) {
				stubDiffRequest(reg, "application/vnd.github.v3.diff", "diff --git a/f b/f\n+ hello \x1b[m world\n")
			},
		},
		{
			name: "piped diff with escape sequences is refused",
			opts: DiffOptions{
				SelectorArg: "123",
				UseColor:    false,
			},
			notTTY:     true,
			wantFields: []string{"number"},
			wantErr:    "the diff contains terminal escape sequences; pass --allow-escape-sequences to output it anyway",
			httpStubs: func(reg *httpmock.Registry) {
				stubDiffRequest(reg, "application/vnd.github.v3.diff", "diff --git a/f b/f\n+ hello \x1b[m world\n")
			},
		},
		{
			name: "piped clean diff passes through raw",
			opts: DiffOptions{
				SelectorArg: "123",
				UseColor:    false,
			},
			notTTY:     true,
			wantFields: []string{"number"},
			wantStdout: "diff --git a/f b/f\n+ hello world\n",
			httpStubs: func(reg *httpmock.Registry) {
				stubDiffRequest(reg, "application/vnd.github.v3.diff", "diff --git a/f b/f\n+ hello world\n")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpReg := &httpmock.Registry{}
			defer httpReg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(httpReg)
			}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: httpReg}, nil
			}

			browser := &browser.Stub{}
			tt.opts.Browser = browser

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(!tt.notTTY)
			tt.opts.IO = ios

			finder := shared.NewMockFinder("123", pr, ghrepo.New("OWNER", "REPO"))
			finder.ExpectFields(tt.wantFields)
			tt.opts.Finder = finder

			err := diffRun(&tt.opts)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
			assert.Equal(t, tt.wantBrowsedURL, browser.BrowsedURL())
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
				"\x1b[1;37m",
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

func Test_changedFileNames(t *testing.T) {
	inputs := []struct {
		input, output string
	}{
		{
			input:  "",
			output: "",
		},
		{
			input:  "\n",
			output: "",
		},
		{
			input:  "diff --git a/cmd.go b/cmd.go\n--- /dev/null\n+++ b/cmd.go\n@@ -0,0 +1,313 @@",
			output: "cmd.go\n",
		},
		{
			input:  "diff --git a/cmd.go b/cmd.go\n--- a/cmd.go\n+++ /dev/null\n@@ -0,0 +1,313 @@",
			output: "cmd.go\n",
		},
		{
			input:  fmt.Sprintf("diff --git a/baz.go b/rename.go\n--- a/baz.go\n+++ b/rename.go\n+foo\n-b%sr", strings.Repeat("a", 2*lineBufferSize)),
			output: "rename.go\n",
		},
		{
			input:  fmt.Sprintf("diff --git a/baz.go b/baz.go\n--- a/baz.go\n+++ b/baz.go\n+foo\n-b%sr", strings.Repeat("a", 2*lineBufferSize)),
			output: "baz.go\n",
		},
		{
			input:  "diff --git \"a/\343\202\212\343\203\274\343\201\251\343\201\277\343\203\274.md\" \"b/\343\202\212\343\203\274\343\201\251\343\201\277\343\203\274.md\"",
			output: "\"\343\202\212\343\203\274\343\201\251\343\201\277\343\203\274.md\"\n",
		},
	}
	for _, tt := range inputs {
		buf := bytes.Buffer{}
		if err := changedFilesNames(&buf, strings.NewReader(tt.input)); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if got := buf.String(); got != tt.output {
			t.Errorf("expected: %q, got: %q", tt.output, got)
		}
	}
}

func stubDiffRequest(reg *httpmock.Registry, accept, diff string) {
	reg.Register(
		func(req *http.Request) bool {
			if !strings.EqualFold(req.Method, "GET") {
				return false
			}
			if req.URL.EscapedPath() != "/repos/OWNER/REPO/pulls/123" {
				return false
			}
			return req.Header.Get("Accept") == accept
		},
		func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Request:    req,
				Body:       io.NopCloser(strings.NewReader(diff)),
			}, nil
		})
}

func Test_filterDiff(t *testing.T) {
	rawDiff := fmt.Sprintf(testDiff, "", "", "", "")

	tests := []struct {
		name     string
		patterns []string
		want     string
	}{
		{
			name:     "exclude yml files",
			patterns: []string{"*.yml"},
			want: `diff --git a/Makefile b/Makefile
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
`,
		},
		{
			name:     "exclude Makefile",
			patterns: []string{"Makefile"},
			want: `diff --git a/.github/workflows/releases.yml b/.github/workflows/releases.yml
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
`,
		},
		{
			name:     "exclude all files",
			patterns: []string{"*.yml", "Makefile"},
			want:     "",
		},
		{
			name:     "no matches",
			patterns: []string{"*.go"},
			want:     rawDiff,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, err := filterDiff(strings.NewReader(rawDiff), tt.patterns)
			require.NoError(t, err)
			got, err := io.ReadAll(reader)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
		})
	}
}

func Test_matchesAny(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		patterns []string
		want     bool
	}{
		{
			name:     "exact match",
			filename: "Makefile",
			patterns: []string{"Makefile"},
			want:     true,
		},
		{
			name:     "glob extension",
			filename: ".github/workflows/releases.yml",
			patterns: []string{"*.yml"},
			want:     true,
		},
		{
			name:     "no match",
			filename: "main.go",
			patterns: []string{"*.yml"},
			want:     false,
		},
		{
			name:     "directory glob",
			filename: ".github/workflows/releases.yml",
			patterns: []string{".github/*/*"},
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, matchesAny(tt.filename, tt.patterns))
		})
	}
}

func Test_sanitizedReader(t *testing.T) {
	input := strings.NewReader("\t hello \x1B[m world! ăѣ𝔠ծề\r\n")
	expected := "\t hello ^[[m world! ăѣ𝔠ծề\r\n"

	err := iotest.TestReader(sanitizedReader(input), []byte(expected))
	if err != nil {
		t.Error(err)
	}
}
