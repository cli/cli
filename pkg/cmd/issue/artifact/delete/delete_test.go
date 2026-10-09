package delete

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/issue/artifact/client"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdDelete(t *testing.T) {
	tests := []struct {
		name        string
		args        string
		stdinTTY    bool
		stdoutTTY   bool
		ghRepo      string
		wantOpts    DeleteOptions
		wantRepo    string
		wantErr     string
		wantFlagErr bool
	}{
		{
			name:      "an issue number and an artifact number",
			args:      "142 2",
			stdinTTY:  true,
			stdoutTTY: true,
			wantOpts:  DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2}},
			wantRepo:  "OWNER/REPO",
		},
		{
			name:      "several artifact numbers keep their order",
			args:      "142 5 2",
			stdinTTY:  true,
			stdoutTTY: true,
			wantOpts:  DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{5, 2}},
			wantRepo:  "OWNER/REPO",
		},
		{
			name:      "a repeated artifact number is kept",
			args:      "142 2 2",
			stdinTTY:  true,
			stdoutTTY: true,
			wantOpts:  DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2, 2}},
			wantRepo:  "OWNER/REPO",
		},
		{
			name:      "an issue URL names the repository",
			args:      "https://github.com/monalisa/monas-cafe/issues/142 2",
			stdinTTY:  true,
			stdoutTTY: true,
			wantOpts:  DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2}},
			wantRepo:  "monalisa/monas-cafe",
		},
		{
			name:      "-R selects the repository",
			args:      "142 2 -R monalisa/monas-cafe",
			stdinTTY:  true,
			stdoutTTY: true,
			wantOpts:  DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2}},
			wantRepo:  "monalisa/monas-cafe",
		},
		{
			name:      "GH_REPO selects the repository",
			args:      "142 2",
			stdinTTY:  true,
			stdoutTTY: true,
			ghRepo:    "monalisa/monas-cafe",
			wantOpts:  DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2}},
			wantRepo:  "monalisa/monas-cafe",
		},
		{
			name:      "--yes",
			args:      "142 2 5 --yes",
			stdinTTY:  true,
			stdoutTTY: true,
			wantOpts:  DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2, 5}, Confirmed: true},
			wantRepo:  "OWNER/REPO",
		},
		{
			name:     "--yes without a terminal",
			args:     "142 2 --yes",
			wantOpts: DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Confirmed: true},
			wantRepo: "OWNER/REPO",
		},
		{
			name:        "piped output without --yes",
			args:        "142 2",
			stdinTTY:    true,
			wantErr:     "--yes required when not running interactively",
			wantFlagErr: true,
		},
		{
			name:        "no terminal and no --yes",
			args:        "142 2",
			wantErr:     "--yes required when not running interactively",
			wantFlagErr: true,
		},
		{
			name:      "no artifact number",
			args:      "142",
			stdinTTY:  true,
			stdoutTTY: true,
			wantErr:   "requires at least 2 arg(s), only received 1",
		},
		{
			name:      "an issue argument that isn't an issue",
			args:      "OAuth 2",
			stdinTTY:  true,
			stdoutTTY: true,
			wantErr:   `invalid issue format: "OAuth"`,
		},
		{
			name:      "an artifact number that isn't a number",
			args:      "142 2 OAuth",
			stdinTTY:  true,
			stdoutTTY: true,
			wantErr:   `invalid artifact number: "OAuth"`,
		},
		{
			name:    "an artifact number that isn't a number fails before --yes is required",
			args:    "142 OAuth",
			wantErr: `invalid artifact number: "OAuth"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GH_REPO", tt.ghRepo)
			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(tt.stdinTTY)
			ios.SetStdoutTTY(tt.stdoutTTY)
			ios.SetStderrTTY(tt.stdoutTTY)
			f := &cmdutil.Factory{
				IOStreams: ios,
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
			}

			var gotOpts *DeleteOptions
			cmd := NewCmdDelete(f, func(opts *DeleteOptions) error {
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
			assert.Equal(t, tt.wantOpts.Confirmed, gotOpts.Confirmed)

			repo, err := gotOpts.BaseRepo()
			require.NoError(t, err)
			assert.Equal(t, tt.wantRepo, ghrepo.FullName(repo))
		})
	}
}

func TestDeleteRun(t *testing.T) {
	// The spec's issue 142, plus an artifact whose type gh doesn't know and
	// one whose name needs cleaning up. Other numbers don't exist.
	artifacts := map[int]*client.ArtifactWithVersions{
		2: {Number: 2, Type: "generic", Name: "OAuth callback plan"},
		3: {Number: 3, Type: "link", Name: "Staging OAuth runbook"},
		5: {Number: 5, Type: "generic", Name: "Barista feedback notes"},
		7: {Number: 7, Type: "bad-type", Name: "Espresso checklist"},
		8: {Number: 8, Type: "generic", Name: "  Espresso\tmachine\n\n manual "},
	}
	apiError := func(status int, message string, number int) error {
		u, err := url.Parse(fmt.Sprintf("https://api.github.com/repos/monalisa/monas-cafe/issues/142/artifacts/%d", number))
		require.NoError(t, err)
		return api.HTTPError{HTTPError: &ghAPI.HTTPError{StatusCode: status, Message: message, RequestURL: u}}
	}

	tests := []struct {
		name          string
		repo          ghrepo.Interface
		opts          DeleteOptions
		tty           bool
		color         bool
		isPullRequest bool
		lookupErr     error
		deleteErrs    map[int]error
		confirm       bool
		promptErr     error
		wantPrompt    string
		wantCalls     []string
		wantStdout    string
		wantStderr    string
		wantErr       string
		wantErrIs     error
	}{
		{
			name:       "confirming the prompt deletes the artifact",
			opts:       DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2}},
			tty:        true,
			confirm:    true,
			wantPrompt: "Delete artifact 2 (OAuth callback plan) from monalisa/monas-cafe#142?",
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 2",
				"Delete monalisa/monas-cafe#142 artifact 2",
			},
			wantStdout: "! Deleted artifacts cannot be recovered.\n" +
				"✓ Deleted artifact 2 (OAuth callback plan) from monalisa/monas-cafe#142\n",
		},
		{
			name:       "one prompt lists every artifact",
			opts:       DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2, 5}},
			tty:        true,
			confirm:    true,
			wantPrompt: "Delete artifacts 2 (OAuth callback plan) and 5 (Barista feedback notes) from monalisa/monas-cafe#142?",
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 2",
				"Get monalisa/monas-cafe#142 artifact 5",
				"Delete monalisa/monas-cafe#142 artifact 2",
				"Delete monalisa/monas-cafe#142 artifact 5",
			},
			wantStdout: "! Deleted artifacts cannot be recovered.\n" +
				"✓ Deleted artifact 2 (OAuth callback plan) from monalisa/monas-cafe#142\n" +
				"✓ Deleted artifact 5 (Barista feedback notes) from monalisa/monas-cafe#142\n",
		},
		{
			name:       "a prompt for three artifacts separates them with commas",
			opts:       DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2, 3, 5}},
			tty:        true,
			confirm:    true,
			wantPrompt: "Delete artifacts 2 (OAuth callback plan), 3 (Staging OAuth runbook), and 5 (Barista feedback notes) from monalisa/monas-cafe#142?",
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 2",
				"Get monalisa/monas-cafe#142 artifact 3",
				"Get monalisa/monas-cafe#142 artifact 5",
				"Delete monalisa/monas-cafe#142 artifact 2",
				"Delete monalisa/monas-cafe#142 artifact 3",
				"Delete monalisa/monas-cafe#142 artifact 5",
			},
			wantStdout: "! Deleted artifacts cannot be recovered.\n" +
				"✓ Deleted artifact 2 (OAuth callback plan) from monalisa/monas-cafe#142\n" +
				"✓ Deleted artifact 3 (Staging OAuth runbook) from monalisa/monas-cafe#142\n" +
				"✓ Deleted artifact 5 (Barista feedback notes) from monalisa/monas-cafe#142\n",
		},
		{
			name:       "declining the prompt deletes nothing",
			opts:       DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2}},
			tty:        true,
			wantPrompt: "Delete artifact 2 (OAuth callback plan) from monalisa/monas-cafe#142?",
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 2",
			},
			wantStdout: "! Deleted artifacts cannot be recovered.\n",
			wantErrIs:  cmdutil.CancelError,
		},
		{
			name:       "a prompt that fails deletes nothing",
			opts:       DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2}},
			tty:        true,
			promptErr:  errors.New("could not prompt: EOF"),
			wantPrompt: "Delete artifact 2 (OAuth callback plan) from monalisa/monas-cafe#142?",
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 2",
			},
			wantStdout: "! Deleted artifacts cannot be recovered.\n",
			wantErr:    "could not prompt: EOF",
		},
		{
			name: "a missing number stops the command before the prompt",
			opts: DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2, 9}},
			tty:  true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 2",
				"Get monalisa/monas-cafe#142 artifact 9",
			},
			wantStderr: "X Failed to delete artifact 9: Not Found\n" +
				"No artifacts were deleted.\n",
			wantErrIs: cmdutil.SilentError,
		},
		{
			name: "every missing number is reported before the prompt",
			opts: DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{4, 2, 9}},
			tty:  true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 4",
				"Get monalisa/monas-cafe#142 artifact 2",
				"Get monalisa/monas-cafe#142 artifact 9",
			},
			wantStderr: "X Failed to delete artifact 4: Not Found\n" +
				"X Failed to delete artifact 9: Not Found\n" +
				"No artifacts were deleted.\n",
			wantErrIs: cmdutil.SilentError,
		},
		{
			name: "--yes deletes without a prompt",
			opts: DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2, 5}, Confirmed: true},
			tty:  true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 2",
				"Get monalisa/monas-cafe#142 artifact 5",
				"Delete monalisa/monas-cafe#142 artifact 2",
				"Delete monalisa/monas-cafe#142 artifact 5",
			},
			wantStdout: "✓ Deleted artifact 2 (OAuth callback plan) from monalisa/monas-cafe#142\n" +
				"✓ Deleted artifact 5 (Barista feedback notes) from monalisa/monas-cafe#142\n",
		},
		{
			name: "--yes attempts every number and reports each failure",
			opts: DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2, 9}, Confirmed: true},
			tty:  true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 2",
				"Get monalisa/monas-cafe#142 artifact 9",
				"Delete monalisa/monas-cafe#142 artifact 2",
			},
			wantStdout: "✓ Deleted artifact 2 (OAuth callback plan) from monalisa/monas-cafe#142\n",
			wantStderr: "X Failed to delete artifact 9: Not Found\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:       "a failed delete is reported, and the rest are still deleted",
			opts:       DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2, 3, 5}, Confirmed: true},
			tty:        true,
			deleteErrs: map[int]error{3: apiError(403, "Must have admin rights to Repository.", 3)},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 2",
				"Get monalisa/monas-cafe#142 artifact 3",
				"Get monalisa/monas-cafe#142 artifact 5",
				"Delete monalisa/monas-cafe#142 artifact 2",
				"Delete monalisa/monas-cafe#142 artifact 3",
				"Delete monalisa/monas-cafe#142 artifact 5",
			},
			wantStdout: "✓ Deleted artifact 2 (OAuth callback plan) from monalisa/monas-cafe#142\n" +
				"✓ Deleted artifact 5 (Barista feedback notes) from monalisa/monas-cafe#142\n",
			wantStderr: "X Failed to delete artifact 3: Must have admin rights to Repository.\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name: "a repeated number gets a line each, and the second delete fails",
			opts: DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2, 2}, Confirmed: true},
			tty:  true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 2",
				"Get monalisa/monas-cafe#142 artifact 2",
				"Delete monalisa/monas-cafe#142 artifact 2",
				"Delete monalisa/monas-cafe#142 artifact 2",
			},
			wantStdout: "✓ Deleted artifact 2 (OAuth callback plan) from monalisa/monas-cafe#142\n",
			wantStderr: "X Failed to delete artifact 2: Not Found\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name: "piped output",
			opts: DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2, 9}, Confirmed: true},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 2",
				"Get monalisa/monas-cafe#142 artifact 9",
				"Delete monalisa/monas-cafe#142 artifact 2",
			},
			wantStdout: "deleted\t2\tOAuth callback plan\t\t\n" +
				"failed\t9\t\t\tNot Found\n",
			wantErrIs: cmdutil.SilentError,
		},
		{
			name:       "piped output names an artifact whose delete failed",
			opts:       DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{3}, Confirmed: true},
			deleteErrs: map[int]error{3: apiError(403, "Must have admin rights to Repository.", 3)},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 3",
				"Delete monalisa/monas-cafe#142 artifact 3",
			},
			wantStdout: "failed\t3\tStaging OAuth runbook\t\tMust have admin rights to Repository.\n",
			wantErrIs:  cmdutil.SilentError,
		},
		{
			name:       "names are cleaned up in the prompt and the result",
			opts:       DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{8}},
			tty:        true,
			confirm:    true,
			wantPrompt: "Delete artifact 8 (Espresso machine manual) from monalisa/monas-cafe#142?",
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 8",
				"Delete monalisa/monas-cafe#142 artifact 8",
			},
			wantStdout: "! Deleted artifacts cannot be recovered.\n" +
				"✓ Deleted artifact 8 (Espresso machine manual) from monalisa/monas-cafe#142\n",
		},
		{
			name: "names are cleaned up in piped output",
			opts: DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{8}, Confirmed: true},
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 8",
				"Delete monalisa/monas-cafe#142 artifact 8",
			},
			wantStdout: "deleted\t8\tEspresso machine manual\t\t\n",
		},
		{
			name:       "colors in a terminal",
			opts:       DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2}},
			tty:        true,
			color:      true,
			confirm:    true,
			wantPrompt: "Delete artifact 2 (OAuth callback plan) from monalisa/monas-cafe#142?",
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 2",
				"Delete monalisa/monas-cafe#142 artifact 2",
			},
			wantStdout: "\x1b[0;33m!\x1b[0m Deleted artifacts cannot be recovered.\n" +
				"\x1b[0;31m✓\x1b[0m Deleted artifact 2 (OAuth callback plan) from monalisa/monas-cafe#142\n",
		},
		{
			name:  "colors for a missing number in a terminal",
			opts:  DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{9}},
			tty:   true,
			color: true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 9",
			},
			wantStderr: "\x1b[0;31mX\x1b[0m Failed to delete artifact 9: Not Found\n" +
				"No artifacts were deleted.\n",
			wantErrIs: cmdutil.SilentError,
		},
		{
			name: "a type gh doesn't know is deleted as usual",
			opts: DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{7}, Confirmed: true},
			tty:  true,
			wantCalls: []string{
				"IsPullRequest monalisa/monas-cafe#142",
				"Get monalisa/monas-cafe#142 artifact 7",
				"Delete monalisa/monas-cafe#142 artifact 7",
			},
			wantStdout: "✓ Deleted artifact 7 (Espresso checklist) from monalisa/monas-cafe#142\n",
		},
		{
			name:          "a pull request is refused before any artifact request",
			opts:          DeleteOptions{IssueNumber: 158, ArtifactNumbers: []int{2}, Confirmed: true},
			tty:           true,
			isPullRequest: true,
			wantCalls:     []string{"IsPullRequest monalisa/monas-cafe#158"},
			wantErr:       "monalisa/monas-cafe#158 is a pull request; artifacts are only supported on issues",
		},
		{
			name:      "the lookup's error",
			opts:      DeleteOptions{IssueNumber: 999, ArtifactNumbers: []int{2}, Confirmed: true},
			tty:       true,
			lookupErr: errors.New("GraphQL: Could not resolve to an issue or pull request with the number of 999. (repository.issue)"),
			wantCalls: []string{"IsPullRequest monalisa/monas-cafe#999"},
			wantErr:   "GraphQL: Could not resolve to an issue or pull request with the number of 999. (repository.issue)",
		},
		{
			name:    "GitHub Enterprise Server is refused before any request",
			repo:    ghrepo.NewWithHost("monalisa", "monas-cafe", "ghes.monas-cafe.example"),
			opts:    DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Confirmed: true},
			tty:     true,
			wantErr: "issue artifacts are not supported on GitHub Enterprise Server",
		},
		{
			name: "a ghe.com host is supported",
			repo: ghrepo.NewWithHost("monalisa", "monas-cafe", "monas-cafe.ghe.com"),
			opts: DeleteOptions{IssueNumber: 142, ArtifactNumbers: []int{2}, Confirmed: true},
			tty:  true,
			wantCalls: []string{
				"IsPullRequest monas-cafe.ghe.com/monalisa/monas-cafe#142",
				"Get monas-cafe.ghe.com/monalisa/monas-cafe#142 artifact 2",
				"Delete monas-cafe.ghe.com/monalisa/monas-cafe#142 artifact 2",
			},
			wantStdout: "✓ Deleted artifact 2 (OAuth callback plan) from monalisa/monas-cafe#142\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			// Like the API, a deleted artifact is gone for later requests.
			deleted := map[int]bool{}
			mock := &client.ArtifactClientMock{
				IsPullRequestFunc: func(r ghrepo.Interface, number int) (bool, error) {
					calls = append(calls, "IsPullRequest "+issueRef(r, number))
					return tt.isPullRequest, tt.lookupErr
				},
				GetFunc: func(r ghrepo.Interface, issueNumber int, number int) (*client.ArtifactWithVersions, error) {
					calls = append(calls, fmt.Sprintf("Get %s artifact %d", issueRef(r, issueNumber), number))
					if a, ok := artifacts[number]; ok && !deleted[number] {
						return a, nil
					}
					return nil, apiError(404, "Not Found", number)
				},
				DeleteFunc: func(r ghrepo.Interface, issueNumber int, number int) error {
					calls = append(calls, fmt.Sprintf("Delete %s artifact %d", issueRef(r, issueNumber), number))
					if err := tt.deleteErrs[number]; err != nil {
						return err
					}
					if _, ok := artifacts[number]; !ok || deleted[number] {
						return apiError(404, "Not Found", number)
					}
					deleted[number] = true
					return nil
				},
			}
			pm := &prompter.PrompterMock{
				ConfirmFunc: func(string, bool) (bool, error) {
					return tt.confirm, tt.promptErr
				},
			}

			opts := tt.opts
			opts.IO = ios
			opts.BaseRepo = func() (ghrepo.Interface, error) { return repo, nil }
			opts.Client = func() (client.ArtifactClient, error) { return mock, nil }
			opts.Prompter = pm

			err := deleteRun(&opts)

			assert.Equal(t, tt.wantCalls, calls)
			switch {
			case tt.wantErrIs != nil:
				require.ErrorIs(t, err, tt.wantErrIs)
			case tt.wantErr != "":
				require.EqualError(t, err, tt.wantErr)
			default:
				require.NoError(t, err)
			}
			if tt.wantPrompt == "" {
				assert.Empty(t, pm.ConfirmCalls())
			} else {
				require.Len(t, pm.ConfirmCalls(), 1)
				assert.Equal(t, tt.wantPrompt, pm.ConfirmCalls()[0].Prompt)
				assert.False(t, pm.ConfirmCalls()[0].DefaultValue, "the prompt defaults to No")
			}
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
