package delete

import (
	"context"
	"fmt"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DeleteOptions struct {
	IO         *iostreams.IOStreams
	GitHubREST func(host string, opts ...githubrest.ClientOption) (*githubrest.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)

	KeyID string
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := &DeleteOptions{
		GitHubREST: f.GitHubREST,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "delete <key-id>",
		Short: "Delete a deploy key from a GitHub repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo
			opts.KeyID = args[0]

			if runF != nil {
				return runF(opts)
			}
			return deleteRun(cmd.Context(), opts)
		},
	}

	return cmd
}

func deleteRun(ctx context.Context, opts *DeleteOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	client, err := opts.GitHubREST(repo.RepoHost())
	if err != nil {
		return err
	}

	if err := deleteDeployKey(ctx, client, repo, opts.KeyID); err != nil {
		return err
	}

	if !opts.IO.IsStdoutTTY() {
		return nil
	}

	cs := opts.IO.ColorScheme()
	_, err = fmt.Fprintf(opts.IO.Out, "%s Deploy key deleted from %s\n", cs.SuccessIconWithColor(cs.Red), cs.Bold(ghrepo.FullName(repo)))
	return err
}
