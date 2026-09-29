package download

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/artifact/client"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdDownload(t *testing.T) {
	tests := []struct {
		name        string
		args        string
		ghRepo      string
		wantOpts    DownloadOptions
		wantRepo    string
		wantErr     string
		wantFlagErr bool
	}{
		{
			name:     "an issue number downloads every artifact into the current directory",
			args:     "142",
			wantOpts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{}, Dir: "."},
			wantRepo: "OWNER/REPO",
		},
		{
			name:     "artifact numbers keep their order",
			args:     "142 5 2",
			wantOpts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{5, 2}, Dir: "."},
			wantRepo: "OWNER/REPO",
		},
		{
			name:     "a repeated artifact number is kept",
			args:     "142 2 2",
			wantOpts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2, 2}, Dir: "."},
			wantRepo: "OWNER/REPO",
		},
		{
			name:     "an issue URL names the repository",
			args:     "https://github.com/monalisa/monas-cafe/issues/142 2",
			wantOpts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Dir: "."},
			wantRepo: "monalisa/monas-cafe",
		},
		{
			name:     "-R selects the repository",
			args:     "142 -R monalisa/monas-cafe",
			wantOpts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{}, Dir: "."},
			wantRepo: "monalisa/monas-cafe",
		},
		{
			name:     "GH_REPO selects the repository",
			args:     "142",
			ghRepo:   "monalisa/monas-cafe",
			wantOpts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{}, Dir: "."},
			wantRepo: "monalisa/monas-cafe",
		},
		{
			name:     "--dir",
			args:     "142 2 5 --dir notes",
			wantOpts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2, 5}, Dir: "notes"},
			wantRepo: "OWNER/REPO",
		},
		{
			name:     "-D",
			args:     "142 -D notes",
			wantOpts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{}, Dir: "notes"},
			wantRepo: "OWNER/REPO",
		},
		{
			name:     "--output",
			args:     "142 2 --output callback-plan.md",
			wantOpts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Dir: ".", Output: "callback-plan.md"},
			wantRepo: "OWNER/REPO",
		},
		{
			name:     "-O - for standard output",
			args:     "142 2 -O -",
			wantOpts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Dir: ".", Output: "-"},
			wantRepo: "OWNER/REPO",
		},
		{
			name:     "--clobber",
			args:     "142 --clobber",
			wantOpts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{}, Dir: ".", Clobber: true},
			wantRepo: "OWNER/REPO",
		},
		{
			name:     "--skip-existing",
			args:     "142 --skip-existing",
			wantOpts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{}, Dir: ".", SkipExisting: true},
			wantRepo: "OWNER/REPO",
		},
		{
			name:     "--version",
			args:     "142 2 --version 5",
			wantOpts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Dir: ".", Version: 5},
			wantRepo: "OWNER/REPO",
		},
		{
			name:     "--version with --output and --clobber",
			args:     "142 2 --version 5 --output callback-plan.md --clobber",
			wantOpts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Dir: ".", Output: "callback-plan.md", Clobber: true, Version: 5},
			wantRepo: "OWNER/REPO",
		},
		{
			name:    "no issue",
			args:    "",
			wantErr: "requires at least 1 arg(s), only received 0",
		},
		{
			name:    "an issue argument that isn't an issue",
			args:    "OAuth",
			wantErr: `invalid issue format: "OAuth"`,
		},
		{
			name:    "an artifact number that isn't a number",
			args:    "142 2 OAuth",
			wantErr: `invalid artifact number: "OAuth"`,
		},
		{
			name:        "--clobber with --skip-existing",
			args:        "142 --clobber --skip-existing",
			wantErr:     "specify only one of `--clobber` or `--skip-existing`",
			wantFlagErr: true,
		},
		{
			name:        "--dir with --output",
			args:        "142 2 --dir notes --output callback-plan.md",
			wantErr:     "specify only one of `--dir` or `--output`",
			wantFlagErr: true,
		},
		{
			name:        "--dir with --output, even when --dir names the current directory",
			args:        "142 2 --dir . --output callback-plan.md",
			wantErr:     "specify only one of `--dir` or `--output`",
			wantFlagErr: true,
		},
		{
			name:        "a blank --output",
			args:        "142 2 --output ''",
			wantErr:     "--output cannot be blank",
			wantFlagErr: true,
		},
		{
			name:        "a blank --output with several artifact numbers",
			args:        "142 2 5 --output ''",
			wantErr:     "--output cannot be blank",
			wantFlagErr: true,
		},
		{
			name:        "--dir with a blank --output",
			args:        "142 2 --dir notes --output ''",
			wantErr:     "specify only one of `--dir` or `--output`",
			wantFlagErr: true,
		},
		{
			name:        "--output without an artifact number",
			args:        "142 --output callback-plan.md",
			wantErr:     "--output requires exactly one artifact number",
			wantFlagErr: true,
		},
		{
			name:        "--output with several artifact numbers",
			args:        "142 2 5 --output callback-plan.md",
			wantErr:     "--output requires exactly one artifact number",
			wantFlagErr: true,
		},
		{
			name:        "--output - with several artifact numbers",
			args:        "142 2 5 --output -",
			wantErr:     "--output requires exactly one artifact number",
			wantFlagErr: true,
		},
		{
			name:        "--version without an artifact number",
			args:        "142 --version 5",
			wantErr:     "--version requires exactly one artifact number",
			wantFlagErr: true,
		},
		{
			name:        "--version with several artifact numbers",
			args:        "142 2 5 --version 5",
			wantErr:     "--version requires exactly one artifact number",
			wantFlagErr: true,
		},
		{
			name:        "--version 0",
			args:        "142 2 --version 0",
			wantErr:     "invalid version: 0",
			wantFlagErr: true,
		},
		{
			name:        "a negative --version",
			args:        "142 2 --version -1",
			wantErr:     "invalid version: -1",
			wantFlagErr: true,
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

			var gotOpts *DownloadOptions
			cmd := NewCmdDownload(f, func(opts *DownloadOptions) error {
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
				assert.Nil(t, gotOpts)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOpts.IssueNumber, gotOpts.IssueNumber)
			assert.Equal(t, tt.wantOpts.ArtifactNumbers, gotOpts.ArtifactNumbers)
			assert.Equal(t, tt.wantOpts.Dir, gotOpts.Dir)
			assert.Equal(t, tt.wantOpts.Output, gotOpts.Output)
			assert.Equal(t, tt.wantOpts.Clobber, gotOpts.Clobber)
			assert.Equal(t, tt.wantOpts.SkipExisting, gotOpts.SkipExisting)
			assert.Equal(t, tt.wantOpts.Version, gotOpts.Version)

			repo, err := gotOpts.BaseRepo()
			require.NoError(t, err)
			assert.Equal(t, tt.wantRepo, ghrepo.FullName(repo))
		})
	}
}

func TestDownloadRun(t *testing.T) {
	const (
		oauthPlanBody   = "# OAuth callback plan\n\n1. Register the callback URL.\n2. Wire up `order-history` sync."
		oauthPlanV5Body = "# OAuth plan\n\n1. Register the callback URL."
		runbookURL      = "https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook"
		runbookShortcut = "[InternetShortcut]\r\nURL=https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook\r\n"
		baristaBody     = "# Barista feedback notes\n\n- The sign-in button is hard to find on the menu page."
		hoursBody       = "# Opening hours\n\nMonday to Friday, 7 to 3."
		manualURL       = " https://github.com/monalisa/monas-cafe/wiki/Espresso-machine-manual\n"
		alreadyExists   = "already exists (use --clobber to overwrite or --skip-existing to skip)"
	)
	generic := func(number int, name, body string) *client.ArtifactWithVersions {
		return &client.ArtifactWithVersions{Number: number, Type: "generic", Name: name, Body: body}
	}
	link := func(number int, name, body string) *client.ArtifactWithVersions {
		return &client.ArtifactWithVersions{Number: number, Type: "link", Name: name, Body: body}
	}

	// Artifact 2 on the spec's issue 142 has 12 versions. Version 9 renamed
	// it, so version 5 has the old name.
	oauthPlan := generic(2, "OAuth callback plan", oauthPlanBody)
	oauthPlan.Versions = []client.Version{
		{Version: 12, Name: "OAuth callback plan", Body: oauthPlanBody},
		{Version: 9, Name: "OAuth callback plan", Body: "# OAuth callback plan\n\n1. Register the callback URL."},
		{Version: 5, Name: "OAuth plan", Body: oauthPlanV5Body},
		{Version: 1, Name: "OAuth plan", Body: "# OAuth plan"},
	}

	// The spec's issues 142, 145 and 161, plus two with what the fixtures
	// can't hold: names older artifacts may have (150), and a type gh doesn't
	// know and links whose bodies need care (151).
	issues := map[int][]*client.ArtifactWithVersions{
		142: {
			oauthPlan,
			link(3, "Staging OAuth runbook", runbookURL),
			generic(5, "Barista feedback notes", baristaBody),
		},
		145: {
			generic(1, "My implementation plan", "# My implementation plan\n\n1. Add the decaf toggle."),
			generic(2, "README.md", "# README\n\nNotes for the next barista."),
			generic(3, "CON", "A name Windows reserves."),
			generic(4, "日本語メモ", "# メモ\n\nラテアートの練習。"),
		},
		150: {
			generic(1, `Menu/Drinks\Hot:Iced*Decaf?Tea"Oat<Soy>Rice|Almond`, "# Menu"),
			generic(2, "Opening\thours\r\nfor\u00a0May\x00", "# Opening hours"),
			generic(3, "Mona's Café.Notes v1.2", "# Notes"),
			generic(4, "Latte art.MD", "# Latte art"),
			link(5, "Menu.URL", "https://github.com/monalisa/monas-cafe/wiki/Menu"),
			link(6, "README.md", "https://github.com/monalisa/monas-cafe/blob/main/README.md"),
			generic(7, "Opening hours.url", "# Opening hours"),
			generic(8, strings.Repeat("a", 255), "# a"),
			generic(9, strings.Repeat("メ", 100), "# メ"),
			generic(10, strings.Repeat("b", 252)+".md", "# b"),
			generic(11, strings.Repeat("c", 249), "# c"),
		},
		151: {
			generic(1, "Opening hours", hoursBody),
			{Number: 7, Type: "bad-type", Name: "Espresso checklist", Body: "- Descale"},
			link(8, "Supplier portal", "file:///srv/suppliers"),
			link(9, "Espresso machine manual", manualURL),
		},
		161: {},
	}
	apiError := func(status int, message string, issueNumber, number int) error {
		u, err := url.Parse(fmt.Sprintf("https://api.github.com/repos/monalisa/monas-cafe/issues/%d/artifacts/%d", issueNumber, number))
		require.NoError(t, err)
		return api.HTTPError{HTTPError: &ghAPI.HTTPError{StatusCode: status, Message: message, RequestURL: u}}
	}
	// Paths the terminal shows use the OS's separator.
	p := filepath.FromSlash

	tests := []struct {
		name          string
		repo          ghrepo.Interface
		opts          DownloadOptions
		tty           bool
		color         bool
		setup         func(t *testing.T)
		skipOnWindows string
		// changeAfterCheck changes the working directory after download
		// checks a file's path and before it writes the file, as another
		// process could.
		changeAfterCheck func(t *testing.T)
		isPullRequest    bool
		lookupErr        error
		listErr          error
		wantCalls        []string
		wantStdout       string
		wantStderr       string
		wantErr          string
		wantErrIs        error
		// wantFiles is everything in the working directory afterwards: a
		// file's content, "" for a directory, whose path ends in "/", or
		// "symlink to <target>".
		wantFiles map[string]string
	}{
		{
			name:      "every artifact, in number order",
			opts:      DownloadOptions{IssueNumber: 142},
			tty:       true,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "List monalisa/monas-cafe#142"},
			wantStdout: "✓ 2-OAuth-callback-plan.md\n" +
				"✓ 3-Staging-OAuth-runbook.url\n" +
				"✓ 5-Barista-feedback-notes.md\n",
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md":    oauthPlanBody,
				"3-Staging-OAuth-runbook.url": runbookShortcut,
				"5-Barista-feedback-notes.md": baristaBody,
			},
		},
		{
			name: "the given artifacts, in argument order",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{5, 2}},
			tty:  true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 5",
				"Get monalisa/monas-cafe#142 artifact 2",
			},
			wantStdout: "✓ 5-Barista-feedback-notes.md\n" +
				"✓ 2-OAuth-callback-plan.md\n",
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md":    oauthPlanBody,
				"5-Barista-feedback-notes.md": baristaBody,
			},
		},
		{
			name:  "colors in a terminal",
			opts:  DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2, 9, 3}, SkipExisting: true},
			tty:   true,
			color: true,
			setup: func(t *testing.T) {
				writeTestFile(t, "2-OAuth-callback-plan.md", "my notes\n")
			},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 2",
				"Get monalisa/monas-cafe#142 artifact 9",
				"Get monalisa/monas-cafe#142 artifact 3",
			},
			wantStdout: "\x1b[38;5;242m-\x1b[0m Skipped 2-OAuth-callback-plan.md: already exists\n" +
				"\x1b[0;32m✓\x1b[0m 3-Staging-OAuth-runbook.url\n",
			wantStderr: "\x1b[0;31mX\x1b[0m Failed to download artifact 9: Not Found\n",
			wantErrIs:  cmdutil.SilentError,
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md":    "my notes\n",
				"3-Staging-OAuth-runbook.url": runbookShortcut,
			},
		},
		{
			name:      "names from the spec: spaces become -, .md isn't added twice, and the number keeps CON safe",
			opts:      DownloadOptions{IssueNumber: 145},
			tty:       true,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#145", "List monalisa/monas-cafe#145"},
			wantStdout: "✓ 1-My-implementation-plan.md\n" +
				"✓ 2-README.md\n" +
				"✓ 3-CON.md\n" +
				"✓ 4-日本語メモ.md\n",
			wantFiles: map[string]string{
				"1-My-implementation-plan.md": "# My implementation plan\n\n1. Add the decaf toggle.",
				"2-README.md":                 "# README\n\nNotes for the next barista.",
				"3-CON.md":                    "A name Windows reserves.",
				"4-日本語メモ.md":                  "# メモ\n\nラテアートの練習。",
			},
		},
		{
			name: "separators, the characters Windows forbids, whitespace and control characters each become -",
			opts: DownloadOptions{IssueNumber: 150, ArtifactNumbers: []int{1, 2, 3}},
			tty:  true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#150",
				"Get monalisa/monas-cafe#150 artifact 1",
				"Get monalisa/monas-cafe#150 artifact 2",
				"Get monalisa/monas-cafe#150 artifact 3",
			},
			wantStdout: "✓ 1-Menu-Drinks-Hot-Iced-Decaf-Tea-Oat-Soy-Rice-Almond.md\n" +
				"✓ 2-Opening-hours--for-May-.md\n" +
				"✓ 3-Mona's-Café.Notes-v1.2.md\n",
			wantFiles: map[string]string{
				"1-Menu-Drinks-Hot-Iced-Decaf-Tea-Oat-Soy-Rice-Almond.md": "# Menu",
				"2-Opening-hours--for-May-.md":                            "# Opening hours",
				"3-Mona's-Café.Notes-v1.2.md":                             "# Notes",
			},
		},
		{
			name: "a name that already ends with the extension, in any case, keeps it",
			opts: DownloadOptions{IssueNumber: 150, ArtifactNumbers: []int{4, 5, 6, 7}},
			tty:  true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#150",
				"Get monalisa/monas-cafe#150 artifact 4",
				"Get monalisa/monas-cafe#150 artifact 5",
				"Get monalisa/monas-cafe#150 artifact 6",
				"Get monalisa/monas-cafe#150 artifact 7",
			},
			wantStdout: "✓ 4-Latte-art.MD\n" +
				"✓ 5-Menu.URL\n" +
				"✓ 6-README.md.url\n" +
				"✓ 7-Opening-hours.url.md\n",
			wantFiles: map[string]string{
				"4-Latte-art.MD":         "# Latte art",
				"5-Menu.URL":             "[InternetShortcut]\r\nURL=https://github.com/monalisa/monas-cafe/wiki/Menu\r\n",
				"6-README.md.url":        "[InternetShortcut]\r\nURL=https://github.com/monalisa/monas-cafe/blob/main/README.md\r\n",
				"7-Opening-hours.url.md": "# Opening hours",
			},
		},
		{
			name: "a name is shortened at a character boundary only when the file name would pass 255 bytes",
			opts: DownloadOptions{IssueNumber: 150, ArtifactNumbers: []int{8, 9, 10, 11}},
			tty:  true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#150",
				"Get monalisa/monas-cafe#150 artifact 8",
				"Get monalisa/monas-cafe#150 artifact 9",
				"Get monalisa/monas-cafe#150 artifact 10",
				"Get monalisa/monas-cafe#150 artifact 11",
			},
			// 255 bytes, 254 bytes where the next character wouldn't fit, and
			// 255 bytes keeping the name's own .md. The last name fits as it is.
			wantStdout: "✓ 8-" + strings.Repeat("a", 250) + ".md\n" +
				"✓ 9-" + strings.Repeat("メ", 83) + ".md\n" +
				"✓ 10-" + strings.Repeat("b", 249) + ".md\n" +
				"✓ 11-" + strings.Repeat("c", 249) + ".md\n",
			wantFiles: map[string]string{
				"8-" + strings.Repeat("a", 250) + ".md":  "# a",
				"9-" + strings.Repeat("メ", 83) + ".md":   "# メ",
				"10-" + strings.Repeat("b", 249) + ".md": "# b",
				"11-" + strings.Repeat("c", 249) + ".md": "# c",
			},
		},
		{
			name:      "--dir creates the directory",
			opts:      DownloadOptions{IssueNumber: 142, Dir: "artifacts"},
			tty:       true,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "List monalisa/monas-cafe#142"},
			wantStdout: "✓ " + p("artifacts/2-OAuth-callback-plan.md") + "\n" +
				"✓ " + p("artifacts/3-Staging-OAuth-runbook.url") + "\n" +
				"✓ " + p("artifacts/5-Barista-feedback-notes.md") + "\n",
			wantFiles: map[string]string{
				"artifacts/":                            "",
				"artifacts/2-OAuth-callback-plan.md":    oauthPlanBody,
				"artifacts/3-Staging-OAuth-runbook.url": runbookShortcut,
				"artifacts/5-Barista-feedback-notes.md": baristaBody,
			},
		},
		{
			name:       "--dir creates missing parent directories",
			opts:       DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Dir: p("notes/oauth")},
			tty:        true,
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: "✓ " + p("notes/oauth/2-OAuth-callback-plan.md") + "\n",
			wantFiles: map[string]string{
				"notes/":                               "",
				"notes/oauth/":                         "",
				"notes/oauth/2-OAuth-callback-plan.md": oauthPlanBody,
			},
		},
		{
			name: "--dir can be a symlink",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Dir: "linked"},
			tty:  true,
			setup: func(t *testing.T) {
				require.NoError(t, os.Mkdir("real", 0755))
				symlinkTestFile(t, "real", "linked")
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: "✓ " + p("linked/2-OAuth-callback-plan.md") + "\n",
			wantFiles: map[string]string{
				"linked":                        "symlink to real",
				"real/":                         "",
				"real/2-OAuth-callback-plan.md": oauthPlanBody,
			},
		},
		{
			name: "an existing file fails, and the rest are written",
			opts: DownloadOptions{IssueNumber: 142},
			tty:  true,
			setup: func(t *testing.T) {
				writeTestFile(t, "2-OAuth-callback-plan.md", "my notes\n")
			},
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "List monalisa/monas-cafe#142"},
			wantStdout: "✓ 3-Staging-OAuth-runbook.url\n" +
				"✓ 5-Barista-feedback-notes.md\n",
			wantStderr: "X Failed to write 2-OAuth-callback-plan.md: " + alreadyExists + "\n",
			wantErrIs:  cmdutil.SilentError,
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md":    "my notes\n",
				"3-Staging-OAuth-runbook.url": runbookShortcut,
				"5-Barista-feedback-notes.md": baristaBody,
			},
		},
		{
			name: "--skip-existing skips an existing file",
			opts: DownloadOptions{IssueNumber: 142, SkipExisting: true},
			tty:  true,
			setup: func(t *testing.T) {
				writeTestFile(t, "2-OAuth-callback-plan.md", "my notes\n")
			},
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "List monalisa/monas-cafe#142"},
			wantStdout: "- Skipped 2-OAuth-callback-plan.md: already exists\n" +
				"✓ 3-Staging-OAuth-runbook.url\n" +
				"✓ 5-Barista-feedback-notes.md\n",
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md":    "my notes\n",
				"3-Staging-OAuth-runbook.url": runbookShortcut,
				"5-Barista-feedback-notes.md": baristaBody,
			},
		},
		{
			name: "--clobber overwrites an existing file",
			opts: DownloadOptions{IssueNumber: 142, Clobber: true},
			tty:  true,
			setup: func(t *testing.T) {
				writeTestFile(t, "2-OAuth-callback-plan.md", "my notes, which are longer than the artifact's body\n")
			},
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "List monalisa/monas-cafe#142"},
			wantStdout: "✓ 2-OAuth-callback-plan.md\n" +
				"✓ 3-Staging-OAuth-runbook.url\n" +
				"✓ 5-Barista-feedback-notes.md\n",
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md":    oauthPlanBody,
				"3-Staging-OAuth-runbook.url": runbookShortcut,
				"5-Barista-feedback-notes.md": baristaBody,
			},
		},
		{
			name: "a symlink fails even with --clobber, and nothing is written through it",
			opts: DownloadOptions{IssueNumber: 142, Clobber: true},
			tty:  true,
			setup: func(t *testing.T) {
				writeTestFile(t, "other.md", "keep\n")
				symlinkTestFile(t, "other.md", "2-OAuth-callback-plan.md")
			},
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "List monalisa/monas-cafe#142"},
			wantStdout: "✓ 3-Staging-OAuth-runbook.url\n" +
				"✓ 5-Barista-feedback-notes.md\n",
			wantStderr: "X Failed to write 2-OAuth-callback-plan.md: is a symlink\n",
			wantErrIs:  cmdutil.SilentError,
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md":    "symlink to other.md",
				"other.md":                    "keep\n",
				"3-Staging-OAuth-runbook.url": runbookShortcut,
				"5-Barista-feedback-notes.md": baristaBody,
			},
		},
		{
			name: "a symlink fails without --clobber too, even one that points nowhere",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}},
			tty:  true,
			setup: func(t *testing.T) {
				symlinkTestFile(t, "missing.md", "2-OAuth-callback-plan.md")
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStderr: "X Failed to write 2-OAuth-callback-plan.md: is a symlink\n",
			wantErrIs:  cmdutil.SilentError,
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md": "symlink to missing.md",
			},
		},
		{
			name: "--skip-existing skips a symlink",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, SkipExisting: true},
			tty:  true,
			setup: func(t *testing.T) {
				writeTestFile(t, "other.md", "keep\n")
				symlinkTestFile(t, "other.md", "2-OAuth-callback-plan.md")
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: "- Skipped 2-OAuth-callback-plan.md: already exists\n",
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md": "symlink to other.md",
				"other.md":                 "keep\n",
			},
		},
		{
			name: "--clobber doesn't replace a directory",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Clobber: true},
			tty:  true,
			setup: func(t *testing.T) {
				require.NoError(t, os.Mkdir("2-OAuth-callback-plan.md", 0755))
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStderr: "X Failed to write 2-OAuth-callback-plan.md: is not a regular file\n",
			wantErrIs:  cmdutil.SilentError,
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md/": "",
			},
		},
		{
			name: "--clobber replaces a symlink to the checked file put in its place, without writing through it",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Clobber: true},
			tty:  true,
			setup: func(t *testing.T) {
				writeTestFile(t, "2-OAuth-callback-plan.md", "my notes\n")
				symlinkTestFile(t, "original.md", "swap.md")
			},
			// The symlink points to the very file the check saw, moved aside.
			changeAfterCheck: func(t *testing.T) {
				require.NoError(t, os.Rename("2-OAuth-callback-plan.md", "original.md"))
				require.NoError(t, os.Rename("swap.md", "2-OAuth-callback-plan.md"))
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: "✓ 2-OAuth-callback-plan.md\n",
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md": oauthPlanBody,
				"original.md":              "my notes\n",
			},
		},
		{
			name: "--clobber refuses a symlink to a directory put in place of the file after the check",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Clobber: true},
			tty:  true,
			setup: func(t *testing.T) {
				writeTestFile(t, "2-OAuth-callback-plan.md", "my notes\n")
				require.NoError(t, os.Mkdir("notes", 0755))
				symlinkTestFile(t, "notes", "swap.md")
			},
			changeAfterCheck: func(t *testing.T) {
				require.NoError(t, os.Remove("2-OAuth-callback-plan.md"))
				require.NoError(t, os.Rename("swap.md", "2-OAuth-callback-plan.md"))
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStderr: "X Failed to write 2-OAuth-callback-plan.md: is a directory\n",
			wantErrIs:  cmdutil.SilentError,
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md": "symlink to notes",
				"notes/":                   "",
			},
		},
		{
			name: "--clobber refuses a directory put in place of the file after the check, naming the file once",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Dir: "notes", Clobber: true},
			tty:  true,
			setup: func(t *testing.T) {
				writeTestFile(t, p("notes/2-OAuth-callback-plan.md"), "my notes\n")
			},
			changeAfterCheck: func(t *testing.T) {
				require.NoError(t, os.Remove(p("notes/2-OAuth-callback-plan.md")))
				require.NoError(t, os.Mkdir(p("notes/2-OAuth-callback-plan.md"), 0755))
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStderr: "X Failed to write " + p("notes/2-OAuth-callback-plan.md") + ": is a directory\n",
			wantErrIs:  cmdutil.SilentError,
			wantFiles: map[string]string{
				"notes/":                          "",
				"notes/2-OAuth-callback-plan.md/": "",
			},
		},
		{
			name: "--output doesn't write through a symlink put at the path after the check",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Output: "callback-plan.md"},
			tty:  true,
			setup: func(t *testing.T) {
				writeTestFile(t, "other.md", "keep\n")
				symlinkTestFile(t, "other.md", "swap.md")
			},
			changeAfterCheck: func(t *testing.T) {
				require.NoError(t, os.Rename("swap.md", "callback-plan.md"))
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStderr: "X Failed to write callback-plan.md: " + alreadyExists + "\n",
			wantErrIs:  cmdutil.SilentError,
			wantFiles: map[string]string{
				"callback-plan.md": "symlink to other.md",
				"other.md":         "keep\n",
			},
		},
		{
			name: "a file that appears after the check isn't overwritten",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}},
			tty:  true,
			changeAfterCheck: func(t *testing.T) {
				writeTestFile(t, "2-OAuth-callback-plan.md", "my notes\n")
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStderr: "X Failed to write 2-OAuth-callback-plan.md: " + alreadyExists + "\n",
			wantErrIs:  cmdutil.SilentError,
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md": "my notes\n",
			},
		},
		{
			name: "--skip-existing skips a file that appears after the check",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, SkipExisting: true},
			tty:  true,
			changeAfterCheck: func(t *testing.T) {
				writeTestFile(t, "2-OAuth-callback-plan.md", "my notes\n")
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: "- Skipped 2-OAuth-callback-plan.md: already exists\n",
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md": "my notes\n",
			},
		},
		{
			name:          "a --dir that can't be opened fails the file and names the directory",
			opts:          DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Dir: "notes"},
			tty:           true,
			skipOnWindows: "Windows words a file in the way of a directory differently",
			changeAfterCheck: func(t *testing.T) {
				writeTestFile(t, "notes", "not a directory\n")
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStderr: "X Failed to write notes/2-OAuth-callback-plan.md: mkdir notes: not a directory\n",
			wantErrIs:  cmdutil.SilentError,
			wantFiles: map[string]string{
				"notes": "not a directory\n",
			},
		},
		{
			name:          "a --dir that is a dangling symlink fails every file, even with --skip-existing",
			opts:          DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2, 5}, Dir: "notes", SkipExisting: true},
			tty:           true,
			skipOnWindows: "Windows reports a dangling symlink in the path differently",
			setup: func(t *testing.T) {
				symlinkTestFile(t, "missing", "notes")
			},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 2",
				"Get monalisa/monas-cafe#142 artifact 5",
			},
			// Creating the directory fails as if it existed. That isn't the
			// file already existing, so neither file is skipped.
			wantStderr: "X Failed to write notes/2-OAuth-callback-plan.md: mkdir notes: file exists\n" +
				"X Failed to write notes/5-Barista-feedback-notes.md: mkdir notes: file exists\n",
			wantErrIs: cmdutil.SilentError,
			wantFiles: map[string]string{
				"notes": "symlink to missing",
			},
		},
		{
			name:          "a dangling symlink in --output's path fails the file, even with --skip-existing",
			opts:          DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Output: "notes/plan.md", SkipExisting: true},
			skipOnWindows: "Windows reports a dangling symlink in the path differently",
			setup: func(t *testing.T) {
				symlinkTestFile(t, "missing", "notes")
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: "failed\t2\tnotes/plan.md\t\tmkdir notes: file exists\n",
			wantErrIs:  cmdutil.SilentError,
			wantFiles: map[string]string{
				"notes": "symlink to missing",
			},
		},
		{
			name:          "an error from the file system names the file once",
			opts:          DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Dir: "notes"},
			tty:           true,
			skipOnWindows: "Windows reports a file in the way of a directory differently",
			setup: func(t *testing.T) {
				writeTestFile(t, "notes", "not a directory\n")
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStderr: "X Failed to write notes/2-OAuth-callback-plan.md: not a directory\n",
			wantErrIs:  cmdutil.SilentError,
			wantFiles: map[string]string{
				"notes": "not a directory\n",
			},
		},
		{
			name:       "--output writes to the path",
			opts:       DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Output: "callback-plan.md"},
			tty:        true,
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: "✓ callback-plan.md\n",
			wantFiles: map[string]string{
				"callback-plan.md": oauthPlanBody,
			},
		},
		{
			name:       "--output writes a link as a shortcut, whatever the path's extension",
			opts:       DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{3}, Output: "runbook.txt"},
			tty:        true,
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantStdout: "✓ runbook.txt\n",
			wantFiles: map[string]string{
				"runbook.txt": runbookShortcut,
			},
		},
		{
			name: "--output to an existing directory uses the usual file name",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Output: "notes"},
			tty:  true,
			setup: func(t *testing.T) {
				require.NoError(t, os.Mkdir("notes", 0755))
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: "✓ " + p("notes/2-OAuth-callback-plan.md") + "\n",
			wantFiles: map[string]string{
				"notes/":                         "",
				"notes/2-OAuth-callback-plan.md": oauthPlanBody,
			},
		},
		{
			name:       "--output ending in a separator creates the directory and uses the usual file name",
			opts:       DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{3}, Output: "notes/"},
			tty:        true,
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 3"},
			wantStdout: "✓ " + p("notes/3-Staging-OAuth-runbook.url") + "\n",
			wantFiles: map[string]string{
				"notes/":                            "",
				"notes/3-Staging-OAuth-runbook.url": runbookShortcut,
			},
		},
		{
			name:       "--output creates missing parent directories",
			opts:       DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Output: p("notes/oauth/callback-plan.md")},
			tty:        true,
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: "✓ " + p("notes/oauth/callback-plan.md") + "\n",
			wantFiles: map[string]string{
				"notes/":                       "",
				"notes/oauth/":                 "",
				"notes/oauth/callback-plan.md": oauthPlanBody,
			},
		},
		{
			name: "--output can name a symlink to a directory",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Output: "linked"},
			tty:  true,
			setup: func(t *testing.T) {
				require.NoError(t, os.Mkdir("real", 0755))
				symlinkTestFile(t, "real", "linked")
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: "✓ " + p("linked/2-OAuth-callback-plan.md") + "\n",
			wantFiles: map[string]string{
				"linked":                        "symlink to real",
				"real/":                         "",
				"real/2-OAuth-callback-plan.md": oauthPlanBody,
			},
		},
		{
			name: "--output to an existing file fails",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Output: "callback-plan.md"},
			tty:  true,
			setup: func(t *testing.T) {
				writeTestFile(t, "callback-plan.md", "my notes\n")
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStderr: "X Failed to write callback-plan.md: " + alreadyExists + "\n",
			wantErrIs:  cmdutil.SilentError,
			wantFiles: map[string]string{
				"callback-plan.md": "my notes\n",
			},
		},
		{
			name: "--output with --skip-existing skips an existing file",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Output: "callback-plan.md", SkipExisting: true},
			tty:  true,
			setup: func(t *testing.T) {
				writeTestFile(t, "callback-plan.md", "my notes\n")
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: "- Skipped callback-plan.md: already exists\n",
			wantFiles: map[string]string{
				"callback-plan.md": "my notes\n",
			},
		},
		{
			name: "--output with --clobber overwrites an existing file",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Output: "callback-plan.md", Clobber: true},
			tty:  true,
			setup: func(t *testing.T) {
				writeTestFile(t, "callback-plan.md", "my notes\n")
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: "✓ callback-plan.md\n",
			wantFiles: map[string]string{
				"callback-plan.md": oauthPlanBody,
			},
		},
		{
			name: "--output to a symlink fails even with --clobber",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Output: "callback-plan.md", Clobber: true},
			tty:  true,
			setup: func(t *testing.T) {
				writeTestFile(t, "other.md", "keep\n")
				symlinkTestFile(t, "other.md", "callback-plan.md")
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStderr: "X Failed to write callback-plan.md: is a symlink\n",
			wantErrIs:  cmdutil.SilentError,
			wantFiles: map[string]string{
				"callback-plan.md": "symlink to other.md",
				"other.md":         "keep\n",
			},
		},
		{
			name:       "--output - prints only the body, as it's stored",
			opts:       DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Output: "-"},
			tty:        true,
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: oauthPlanBody,
			wantFiles:  map[string]string{},
		},
		{
			name:       "--output - prints a link's URL as it's stored",
			opts:       DownloadOptions{IssueNumber: 151, ArtifactNumbers: []int{9}, Output: "-"},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#151", "Get monalisa/monas-cafe#151 artifact 9"},
			wantStdout: manualURL,
			wantFiles:  map[string]string{},
		},
		{
			name:       "--output - with --version prints that version's body",
			opts:       DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Output: "-", Version: 5},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: oauthPlanV5Body,
			wantFiles:  map[string]string{},
		},
		{
			name:      "--output - returns an API error as the API gives it",
			opts:      DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{9}, Output: "-"},
			tty:       true,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 9"},
			wantErr:   "HTTP 404: Not Found (https://api.github.com/repos/monalisa/monas-cafe/issues/142/artifacts/9)",
			wantFiles: map[string]string{},
		},
		{
			name:      "--output - refuses a type gh doesn't know",
			opts:      DownloadOptions{IssueNumber: 151, ArtifactNumbers: []int{7}, Output: "-"},
			tty:       true,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#151", "Get monalisa/monas-cafe#151 artifact 7"},
			wantErr:   `artifact 7 has type "bad-type", which this version of gh doesn't support; upgrade gh`,
			wantFiles: map[string]string{},
		},
		{
			name:      "--output - with a version the API didn't return",
			opts:      DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Output: "-", Version: 99},
			tty:       true,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantErr:   "version 99 not found for artifact 2",
			wantFiles: map[string]string{},
		},
		{
			name:       "--version writes an earlier version under that version's name",
			opts:       DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Version: 5},
			tty:        true,
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: "✓ 2-OAuth-plan.md\n",
			wantFiles: map[string]string{
				"2-OAuth-plan.md": oauthPlanV5Body,
			},
		},
		{
			name:       "--version with the current version's number writes the current version",
			opts:       DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Version: 12},
			tty:        true,
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: "✓ 2-OAuth-callback-plan.md\n",
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md": oauthPlanBody,
			},
		},
		{
			name: "--version with the current name doesn't overwrite the current version's file",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Version: 9},
			tty:  true,
			setup: func(t *testing.T) {
				writeTestFile(t, "2-OAuth-callback-plan.md", oauthPlanBody)
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStderr: "X Failed to write 2-OAuth-callback-plan.md: " + alreadyExists + "\n",
			wantErrIs:  cmdutil.SilentError,
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md": oauthPlanBody,
			},
		},
		{
			name:       "--version the API didn't return fails the artifact",
			opts:       DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Version: 99},
			tty:        true,
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStderr: "X Failed to download artifact 2: version 99 not found for artifact 2\n",
			wantErrIs:  cmdutil.SilentError,
			wantFiles:  map[string]string{},
		},
		{
			name:       "--version with --output",
			opts:       DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Output: "plan-v5.md", Version: 5},
			tty:        true,
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#142", "Get monalisa/monas-cafe#142 artifact 2"},
			wantStdout: "✓ plan-v5.md\n",
			wantFiles: map[string]string{
				"plan-v5.md": oauthPlanV5Body,
			},
		},
		{
			name: "a type gh doesn't know and a link that isn't an http(s) URL fail, and the rest are written",
			opts: DownloadOptions{IssueNumber: 151},
			tty:  true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#151",
				"List monalisa/monas-cafe#151",
			},
			wantStdout: "✓ 1-Opening-hours.md\n" +
				"✓ 9-Espresso-machine-manual.url\n",
			wantStderr: `X Failed to download artifact 7: artifact 7 has type "bad-type", which this version of gh doesn't support; upgrade gh` + "\n" +
				"X Failed to write 8-Supplier-portal.url: artifact 8 doesn't link to an http(s) URL\n",
			wantErrIs: cmdutil.SilentError,
			wantFiles: map[string]string{
				"1-Opening-hours.md":            hoursBody,
				"9-Espresso-machine-manual.url": "[InternetShortcut]\r\nURL=https://github.com/monalisa/monas-cafe/wiki/Espresso-machine-manual\r\n",
			},
		},
		{
			name: "--skip-existing skips an existing file before looking at the link",
			opts: DownloadOptions{IssueNumber: 151, ArtifactNumbers: []int{8}, SkipExisting: true},
			tty:  true,
			setup: func(t *testing.T) {
				writeTestFile(t, "8-Supplier-portal.url", "my shortcut\n")
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#151", "Get monalisa/monas-cafe#151 artifact 8"},
			wantStdout: "- Skipped 8-Supplier-portal.url: already exists\n",
			wantFiles: map[string]string{
				"8-Supplier-portal.url": "my shortcut\n",
			},
		},
		{
			name: "--clobber leaves an existing file alone when the link isn't an http(s) URL",
			opts: DownloadOptions{IssueNumber: 151, ArtifactNumbers: []int{8}, Clobber: true},
			tty:  true,
			setup: func(t *testing.T) {
				writeTestFile(t, "8-Supplier-portal.url", "my shortcut\n")
			},
			wantCalls:  []string{"IsPullRequest monalisa/monas-cafe#151", "Get monalisa/monas-cafe#151 artifact 8"},
			wantStderr: "X Failed to write 8-Supplier-portal.url: artifact 8 doesn't link to an http(s) URL\n",
			wantErrIs:  cmdutil.SilentError,
			wantFiles: map[string]string{
				"8-Supplier-portal.url": "my shortcut\n",
			},
		},
		{
			name: "a missing artifact fails, and the rest are written",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2, 9, 5}},
			tty:  true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 2",
				"Get monalisa/monas-cafe#142 artifact 9",
				"Get monalisa/monas-cafe#142 artifact 5",
			},
			wantStdout: "✓ 2-OAuth-callback-plan.md\n" +
				"✓ 5-Barista-feedback-notes.md\n",
			wantStderr: "X Failed to download artifact 9: Not Found\n",
			wantErrIs:  cmdutil.SilentError,
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md":    oauthPlanBody,
				"5-Barista-feedback-notes.md": baristaBody,
			},
		},
		{
			name: "a repeated artifact gets a line each, and the second finds the first's file",
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2, 2}},
			tty:  true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 2",
				"Get monalisa/monas-cafe#142 artifact 2",
			},
			wantStdout: "✓ 2-OAuth-callback-plan.md\n",
			wantStderr: "X Failed to write 2-OAuth-callback-plan.md: " + alreadyExists + "\n",
			wantErrIs:  cmdutil.SilentError,
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md": oauthPlanBody,
			},
		},
		{
			name:      "piped output",
			opts:      DownloadOptions{IssueNumber: 142},
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "List monalisa/monas-cafe#142"},
			wantStdout: "written\t2\t2-OAuth-callback-plan.md\t\t\n" +
				"written\t3\t3-Staging-OAuth-runbook.url\t\t\n" +
				"written\t5\t5-Barista-feedback-notes.md\t\t\n",
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md":    oauthPlanBody,
				"3-Staging-OAuth-runbook.url": runbookShortcut,
				"5-Barista-feedback-notes.md": baristaBody,
			},
		},
		{
			name: "piped output with an existing file",
			opts: DownloadOptions{IssueNumber: 142},
			setup: func(t *testing.T) {
				writeTestFile(t, "2-OAuth-callback-plan.md", "my notes\n")
			},
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "List monalisa/monas-cafe#142"},
			wantStdout: "failed\t2\t2-OAuth-callback-plan.md\t\t" + alreadyExists + "\n" +
				"written\t3\t3-Staging-OAuth-runbook.url\t\t\n" +
				"written\t5\t5-Barista-feedback-notes.md\t\t\n",
			wantErrIs: cmdutil.SilentError,
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md":    "my notes\n",
				"3-Staging-OAuth-runbook.url": runbookShortcut,
				"5-Barista-feedback-notes.md": baristaBody,
			},
		},
		{
			name: "piped output with --skip-existing",
			opts: DownloadOptions{IssueNumber: 142, Dir: "artifacts", SkipExisting: true},
			setup: func(t *testing.T) {
				writeTestFile(t, p("artifacts/2-OAuth-callback-plan.md"), "my notes\n")
			},
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "List monalisa/monas-cafe#142"},
			wantStdout: "skipped\t2\t" + p("artifacts/2-OAuth-callback-plan.md") + "\t\talready exists\n" +
				"written\t3\t" + p("artifacts/3-Staging-OAuth-runbook.url") + "\t\t\n" +
				"written\t5\t" + p("artifacts/5-Barista-feedback-notes.md") + "\t\t\n",
			wantFiles: map[string]string{
				"artifacts/":                            "",
				"artifacts/2-OAuth-callback-plan.md":    "my notes\n",
				"artifacts/3-Staging-OAuth-runbook.url": runbookShortcut,
				"artifacts/5-Barista-feedback-notes.md": baristaBody,
			},
		},
		{
			name:      "piped output leaves the path empty when there is no file",
			opts:      DownloadOptions{IssueNumber: 151},
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#151", "List monalisa/monas-cafe#151"},
			wantStdout: "written\t1\t1-Opening-hours.md\t\t\n" +
				"failed\t7\t\t\tartifact 7 has type \"bad-type\", which this version of gh doesn't support; upgrade gh\n" +
				"failed\t8\t8-Supplier-portal.url\t\tartifact 8 doesn't link to an http(s) URL\n" +
				"written\t9\t9-Espresso-machine-manual.url\t\t\n",
			wantErrIs: cmdutil.SilentError,
			wantFiles: map[string]string{
				"1-Opening-hours.md":            hoursBody,
				"9-Espresso-machine-manual.url": "[InternetShortcut]\r\nURL=https://github.com/monalisa/monas-cafe/wiki/Espresso-machine-manual\r\n",
			},
		},
		{
			name:      "an issue with no artifacts fails, and nothing is written",
			opts:      DownloadOptions{IssueNumber: 161, Dir: "artifacts"},
			tty:       true,
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#161", "List monalisa/monas-cafe#161"},
			wantErr:   "no artifacts to download from monalisa/monas-cafe#161",
			wantFiles: map[string]string{},
		},
		{
			name: "--dir isn't created when no file is written",
			opts: DownloadOptions{IssueNumber: 151, ArtifactNumbers: []int{7, 8}, Dir: "artifacts"},
			tty:  true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#151",
				"Get monalisa/monas-cafe#151 artifact 7",
				"Get monalisa/monas-cafe#151 artifact 8",
			},
			wantStderr: `X Failed to download artifact 7: artifact 7 has type "bad-type", which this version of gh doesn't support; upgrade gh` + "\n" +
				"X Failed to write " + p("artifacts/8-Supplier-portal.url") + ": artifact 8 doesn't link to an http(s) URL\n",
			wantErrIs: cmdutil.SilentError,
			wantFiles: map[string]string{},
		},
		{
			name:      "a list error is returned as the API gives it",
			opts:      DownloadOptions{IssueNumber: 142},
			tty:       true,
			listErr:   errors.New("HTTP 503: Service Unavailable (https://api.github.com/repos/monalisa/monas-cafe/issues/142/artifacts?per_page=100)"),
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#142", "List monalisa/monas-cafe#142"},
			wantErr:   "HTTP 503: Service Unavailable (https://api.github.com/repos/monalisa/monas-cafe/issues/142/artifacts?per_page=100)",
			wantFiles: map[string]string{},
		},
		{
			name:          "a pull request is refused before any artifact request",
			opts:          DownloadOptions{IssueNumber: 158},
			tty:           true,
			isPullRequest: true,
			wantCalls:     []string{"IsPullRequest monalisa/monas-cafe#158"},
			wantErr:       "monalisa/monas-cafe#158 is a pull request; artifacts are only supported on issues",
			wantFiles:     map[string]string{},
		},
		{
			name:      "the lookup's error",
			opts:      DownloadOptions{IssueNumber: 999},
			tty:       true,
			lookupErr: errors.New("GraphQL: Could not resolve to an issue or pull request with the number of 999. (repository.issue)"),
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#999"},
			wantErr:   "GraphQL: Could not resolve to an issue or pull request with the number of 999. (repository.issue)",
			wantFiles: map[string]string{},
		},
		{
			name:      "GitHub Enterprise Server is refused before any request",
			repo:      ghrepo.NewWithHost("monalisa", "monas-cafe", "ghes.monas-cafe.example"),
			opts:      DownloadOptions{IssueNumber: 142},
			tty:       true,
			wantErr:   "issue artifacts are not supported on GitHub Enterprise Server",
			wantFiles: map[string]string{},
		},
		{
			name: "a ghe.com host is supported",
			repo: ghrepo.NewWithHost("monalisa", "monas-cafe", "monas-cafe.ghe.com"),
			opts: DownloadOptions{IssueNumber: 142, ArtifactNumbers: []int{2}},
			tty:  true,
			wantCalls: []string{
				"IsPullRequest monas-cafe.ghe.com/monalisa/monas-cafe#142",
				"Get monas-cafe.ghe.com/monalisa/monas-cafe#142 artifact 2",
			},
			wantStdout: "✓ 2-OAuth-callback-plan.md\n",
			wantFiles: map[string]string{
				"2-OAuth-callback-plan.md": oauthPlanBody,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnWindows != "" && runtime.GOOS == "windows" {
				t.Skip(tt.skipOnWindows)
			}
			t.Chdir(t.TempDir())
			if tt.setup != nil {
				tt.setup(t)
			}
			if tt.changeAfterCheck != nil {
				original := afterCheck
				afterCheck = func() { tt.changeAfterCheck(t) }
				t.Cleanup(func() { afterCheck = original })
			}

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)
			ios.SetColorEnabled(tt.color)

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
				ListFunc: func(r ghrepo.Interface, issueNumber int, _ string, _ int) ([]client.Artifact, error) {
					calls = append(calls, "List "+issueRef(r, issueNumber))
					if tt.listErr != nil {
						return nil, tt.listErr
					}
					// Like the API, a list has no edit history.
					var artifacts []client.Artifact
					for _, a := range issues[issueNumber] {
						artifacts = append(artifacts, a.Artifact)
					}
					return artifacts, nil
				},
				GetFunc: func(r ghrepo.Interface, issueNumber int, number int) (*client.ArtifactWithVersions, error) {
					calls = append(calls, fmt.Sprintf("Get %s artifact %d", issueRef(r, issueNumber), number))
					for _, a := range issues[issueNumber] {
						if a.Number == number {
							return a, nil
						}
					}
					return nil, apiError(404, "Not Found", issueNumber, number)
				},
			}

			opts := tt.opts
			opts.IO = ios
			opts.BaseRepo = func() (ghrepo.Interface, error) { return repo, nil }
			opts.Client = func() (client.ArtifactClient, error) { return mock, nil }
			if opts.Dir == "" {
				opts.Dir = "."
			}

			err := downloadRun(&opts)

			assert.Equal(t, tt.wantCalls, calls)
			for _, call := range mock.ListCalls() {
				assert.Equal(t, "", call.ArtifactType, "every type is downloaded")
				assert.Equal(t, 0, call.Limit, "every artifact is downloaded")
			}
			switch {
			case tt.wantErrIs != nil:
				require.ErrorIs(t, err, tt.wantErrIs)
			case tt.wantErr != "":
				require.EqualError(t, err, tt.wantErr)
			default:
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
			assert.Equal(t, tt.wantFiles, workingDirFiles(t))
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
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
}

// workingDirFiles returns everything in the working directory, with
// slash-separated paths: a file's content, "" for a directory, whose path ends
// in "/", and "symlink to <target>" for a symlink, which isn't followed.
func workingDirFiles(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == "." {
			return err
		}
		name := filepath.ToSlash(path)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			files[name] = "symlink to " + filepath.ToSlash(target)
		case d.IsDir():
			files[name+"/"] = ""
		default:
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files[name] = string(content)
		}
		return nil
	})
	require.NoError(t, err)
	return files
}
