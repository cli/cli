package edit

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	runbookURL   = "https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook"
	runbookURLv2 = "https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook-v2"
	// pngBytes is the start of a PNG image, which is binary.
	pngBytes = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00@\x00\x00\x00(\x08\x02\x00\x00\x00"
)

func TestNewCmdEdit(t *testing.T) {
	attachEvent := func(count int64) []ghtelemetry.Event {
		return []ghtelemetry.Event{{
			Type:       "attachment_invocation",
			Dimensions: ghtelemetry.Dimensions{"command": "edit"},
			Measures: ghtelemetry.Measures{
				"attach_count": count, "append_ops_count": 0, "replace_ops_count": 0,
			},
		}}
	}

	tests := []struct {
		name           string
		args           string
		ghRepo         string
		wantOpts       EditOptions
		wantAssetPaths []string
		wantEvents     []ghtelemetry.Event
		wantRepo       string
		wantErr        string
		wantFlagErr    bool
	}{
		{
			name: "--name",
			args: "142 2 --name 'OAuth callback plan v2'",
			wantOpts: EditOptions{
				IssueNumber:    142,
				ArtifactNumber: 2,
				Name:           new("OAuth callback plan v2"),
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "a name of spaces is left to the server",
			args: "142 2 --name '  '",
			wantOpts: EditOptions{
				IssueNumber:    142,
				ArtifactNumber: 2,
				Name:           new("  "),
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "--body",
			args: "142 3 --body " + runbookURLv2,
			wantOpts: EditOptions{
				IssueNumber:    142,
				ArtifactNumber: 3,
				Body:           new(runbookURLv2),
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "-b is --body",
			args: "142 2 -b '# OAuth callback plan'",
			wantOpts: EditOptions{
				IssueNumber:    142,
				ArtifactNumber: 2,
				Body:           new("# OAuth callback plan"),
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "--body-file",
			args: "142 2 --body-file 2-OAuth-callback-plan.md",
			wantOpts: EditOptions{
				IssueNumber:    142,
				ArtifactNumber: 2,
				BodyFile:       "2-OAuth-callback-plan.md",
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "-F is --body-file, and - reads standard input later",
			args: "142 2 -F -",
			wantOpts: EditOptions{
				IssueNumber:    142,
				ArtifactNumber: 2,
				BodyFile:       "-",
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "--name and --body-file",
			args: "142 2 --name 'OAuth callback plan v2' --body-file 2-OAuth-callback-plan.md",
			wantOpts: EditOptions{
				IssueNumber:    142,
				ArtifactNumber: 2,
				Name:           new("OAuth callback plan v2"),
				BodyFile:       "2-OAuth-callback-plan.md",
			},
			wantRepo: "OWNER/REPO",
		},
		{
			name: "--attach on its own",
			args: "142 2 --attach signin-flow.png",
			wantOpts: EditOptions{
				IssueNumber:    142,
				ArtifactNumber: 2,
			},
			wantAssetPaths: []string{"signin-flow.png"},
			wantEvents:     attachEvent(1),
			wantRepo:       "OWNER/REPO",
		},
		{
			name: "--attach with --body-file",
			args: "142 2 --body-file 2-OAuth-callback-plan.md --attach signin-flow.png --attach './landing-page.png#Landing page'",
			wantOpts: EditOptions{
				IssueNumber:    142,
				ArtifactNumber: 2,
				BodyFile:       "2-OAuth-callback-plan.md",
			},
			wantAssetPaths: []string{"signin-flow.png", "./landing-page.png"},
			wantEvents:     attachEvent(2),
			wantRepo:       "OWNER/REPO",
		},
		{
			name: "an issue URL names the repository",
			args: "https://github.com/monalisa/monas-cafe/issues/142 2 --name 'OAuth callback plan v2'",
			wantOpts: EditOptions{
				IssueNumber:    142,
				ArtifactNumber: 2,
				Name:           new("OAuth callback plan v2"),
			},
			wantRepo: "monalisa/monas-cafe",
		},
		{
			name: "-R selects the repository",
			args: "142 2 --name 'OAuth callback plan v2' -R monalisa/monas-cafe",
			wantOpts: EditOptions{
				IssueNumber:    142,
				ArtifactNumber: 2,
				Name:           new("OAuth callback plan v2"),
			},
			wantRepo: "monalisa/monas-cafe",
		},
		{
			name:   "GH_REPO selects the repository",
			args:   "142 2 --name 'OAuth callback plan v2'",
			ghRepo: "monalisa/monas-cafe",
			wantOpts: EditOptions{
				IssueNumber:    142,
				ArtifactNumber: 2,
				Name:           new("OAuth callback plan v2"),
			},
			wantRepo: "monalisa/monas-cafe",
		},
		{
			name:    "no artifact number",
			args:    "142 --name 'OAuth callback plan v2'",
			wantErr: "accepts 2 arg(s), received 1",
		},
		{
			name:    "an artifact number that isn't a number",
			args:    "142 two --name 'OAuth callback plan v2'",
			wantErr: `invalid artifact number: "two"`,
		},
		{
			name:    "an issue argument that isn't an issue",
			args:    "OAuth 2 --name 'OAuth callback plan v2'",
			wantErr: `invalid issue format: "OAuth"`,
		},
		{
			name:        "nothing to change",
			args:        "142 2",
			wantErr:     "specify at least one of `--name`, `--body`, `--body-file`, or `--attach`",
			wantFlagErr: true,
		},
		{
			name:        "an empty --body-file names no file",
			args:        "142 2 --body-file ''",
			wantErr:     "specify at least one of `--name`, `--body`, `--body-file`, or `--attach`",
			wantFlagErr: true,
		},
		{
			name:        "--body and --body-file",
			args:        "142 2 --body 'Text' --body-file signin-plan.md",
			wantErr:     "specify only one of `--body` or `--body-file`",
			wantFlagErr: true,
		},
		{
			name:        "an empty --body and --body-file",
			args:        "142 2 --body '' --body-file signin-plan.md",
			wantErr:     "specify only one of `--body` or `--body-file`",
			wantFlagErr: true,
		},
		{
			name:        "a blank --name",
			args:        "142 2 --name ''",
			wantErr:     "--name cannot be blank",
			wantFlagErr: true,
		},
		{
			name:        "a blank --body",
			args:        "142 2 --body ' \n'",
			wantErr:     "--body cannot be blank",
			wantFlagErr: true,
		},
		{
			name:       "--attach with a file it can't upload",
			args:       "142 2 --attach 2-OAuth-callback-plan.md",
			wantErr:    "2-OAuth-callback-plan.md is not a supported file type (supported: png, jpg, jpeg, gif, webp, svg, mp4, mov, webm)",
			wantEvents: attachEvent(1),
		},
		{
			name:       "--attach with an empty path",
			args:       "142 2 --attach ''",
			wantErr:    "cannot attach an empty path; --attach needs a file path",
			wantEvents: attachEvent(1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			writeTestFile(t, "2-OAuth-callback-plan.md", "# OAuth callback plan\n")
			writeTestFile(t, "signin-flow.png", pngBytes)
			writeTestFile(t, "landing-page.png", pngBytes)

			t.Setenv("GH_REPO", tt.ghRepo)
			ios, stdin, stdout, stderr := iostreams.Test()
			ios.SetStdinTTY(false)
			ios.SetStdoutTTY(true)
			ios.SetStderrTTY(true)
			stdin.WriteString("# OAuth callback plan\n")
			f := &cmdutil.Factory{
				IOStreams: ios,
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
			}

			var gotOpts *EditOptions
			recorder := &telemetry.InvocationRecorderSpy{}
			cmd := NewCmdEdit(f, recorder, func(opts *EditOptions) error {
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

			assert.Equal(t, "# OAuth callback plan\n", stdin.String(), "standard input is only read by the run function")
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
			assert.Equal(t, tt.wantOpts.ArtifactNumber, gotOpts.ArtifactNumber)
			assert.Equal(t, tt.wantOpts.Name, gotOpts.Name)
			assert.Equal(t, tt.wantOpts.Body, gotOpts.Body)
			assert.Equal(t, tt.wantOpts.BodyFile, gotOpts.BodyFile)

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

func TestEditRun(t *testing.T) {
	apiError := func(status int, message string) error {
		u, err := url.Parse("https://api.github.com/repos/monalisa/monas-cafe/issues/142/artifacts/2")
		require.NoError(t, err)
		return api.HTTPError{HTTPError: &ghAPI.HTTPError{StatusCode: status, Message: message, RequestURL: u}}
	}
	uploaded := func(name, asset string) attachments.UploadStub {
		return attachments.UploadStub{Name: name, Status: 200, Body: fmt.Sprintf(`{"url": "https://github.com/user-attachments/assets/%s"}`, asset)}
	}
	const (
		planBody    = "# OAuth callback plan\n\n1. Register the callback URL.\n2. Wire up `order-history` sync."
		newPlan     = "# OAuth callback plan\n\n1. Register the callback URL.\n2. Test the callback on staging.\n"
		newPlanFlow = "# OAuth callback plan\n\n![Sign-in flow](./signin-flow.png)\n"
		shortcut    = "[InternetShortcut]\r\nURL=" + runbookURLv2 + "\r\n"
	)
	plan := client.Artifact{Number: 2, Type: "generic", Name: "OAuth callback plan", Body: planBody}
	approvedPlan := client.Artifact{Number: 1, Type: "plan", Name: "Approved plan", Body: "# Approved plan"}
	runbook := client.Artifact{Number: 3, Type: "link", Name: "Staging OAuth runbook", Body: runbookURL}
	badType := client.Artifact{Number: 2, Type: "bad-type", Name: "OAuth callback plan", Body: planBody}

	tests := []struct {
		name     string
		repo     ghrepo.Interface
		files    map[string]string
		symlinks map[string]string
		attach   []string
		uploads  []attachments.UploadStub
		stdin    string
		opts     EditOptions
		// artifact is what the artifact's lookup returns.
		artifact      client.Artifact
		tty           bool
		color         bool
		isPullRequest bool
		lookupErr     error
		getErr        error
		permission    string
		token         string
		// updatedName is the name the save returns, when it isn't the one
		// sent.
		updatedName string
		updateErr   error
		wantCalls   []string
		// wantName and wantBody are what the save sent, where nil keeps the
		// current value.
		wantName   *string
		wantBody   *string
		wantOps    *attachments.UploadResult
		wantStdout string
		wantStderr string
		wantErr    string
		wantErrIs  error
	}{
		{
			name:     "--name renames a document and keeps its body",
			opts:     EditOptions{Name: new("OAuth callback plan v2")},
			artifact: plan,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantName:   new("OAuth callback plan v2"),
			wantStdout: "✓ Updated artifact 2 (OAuth callback plan v2) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "--body-file replaces a document's body and keeps its name",
			files:    map[string]string{"2-OAuth-callback-plan.md": newPlan},
			opts:     EditOptions{BodyFile: "2-OAuth-callback-plan.md"},
			artifact: plan,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantBody:   new(newPlan),
			wantStdout: "✓ Updated artifact 2 (OAuth callback plan) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "--body-file - reads standard input",
			stdin:    newPlan,
			opts:     EditOptions{BodyFile: "-"},
			artifact: plan,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantBody:   new(newPlan),
			wantStdout: "✓ Updated artifact 2 (OAuth callback plan) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "--body is saved as typed",
			stdin:    "Standard input isn't read.\n",
			opts:     EditOptions{Body: new("  # OAuth callback plan\n")},
			artifact: plan,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantBody:   new("  # OAuth callback plan\n"),
			wantStdout: "✓ Updated artifact 2 (OAuth callback plan) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "--name and --body-file at once",
			files:    map[string]string{"2-OAuth-callback-plan.md": newPlan},
			opts:     EditOptions{Name: new("OAuth callback plan v2"), BodyFile: "2-OAuth-callback-plan.md"},
			artifact: plan,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantName:   new("OAuth callback plan v2"),
			wantBody:   new(newPlan),
			wantStdout: "✓ Updated artifact 2 (OAuth callback plan v2) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "a plan takes new content like any document",
			files:    map[string]string{"approved-plan.md": newPlan},
			opts:     EditOptions{BodyFile: "approved-plan.md"},
			artifact: approvedPlan,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 1",
				"Update monalisa/monas-cafe#142 1",
			},
			wantBody:   new(newPlan),
			wantStdout: "✓ Updated artifact 1 (Approved plan) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "a symlink is read",
			files:    map[string]string{"outside.md": newPlan},
			symlinks: map[string]string{"docs/2-OAuth-callback-plan.md": "../outside.md"},
			opts:     EditOptions{BodyFile: "docs/2-OAuth-callback-plan.md"},
			artifact: plan,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantBody:   new(newPlan),
			wantStdout: "✓ Updated artifact 2 (OAuth callback plan) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "a .url file can't update a document",
			files:    map[string]string{"oauth-runbook.url": shortcut},
			opts:     EditOptions{BodyFile: "oauth-runbook.url"},
			artifact: plan,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
			},
			wantErr: "artifact 2 is not a link; .url files can only update link artifacts",
		},
		{
			name:     "--body with a URL updates a link",
			opts:     EditOptions{Body: new(runbookURLv2)},
			artifact: runbook,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 3",
				"Update monalisa/monas-cafe#142 3",
			},
			wantBody:   new(runbookURLv2),
			wantStdout: "✓ Updated artifact 3 (Staging OAuth runbook) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "a link's new URL is saved without its surrounding whitespace",
			opts:     EditOptions{Body: new(" \t" + runbookURLv2 + "\n")},
			artifact: runbook,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 3",
				"Update monalisa/monas-cafe#142 3",
			},
			wantBody:   new(runbookURLv2),
			wantStdout: "✓ Updated artifact 3 (Staging OAuth runbook) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "a .url file updates a link to its URL",
			files:    map[string]string{"3-Staging-OAuth-runbook.url": shortcut},
			opts:     EditOptions{BodyFile: "3-Staging-OAuth-runbook.url"},
			artifact: runbook,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 3",
				"Update monalisa/monas-cafe#142 3",
			},
			wantBody:   new(runbookURLv2),
			wantStdout: "✓ Updated artifact 3 (Staging OAuth runbook) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "a .url file's extension is matched in any case",
			files:    map[string]string{"OAUTH-RUNBOOK.URL": shortcut},
			opts:     EditOptions{BodyFile: "OAUTH-RUNBOOK.URL"},
			artifact: runbook,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 3",
				"Update monalisa/monas-cafe#142 3",
			},
			wantBody:   new(runbookURLv2),
			wantStdout: "✓ Updated artifact 3 (Staging OAuth runbook) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "any other file updates a link with the bare URL it holds",
			files:    map[string]string{"runbook-v2.txt": runbookURLv2 + "\n"},
			opts:     EditOptions{BodyFile: "runbook-v2.txt"},
			artifact: runbook,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 3",
				"Update monalisa/monas-cafe#142 3",
			},
			wantBody:   new(runbookURLv2),
			wantStdout: "✓ Updated artifact 3 (Staging OAuth runbook) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "standard input updates a link with the bare URL it holds",
			stdin:    runbookURLv2 + "\n",
			opts:     EditOptions{BodyFile: "-"},
			artifact: runbook,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 3",
				"Update monalisa/monas-cafe#142 3",
			},
			wantBody:   new(runbookURLv2),
			wantStdout: "✓ Updated artifact 3 (Staging OAuth runbook) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "--name renames a link and keeps its URL",
			opts:     EditOptions{Name: new("OAuth runbook")},
			artifact: runbook,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 3",
				"Update monalisa/monas-cafe#142 3",
			},
			wantName:   new("OAuth runbook"),
			wantStdout: "✓ Updated artifact 3 (OAuth runbook) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "a text file can't update a link",
			files:    map[string]string{"barista-notes.md": "# Barista feedback notes\n"},
			opts:     EditOptions{BodyFile: "barista-notes.md"},
			artifact: runbook,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 3",
			},
			wantErr: "artifact 3 is a link; its new content must be an http(s) URL or a .url file",
		},
		{
			name:     "--body text can't update a link",
			opts:     EditOptions{Body: new("The runbook moved to the wiki.")},
			artifact: runbook,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 3",
			},
			wantErr: "artifact 3 is a link; its new content must be an http(s) URL or a .url file",
		},
		{
			name:     "a URL that isn't a strict http(s) URL can't update a link",
			stdin:    "https://ラテ.example/menu\n",
			opts:     EditOptions{BodyFile: "-"},
			artifact: runbook,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 3",
			},
			wantErr: "artifact 3 is a link; its new content must be an http(s) URL or a .url file",
		},
		{
			name:     "a .url file that isn't a shortcut",
			files:    map[string]string{"broken.url": runbookURLv2 + "\n"},
			opts:     EditOptions{BodyFile: "broken.url"},
			artifact: runbook,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 3",
			},
			wantErr: "broken.url: no [InternetShortcut] section; pass the URL with `--body` instead",
		},
		{
			name:     "a .url file without a URL",
			files:    map[string]string{"broken.url": "[InternetShortcut]\r\nIconIndex=0\r\n"},
			opts:     EditOptions{BodyFile: "broken.url"},
			artifact: runbook,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 3",
			},
			wantErr: "broken.url: no URL in the [InternetShortcut] section",
		},
		{
			name:     "a .url file that doesn't link to an http(s) URL",
			files:    map[string]string{"broken.url": "[InternetShortcut]\r\nURL=file:///etc/hosts\r\n"},
			opts:     EditOptions{BodyFile: "broken.url"},
			artifact: runbook,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 3",
			},
			wantErr: "broken.url: the shortcut doesn't link to an http(s) URL",
		},
		{
			name:     "--attach can't be used with a link, and nothing uploads",
			attach:   []string{"signin-flow.png"},
			artifact: runbook,
			tty:      true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 3",
			},
			wantOps: &attachments.UploadResult{},
			wantErr: "artifact 3 is a link; --attach can't be used with link artifacts",
		},
		{
			name:     "a type gh doesn't know is refused before anything is saved",
			opts:     EditOptions{Name: new("OAuth callback plan v2")},
			artifact: badType,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
			},
			wantErr: `artifact 2 has type "bad-type", which this version of gh doesn't support; upgrade gh`,
		},
		{
			name:     "with --attach, a type gh doesn't know is refused before anything uploads",
			attach:   []string{"signin-flow.png"},
			artifact: badType,
			tty:      true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
			},
			wantOps: &attachments.UploadResult{},
			wantErr: `artifact 2 has type "bad-type", which this version of gh doesn't support; upgrade gh`,
		},
		{
			name:      "a missing file fails before any request",
			opts:      EditOptions{BodyFile: "missing.md"},
			artifact:  plan,
			tty:       true,
			wantErr:   "failed to update artifact 2 from missing.md: ",
			wantErrIs: fs.ErrNotExist,
		},
		{
			name:     "a binary file fails before any request",
			files:    map[string]string{"signin-flow.png": pngBytes},
			opts:     EditOptions{BodyFile: "signin-flow.png"},
			artifact: plan,
			tty:      true,
			wantErr:  "failed to update artifact 2 from signin-flow.png: binary file not supported",
		},
		{
			name:     "an empty file fails before any request",
			files:    map[string]string{"empty.md": "\n  \n"},
			opts:     EditOptions{BodyFile: "empty.md"},
			artifact: plan,
			tty:      true,
			wantErr:  "failed to update artifact 2 from empty.md: file is empty",
		},
		{
			name:     "empty standard input fails before any request",
			stdin:    " \n",
			opts:     EditOptions{BodyFile: "-"},
			artifact: runbook,
			tty:      true,
			wantErr:  "failed to update artifact 3 from standard input: input is empty",
		},
		{
			name:     "binary standard input fails before any request",
			stdin:    pngBytes,
			opts:     EditOptions{BodyFile: "-"},
			artifact: plan,
			tty:      true,
			wantErr:  "failed to update artifact 2 from standard input: binary input not supported",
		},
		{
			name:     "a failed lookup is the artifact's failure line",
			opts:     EditOptions{ArtifactNumber: 9, Name: new("OAuth callback plan v2")},
			artifact: plan,
			tty:      true,
			getErr:   apiError(404, "Not Found"),
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 9",
			},
			wantStderr: "X Failed to update artifact 9: Not Found\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:     "piped, a failed lookup leaves the name empty",
			files:    map[string]string{"2-OAuth-callback-plan.md": newPlan},
			opts:     EditOptions{ArtifactNumber: 9, BodyFile: "2-OAuth-callback-plan.md"},
			artifact: plan,
			getErr:   apiError(404, "Not Found"),
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 9",
			},
			wantStdout: "failed\t9\t\t2-OAuth-callback-plan.md\tNot Found\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:      "a name the server refuses fails on the artifact's line",
			opts:      EditOptions{Name: new("Plan (1)")},
			artifact:  plan,
			tty:       true,
			updateErr: apiError(422, "Validation Failed"),
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantName:   new("Plan (1)"),
			wantStderr: "X Failed to update artifact 2: Validation Failed\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:      "piped, a failed save keeps the current name",
			opts:      EditOptions{Name: new("Plan (1)")},
			artifact:  plan,
			updateErr: apiError(422, "Validation Failed"),
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantName:   new("Plan (1)"),
			wantStdout: "failed\t2\tOAuth callback plan\t\tValidation Failed\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:      "without write access, the API's message is the reason",
			files:     map[string]string{"2-OAuth-callback-plan.md": newPlan},
			opts:      EditOptions{BodyFile: "2-OAuth-callback-plan.md"},
			artifact:  plan,
			tty:       true,
			updateErr: apiError(404, "Not Found"),
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantBody:   new(newPlan),
			wantStderr: "X Failed to update artifact 2: Not Found\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:     "piped output",
			opts:     EditOptions{Name: new("OAuth callback plan v2")},
			artifact: plan,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantName:   new("OAuth callback plan v2"),
			wantStdout: "updated\t2\tOAuth callback plan v2\t\t\n",
		},
		{
			name:     "piped, a --body-file's source is its path",
			files:    map[string]string{"docs/2-OAuth-callback-plan.md": newPlan},
			opts:     EditOptions{BodyFile: "docs/2-OAuth-callback-plan.md"},
			artifact: plan,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantBody:   new(newPlan),
			wantStdout: "updated\t2\tOAuth callback plan\tdocs/2-OAuth-callback-plan.md\t\n",
		},
		{
			name:     "piped, standard input's source is -",
			stdin:    runbookURLv2,
			opts:     EditOptions{BodyFile: "-"},
			artifact: runbook,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 3",
				"Update monalisa/monas-cafe#142 3",
			},
			wantBody:   new(runbookURLv2),
			wantStdout: "updated\t3\tStaging OAuth runbook\t-\t\n",
		},
		{
			name:        "the saved name is printed with its whitespace cleaned up",
			opts:        EditOptions{Name: new(" OAuth\tcallback plan\nv2 ")},
			artifact:    plan,
			updatedName: " OAuth\tcallback plan\nv2 ",
			tty:         true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantName:   new(" OAuth\tcallback plan\nv2 "),
			wantStdout: "✓ Updated artifact 2 (OAuth callback plan v2) on monalisa/monas-cafe#142\n",
		},
		{
			name:        "piped, the saved name is cleaned up too",
			opts:        EditOptions{Name: new(" OAuth\tcallback plan\nv2 ")},
			artifact:    plan,
			updatedName: " OAuth\tcallback plan\nv2 ",
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantName:   new(" OAuth\tcallback plan\nv2 "),
			wantStdout: "updated\t2\tOAuth callback plan v2\t\t\n",
		},
		{
			name:     "colors in a terminal",
			opts:     EditOptions{Name: new("OAuth callback plan v2")},
			artifact: plan,
			tty:      true,
			color:    true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantName:   new("OAuth callback plan v2"),
			wantStdout: "\x1b[0;32m✓\x1b[0m Updated artifact 2 (OAuth callback plan v2) on monalisa/monas-cafe#142\n",
		},
		{
			name:      "colors in a terminal, for a failure",
			opts:      EditOptions{Name: new("OAuth callback plan v2")},
			artifact:  plan,
			tty:       true,
			color:     true,
			updateErr: apiError(404, "Not Found"),
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantName:   new("OAuth callback plan v2"),
			wantStderr: "\x1b[0;31mX\x1b[0m Failed to update artifact 2: Not Found\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:          "a pull request is refused before any artifact request",
			opts:          EditOptions{IssueNumber: 158, Name: new("OAuth callback plan v2")},
			artifact:      plan,
			tty:           true,
			isPullRequest: true,
			wantCalls:     []string{"IsPullRequest monalisa/monas-cafe#158"},
			wantErr:       "monalisa/monas-cafe#158 is a pull request; artifacts are only supported on issues",
		},
		{
			name:      "the issue lookup's error",
			opts:      EditOptions{IssueNumber: 999, Name: new("OAuth callback plan v2")},
			artifact:  plan,
			tty:       true,
			lookupErr: fmt.Errorf("GraphQL: Could not resolve to an issue or pull request with the number of 999. (repository.issue)"),
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#999"},
			wantErr:   "GraphQL: Could not resolve to an issue or pull request with the number of 999. (repository.issue)",
		},
		{
			name:     "GitHub Enterprise Server is refused before any request",
			repo:     ghrepo.NewWithHost("monalisa", "monas-cafe", "ghes.monas-cafe.example"),
			opts:     EditOptions{Name: new("OAuth callback plan v2")},
			artifact: plan,
			tty:      true,
			wantErr:  "issue artifacts are not supported on GitHub Enterprise Server",
		},
		{
			name:     "a ghe.com host is supported",
			repo:     ghrepo.NewWithHost("monalisa", "monas-cafe", "monas-cafe.ghe.com"),
			opts:     EditOptions{Name: new("OAuth callback plan v2")},
			artifact: plan,
			tty:      true,
			wantCalls: []string{
				"IsPullRequest monas-cafe.ghe.com/monalisa/monas-cafe#142",
				"Get monas-cafe.ghe.com/monalisa/monas-cafe#142 2",
				"Update monas-cafe.ghe.com/monalisa/monas-cafe#142 2",
			},
			wantName:   new("OAuth callback plan v2"),
			wantStdout: "✓ Updated artifact 2 (OAuth callback plan v2) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "--attach on its own appends to the current body",
			attach:   []string{"signin-flow.png"},
			uploads:  []attachments.UploadStub{uploaded("signin-flow.png", "AAA")},
			artifact: plan,
			tty:      true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantBody:   new(planBody + "\n\n![signin-flow](https://github.com/user-attachments/assets/AAA)"),
			wantOps:    &attachments.UploadResult{AppendOperations: 1},
			wantStdout: "✓ Updated artifact 2 (OAuth callback plan) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "--attach with --name renames and appends",
			attach:   []string{"signin-flow.png"},
			uploads:  []attachments.UploadStub{uploaded("signin-flow.png", "AAA")},
			opts:     EditOptions{Name: new("OAuth callback plan v2")},
			artifact: plan,
			tty:      true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantName:   new("OAuth callback plan v2"),
			wantBody:   new(planBody + "\n\n![signin-flow](https://github.com/user-attachments/assets/AAA)"),
			wantOps:    &attachments.UploadResult{AppendOperations: 1},
			wantStdout: "✓ Updated artifact 2 (OAuth callback plan v2) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "--attach replaces a reference in the new body and appends what isn't referenced",
			files:    map[string]string{"2-OAuth-callback-plan.md": newPlanFlow},
			attach:   []string{"signin-flow.png", "landing-page.png"},
			uploads:  []attachments.UploadStub{uploaded("signin-flow.png", "AAA"), uploaded("landing-page.png", "BBB")},
			opts:     EditOptions{BodyFile: "2-OAuth-callback-plan.md"},
			artifact: plan,
			tty:      true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantBody: new("# OAuth callback plan\n\n![Sign-in flow](https://github.com/user-attachments/assets/AAA)\n\n" +
				"![landing-page](https://github.com/user-attachments/assets/BBB)"),
			wantOps:    &attachments.UploadResult{AppendOperations: 1, ReplaceOperations: 1},
			wantStdout: "✓ Updated artifact 2 (OAuth callback plan) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "--attach resolves a --body-file's references against its own directory",
			files:    map[string]string{"docs/2-OAuth-callback-plan.md": newPlanFlow},
			attach:   []string{"docs/signin-flow.png"},
			uploads:  []attachments.UploadStub{uploaded("signin-flow.png", "AAA")},
			opts:     EditOptions{BodyFile: "docs/2-OAuth-callback-plan.md"},
			artifact: plan,
			tty:      true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantBody:   new("# OAuth callback plan\n\n![Sign-in flow](https://github.com/user-attachments/assets/AAA)\n"),
			wantOps:    &attachments.UploadResult{ReplaceOperations: 1},
			wantStdout: "✓ Updated artifact 2 (OAuth callback plan) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "--attach falls back to the working directory",
			files:    map[string]string{"notes/2-OAuth-callback-plan.md": newPlanFlow},
			attach:   []string{"signin-flow.png"},
			uploads:  []attachments.UploadStub{uploaded("signin-flow.png", "AAA")},
			opts:     EditOptions{BodyFile: "notes/2-OAuth-callback-plan.md"},
			artifact: plan,
			tty:      true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantBody:   new("# OAuth callback plan\n\n![Sign-in flow](https://github.com/user-attachments/assets/AAA)\n"),
			wantOps:    &attachments.UploadResult{ReplaceOperations: 1},
			wantStdout: "✓ Updated artifact 2 (OAuth callback plan) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "--attach resolves standard input's references against the working directory",
			stdin:    newPlanFlow,
			attach:   []string{"signin-flow.png"},
			uploads:  []attachments.UploadStub{uploaded("signin-flow.png", "AAA")},
			opts:     EditOptions{BodyFile: "-"},
			artifact: plan,
			tty:      true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantBody:   new("# OAuth callback plan\n\n![Sign-in flow](https://github.com/user-attachments/assets/AAA)\n"),
			wantOps:    &attachments.UploadResult{ReplaceOperations: 1},
			wantStdout: "✓ Updated artifact 2 (OAuth callback plan) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "--attach resolves the current body's references against the working directory",
			attach:   []string{"signin-flow.png"},
			uploads:  []attachments.UploadStub{uploaded("signin-flow.png", "AAA")},
			artifact: client.Artifact{Number: 2, Type: "generic", Name: "OAuth callback plan", Body: newPlanFlow},
			tty:      true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantBody:   new("# OAuth callback plan\n\n![Sign-in flow](https://github.com/user-attachments/assets/AAA)\n"),
			wantOps:    &attachments.UploadResult{ReplaceOperations: 1},
			wantStdout: "✓ Updated artifact 2 (OAuth callback plan) on monalisa/monas-cafe#142\n",
		},
		{
			name:     "with --attach, a .url file for a document fails before anything uploads",
			files:    map[string]string{"oauth-runbook.url": shortcut},
			attach:   []string{"signin-flow.png"},
			opts:     EditOptions{BodyFile: "oauth-runbook.url"},
			artifact: plan,
			tty:      true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
			},
			wantOps: &attachments.UploadResult{},
			wantErr: "artifact 2 is not a link; .url files can only update link artifacts",
		},
		{
			name:     "an artifact none of whose files uploaded isn't saved",
			attach:   []string{"signin-flow.png"},
			uploads:  []attachments.UploadStub{{Name: "signin-flow.png", Status: 404, Body: `{"message": "Not Found"}`}},
			opts:     EditOptions{Name: new("OAuth callback plan v2")},
			artifact: plan,
			tty:      true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
			},
			wantOps:    &attachments.UploadResult{},
			wantStderr: "X Failed to update artifact 2: could not upload ./signin-flow.png: attaching files requires write access to the repository\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:     "piped, an artifact none of whose files uploaded fails",
			files:    map[string]string{"2-OAuth-callback-plan.md": newPlanFlow},
			attach:   []string{"signin-flow.png"},
			uploads:  []attachments.UploadStub{{Name: "signin-flow.png", Status: 404, Body: `{"message": "Not Found"}`}},
			opts:     EditOptions{BodyFile: "2-OAuth-callback-plan.md"},
			artifact: plan,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
			},
			wantOps:    &attachments.UploadResult{},
			wantStdout: "failed\t2\tOAuth callback plan\t2-OAuth-callback-plan.md\tcould not upload ./signin-flow.png: attaching files requires write access to the repository\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:   "an artifact saved with some of its files names the failure",
			files:  map[string]string{"2-OAuth-callback-plan.md": "![Sign-in flow](./signin-flow.png)\n\n![Latte art](./latte-art.png)\n"},
			attach: []string{"signin-flow.png", "latte-art.png"},
			uploads: []attachments.UploadStub{
				uploaded("signin-flow.png", "AAA"),
				{Name: "latte-art.png", Status: 429, Body: `{"message": "Too Many Requests"}`},
			},
			opts:     EditOptions{BodyFile: "2-OAuth-callback-plan.md"},
			artifact: plan,
			tty:      true,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantBody:   new("![Sign-in flow](https://github.com/user-attachments/assets/AAA)\n\n![Latte art](./latte-art.png)\n"),
			wantOps:    &attachments.UploadResult{ReplaceOperations: 1},
			wantStderr: "! Updated artifact 2 (OAuth callback plan) on monalisa/monas-cafe#142, but could not upload ./latte-art.png: rate limited; wait and try again\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:   "piped, an artifact saved with some of its files keeps its status",
			files:  map[string]string{"2-OAuth-callback-plan.md": "![Sign-in flow](./signin-flow.png)\n\n![Latte art](./latte-art.png)\n"},
			attach: []string{"signin-flow.png", "latte-art.png"},
			uploads: []attachments.UploadStub{
				uploaded("signin-flow.png", "AAA"),
				{Name: "latte-art.png", Status: 429, Body: `{"message": "Too Many Requests"}`},
			},
			opts:     EditOptions{BodyFile: "2-OAuth-callback-plan.md"},
			artifact: plan,
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantBody:   new("![Sign-in flow](https://github.com/user-attachments/assets/AAA)\n\n![Latte art](./latte-art.png)\n"),
			wantOps:    &attachments.UploadResult{ReplaceOperations: 1},
			wantStdout: "updated\t2\tOAuth callback plan\t2-OAuth-callback-plan.md\tcould not upload ./latte-art.png: rate limited; wait and try again\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:      "a save that fails after uploading is an ordinary failure",
			attach:    []string{"signin-flow.png"},
			uploads:   []attachments.UploadStub{uploaded("signin-flow.png", "AAA")},
			artifact:  plan,
			tty:       true,
			updateErr: apiError(404, "Not Found"),
			wantCalls: []string{
				"UploadTarget monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 2",
				"Update monalisa/monas-cafe#142 2",
			},
			wantBody:   new(planBody + "\n\n![signin-flow](https://github.com/user-attachments/assets/AAA)"),
			wantOps:    &attachments.UploadResult{AppendOperations: 1},
			wantStderr: "X Failed to update artifact 2: Not Found\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:          "--attach refuses a pull request with its one lookup",
			attach:        []string{"signin-flow.png"},
			opts:          EditOptions{IssueNumber: 158},
			artifact:      plan,
			tty:           true,
			isPullRequest: true,
			wantCalls:     []string{"UploadTarget monalisa/monas-cafe#158"},
			wantErr:       "monalisa/monas-cafe#158 is a pull request; artifacts are only supported on issues",
		},
		{
			name:       "--attach needs write access before any artifact request",
			attach:     []string{"signin-flow.png"},
			artifact:   plan,
			tty:        true,
			permission: "READ",
			wantCalls:  []string{"UploadTarget monalisa/monas-cafe#142"},
			wantErr:    "attaching files requires write access to the repository",
		},
		{
			name:      "--attach needs a token that can upload",
			attach:    []string{"signin-flow.png"},
			artifact:  plan,
			tty:       true,
			token:     "ghs_aninstallationtoken",
			wantCalls: []string{"UploadTarget monalisa/monas-cafe#142"},
			wantErr:   "unsupported authentication type",
		},
		{
			name:     "--attach uploads to a ghe.com host",
			repo:     ghrepo.NewWithHost("monalisa", "monas-cafe", "monas-cafe.ghe.com"),
			attach:   []string{"signin-flow.png"},
			uploads:  []attachments.UploadStub{uploaded("signin-flow.png", "AAA")},
			artifact: plan,
			tty:      true,
			wantCalls: []string{
				"UploadTarget monas-cafe.ghe.com/monalisa/monas-cafe#142",
				"Get monas-cafe.ghe.com/monalisa/monas-cafe#142 2",
				"Update monas-cafe.ghe.com/monalisa/monas-cafe#142 2",
			},
			wantBody:   new(planBody + "\n\n![signin-flow](https://github.com/user-attachments/assets/AAA)"),
			wantOps:    &attachments.UploadResult{AppendOperations: 1},
			wantStdout: "✓ Updated artifact 2 (OAuth callback plan) on monalisa/monas-cafe#142\n",
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
				GetFunc: func(r ghrepo.Interface, issueNumber, number int) (*client.ArtifactWithVersions, error) {
					calls = append(calls, fmt.Sprintf("Get %s %d", issueRef(r, issueNumber), number))
					if tt.getErr != nil {
						return nil, tt.getErr
					}
					return &client.ArtifactWithVersions{Artifact: tt.artifact}, nil
				},
				UpdateFunc: func(r ghrepo.Interface, issueNumber, number int, name, body *string) (*client.Artifact, error) {
					calls = append(calls, fmt.Sprintf("Update %s %d", issueRef(r, issueNumber), number))
					if tt.updateErr != nil {
						return nil, tt.updateErr
					}
					updated := tt.artifact
					if name != nil {
						updated.Name = *name
					}
					if tt.updatedName != "" {
						updated.Name = tt.updatedName
					}
					if body != nil {
						updated.Body = *body
					}
					return &updated, nil
				},
			}

			recorder := &telemetry.InvocationRecorderSpy{}
			if len(tt.attach) > 0 {
				opts.AttachEvent = attachments.BeginTelemetry(recorder, "gh test", len(tt.attach))
			}
			if opts.IssueNumber == 0 {
				opts.IssueNumber = 142
			}
			if opts.ArtifactNumber == 0 {
				opts.ArtifactNumber = tt.artifact.Number
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

			err := editRun(&opts)

			assert.Equal(t, tt.wantCalls, calls)
			if updates := mock.UpdateCalls(); len(updates) > 0 {
				assert.Equal(t, tt.wantName, updates[0].Name, "the name sent")
				assert.Equal(t, tt.wantBody, updates[0].Body, "the body sent")
			}
			if opts.BodyFile != "-" {
				assert.Equal(t, tt.stdin, stdin.String(), "standard input is only read with --body-file -")
			}
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
