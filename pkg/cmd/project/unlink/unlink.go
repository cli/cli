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
	team      string
	projectID string
	repoID    string
	teamID    string
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

type unlinkProjectFromTeamMutation struct {
	UnlinkProjectV2FromTeam struct {
		Team queries.Team `graphql:"team"`
	} `graphql:"unlinkProjectV2FromTeam(input:$input)"`
}

func NewCmdUnlink(f *cmdutil.Factory, runF func(config unlinkConfig) error) *cobra.Command {
	opts := unlinkOpts{}
	linkCmd := &cobra.Command{
		Short: "Unlink a project from a repository or a team",
		Use:   "unlink [<number>] [flag]",
		Example: heredoc.Doc(`
			# unlink monalisa's project 1 from her repository "my_repo"
			gh project unlink 1 --owner monalisa --repo my_repo

			# unlink monalisa's project 1 from her team "my_team"
			gh project unlink 1 --owner my_organization --team my_team
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

			if config.opts.repo != "" && config.opts.team != "" {
				return fmt.Errorf("specify only one repo or team")
			} else if config.opts.repo == "" && config.opts.team == "" {
				return fmt.Errorf("specify at least one repo or team")
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
	linkCmd.Flags().StringVarP(&opts.team, "team", "T", "", "The team to be unlinked from this project")
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

	if config.opts.repo != "" {
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
		return printResults(config, query.UnlinkProjectV2FromRepository.Repository.URL)

	} else if config.opts.team != "" {
		teams, err := api.OrganizationTeams(c, ghrepo.New(owner.Login, ""))
		if err != nil {
			return err
		}
		for _, team := range teams {
			if team.Slug == config.opts.team {
				config.opts.teamID = team.ID
				break
			}
		}
		if config.opts.teamID == "" {
			return fmt.Errorf("can't find team %s", config.opts.team)
		}

		query, variable := unlinkTeamArgs(config)
		err = config.client.Mutate("UnlinkProjectV2FromTeam", query, variable)
		if err != nil {
			return err
		}

		if config.opts.exporter != nil {
			return config.opts.exporter.Write(config.io, query.UnlinkProjectV2FromTeam.Team)
		}
		return printResults(config, query.UnlinkProjectV2FromTeam.Team.URL)
	}
	return fmt.Errorf("specify at least one repo or team")
}

func unlinkRepoArgs(config unlinkConfig) (*unlinkProjectFromRepoMutation, map[string]interface{}) {
	return &unlinkProjectFromRepoMutation{}, map[string]interface{}{
		"input": githubv4.UnlinkProjectV2FromRepositoryInput{
			ProjectID:    githubv4.ID(config.opts.projectID),
			RepositoryID: githubv4.ID(config.opts.repoID),
		},
	}
}

func unlinkTeamArgs(config unlinkConfig) (*unlinkProjectFromTeamMutation, map[string]interface{}) {
	return &unlinkProjectFromTeamMutation{}, map[string]interface{}{
		"input": githubv4.UnlinkProjectV2FromTeamInput{
			ProjectID: githubv4.ID(config.opts.projectID),
			TeamID:    githubv4.ID(config.opts.teamID),
		},
	}
}

func printResults(config unlinkConfig, url string) error {
	if !config.io.IsStdoutTTY() {
		return nil
	}

	_, err := fmt.Fprintf(config.io.Out, "%s\n", url)
	return err
}
