package delete

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
	"github.com/spf13/cobra"
)

type iprompter interface {
	Confirm(string, bool) (bool, error)
	ConfirmDeletion(string) error
}

type DeleteOptions struct {
	HttpClient                            func() (*http.Client, error)
	BaseRepo                              func() (ghrepo.Interface, error)
	Prompter                              iprompter
	IO                                    *iostreams.IOStreams
	RepoArg                               string
	Confirmed                             bool
	AcceptCascadingDeletionOfPrivateForks bool
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := &DeleteOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "delete [<repository>]",
		Short: "Delete a repository",
		Long: heredoc.Docf(`
			Delete a GitHub repository.

			With no argument, deletes the current repository. Otherwise, deletes the specified repository.
			
			For safety, when no repository argument is provided, the %[1]s--yes%[1]s flag is ignored 
			and you will be prompted for confirmation. To delete the current repository non-interactively, 
			specify it explicitly (e.g., %[1]sgh repo delete owner/repo --yes%[1]s).

			Deleting a private repository that is not itself a fork also deletes its forks, including
			forks you may not be able to see. If it has forks, you will be asked to confirm deleting
			them, even with %[1]s--yes%[1]s. Pass %[1]s--accept-cascading-deletion-of-private-forks%[1]s
			to skip that prompt. When not running interactively, it is required along with %[1]s--yes%[1]s.
			Like %[1]s--yes%[1]s, it is ignored when no repository argument is provided.

			Deletion requires authorization with the %[1]sdelete_repo%[1]s scope.
			To authorize, run %[1]sgh auth refresh -s delete_repo%[1]s
		`, "`"),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RepoArg = args[0]
			}

			// Ignore confirmation flags when no argument provided to prevent accidental deletion
			if len(args) == 0 && (opts.Confirmed || opts.AcceptCascadingDeletionOfPrivateForks) {
				if !opts.IO.CanPrompt() {
					return cmdutil.FlagErrorf("cannot non-interactively delete current repository. Please specify a repository or run interactively")
				}
				if opts.Confirmed {
					_, _ = fmt.Fprintln(opts.IO.ErrOut, "Warning: `--yes` is ignored since no repository was specified")
					opts.Confirmed = false
				}
				if opts.AcceptCascadingDeletionOfPrivateForks {
					_, _ = fmt.Fprintln(opts.IO.ErrOut, "Warning: `--accept-cascading-deletion-of-private-forks` is ignored since no repository was specified")
					opts.AcceptCascadingDeletionOfPrivateForks = false
				}
			}

			if !opts.IO.CanPrompt() && !opts.Confirmed {
				return cmdutil.FlagErrorf("--yes required when not running interactively")
			}

			if runF != nil {
				return runF(opts)
			}

			return deleteRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Confirmed, "confirm", false, "Confirm deletion without prompting")
	_ = cmd.Flags().MarkDeprecated("confirm", "use `--yes` instead")
	cmd.Flags().BoolVar(&opts.Confirmed, "yes", false, "Confirm deletion without prompting for confirmation")
	cmd.Flags().BoolVar(&opts.AcceptCascadingDeletionOfPrivateForks, "accept-cascading-deletion-of-private-forks", false, "Accept that deleting a private repository also deletes its forks")
	return cmd
}

func deleteRun(opts *DeleteOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	var toDelete ghrepo.Interface

	if opts.RepoArg == "" {
		toDelete, err = opts.BaseRepo()
		if err != nil {
			return err
		}
	} else {
		repoSelector := opts.RepoArg
		if !strings.Contains(repoSelector, "/") {
			defaultHost, _ := ghauth.DefaultHost()
			currentUser, err := api.CurrentLoginName(apiClient, defaultHost)
			if err != nil {
				return err
			}
			repoSelector = currentUser + "/" + repoSelector
		}
		toDelete, err = ghrepo.FromFullName(repoSelector)
		if err != nil {
			return fmt.Errorf("argument error: %w", err)
		}
	}
	fullName := ghrepo.FullName(toDelete)

	if !opts.AcceptCascadingDeletionOfPrivateForks {
		if err := confirmForkDeletion(opts, httpClient, toDelete); err != nil {
			return err
		}
	}

	if !opts.Confirmed {
		if err := opts.Prompter.ConfirmDeletion(fullName); err != nil {
			return err
		}
	}

	err = deleteRepo(httpClient, toDelete)
	if err != nil {
		return explainMovedRepo(opts, fullName, err)
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Out,
			"%s Deleted repository %s\n",
			cs.SuccessIcon(),
			fullName)
	}

	return nil
}

// confirmForkDeletion asks before deleting a private repository that has forks,
// because GitHub deletes those forks with it. The caller often cannot see them:
// private forks inherit only team access from their upstream.
func confirmForkDeletion(opts *DeleteOptions, httpClient *http.Client, repo ghrepo.Interface) error {
	fullName := ghrepo.FullName(repo)

	info, err := fetchForkInfo(httpClient, repo)
	if err != nil {
		return explainMovedRepo(opts, fullName, err)
	}

	// Deleting a fork leaves the fork's own forks in place.
	if !info.Private || info.Fork || info.ForksCount == 0 {
		return nil
	}

	// Match the wording of the web UI's delete dialog.
	forks := text.Pluralize(info.ForksCount, "fork")
	if info.ForksCount > 1 {
		forks = "all " + forks
	}

	if !opts.IO.CanPrompt() {
		return cmdutil.FlagErrorf("--accept-cascading-deletion-of-private-forks required when not running interactively: deleting %s will also delete %s since this is a private repository", fullName, forks)
	}

	cs := opts.IO.ColorScheme()
	fmt.Fprintf(opts.IO.ErrOut, "%s This will also delete %s since this is a private repository.\n", cs.WarningIcon(), forks)

	confirmed, err := opts.Prompter.Confirm(fmt.Sprintf("Delete %s and its forks?", fullName), false)
	if err != nil {
		return err
	}
	if !confirmed {
		return cmdutil.CancelError
	}
	return nil
}

// explainMovedRepo reports the redirect that GitHub returns for a renamed or
// transferred repository. Requests here never follow it, so a moved repository
// is not looked up or deleted under its old name.
func explainMovedRepo(opts *DeleteOptions, fullName string, err error) error {
	if httpErr, ok := errors.AsType[api.HTTPError](err); ok {
		statusCode := httpErr.HTTPError.StatusCode
		if statusCode == http.StatusMovedPermanently ||
			statusCode == http.StatusTemporaryRedirect ||
			statusCode == http.StatusPermanentRedirect {
			cs := opts.IO.ColorScheme()
			fmt.Fprintf(opts.IO.ErrOut, "%s Failed to delete repository: %s has changed name or transferred ownership\n", cs.FailureIcon(), fullName)
			return cmdutil.SilentError
		}
	}
	return err
}
