package archive

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ArchiveOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Confirmed  bool
	IO         *iostreams.IOStreams
	RepoArg    string
	Prompter   prompter.Prompter
}

func NewCmdArchive(f *cmdutil.Factory, runF func(*ArchiveOptions) error) *cobra.Command {
	opts := &ArchiveOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		BaseRepo:   f.BaseRepo,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "archive [<repository>]",
		Short: "Archive a repository",
		Long: heredoc.Doc(`Archive a GitHub repository.

With no argument, archives the current repository.`),
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

			return archiveRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Confirmed, "confirm", false, "Skip the confirmation prompt")
	_ = cmd.Flags().MarkDeprecated("confirm", "use `--yes` instead")
	cmd.Flags().BoolVarP(&opts.Confirmed, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}

func archiveRun(opts *ArchiveOptions) error {
	cs := opts.IO.ColorScheme()
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	var toArchive ghrepo.Interface

	if opts.RepoArg == "" {
		toArchive, err = opts.BaseRepo()
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

		toArchive, err = ghrepo.FromFullName(repoSelector)
		if err != nil {
			return err
		}
	}

	fields := []string{"name", "owner", "isArchived", "id"}
	repo, err := api.FetchRepository(apiClient, toArchive, fields)
	if err != nil {
		return err
	}

	fullName := ghrepo.FullName(toArchive)
	if repo.IsArchived {
		fmt.Fprintf(opts.IO.ErrOut, "%s Repository %s is already archived\n", cs.WarningIcon(), fullName)
		return nil
	}

	if !opts.Confirmed {
		confirmed, err := opts.Prompter.Confirm(fmt.Sprintf("Archive %s?", fullName), false)
		if err != nil {
			return fmt.Errorf("failed to prompt: %w", err)
		}
		if !confirmed {
			return cmdutil.CancelError
		}
	}

	err = archiveRepo(httpClient, repo)
	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out,
			"%s Archived repository %s\n",
			cs.SuccessIcon(),
			fullName)
	}

	return nil
}
