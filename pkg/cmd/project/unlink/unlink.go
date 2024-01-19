package unlink

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

type unlinkOpts struct {
	number    int32
	owner     string
	repo      string
	projectID string
	repoID    string
	format    string
	exporter  cmdutil.Exporter
}

type unlinkConfig struct {
	httpClient func() (*http.Client, error)
	client     *queries.Client
	opts       unlinkOpts
	io         *iostreams.IOStreams
}

type unlinkProjectFromRepoMutation struct {
	UnlinkProjectV2FromRepository struct {
		Repository queries.Repository `graphql:"repository"`
	} `graphql:"unlinkProjectV2FromRepository(input:$input)"`
}

func NewCmdUnlink(f *cmdutil.Factory, runF func(config unlinkConfig) error) *cobra.Command {
	opts := unlinkOpts{}
	linkCmd := &cobra.Command{
		Short: "Unlink a project from a repository",
		Use:   "unlink [<number>] [flag]",
		Example: heredoc.Doc(`
			# unlink monalisa's project "1" from her repository "my_repo"
			gh project unlink 1 --repo my_repo
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

			config := unlinkConfig{
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
			return runUnlink(config)
		},
	}

	linkCmd.Flags().StringVar(&opts.owner, "owner", "", "Login of the owner. Use \"@me\" for the current user.")
	linkCmd.Flags().StringVarP(&opts.repo, "repo", "R", "", "The repository to be unlinked from this project")
	cmdutil.AddFormatFlags(linkCmd, &opts.exporter)

	return linkCmd
}

func runUnlink(config unlinkConfig) error {
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

	query, variable := unlinkRepoArgs(config)
	err = config.client.Mutate("UnlinkProjectV2FromRepository", query, variable)
	if err != nil {
		return err
	}

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, query.UnlinkProjectV2FromRepository.Repository)
	}
	return printResults(config, query.UnlinkProjectV2FromRepository.Repository)
}

func unlinkRepoArgs(config unlinkConfig) (*unlinkProjectFromRepoMutation, map[string]interface{}) {
	return &unlinkProjectFromRepoMutation{}, map[string]interface{}{
		"input": githubv4.UnlinkProjectV2FromRepositoryInput{
			ProjectID:    githubv4.ID(config.opts.projectID),
			RepositoryID: githubv4.ID(config.opts.repoID),
		},
	}
}

func printResults(config unlinkConfig, repo queries.Repository) error {
	if !config.io.IsStdoutTTY() {
		return nil
	}

	_, err := fmt.Fprintf(config.io.Out, "%s\n", repo.URL)
	return err
}
