package link

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/client"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type linkOpts struct {
	number       int32
	host         string
	owner        string
	repo         string
	team         string
	projectID    string
	projectTitle string
	repoID       string
	teamID       string
}

type linkConfig struct {
	httpClient func() (*http.Client, error)
	config     func() (gh.Config, error)
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

			# link monalisa's project 1 to the repository of current directory if neither --repo nor --team is specified
			gh project link 1
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

			if err = validateRepoOrTeamFlag(&opts); err != nil {
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

	return linkCmd
}

func validateRepoOrTeamFlag(opts *linkOpts) error {

	linkedTarget := ""
	if opts.repo != "" {
		linkedTarget = opts.repo
	} else if opts.team != "" {
		linkedTarget = opts.team
	}

	if strings.Contains(linkedTarget, "/") {
		nameArgs := strings.Split(linkedTarget, "/")
		var host, owner, name string

		if len(nameArgs) == 2 {
			owner = nameArgs[0]
			name = nameArgs[1]
		} else if len(nameArgs) == 3 {
			host = nameArgs[0]
			owner = nameArgs[1]
			name = nameArgs[2]
		} else {
			if opts.repo != "" {
				return fmt.Errorf("expected the \"[HOST/]OWNER/REPO\" or \"REPO\" format, got \"%s\"", linkedTarget)
			} else if opts.team != "" {
				return fmt.Errorf("expected the \"[HOST/]OWNER/TEAM\" or \"TEAM\" format, got \"%s\"", linkedTarget)
			}
		}

		if opts.owner != "" && opts.owner != owner {
			return fmt.Errorf("'%s' has different owner from '%s'", linkedTarget, opts.owner)
		}

		opts.owner = owner
		opts.host = host
		if opts.repo != "" {
			opts.repo = name
		} else if opts.team != "" {
			opts.team = name
		}
	}
	return nil
}

func runLink(config linkConfig) error {
	canPrompt := config.io.CanPrompt()
	owner, err := config.client.NewOwner(canPrompt, config.opts.owner)
	if err != nil {
		return err
	}
	config.opts.owner = owner.Login

	project, err := config.client.NewProject(canPrompt, owner, config.opts.number, false)
	if err != nil {
		return err
	}
	config.opts.projectTitle = project.Title
	config.opts.projectID = project.ID
	if config.opts.number == 0 {
		config.opts.number = project.Number
	}

	httpClient, err := config.httpClient()
	if err != nil {
		return err
	}
	c := api.NewClientFromHTTP(httpClient)

	if config.opts.host == "" {
		cfg, err := config.config()
		if err != nil {
			return err
		}
		host, _ := cfg.Authentication().DefaultHost()
		config.opts.host = host
	}

	if config.opts.repo != "" {
		return linkRepo(c, config)
	} else if config.opts.team != "" {
		return linkTeam(c, config)
	}
	return nil
}

func linkRepo(c *api.Client, config linkConfig) error {
	repo, err := api.GitHubRepo(c, ghrepo.NewWithHost(config.opts.owner, config.opts.repo, config.opts.host))
	if err != nil {
		return err
	}
	config.opts.repoID = repo.ID

	err = config.client.LinkProjectToRepository(config.opts.projectID, config.opts.repoID)
	if err != nil {
		return err
	}

	return printResults(config, config.opts.repo)
}

func linkTeam(c *api.Client, config linkConfig) error {
	team, err := api.OrganizationTeam(c, config.opts.host, config.opts.owner, config.opts.team)
	if err != nil {
		return err
	}
	config.opts.teamID = team.ID

	err = config.client.LinkProjectToTeam(config.opts.projectID, config.opts.teamID)
	if err != nil {
		return err
	}

	return printResults(config, config.opts.team)
}

func printResults(config linkConfig, linkedTarget string) error {
	if !config.io.IsStdoutTTY() {
		return nil
	}

	_, err := fmt.Fprintf(config.io.Out, "Linked '%s/%s' to project #%d '%s'\n", config.opts.owner, linkedTarget, config.opts.number, config.opts.projectTitle)
	return err
}
