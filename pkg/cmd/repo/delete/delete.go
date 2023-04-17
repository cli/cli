package delete

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	ghAuth "github.com/cli/go-gh/v2/pkg/auth"
	"github.com/spf13/cobra"
)

type iprompter interface {
	ConfirmDeletion(string) error
}

type DeleteOptions struct {
	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Prompter   iprompter
	IO         *iostreams.IOStreams
	RepoArg    string
	Confirmed  bool
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
		Long: `Delete a GitHub repository.

With no argument, deletes the current repository. Otherwise, deletes the specified repository.

Deletion requires authorization with the "delete_repo" scope. 
To authorize, run "gh auth refresh -s delete_repo"`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RepoArg = args[0]
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

	cmd.Flags().BoolVar(&opts.Confirmed, "confirm", false, "confirm deletion without prompting")
	_ = cmd.Flags().MarkDeprecated("confirm", "use `--yes` instead")
	cmd.Flags().BoolVar(&opts.Confirmed, "yes", false, "confirm deletion without prompting")
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
			defaultHost, _ := ghAuth.DefaultHost()
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

	if !opts.Confirmed {
		if err := opts.Prompter.ConfirmDeletion(fullName); err != nil {
			return err
		}
	}

	err = deleteRepo(httpClient, toDelete)
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) {
			statusCode := httpErr.HTTPError.StatusCode
			if statusCode == http.StatusMovedPermanently ||
				statusCode == http.StatusTemporaryRedirect ||
				statusCode == http.StatusPermanentRedirect {
				cs := opts.IO.ColorScheme()
				fmt.Fprintf(opts.IO.ErrOut, "%s Failed to delete repository: %s has changed name or transfered ownership\n", cs.FailureIcon(), fullName)
				return cmdutil.SilentError
			}
		}
		return err
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
