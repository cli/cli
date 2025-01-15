package unarchive

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type UnarchiveOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (gh.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Confirmed  bool
	IO         *iostreams.IOStreams
	RepoArg    string
	Prompter   prompter.Prompter
}

func NewCmdUnarchive(f *cmdutil.Factory, runF func(*UnarchiveOptions) error) *cobra.Command {
	opts := &UnarchiveOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		BaseRepo:   f.BaseRepo,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "unarchive [<repository>]",
		Short: "Unarchive a repository",
		Long: heredoc.Doc(`Unarchive a GitHub repository.

With no argument, unarchives the current repository.`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RepoArg = args[0]
			}

			if !opts.Confirmed && !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("--yes required when not running interactively")
			}

			if runF != nil {
				return runF(opts)
			}

			return unarchiveRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Confirmed, "confirm", false, "Skip the confirmation prompt")
	_ = cmd.Flags().MarkDeprecated("confirm", "use `--yes` instead")
	cmd.Flags().BoolVarP(&opts.Confirmed, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}

func unarchiveRun(opts *UnarchiveOptions) error {
	cs := opts.IO.ColorScheme()
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	var toUnarchive ghrepo.Interface

	if opts.RepoArg == "" {
		toUnarchive, err = opts.BaseRepo()
		if err != nil {
			return err
		}
	} else {
		repoSelector := opts.RepoArg
		if !strings.Contains(repoSelector, "/") {
			cfg, err := opts.Config()
			if err != nil {
				return err
			}

			hostname, _ := cfg.Authentication().DefaultHost()

			currentUser, err := api.CurrentLoginName(apiClient, hostname)
			if err != nil {
				return err
			}
			repoSelector = currentUser + "/" + repoSelector
		}

		toUnarchive, err = ghrepo.FromFullName(repoSelector)
		if err != nil {
			return err
		}
	}

	fields := []string{"name", "owner", "isArchived", "id"}
	repo, err := api.FetchRepository(apiClient, toUnarchive, fields)
	if err != nil {
		return err
	}

	fullName := ghrepo.FullName(toUnarchive)
	if !repo.IsArchived {
		fmt.Fprintf(opts.IO.ErrOut, "%s Repository %s is not archived\n", cs.WarningIcon(), fullName)
		return nil
	}

	if !opts.Confirmed {
		confirmed, err := opts.Prompter.Confirm(fmt.Sprintf("Unarchive %s?", fullName), false)
		if err != nil {
			return err
		}
		if !confirmed {
			return cmdutil.CancelError
		}
	}

	err = unarchiveRepo(httpClient, repo)
	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "%s Unarchived repository %s\n", cs.SuccessIcon(), fullName)
	}

	return nil
}
