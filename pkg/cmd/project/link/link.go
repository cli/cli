package link

import (
	"fmt"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/client"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
	"net/http"
	"strconv"
)

type linkOpts struct {
	number    int32
	owner     string
	repo      string
	projectID string
	repoID    string
	format    string
	exporter  cmdutil.Exporter
}

type linkConfig struct {
	httpClient func() (*http.Client, error)
	client     *queries.Client
	opts       linkOpts
	io         *iostreams.IOStreams
}

type linkProjectToRepoMutation struct {
	LinkProjectV2ToRepository struct {
		Repository queries.Repository `graphql:"repository"`
	} `graphql:"linkProjectV2ToRepository(input:$input)"`
}

func NewCmdLink(f *cmdutil.Factory, runF func(config linkConfig) error) *cobra.Command {
	opts := linkOpts{}
	linkCmd := &cobra.Command{
		Short: "Link a project to a repository",
		Use:   "link [<number>] [flag]",
		Example: heredoc.Doc(`
			# link monalisa's project "1" to her repository "my_repo"
			gh project link 1 --repo my_repo
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client.New(f)
			if err != nil {
				return err
			}

			if len(args) == 1 {
				num, err := strconv.ParseInt(args[0], 10, 32)
				if err != nil {
					return cmdutil.FlagErrorf("invalid number: %v", args[0])
				}
				opts.number = int32(num)
			}

			config := linkConfig{
				httpClient: f.HttpClient,
				client:     client,
				opts:       opts,
				io:         f.IOStreams,
			}

			if config.opts.repo == "" {
				return fmt.Errorf("required flag: repo")
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runLink(config)
		},
	}

	linkCmd.Flags().StringVar(&opts.owner, "owner", "", "Login of the owner. Use \"@me\" for the current user.")
	linkCmd.Flags().StringVarP(&opts.repo, "repo", "R", "", "The repository to be linked to this project")
	cmdutil.AddFormatFlags(linkCmd, &opts.exporter)

	return linkCmd
}

func runLink(config linkConfig) error {
	canPrompt := config.io.CanPrompt()
	owner, err := config.client.NewOwner(canPrompt, config.opts.owner)
	if err != nil {
		return err
	}

	project, err := config.client.NewProject(canPrompt, owner, config.opts.number, false)
	if err != nil {
		return err
	}
	config.opts.projectID = project.ID

	httpClient, err := config.httpClient()
	if err != nil {
		return err
	}
	c := api.NewClientFromHTTP(httpClient)

	repo, err := api.GitHubRepo(c, ghrepo.New(owner.Login, config.opts.repo))
	if err != nil {
		return err
	}
	config.opts.repoID = repo.ID

	query, variable := linkRepoArgs(config)
	err = config.client.Mutate("LinkProjectV2ToRepository", query, variable)
	if err != nil {
		return err
	}

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, query.LinkProjectV2ToRepository.Repository)
	}
	return printResults(config, query.LinkProjectV2ToRepository.Repository)
}

func linkRepoArgs(config linkConfig) (*linkProjectToRepoMutation, map[string]interface{}) {
	return &linkProjectToRepoMutation{}, map[string]interface{}{
		"input": githubv4.LinkProjectV2ToRepositoryInput{
			ProjectID:    githubv4.ID(config.opts.projectID),
			RepositoryID: githubv4.ID(config.opts.repoID),
		},
	}
}

func printResults(config linkConfig, repo queries.Repository) error {
	if !config.io.IsStdoutTTY() {
		return nil
	}

	_, err := fmt.Fprintf(config.io.Out, "%s\n", repo.Project.URL)
	return err
}
