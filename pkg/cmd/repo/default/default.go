package base

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DefaultOptions struct {
	IO         *iostreams.IOStreams
	Remotes    func() (context.Remotes, error)
	HttpClient func() (*http.Client, error)

	ViewFlag bool
}

func NewCmdDefault(f *cmdutil.Factory) *cobra.Command {
	opts := &DefaultOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Remotes:    f.Remotes,
	}

	cmd := &cobra.Command{
		Use:   "default",
		Short: "Configure the default repository used for various commands",
		Long: heredoc.Doc(`
		The default repository is used to determine which remote 
		repository gh should automatically point to.
		`),
		Example: heredoc.Doc(`
			$ gh repo default
			#=> prompts remote options

			$ gh repo default -v
			#=> cli/cli
		`),
		Annotations: map[string]string{
			"help:environment": heredoc.Doc(`
				To manually configure a remote for gh to use, modify your local repo's git config

				; Ex: setting gh to use the upstream remote
				[remote "upstream"]
 					gh-resolved = base
				...
			`),
		},
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDefault(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.ViewFlag, "view", "v", false, "view the default repository used for various commands")
	return cmd
}

func runDefault(opts *DefaultOptions) error {
	remotes, err := opts.Remotes()
	if err != nil {
		return err
	}

	if opts.ViewFlag {
		baseRepo, err := context.GetBaseRepo(remotes)
		if err != nil {
			return err
		}
		fmt.Fprintln(opts.IO.Out, ghrepo.FullName(baseRepo))
		return nil
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)
	repoContext, err := context.ResolveRemotesToRepos(remotes, apiClient, "")
	if err != nil {
		return err
	}
	if err = context.RemoveBaseRepo(remotes); err != nil {
		return err
	}
	return repoContext.SetBaseRepo(opts.IO)
}
