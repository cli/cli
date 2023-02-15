package list

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)

	Organization string
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List rulesets for a repository or organization",
		Long: heredoc.Doc(`
			TODO
		`),
		Example: "TODO",
		Args:    cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if runF != nil {
				return runF(opts)
			}

			return listRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Organization, "org", "o", "", "List organization-wide rules")

	return cmd
}

type Ruleset struct {
	// TODO
}

func listRun(opts *ListOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(httpClient)

	repoI, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("repos/%s/%s/rulesets",
		repoI.RepoOwner(), repoI.RepoName())
	hostname := repoI.RepoHost()

	if opts.Organization != "" {
		endpoint = fmt.Sprintf("orgs/%s/rulesets", opts.Organization)
		hostname, _ = cfg.DefaultHost()
	}

	//var response []Ruleset
	var response interface{}

	err = client.REST(hostname, "GET", endpoint, nil, &response)
	if err != nil {
		return fmt.Errorf("failed to call '%s': %w", endpoint, err)
	}

	fmt.Printf("DBG %#v\n", response)

	return nil
}
