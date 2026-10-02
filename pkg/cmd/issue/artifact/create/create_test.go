package create

import (
	"bytes"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/attachments"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/cli/cli/v2/pkg/cmd/issue/artifact/client"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	"github.com/google/shlex"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	runbookURL = "https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook"
	// pngBytes is the start of a PNG image, which is binary.
	pngBytes = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00@\x00\x00\x00(\x08\x02\x00\x00\x00"
)

func TestNewCmdCreate(t *testing.T) {
	attachEvent := func(count int64) []ghtelemetry.Event {
		return []ghtelemetry.Event{{
			Type:       "attachment_invocation",
			Dimensions: ghtelemetry.Dimensions{"command": "create"},
			Measures: ghtelemetry.Measures{
				"attach_count": count, "append_ops_count": 0, "replace_ops_count": 0,
			},
		}}
	}

	tests := []struct {
		name string
		// alias runs the command through a parent by this name, such as add.
		alias          string
		args           string
		ghRepo         string
		wantOpts       CreateOptions
		wantAssetPaths []string
		wantEvents     []ghtelemetry.Event
		wantRepo       string
		wantErr        string
		wantFlagErr    bool
	}{
		{
			name: "a file",
			args: "142 signin-plan.md",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromFile, arg: "signin-plan.md"}},
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "a name after #",
			args: "142 'docs/signin-plan.md#Sign-in plan'",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromFile, arg: "docs/signin-plan.md", name: "Sign-in plan"}},
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "a file whose name contains # stays whole, and a name can follow it",
			args: "142 'barista-notes#2.md' 'barista-notes#2.md#Barista notes round 2'",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources: []source{
					{kind: fromFile, arg: "barista-notes#2.md"},
					{kind: fromFile, arg: "barista-notes#2.md", name: "Barista notes round 2"},
				},
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "a name can contain #",
			args: "142 'signin-plan.md#Plan #2'",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromFile, arg: "signin-plan.md", name: "Plan #2"}},
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "a file that doesn't exist splits at the last #, so reading it names the file meant",
			args: "142 'missing.md#Plan'",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromFile, arg: "missing.md", name: "Plan"}},
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "files and URLs keep their order",
			args: "142 signin-plan.md " + runbookURL + " oauth-research.md",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources: []source{
					{kind: fromFile, arg: "signin-plan.md"},
					{kind: fromURL, arg: runbookURL},
					{kind: fromFile, arg: "oauth-research.md"},
				},
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "a URL keeps its #",
			args: "142 " + runbookURL + "#staging",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromURL, arg: runbookURL + "#staging"}},
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "a URL's surrounding whitespace is trimmed",
			args: "142 ' " + runbookURL + "\t\n'",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromURL, arg: runbookURL}},
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "an http URL, and a scheme in capitals",
			args: "142 http://staging.monas-cafe.example/menu HTTPS://github.com/monalisa/monas-cafe",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources: []source{
					{kind: fromURL, arg: "http://staging.monas-cafe.example/menu"},
					{kind: fromURL, arg: "HTTPS://github.com/monalisa/monas-cafe"},
				},
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "a .url file",
			args: "142 oauth-runbook.url",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromFile, arg: "oauth-runbook.url"}},
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "--name names the one file",
			args: "142 signin-plan.md --name 'Sign-in plan'",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromFile, arg: "signin-plan.md", name: "Sign-in plan"}},
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "--name names the one URL",
			args: "142 " + runbookURL + " --name 'Staging OAuth runbook'",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromURL, arg: runbookURL, name: "Staging OAuth runbook"}},
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "--body",
			args: "142 --name 'Root cause' --body 'The order sync skipped decaf orders.'",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromBody, name: "Root cause", body: "The order sync skipped decaf orders."}},
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "--body-file - reads standard input later",
			args: "142 --name 'OAuth research notes' --body-file -",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromStdin, arg: "-", name: "OAuth research notes"}},
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "--body-file with a path is like a file argument",
			args: "142 -F docs/signin-plan.md",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromFile, arg: "docs/signin-plan.md", bodyFile: true}},
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "--body-file with a path and --name",
			args: "142 --body-file oauth-runbook.url --name 'Staging OAuth runbook'",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromFile, arg: "oauth-runbook.url", name: "Staging OAuth runbook", bodyFile: true}},
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "--body-file doesn't split at #",
			args: "142 --body-file 'barista-notes#2.md'",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromFile, arg: "barista-notes#2.md", bodyFile: true}},
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "--type plan",
			args: "142 signin-plan.md --type plan",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromFile, arg: "signin-plan.md"}},
				Type:        "plan",
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "--type generic",
			args: "142 --body 'Notes' --name Notes --type generic",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromBody, name: "Notes", body: "Notes"}},
				Type:        "generic",
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "--attach",
			args: "142 signin-plan.md --attach signin-flow.png --attach './landing-page.png#Landing page'",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromFile, arg: "signin-plan.md"}},
			},
			wantAssetPaths: []string{"signin-flow.png", "./landing-page.png"},
			wantEvents:     attachEvent(2),
			wantRepo:       "OWNER/REPO",
		},
		{
			name: "--attach with a document and a link",
			args: "142 signin-plan.md " + runbookURL + " --attach signin-flow.png",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources: []source{
					{kind: fromFile, arg: "signin-plan.md"},
					{kind: fromURL, arg: runbookURL},
				},
			},
			wantAssetPaths: []string{"signin-flow.png"},
			wantEvents:     attachEvent(1),
			wantRepo:       "OWNER/REPO",
		},
		{
			name:  "the add alias",
			alias: "add",
			args:  "142 signin-plan.md",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromFile, arg: "signin-plan.md"}},
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name:  "the new alias",
			alias: "new",
			args:  "142 signin-plan.md",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromFile, arg: "signin-plan.md"}},
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "an issue URL names the repository",
			args: "https://github.com/monalisa/monas-cafe/issues/142 signin-plan.md",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromFile, arg: "signin-plan.md"}},
			},
			wantRepo: "monalisa/monas-cafe",
		},
		{
			name: "-R selects the repository",
			args: "142 signin-plan.md -R monalisa/monas-cafe",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromFile, arg: "signin-plan.md"}},
			},
			wantRepo: "monalisa/monas-cafe",
		},
		{
			name:   "GH_REPO selects the repository",
			args:   "142 signin-plan.md",
			ghRepo: "monalisa/monas-cafe",
			wantOpts: CreateOptions{
				IssueNumber: 142,
				Sources:     []source{{kind: fromFile, arg: "signin-plan.md"}},
			},
			wantRepo: "monalisa/monas-cafe",
		},
		{
			name:    "no issue",
			args:    "",
			wantErr: "requires at least 1 arg(s), only received 0",
		},
		{
			name:    "an issue argument that isn't an issue",
			args:    "OAuth signin-plan.md",
			wantErr: `invalid issue format: "OAuth"`,
		},
		{
			name:        "nothing to create",
			args:        "142",
			wantErr:     "specify a file or URL, or use `--body` or `--body-file`",
			wantFlagErr: true,
		},
		{
			name:        "an empty --body-file names no file",
			args:        "142 --body-file ''",
			wantErr:     "specify a file or URL, or use `--body` or `--body-file`",
			wantFlagErr: true,
		},
		{
			name:        "--body and --body-file",
			args:        "142 --name Notes --body 'Text' --body-file signin-plan.md",
			wantErr:     "specify only one of `--body` or `--body-file`",
			wantFlagErr: true,
		},
		{
			name:        "files and --body",
			args:        "142 signin-plan.md --name Plan --body 'More text'",
			wantErr:     "specify files or one of --body and --body-file, not both",
			wantFlagErr: true,
		},
		{
			name:        "a URL and --body-file",
			args:        "142 " + runbookURL + " --body-file signin-plan.md",
			wantErr:     "specify files or one of --body and --body-file, not both",
			wantFlagErr: true,
		},
		{
			name:        "--name with several files",
			args:        "142 signin-plan.md oauth-research.md --name Plan",
			wantErr:     "--name can only be used when creating one artifact",
			wantFlagErr: true,
		},
		{
			name:        "--name with #<name>",
			args:        "142 'signin-plan.md#Sign-in plan' --name Plan",
			wantErr:     "specify the name with `#<name>` or `--name`, not both",
			wantFlagErr: true,
		},
		{
			name:        "a blank --name",
			args:        "142 signin-plan.md --name ''",
			wantErr:     "--name cannot be blank",
			wantFlagErr: true,
		},
		{
			name:    "a blank name after #",
			args:    "142 'signin-plan.md#'",
			wantErr: "failed to create artifact from signin-plan.md: the name after `#` cannot be blank",
		},
		{
			name:        "--body needs --name",
			args:        "142 --body 'The order sync skipped decaf orders.'",
			wantErr:     "--name is required when the content has no file name",
			wantFlagErr: true,
		},
		{
			name:        "standard input needs --name",
			args:        "142 --body-file -",
			wantErr:     "--name is required when the content has no file name",
			wantFlagErr: true,
		},
		{
			name:        "a blank --body",
			args:        "142 --name 'Root cause' --body ' \n'",
			wantErr:     "--body cannot be blank",
			wantFlagErr: true,
		},
		{
			name:        "- is only standard input with --body-file",
			args:        "142 -",
			wantErr:     "cannot create an artifact from `-`; use `--body-file -` to read standard input",
			wantFlagErr: true,
		},
		{
			name:        "--type with a URL",
			args:        "142 " + runbookURL + " --type plan",
			wantErr:     "--type only applies to documents; links are created from URLs and .url files",
			wantFlagErr: true,
		},
		{
			name:        "--type with a .url file among documents",
			args:        "142 signin-plan.md oauth-runbook.url --type generic",
			wantErr:     "--type only applies to documents; links are created from URLs and .url files",
			wantFlagErr: true,
		},
		{
			name:        "--type with --body-file and a .url file",
			args:        "142 --body-file OAUTH-RUNBOOK.URL --type plan",
			wantErr:     "--type only applies to documents; links are created from URLs and .url files",
			wantFlagErr: true,
		},
		{
			name:    "a --type gh doesn't know",
			args:    "142 signin-plan.md --type bad-type",
			wantErr: `invalid argument "bad-type" for "--type" flag: valid values are {generic|plan}`,
		},
		{
			name:        "--attach with only links",
			args:        "142 " + runbookURL + " oauth-runbook.url --attach signin-flow.png",
			wantErr:     "--attach can't be used with link artifacts",
			wantFlagErr: true,
		},
		{
			name:       "--attach with a file it can't upload",
			args:       "142 signin-plan.md --attach signin-plan.md",
			wantErr:    "signin-plan.md is not a supported file type (supported: png, jpg, jpeg, gif, webp, svg, mp4, mov, webm)",
			wantEvents: attachEvent(1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			writeTestFile(t, "signin-plan.md", "# Sign-in plan\n")
			writeTestFile(t, "docs/signin-plan.md", "# Sign-in plan\n")
			writeTestFile(t, "oauth-research.md", "# OAuth research\n")
			writeTestFile(t, "barista-notes#2.md", "# Barista notes\n")
			writeTestFile(t, "oauth-runbook.url", "[InternetShortcut]\r\nURL="+runbookURL+"\r\n")
			writeTestFile(t, "signin-flow.png", pngBytes)
			writeTestFile(t, "landing-page.png", pngBytes)

			t.Setenv("GH_REPO", tt.ghRepo)
			ios, stdin, stdout, stderr := iostreams.Test()
			ios.SetStdinTTY(false)
			ios.SetStdoutTTY(true)
			ios.SetStderrTTY(true)
			stdin.WriteString("# OAuth research\n")
			f := &cmdutil.Factory{
				IOStreams: ios,
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
			}

			var gotOpts *CreateOptions
			recorder := &telemetry.InvocationRecorderSpy{}
			cmd := NewCmdCreate(f, recorder, func(opts *CreateOptions) error {
				gotOpts = opts
				return nil
			})
			// gh issue adds -R and its hook, which replaces f.BaseRepo before RunE.
			cmdutil.EnableRepoOverride(cmd, f)

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)
			run := cmd
			if tt.alias != "" {
				run = &cobra.Command{Use: "artifact"}
				run.AddCommand(cmd)
				argv = append([]string{tt.alias}, argv...)
			}
			run.SetArgs(argv)
			run.SetOut(&bytes.Buffer{})
			run.SetErr(&bytes.Buffer{})

			_, err = run.ExecuteC()

			assert.Equal(t, "# OAuth research\n", stdin.String(), "standard input is only read by the run function")
			assert.Equal(t, tt.wantEvents, recorder.Events())
			assert.Equal(t, "", stdout.String())
			assert.Equal(t, "", stderr.String())
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				if tt.wantFlagErr {
					var flagErr *cmdutil.FlagError
					require.ErrorAs(t, err, &flagErr)
				}
				assert.Nil(t, gotOpts)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOpts.IssueNumber, gotOpts.IssueNumber)
			assert.Equal(t, tt.wantOpts.Sources, gotOpts.Sources)
			assert.Equal(t, tt.wantOpts.Type, gotOpts.Type)

			var assetPaths []string
			for _, a := range gotOpts.Assets {
				assetPaths = append(assetPaths, a.Path())
			}
			assert.Equal(t, tt.wantAssetPaths, assetPaths)
			assert.Equal(t, tt.wantAssetPaths != nil, gotOpts.AttachEvent != nil, "--attach use is recorded")

			repo, err := gotOpts.BaseRepo()
			require.NoError(t, err)
			assert.Equal(t, tt.wantRepo, ghrepo.FullName(repo))
		})
	}
}

func TestCreateRun(t *testing.T) {
	apiError := func(status int, message string) error {
		u, err := url.Parse("https://api.github.com/repos/monalisa/monas-cafe/issues/142/artifacts")
		require.NoError(t, err)
		return api.HTTPError{HTTPError: &ghAPI.HTTPError{StatusCode: status, Message: message, RequestURL: u}}
	}
	uploaded := func(name, asset string) attachments.UploadStub {
		return attachments.UploadStub{Name: name, Status: 200, Body: fmt.Sprintf(`{"url": "https://github.com/user-attachments/assets/%s"}`, asset)}
	}
	const (
		signinPlan     = "# Sign-in plan\n\n1. Register the callback URL.\n"
		oauthResearch  = "# OAuth research\n\nGitHub is the only provider we need for now.\n"
		signinPlanFlow = "# Sign-in plan\n\n![Sign-in flow](./signin-flow.png)\n"
		baristaFlow    = "# Barista notes\n\nThe flow in ![Sign-in flow](./signin-flow.png) confused them.\n"
		shortcut       = "[InternetShortcut]\r\nURL=" + runbookURL + "\r\n"
	)

	tests := []struct {
		name          string
		repo          ghrepo.Interface
		files         map[string]string
		symlinks      map[string]string
		attach        []string
		uploads       []attachments.UploadStub
		stdin         string
		opts          CreateOptions
		tty           bool
		color         bool
		isPullRequest bool
		lookupErr     error
		permission    string
		token         string
		createErrs    map[string]error
		wantCalls     []string
		wantBodies    []string
		wantOps       *attachments.UploadResult
		wantStdout    string
		wantStderr    string
		wantErr       string
		wantErrIs     error
	}{
		{
			name:  "a file, named after the file",
			files: map[string]string{"signin-plan.md": signinPlan},
			opts:  CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}}},
			tty:   true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "signin-plan.md"`,
			},
			wantBodies: []string{signinPlan},
			wantStdout: "✓ Created artifact 6 (signin-plan.md) on monalisa/monas-cafe#142\n",
		},
		{
			name:  "a file with #<name>",
			files: map[string]string{"docs/signin-plan.md": signinPlan},
			opts:  CreateOptions{Sources: []source{{kind: fromFile, arg: "docs/signin-plan.md", name: "Sign-in plan"}}},
			tty:   true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "Sign-in plan"`,
			},
			wantBodies: []string{signinPlan},
			wantStdout: "✓ Created artifact 6 (Sign-in plan) on monalisa/monas-cafe#142\n",
		},
		{
			name: "names taken from file names are cleaned up, and typed names aren't",
			files: map[string]string{
				"signin-plan (1).md":     signinPlan,
				"_draft.md":              signinPlan,
				"[wip] café notes!.md":   signinPlan,
				"日本語メモ.md":               signinPlan,
				"docs/plan (final).md":   signinPlan,
				"docs/plan (final) 2.md": signinPlan,
			},
			opts: CreateOptions{Sources: []source{
				{kind: fromFile, arg: "signin-plan (1).md"},
				{kind: fromFile, arg: "_draft.md"},
				{kind: fromFile, arg: "[wip] café notes!.md"},
				{kind: fromFile, arg: "日本語メモ.md"},
				{kind: fromFile, arg: "docs/plan (final).md"},
				{kind: fromFile, arg: "docs/plan (final) 2.md", name: "Plan (final)"},
			}},
			tty: true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "signin-plan 1.md"`,
				`Create monalisa/monas-cafe#142 generic "draft.md"`,
				`Create monalisa/monas-cafe#142 generic "wip café notes.md"`,
				`Create monalisa/monas-cafe#142 generic "日本語メモ.md"`,
				`Create monalisa/monas-cafe#142 generic "plan final.md"`,
				`Create monalisa/monas-cafe#142 generic "Plan (final)"`,
			},
			wantBodies: []string{signinPlan, signinPlan, signinPlan, signinPlan, signinPlan, signinPlan},
			wantStdout: "✓ Created artifact 6 (signin-plan 1.md) on monalisa/monas-cafe#142\n" +
				"✓ Created artifact 7 (draft.md) on monalisa/monas-cafe#142\n" +
				"✓ Created artifact 8 (wip café notes.md) on monalisa/monas-cafe#142\n" +
				"✓ Created artifact 9 (日本語メモ.md) on monalisa/monas-cafe#142\n" +
				"✓ Created artifact 10 (plan final.md) on monalisa/monas-cafe#142\n" +
				"✓ Created artifact 11 (Plan (final)) on monalisa/monas-cafe#142\n",
		},
		{
			name:    "a file name with no letters or numbers can't name an artifact",
			files:   map[string]string{"signin-plan.md": signinPlan, "---": signinPlan},
			opts:    CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}, {kind: fromFile, arg: "---"}}},
			tty:     true,
			wantErr: "failed to create artifact from ---: the file name has no letters or numbers; name it with `#<name>` or `--name`",
		},
		{
			name:    "a --body-file whose file name has no letters or numbers can only be named with --name",
			files:   map[string]string{"---": signinPlan},
			opts:    CreateOptions{Sources: []source{{kind: fromFile, arg: "---", bodyFile: true}}},
			tty:     true,
			wantErr: "failed to create artifact from ---: the file name has no letters or numbers; name it with `--name`",
		},
		{
			name:  "a file name with no letters or numbers, and a typed name",
			files: map[string]string{"---": signinPlan},
			opts:  CreateOptions{Sources: []source{{kind: fromFile, arg: "---", name: "Sign-in plan"}}},
			tty:   true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "Sign-in plan"`,
			},
			wantBodies: []string{signinPlan},
			wantStdout: "✓ Created artifact 6 (Sign-in plan) on monalisa/monas-cafe#142\n",
		},
		{
			name:  "several files, created in argument order",
			files: map[string]string{"signin-plan.md": signinPlan, "oauth-research.md": oauthResearch},
			opts:  CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}, {kind: fromFile, arg: "oauth-research.md"}}},
			tty:   true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "signin-plan.md"`,
				`Create monalisa/monas-cafe#142 generic "oauth-research.md"`,
			},
			wantBodies: []string{signinPlan, oauthResearch},
			wantStdout: "✓ Created artifact 6 (signin-plan.md) on monalisa/monas-cafe#142\n" +
				"✓ Created artifact 7 (oauth-research.md) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "a symlink is read, and named by the path as typed",
			files:    map[string]string{"outside.md": signinPlan},
			symlinks: map[string]string{"docs/signin-plan.md": "../outside.md"},
			opts:     CreateOptions{Sources: []source{{kind: fromFile, arg: "docs/signin-plan.md"}}},
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "signin-plan.md"`,
			},
			wantBodies: []string{signinPlan},
			wantStdout: "✓ Created artifact 6 (signin-plan.md) on monalisa/monas-cafe#142\n",
		},
		{
			name:  "--type plan",
			files: map[string]string{"signin-plan.md": signinPlan},
			opts:  CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}}, Type: "plan"},
			tty:   true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 plan "signin-plan.md"`,
			},
			wantBodies: []string{signinPlan},
			wantStdout: "✓ Created artifact 6 (signin-plan.md) on monalisa/monas-cafe#142\n",
		},
		{
			name:  "standard input",
			stdin: oauthResearch,
			opts:  CreateOptions{Sources: []source{{kind: fromStdin, arg: "-", name: "OAuth research notes"}}},
			tty:   true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "OAuth research notes"`,
			},
			wantBodies: []string{oauthResearch},
			wantStdout: "✓ Created artifact 6 (OAuth research notes) on monalisa/monas-cafe#142\n",
		},
		{
			name: "--body",
			opts: CreateOptions{Sources: []source{{kind: fromBody, name: "Root cause", body: "The order sync skipped decaf orders."}}},
			tty:  true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "Root cause"`,
			},
			wantBodies: []string{"The order sync skipped decaf orders."},
			wantStdout: "✓ Created artifact 6 (Root cause) on monalisa/monas-cafe#142\n",
		},
		{
			name: "a typed name is sent as typed, and printed with its whitespace cleaned up",
			opts: CreateOptions{Sources: []source{{kind: fromBody, name: " Root\tcause\n", body: "The order sync skipped decaf orders."}}},
			tty:  true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic " Root\tcause\n"`,
			},
			wantBodies: []string{"The order sync skipped decaf orders."},
			wantStdout: "✓ Created artifact 6 (Root cause) on monalisa/monas-cafe#142\n",
		},
		{
			name: "a URL, named with --name",
			opts: CreateOptions{Sources: []source{{kind: fromURL, arg: runbookURL, name: "Staging OAuth runbook"}}},
			tty:  true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 link "Staging OAuth runbook"`,
			},
			wantBodies: []string{runbookURL},
			wantStdout: "✓ Created artifact 6 (Staging OAuth runbook) on monalisa/monas-cafe#142\n",
		},
		{
			name: "URLs are named after their hosts, without a port, and keep their #",
			opts: CreateOptions{Sources: []source{
				{kind: fromURL, arg: runbookURL},
				{kind: fromURL, arg: "http://staging.monas-cafe.example:8080/menu#specials"},
			}},
			tty: true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 link "github.com"`,
				`Create monalisa/monas-cafe#142 link "staging.monas-cafe.example"`,
			},
			wantBodies: []string{runbookURL, "http://staging.monas-cafe.example:8080/menu#specials"},
			wantStdout: "✓ Created artifact 6 (github.com) on monalisa/monas-cafe#142\n" +
				"✓ Created artifact 7 (staging.monas-cafe.example) on monalisa/monas-cafe#142\n",
		},
		{
			name:    "a URL that isn't a strict http(s) URL fails before any request",
			opts:    CreateOptions{Sources: []source{{kind: fromURL, arg: "https://ラテ.example/menu"}}},
			tty:     true,
			wantErr: "failed to create artifact from https://ラテ.example/menu: links must be http(s) URLs with a host, in printable ASCII with no spaces",
		},
		{
			name:  "a .url file",
			files: map[string]string{"oauth-runbook.url": shortcut},
			opts:  CreateOptions{Sources: []source{{kind: fromFile, arg: "oauth-runbook.url"}}},
			tty:   true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 link "oauth-runbook.url"`,
			},
			wantBodies: []string{runbookURL},
			wantStdout: "✓ Created artifact 6 (oauth-runbook.url) on monalisa/monas-cafe#142\n",
		},
		{
			name:    "a .url file that isn't a shortcut",
			files:   map[string]string{"broken.url": runbookURL + "\n"},
			opts:    CreateOptions{Sources: []source{{kind: fromFile, arg: "broken.url"}}},
			tty:     true,
			wantErr: "broken.url: no [InternetShortcut] section; pass the URL as an argument instead",
		},
		{
			name:    "a .url file without a URL",
			files:   map[string]string{"broken.url": "[InternetShortcut]\r\nIconIndex=0\r\n"},
			opts:    CreateOptions{Sources: []source{{kind: fromFile, arg: "broken.url"}}},
			tty:     true,
			wantErr: "broken.url: no URL in the [InternetShortcut] section",
		},
		{
			name:    "a .url file that doesn't link to an http(s) URL",
			files:   map[string]string{"broken.url": "[InternetShortcut]\r\nURL=file:///etc/hosts\r\n"},
			opts:    CreateOptions{Sources: []source{{kind: fromFile, arg: "broken.url"}}},
			tty:     true,
			wantErr: "broken.url: the shortcut doesn't link to an http(s) URL",
		},
		{
			name:    "a binary file fails before anything is created",
			files:   map[string]string{"signin-plan.md": signinPlan, "signin-flow.png": pngBytes},
			opts:    CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}, {kind: fromFile, arg: "signin-flow.png"}}},
			tty:     true,
			wantErr: "failed to create artifact from signin-flow.png: binary file not supported",
		},
		{
			name:    "an empty file fails before anything is created",
			files:   map[string]string{"signin-plan.md": signinPlan, "empty.md": "\n  \n"},
			opts:    CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}, {kind: fromFile, arg: "empty.md"}}},
			tty:     true,
			wantErr: "failed to create artifact from empty.md: file is empty",
		},
		{
			name:    "an empty .url file",
			files:   map[string]string{"empty.url": ""},
			opts:    CreateOptions{Sources: []source{{kind: fromFile, arg: "empty.url"}}},
			tty:     true,
			wantErr: "failed to create artifact from empty.url: file is empty",
		},
		{
			name:      "a missing file",
			files:     map[string]string{"signin-plan.md": signinPlan},
			opts:      CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}, {kind: fromFile, arg: "missing.md"}}},
			tty:       true,
			wantErr:   "failed to create artifact from missing.md: ",
			wantErrIs: fs.ErrNotExist,
		},
		{
			name:    "empty standard input",
			stdin:   " \n",
			opts:    CreateOptions{Sources: []source{{kind: fromStdin, arg: "-", name: "OAuth research notes"}}},
			tty:     true,
			wantErr: "failed to create artifact from standard input: input is empty",
		},
		{
			name:    "binary standard input",
			stdin:   pngBytes,
			opts:    CreateOptions{Sources: []source{{kind: fromStdin, arg: "-", name: "Sign-in flow"}}},
			tty:     true,
			wantErr: "failed to create artifact from standard input: binary input not supported",
		},
		{
			name:       "a file the server refuses fails on its own line, and the others are created",
			files:      map[string]string{"signin-plan.md": signinPlan, "order-sync.log": "sync started\n"},
			opts:       CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}, {kind: fromFile, arg: "order-sync.log"}}},
			tty:        true,
			createErrs: map[string]error{"order-sync.log": apiError(422, "artifact body exceeds the size limit")},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "signin-plan.md"`,
				`Create monalisa/monas-cafe#142 generic "order-sync.log"`,
			},
			wantBodies: []string{signinPlan, "sync started\n"},
			wantStdout: "✓ Created artifact 6 (signin-plan.md) on monalisa/monas-cafe#142\n",
			wantStderr: "X Failed to create artifact from order-sync.log: artifact body exceeds the size limit\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:       "without write access, the API's message is the reason",
			files:      map[string]string{"signin-plan.md": signinPlan},
			opts:       CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}}},
			tty:        true,
			createErrs: map[string]error{"signin-plan.md": apiError(404, "Not Found")},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "signin-plan.md"`,
			},
			wantBodies: []string{signinPlan},
			wantStderr: "X Failed to create artifact from signin-plan.md: Not Found\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:       "a failure from --body names the artifact",
			opts:       CreateOptions{Sources: []source{{kind: fromBody, name: "Plan (1)", body: "The order sync skipped decaf orders."}}},
			tty:        true,
			createErrs: map[string]error{"Plan (1)": apiError(422, "Validation Failed")},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "Plan (1)"`,
			},
			wantBodies: []string{"The order sync skipped decaf orders."},
			wantStderr: "X Failed to create artifact (Plan (1)): Validation Failed\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:       "a failure from standard input",
			stdin:      oauthResearch,
			opts:       CreateOptions{Sources: []source{{kind: fromStdin, arg: "-", name: "OAuth research notes"}}},
			tty:        true,
			createErrs: map[string]error{"OAuth research notes": apiError(404, "Not Found")},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "OAuth research notes"`,
			},
			wantBodies: []string{oauthResearch},
			wantStderr: "X Failed to create artifact from standard input: Not Found\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:  "piped output",
			files: map[string]string{"docs/signin-plan.md": signinPlan, "oauth-research.md": oauthResearch},
			opts:  CreateOptions{Sources: []source{{kind: fromFile, arg: "docs/signin-plan.md"}, {kind: fromFile, arg: "oauth-research.md"}}},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "signin-plan.md"`,
				`Create monalisa/monas-cafe#142 generic "oauth-research.md"`,
			},
			wantBodies: []string{signinPlan, oauthResearch},
			wantStdout: "created\t6\tsignin-plan.md\tdocs/signin-plan.md\t\n" +
				"created\t7\toauth-research.md\toauth-research.md\t\n",
		},
		{
			name:       "piped output with a failure",
			files:      map[string]string{"signin-plan.md": signinPlan, "oauth-research.md": oauthResearch},
			opts:       CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}, {kind: fromFile, arg: "oauth-research.md"}}},
			createErrs: map[string]error{"oauth-research.md": apiError(503, "artifact storage is temporarily unavailable")},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "signin-plan.md"`,
				`Create monalisa/monas-cafe#142 generic "oauth-research.md"`,
			},
			wantBodies: []string{signinPlan, oauthResearch},
			wantStdout: "created\t6\tsignin-plan.md\tsignin-plan.md\t\n" +
				"failed\t\toauth-research.md\toauth-research.md\tartifact storage is temporarily unavailable\n",
			wantErrIs: cmdutil.SilentError,
		},
		{
			name: "piped, a URL's source is the URL",
			opts: CreateOptions{Sources: []source{{kind: fromURL, arg: runbookURL}}},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 link "github.com"`,
			},
			wantBodies: []string{runbookURL},
			wantStdout: "created\t6\tgithub.com\t" + runbookURL + "\t\n",
		},
		{
			name:  "piped, standard input's source is -",
			stdin: oauthResearch,
			opts:  CreateOptions{Sources: []source{{kind: fromStdin, arg: "-", name: "OAuth research notes"}}},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "OAuth research notes"`,
			},
			wantBodies: []string{oauthResearch},
			wantStdout: "created\t6\tOAuth research notes\t-\t\n",
		},
		{
			name: "piped, --body has no source, and the name is cleaned up",
			opts: CreateOptions{Sources: []source{{kind: fromBody, name: " Root\tcause\n", body: "The order sync skipped decaf orders."}}},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic " Root\tcause\n"`,
			},
			wantBodies: []string{"The order sync skipped decaf orders."},
			wantStdout: "created\t6\tRoot cause\t\t\n",
		},
		{
			name:       "colors in a terminal",
			files:      map[string]string{"signin-plan.md": signinPlan, "oauth-research.md": oauthResearch},
			opts:       CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}, {kind: fromFile, arg: "oauth-research.md"}}},
			tty:        true,
			color:      true,
			createErrs: map[string]error{"oauth-research.md": apiError(404, "Not Found")},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "signin-plan.md"`,
				`Create monalisa/monas-cafe#142 generic "oauth-research.md"`,
			},
			wantBodies: []string{signinPlan, oauthResearch},
			wantStdout: "\x1b[0;32m✓\x1b[0m Created artifact 6 (signin-plan.md) on monalisa/monas-cafe#142\n",
			wantStderr: "\x1b[0;31mX\x1b[0m Failed to create artifact from oauth-research.md: Not Found\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:          "a pull request is refused before any artifact request",
			files:         map[string]string{"signin-plan.md": signinPlan},
			opts:          CreateOptions{IssueNumber: 158, Sources: []source{{kind: fromFile, arg: "signin-plan.md"}}},
			tty:           true,
			isPullRequest: true,
			wantCalls:     []string{"IsPullRequest monalisa/monas-cafe#158"},
			wantErr:       "monalisa/monas-cafe#158 is a pull request; artifacts are only supported on issues",
		},
		{
			name:      "the lookup's error",
			files:     map[string]string{"signin-plan.md": signinPlan},
			opts:      CreateOptions{IssueNumber: 999, Sources: []source{{kind: fromFile, arg: "signin-plan.md"}}},
			tty:       true,
			lookupErr: fmt.Errorf("GraphQL: Could not resolve to an issue or pull request with the number of 999. (repository.issue)"),
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#999"},
			wantErr:   "GraphQL: Could not resolve to an issue or pull request with the number of 999. (repository.issue)",
		},
		{
			name:    "GitHub Enterprise Server is refused before any request",
			repo:    ghrepo.NewWithHost("monalisa", "monas-cafe", "ghes.monas-cafe.example"),
			files:   map[string]string{"signin-plan.md": signinPlan},
			opts:    CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}}},
			tty:     true,
			wantErr: "issue artifacts are not supported on GitHub Enterprise Server",
		},
		{
			name:  "a ghe.com host is supported",
			repo:  ghrepo.NewWithHost("monalisa", "monas-cafe", "monas-cafe.ghe.com"),
			files: map[string]string{"signin-plan.md": signinPlan},
			opts:  CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}}},
			tty:   true,
			wantCalls: []string{
				"IsPullRequest monas-cafe.ghe.com/monalisa/monas-cafe#142",
				`Create monas-cafe.ghe.com/monalisa/monas-cafe#142 generic "signin-plan.md"`,
			},
			wantBodies: []string{signinPlan},
			wantStdout: "✓ Created artifact 6 (signin-plan.md) on monalisa/monas-cafe#142\n",
		},
		{
			name:    "--attach replaces a reference and appends what isn't referenced",
			files:   map[string]string{"signin-plan.md": signinPlanFlow},
			attach:  []string{"signin-flow.png", "landing-page.png"},
			uploads: []attachments.UploadStub{uploaded("signin-flow.png", "AAA"), uploaded("landing-page.png", "BBB")},
			opts:    CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}}},
			tty:     true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "signin-plan.md"`,
			},
			wantBodies: []string{"# Sign-in plan\n\n![Sign-in flow](https://github.com/user-attachments/assets/AAA)\n\n" +
				"![landing-page](https://github.com/user-attachments/assets/BBB)"},
			wantOps:    &attachments.UploadResult{AppendOperations: 1, ReplaceOperations: 1},
			wantStdout: "✓ Created artifact 6 (signin-plan.md) on monalisa/monas-cafe#142\n",
		},
		{
			name:    "--attach resolves a file's references against its own directory",
			files:   map[string]string{"docs/signin-plan.md": "![Sign-in flow](./signin-flow.png)\n"},
			attach:  []string{"docs/signin-flow.png"},
			uploads: []attachments.UploadStub{uploaded("signin-flow.png", "AAA")},
			opts:    CreateOptions{Sources: []source{{kind: fromFile, arg: "docs/signin-plan.md"}}},
			tty:     true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "signin-plan.md"`,
			},
			wantBodies: []string{"![Sign-in flow](https://github.com/user-attachments/assets/AAA)\n"},
			wantOps:    &attachments.UploadResult{ReplaceOperations: 1},
			wantStdout: "✓ Created artifact 6 (signin-plan.md) on monalisa/monas-cafe#142\n",
		},
		{
			name:    "--attach falls back to the working directory",
			files:   map[string]string{"notes/barista-notes.md": "![Menu photo](./menu-photo.png)\n"},
			attach:  []string{"menu-photo.png"},
			uploads: []attachments.UploadStub{uploaded("menu-photo.png", "AAA")},
			opts:    CreateOptions{Sources: []source{{kind: fromFile, arg: "notes/barista-notes.md"}}},
			tty:     true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "barista-notes.md"`,
			},
			wantBodies: []string{"![Menu photo](https://github.com/user-attachments/assets/AAA)\n"},
			wantOps:    &attachments.UploadResult{ReplaceOperations: 1},
			wantStdout: "✓ Created artifact 6 (barista-notes.md) on monalisa/monas-cafe#142\n",
		},
		{
			name:    "--attach resolves standard input's references against the working directory",
			stdin:   "![Sign-in flow](./signin-flow.png)\n",
			attach:  []string{"signin-flow.png"},
			uploads: []attachments.UploadStub{uploaded("signin-flow.png", "AAA")},
			opts:    CreateOptions{Sources: []source{{kind: fromStdin, arg: "-", name: "Sign-in plan"}}},
			tty:     true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "Sign-in plan"`,
			},
			wantBodies: []string{"![Sign-in flow](https://github.com/user-attachments/assets/AAA)\n"},
			wantOps:    &attachments.UploadResult{ReplaceOperations: 1},
			wantStdout: "✓ Created artifact 6 (Sign-in plan) on monalisa/monas-cafe#142\n",
		},
		{
			name:    "several files share one upload, and nothing is appended",
			files:   map[string]string{"signin-plan.md": signinPlanFlow, "barista-notes.md": baristaFlow},
			attach:  []string{"signin-flow.png"},
			uploads: []attachments.UploadStub{uploaded("signin-flow.png", "AAA")},
			opts:    CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}, {kind: fromFile, arg: "barista-notes.md"}}},
			tty:     true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "signin-plan.md"`,
				`Create monalisa/monas-cafe#142 generic "barista-notes.md"`,
			},
			wantBodies: []string{
				"# Sign-in plan\n\n![Sign-in flow](https://github.com/user-attachments/assets/AAA)\n",
				"# Barista notes\n\nThe flow in ![Sign-in flow](https://github.com/user-attachments/assets/AAA) confused them.\n",
			},
			wantOps: &attachments.UploadResult{ReplaceOperations: 2},
			wantStdout: "✓ Created artifact 6 (signin-plan.md) on monalisa/monas-cafe#142\n" +
				"✓ Created artifact 7 (barista-notes.md) on monalisa/monas-cafe#142\n",
		},
		{
			name:    "with several files, one that references no file is saved as it is",
			files:   map[string]string{"signin-plan.md": signinPlanFlow, "oauth-research.md": oauthResearch},
			attach:  []string{"signin-flow.png"},
			uploads: []attachments.UploadStub{uploaded("signin-flow.png", "AAA")},
			opts:    CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}, {kind: fromFile, arg: "oauth-research.md"}}},
			tty:     true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "signin-plan.md"`,
				`Create monalisa/monas-cafe#142 generic "oauth-research.md"`,
			},
			wantBodies: []string{
				"# Sign-in plan\n\n![Sign-in flow](https://github.com/user-attachments/assets/AAA)\n",
				oauthResearch,
			},
			wantOps: &attachments.UploadResult{ReplaceOperations: 1},
			wantStdout: "✓ Created artifact 6 (signin-plan.md) on monalisa/monas-cafe#142\n" +
				"✓ Created artifact 7 (oauth-research.md) on monalisa/monas-cafe#142\n",
		},
		{
			name:    "a file that references none of the files is saved when an upload fails",
			files:   map[string]string{"signin-plan.md": signinPlanFlow, "oauth-research.md": oauthResearch},
			attach:  []string{"signin-flow.png"},
			uploads: []attachments.UploadStub{{Name: "signin-flow.png", Status: 404, Body: `{"message": "Not Found"}`}},
			opts:    CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}, {kind: fromFile, arg: "oauth-research.md"}}},
			tty:     true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "oauth-research.md"`,
			},
			wantBodies: []string{oauthResearch},
			wantOps:    &attachments.UploadResult{},
			wantStdout: "✓ Created artifact 6 (oauth-research.md) on monalisa/monas-cafe#142\n",
			wantStderr: "X Failed to create artifact from signin-plan.md: could not upload ./signin-flow.png: attaching files requires write access to the repository\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:      "with several files, an attachment no file references uploads and creates nothing",
			files:     map[string]string{"signin-plan.md": signinPlanFlow, "barista-notes.md": baristaFlow},
			attach:    []string{"signin-flow.png", "latte-art.png"},
			opts:      CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}, {kind: fromFile, arg: "barista-notes.md"}}},
			tty:       true,
			wantCalls: []string{"UploadTarget monalisa/monas-cafe#142"},
			wantOps:   &attachments.UploadResult{},
			wantErr:   "no file references ./latte-art.png; reference it in a file, or attach it when creating a single artifact",
		},
		{
			name:      "several attachments no file references",
			files:     map[string]string{"signin-plan.md": signinPlanFlow, "barista-notes.md": baristaFlow},
			attach:    []string{"latte-art.png", "signin-flow.png", "menu-photo.png", "landing-page.png"},
			opts:      CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}, {kind: fromFile, arg: "barista-notes.md"}}},
			tty:       true,
			wantCalls: []string{"UploadTarget monalisa/monas-cafe#142"},
			wantOps:   &attachments.UploadResult{},
			wantErr:   "no file references ./latte-art.png, ./menu-photo.png, or ./landing-page.png; reference them in a file, or attach them when creating a single artifact",
		},
		{
			name:      "two attachments no file references",
			files:     map[string]string{"signin-plan.md": signinPlanFlow, "barista-notes.md": baristaFlow},
			attach:    []string{"latte-art.png", "signin-flow.png", "menu-photo.png"},
			opts:      CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}, {kind: fromFile, arg: "barista-notes.md"}}},
			tty:       true,
			wantCalls: []string{"UploadTarget monalisa/monas-cafe#142"},
			wantOps:   &attachments.UploadResult{},
			wantErr:   "no file references ./latte-art.png or ./menu-photo.png; reference them in a file, or attach them when creating a single artifact",
		},
		{
			name:      "a video embedded through a reference definition uploads and creates nothing",
			files:     map[string]string{"signin-plan.md": "![Repro][clip]\n\n[clip]: ./repro.mp4\n"},
			attach:    []string{"repro.mp4"},
			opts:      CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}}},
			tty:       true,
			wantCalls: []string{"UploadTarget monalisa/monas-cafe#142"},
			wantOps:   &attachments.UploadResult{},
			wantErr:   "cannot embed a video as a reference-style image: ./repro.mp4",
		},
		{
			name:    "with one document and a link, the document takes what isn't referenced",
			files:   map[string]string{"signin-plan.md": signinPlan},
			attach:  []string{"latte-art.png"},
			uploads: []attachments.UploadStub{uploaded("latte-art.png", "AAA")},
			opts:    CreateOptions{Sources: []source{{kind: fromURL, arg: runbookURL}, {kind: fromFile, arg: "signin-plan.md"}}},
			tty:     true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 link "github.com"`,
				`Create monalisa/monas-cafe#142 generic "signin-plan.md"`,
			},
			wantBodies: []string{runbookURL, "# Sign-in plan\n\n1. Register the callback URL.\n\n![latte-art](https://github.com/user-attachments/assets/AAA)"},
			wantOps:    &attachments.UploadResult{AppendOperations: 1},
			wantStdout: "✓ Created artifact 6 (github.com) on monalisa/monas-cafe#142\n" +
				"✓ Created artifact 7 (signin-plan.md) on monalisa/monas-cafe#142\n",
		},
		{
			name:       "a document none of whose files uploaded isn't saved",
			files:      map[string]string{"signin-plan.md": signinPlanFlow},
			attach:     []string{"signin-flow.png"},
			uploads:    []attachments.UploadStub{{Name: "signin-flow.png", Status: 404, Body: `{"message": "Not Found"}`}},
			opts:       CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}}},
			tty:        true,
			wantCalls:  []string{"UploadTarget monalisa/monas-cafe#142"},
			wantOps:    &attachments.UploadResult{},
			wantStderr: "X Failed to create artifact from signin-plan.md: could not upload ./signin-flow.png: attaching files requires write access to the repository\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:   "a document saved with some of its files names the failure",
			files:  map[string]string{"signin-plan.md": "![Sign-in flow](./signin-flow.png)\n\n![Latte art](./latte-art.png)\n"},
			attach: []string{"signin-flow.png", "latte-art.png"},
			uploads: []attachments.UploadStub{
				uploaded("signin-flow.png", "AAA"),
				{Name: "latte-art.png", Status: 429, Body: `{"message": "Too Many Requests"}`},
			},
			opts: CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}}},
			tty:  true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "signin-plan.md"`,
			},
			wantBodies: []string{"![Sign-in flow](https://github.com/user-attachments/assets/AAA)\n\n![Latte art](./latte-art.png)\n"},
			wantOps:    &attachments.UploadResult{ReplaceOperations: 1},
			wantStderr: "! Created artifact 6 (signin-plan.md) on monalisa/monas-cafe#142, but could not upload ./latte-art.png: rate limited; wait and try again\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:   "piped, a document saved with some of its files keeps its status",
			files:  map[string]string{"signin-plan.md": "![Sign-in flow](./signin-flow.png)\n\n![Latte art](./latte-art.png)\n"},
			attach: []string{"signin-flow.png", "latte-art.png"},
			uploads: []attachments.UploadStub{
				uploaded("signin-flow.png", "AAA"),
				{Name: "latte-art.png", Status: 429, Body: `{"message": "Too Many Requests"}`},
			},
			opts: CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}}},
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "signin-plan.md"`,
			},
			wantBodies: []string{"![Sign-in flow](https://github.com/user-attachments/assets/AAA)\n\n![Latte art](./latte-art.png)\n"},
			wantOps:    &attachments.UploadResult{ReplaceOperations: 1},
			wantStdout: "created\t6\tsignin-plan.md\tsignin-plan.md\tcould not upload ./latte-art.png: rate limited; wait and try again\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name: "with several files, each is saved by its own files' uploads",
			files: map[string]string{
				"signin-plan.md":   signinPlanFlow,
				"barista-notes.md": "# Barista notes\n\n![Latte art](./latte-art.png)\n",
			},
			attach: []string{"signin-flow.png", "latte-art.png"},
			uploads: []attachments.UploadStub{
				uploaded("signin-flow.png", "AAA"),
				{Name: "latte-art.png", Status: 429, Body: `{"message": "Too Many Requests"}`},
			},
			opts: CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}, {kind: fromFile, arg: "barista-notes.md"}}},
			tty:  true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "signin-plan.md"`,
			},
			wantBodies: []string{"# Sign-in plan\n\n![Sign-in flow](https://github.com/user-attachments/assets/AAA)\n"},
			wantOps:    &attachments.UploadResult{ReplaceOperations: 1},
			wantStdout: "✓ Created artifact 6 (signin-plan.md) on monalisa/monas-cafe#142\n",
			wantStderr: "X Failed to create artifact from barista-notes.md: could not upload ./latte-art.png: rate limited; wait and try again\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name: "a document whose files were never tried isn't saved",
			files: map[string]string{
				"signin-plan.md":   signinPlanFlow,
				"barista-notes.md": "# Barista notes\n\n![Latte art](./latte-art.png)\n",
			},
			attach:  []string{"signin-flow.png", "latte-art.png"},
			uploads: []attachments.UploadStub{{Name: "signin-flow.png", Status: 404, Body: `{"message": "Not Found"}`}},
			opts:    CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}, {kind: fromFile, arg: "barista-notes.md"}}},
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
			},
			wantOps: &attachments.UploadResult{},
			wantStdout: "failed\t\tsignin-plan.md\tsignin-plan.md\tcould not upload ./signin-flow.png: attaching files requires write access to the repository\n" +
				"failed\t\tbarista-notes.md\tbarista-notes.md\tcould not upload ./signin-flow.png: attaching files requires write access to the repository\n",
			wantErrIs: cmdutil.SilentError,
		},
		{
			name:       "a save that fails after uploading is an ordinary failure",
			files:      map[string]string{"signin-plan.md": signinPlanFlow},
			attach:     []string{"signin-flow.png"},
			uploads:    []attachments.UploadStub{uploaded("signin-flow.png", "AAA")},
			opts:       CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}}},
			tty:        true,
			createErrs: map[string]error{"signin-plan.md": apiError(404, "Not Found")},
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				`Create monalisa/monas-cafe#142 generic "signin-plan.md"`,
			},
			wantBodies: []string{"# Sign-in plan\n\n![Sign-in flow](https://github.com/user-attachments/assets/AAA)\n"},
			wantOps:    &attachments.UploadResult{ReplaceOperations: 1},
			wantStderr: "X Failed to create artifact from signin-plan.md: Not Found\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:          "--attach refuses a pull request with its one lookup",
			files:         map[string]string{"signin-plan.md": signinPlanFlow},
			attach:        []string{"signin-flow.png"},
			opts:          CreateOptions{IssueNumber: 158, Sources: []source{{kind: fromFile, arg: "signin-plan.md"}}},
			tty:           true,
			isPullRequest: true,
			wantCalls:     []string{"UploadTarget monalisa/monas-cafe#158"},
			wantErr:       "monalisa/monas-cafe#158 is a pull request; artifacts are only supported on issues",
		},
		{
			name:       "--attach needs write access before anything uploads",
			files:      map[string]string{"signin-plan.md": signinPlanFlow},
			attach:     []string{"signin-flow.png"},
			opts:       CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}}},
			tty:        true,
			permission: "READ",
			wantCalls:  []string{"UploadTarget monalisa/monas-cafe#142"},
			wantErr:    "attaching files requires write access to the repository",
		},
		{
			name:      "--attach needs a token that can upload",
			files:     map[string]string{"signin-plan.md": signinPlanFlow},
			attach:    []string{"signin-flow.png"},
			opts:      CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}}},
			tty:       true,
			token:     "ghs_aninstallationtoken",
			wantCalls: []string{"UploadTarget monalisa/monas-cafe#142"},
			wantErr:   "unsupported authentication type",
		},
		{
			name:    "--attach uploads to a ghe.com host",
			repo:    ghrepo.NewWithHost("monalisa", "monas-cafe", "monas-cafe.ghe.com"),
			files:   map[string]string{"signin-plan.md": signinPlanFlow},
			attach:  []string{"signin-flow.png"},
			uploads: []attachments.UploadStub{uploaded("signin-flow.png", "AAA")},
			opts:    CreateOptions{Sources: []source{{kind: fromFile, arg: "signin-plan.md"}}},
			tty:     true,
			wantCalls: []string{
				"UploadTarget monas-cafe.ghe.com/monalisa/monas-cafe#142",
				`Create monas-cafe.ghe.com/monalisa/monas-cafe#142 generic "signin-plan.md"`,
			},
			wantBodies: []string{"# Sign-in plan\n\n![Sign-in flow](https://github.com/user-attachments/assets/AAA)\n"},
			wantOps:    &attachments.UploadResult{ReplaceOperations: 1},
			wantStdout: "✓ Created artifact 6 (signin-plan.md) on monalisa/monas-cafe#142\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.repo
			if repo == nil {
				repo = ghrepo.New("monalisa", "monas-cafe")
			}

			// NewTestAssets moves into a new temporary directory, so it runs
			// before anything else is written there.
			opts := tt.opts
			if len(tt.attach) > 0 {
				opts.Assets = attachments.NewTestAssets(t, tt.attach...)
			} else {
				t.Chdir(t.TempDir())
			}
			for path, content := range tt.files {
				writeTestFile(t, path, content)
			}
			for path, target := range tt.symlinks {
				symlinkTestFile(t, target, path)
			}

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			for _, u := range tt.uploads {
				attachments.StubUploadToHost(t, reg, "uploads."+repo.RepoHost(), 1234, u.Name, u.Status, u.Body)
			}

			ios, stdin, stdout, stderr := iostreams.Test()
			ios.SetStdinTTY(false)
			ios.SetStdoutTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)
			ios.SetColorEnabled(tt.color)
			stdin.WriteString(tt.stdin)

			var calls []string
			issueRef := func(r ghrepo.Interface, number int) string {
				if r.RepoHost() != "github.com" {
					return fmt.Sprintf("%s/%s#%d", r.RepoHost(), ghrepo.FullName(r), number)
				}
				return fmt.Sprintf("%s#%d", ghrepo.FullName(r), number)
			}
			// Like the API, numbers go up from the issue's last artifact, 5.
			lastNumber := 5
			mock := &client.ArtifactClientMock{
				IsPullRequestFunc: func(r ghrepo.Interface, number int) (bool, error) {
					calls = append(calls, "IsPullRequest "+issueRef(r, number))
					return tt.isPullRequest, tt.lookupErr
				},
				UploadTargetFunc: func(r ghrepo.Interface, number int) (*client.UploadTarget, error) {
					calls = append(calls, "UploadTarget "+issueRef(r, number))
					if tt.lookupErr != nil {
						return nil, tt.lookupErr
					}
					permission := tt.permission
					if permission == "" {
						permission = "WRITE"
					}
					return &client.UploadTarget{IsPullRequest: tt.isPullRequest, RepositoryID: 1234, ViewerPermission: permission}, nil
				},
				CreateFunc: func(r ghrepo.Interface, issueNumber int, artifactType, name, body string) (*client.Artifact, error) {
					calls = append(calls, fmt.Sprintf("Create %s %s %q", issueRef(r, issueNumber), artifactType, name))
					if err := tt.createErrs[name]; err != nil {
						return nil, err
					}
					lastNumber++
					return &client.Artifact{Number: lastNumber, Type: artifactType, Name: name, Body: body}, nil
				},
			}

			recorder := &telemetry.InvocationRecorderSpy{}
			if len(tt.attach) > 0 {
				opts.AttachEvent = attachments.BeginTelemetry(recorder, "gh test", len(tt.attach))
			}
			if opts.IssueNumber == 0 {
				opts.IssueNumber = 142
			}
			opts.IO = ios
			opts.BaseRepo = func() (ghrepo.Interface, error) { return repo, nil }
			opts.Client = func() (client.ArtifactClient, error) { return mock, nil }
			opts.HttpClient = func() (*http.Client, error) { return &http.Client{Transport: reg}, nil }
			opts.Config = func() (gh.Config, error) {
				token := tt.token
				if token == "" {
					token = "gho_atokenthatcanupload"
				}
				return config.NewMockConfigFromString(fmt.Sprintf("hosts:\n  %s:\n    user: monalisa\n    oauth_token: %s\n", repo.RepoHost(), token)), nil
			}

			err := createRun(&opts)

			assert.Equal(t, tt.wantCalls, calls)
			var bodies []string
			for _, call := range mock.CreateCalls() {
				bodies = append(bodies, call.Body)
			}
			assert.Equal(t, tt.wantBodies, bodies)
			if tt.wantOps != nil {
				attachments.AssertTestTelemetryEvents(t, recorder.Events(), len(tt.attach), *tt.wantOps)
			}
			switch {
			case tt.wantErrIs != nil:
				require.ErrorIs(t, err, tt.wantErrIs)
				if tt.wantErr != "" {
					require.ErrorContains(t, err, tt.wantErr)
				}
			case tt.wantErr != "":
				require.EqualError(t, err, tt.wantErr)
			default:
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}

// writeTestFile creates a file in the working directory, with any missing
// parent directories.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

// symlinkTestFile creates a symlink, or skips the test where that needs a
// privilege the system doesn't grant, as on Windows by default.
func symlinkTestFile(t *testing.T, target, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
}
