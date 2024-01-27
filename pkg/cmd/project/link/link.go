package link

import (
	"fmt"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/client"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"net/http"
	"strconv"
)

type linkOpts struct {
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

type linkConfig struct {
	httpClient func() (*http.Client, error)
	config     func() (config.Config, error)
	client     *queries.Client
	opts       linkOpts
	io         *iostreams.IOStreams
}

func NewCmdLink(f *cmdutil.Factory, runF func(config linkConfig) error) *cobra.Command {
	opts := linkOpts{}
	linkCmd := &cobra.Command{
		Short: "Link a project to a repository or a team",
		Use:   "link [<number>] [flag]",
		Example: heredoc.Doc(`
			# link monalisa's project 1 to her repository "my_repo"
			gh project link 1 --owner monalisa --repo my_repo

			# link monalisa's organization's project 1 to her team "my_team"
			gh project link 1 --owner my_organization --team my_team
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

			if opts.repo == "" && opts.team == "" {
				repo, err := f.BaseRepo()
				if err != nil {
					return err
				}
				opts.repo = repo.RepoName()
				if opts.owner == "" {
					opts.owner = repo.RepoOwner()
				}
			}

			if err := cmdutil.MutuallyExclusive("specify only one of `--repo` or `--team`", opts.repo != "", opts.team != ""); err != nil {
				return err
			}

			config := linkConfig{
				httpClient: f.HttpClient,
				config:     f.Config,
				client:     client,
				opts:       opts,
				io:         f.IOStreams,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runLink(config)
		},
	}

	cmdutil.EnableRepoOverride(linkCmd, f)
	linkCmd.Flags().StringVar(&opts.owner, "owner", "", "Login of the owner. Use \"@me\" for the current user.")
	linkCmd.Flags().StringVarP(&opts.repo, "repo", "R", "", "The repository to be linked to this project")
	linkCmd.Flags().StringVarP(&opts.team, "team", "T", "", "The team to be linked to this project")
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
	if config.opts.number == 0 {
		config.opts.number = project.Number
	}

	httpClient, err := config.httpClient()
	if err != nil {
		return err
	}
	c := api.NewClientFromHTTP(httpClient)

	cfg, err := config.config()
	if err != nil {
		return err
	}
	host, _ := cfg.Authentication().DefaultHost()

	if config.opts.repo != "" {
		return linkRepo(c, owner, host, config)
	} else if config.opts.team != "" {
		return linkTeam(c, owner, host, config)
	}
	return nil
}

func linkRepo(c *api.Client, owner *queries.Owner, host string, config linkConfig) error {
	repo, err := api.GitHubRepo(c, ghrepo.NewWithHost(owner.Login, config.opts.repo, host))
	if err != nil {
		return err
	}
	config.opts.repoID = repo.ID

	result, err := config.client.LinkProjectToRepository(config.opts.projectID, config.opts.repoID)
	if err != nil {
		return err
	}

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, result)
	}
	return printResults(config, owner, config.opts.repo)
}

func linkTeam(c *api.Client, owner *queries.Owner, host string, config linkConfig) error {
	team, err := api.OrganizationTeam(c, host, owner.Login, config.opts.team)
	if err != nil {
		return err
	}
	config.opts.teamID = team.ID

	result, err := config.client.LinkProjectToTeam(config.opts.projectID, config.opts.teamID)
	if err != nil {
		return err
	}

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, result)
	}
	return printResults(config, owner, config.opts.team)
}

func printResults(config linkConfig, owner *queries.Owner, linkedTarget string) error {
	if !config.io.IsStdoutTTY() {
		return nil
	}

	_, err := fmt.Fprintf(config.io.Out, "Linked '%s/%s' to project #%d\n", owner.Login, linkedTarget, config.opts.number)
	return err
}
