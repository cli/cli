package remove

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type RemoveOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	SecretName string
	OrgName    string
}

func NewCmdRemove(f *cmdutil.Factory, runF func(*RemoveOptions) error) *cobra.Command {
	opts := &RemoveOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "remove <secret name>",
		Short: "Remove an organization or repository secret",
		Example: heredoc.Doc(`
			$ gh secret remove REPO_SECRET
			$ gh secret remove --org ORG_SECRET
			$ gh secret remove --org="anotherOrg" ORG_SECRET
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			opts.SecretName = args[0]

			if runF != nil {
				return runF(opts)
			}

			return removeRun(opts)
		},
	}
	cmd.Flags().StringVar(&opts.OrgName, "org", "", "List secrets for an organization")
	cmd.Flags().Lookup("org").NoOptDefVal = "@owner"

	return cmd
}

func removeRun(opts *RemoveOptions) error {
	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not create http client: %w", err)
	}
	client := api.NewClientFromHTTP(c)

	var baseRepo ghrepo.Interface
	if opts.OrgName == "" || opts.OrgName == "@owner" {
		baseRepo, err = opts.BaseRepo()
		if err != nil {
			return fmt.Errorf("could not determine base repo: %w", err)
		}
	}

	host := ghinstance.OverridableDefault()
	if opts.OrgName == "@owner" {
		opts.OrgName = baseRepo.RepoOwner()
		host = baseRepo.RepoHost()
	}

	var path string
	if opts.OrgName == "" {
		path = fmt.Sprintf("repos/%s/actions/secrets/%s", ghrepo.FullName(baseRepo), opts.SecretName)
	} else {
		path = fmt.Sprintf("orgs/%s/actions/secrets/%s", opts.OrgName, opts.SecretName)
	}

	err = client.REST(host, "DELETE", path, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to delete secret %s: %w", opts.SecretName, err)
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Out, "%s Removed secret %s\n", cs.SuccessIcon(), opts.SecretName)
	}

	return nil
}
