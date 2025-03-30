package delete

import (
	"fmt"

	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/repo/autolink/view"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type deleteOptions struct {
	BaseRepo             func() (ghrepo.Interface, error)
	Browser              browser.Browser
	AutolinkDeleteClient AutolinkDeleteClient
	AutolinkViewClient   view.AutolinkViewClient
	IO                   *iostreams.IOStreams

	ID        string
	Confirmed bool
	Prompter  prompter.Prompter
}

type AutolinkDeleteClient interface {
	Delete(repo ghrepo.Interface, id string) error
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*deleteOptions) error) *cobra.Command {
	opts := &deleteOptions{
		Browser:  f.Browser,
		IO:       f.IOStreams,
		Prompter: f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete an autolink reference",
		Long:  "Delete an autolink reference for a repository.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo

			httpClient, err := f.HttpClient()
			if err != nil {
				return err
			}

			opts.AutolinkDeleteClient = &AutolinkDeleter{HTTPClient: httpClient}
			opts.AutolinkViewClient = &view.AutolinkViewer{HTTPClient: httpClient}
			opts.ID = args[0]

			if !opts.IO.CanPrompt() && !opts.Confirmed {
				return cmdutil.FlagErrorf("--yes required when not running interactively")
			}

			if runF != nil {
				return runF(opts)
			}

			return deleteRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Confirmed, "yes", false, "Confirm deletion without prompting")

	return cmd
}

func deleteRun(opts *deleteOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	out := opts.IO.Out
	cs := opts.IO.ColorScheme()

	autolink, err := opts.AutolinkViewClient.View(repo, opts.ID)

	if err != nil {
		return fmt.Errorf("%s %w", cs.Red("error deleting autolink:"), err)
	}

	if opts.IO.CanPrompt() && !opts.Confirmed {
		fmt.Fprintf(out, "Autolink %s has key prefix %s.\n", cs.Cyan(opts.ID), autolink.KeyPrefix)

		err := opts.Prompter.ConfirmDeletion(autolink.KeyPrefix)

		if err != nil {
			return err
		}
	}

	err = opts.AutolinkDeleteClient.Delete(repo, opts.ID)
	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(out, "%s Autolink %s deleted from %s\n", cs.SuccessIcon(), cs.Cyan(opts.ID), cs.Bold(ghrepo.FullName(repo)))
	}

	return nil
}
