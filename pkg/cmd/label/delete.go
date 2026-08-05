package label

import (
	"context"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type iprompter interface {
	ConfirmDeletion(string) error
}

type deleteOptions struct {
	BaseRepo   func() (ghrepo.Interface, error)
	GitHubREST func(host string, opts ...githubrest.ClientOption) (*githubrest.Client, error)
	IO         *iostreams.IOStreams
	Prompter   iprompter

	Name      string
	Confirmed bool
}

func newCmdDelete(f *cmdutil.Factory, runF func(*deleteOptions) error) *cobra.Command {
	opts := deleteOptions{
		GitHubREST: f.GitHubREST,
		IO:         f.IOStreams,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a label from a repository",
		Args:  cmdutil.ExactArgs(1, "cannot delete label: name argument required"),
		RunE: func(c *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo
			opts.Name = args[0]

			if !opts.IO.CanPrompt() && !opts.Confirmed {
				return cmdutil.FlagErrorf("--yes required when not running interactively")
			}

			if runF != nil {
				return runF(&opts)
			}
			return deleteRun(c.Context(), &opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Confirmed, "confirm", false, "Confirm deletion without prompting")
	_ = cmd.Flags().MarkDeprecated("confirm", "use `--yes` instead")
	cmd.Flags().BoolVar(&opts.Confirmed, "yes", false, "Confirm deletion without prompting")

	return cmd
}

func deleteRun(ctx context.Context, opts *deleteOptions) error {
	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	client, err := opts.GitHubREST(baseRepo.RepoHost())
	if err != nil {
		return err
	}

	if !opts.Confirmed {
		if err := opts.Prompter.ConfirmDeletion(opts.Name); err != nil {
			return err
		}
	}

	opts.IO.StartProgressIndicator()
	err = deleteLabel(ctx, client, baseRepo, opts.Name)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		successMsg := fmt.Sprintf("%s Label %q deleted from %s\n", cs.SuccessIcon(), opts.Name, ghrepo.FullName(baseRepo))
		fmt.Fprint(opts.IO.Out, successMsg)
	}

	return nil
}

func deleteLabel(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface, name string) error {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "labels", name)
	if err != nil {
		return err
	}

	req, err := client.NewRequest(ctx, http.MethodDelete, path.String(), nil)
	if err != nil {
		return err
	}
	_, err = client.Do(req, nil)
	return err
}
