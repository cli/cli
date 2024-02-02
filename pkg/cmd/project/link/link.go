package link

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/client"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type linkOpts struct {
	number   int32
	owner    string
	repo     string
	teamSlug string
}

type linkConfig struct {
	io          *iostreams.IOStreams
	httpClient  func() (*http.Client, error)
	config      func() (config.Config, error)
	baseRepo    func() (ghrepo.Interface, error)
	queryClient *queries.Client
	opts        linkOpts
}

func NewCmdLink(f *cmdutil.Factory, runF func(config linkConfig) error) *cobra.Command {
	opts := linkOpts{}
	linkCmd := &cobra.Command{
		Short: "Link a project to a repository or a team",
		Use:   "link [<number>]",
		Example: heredoc.Doc(`
			# link monalisa's project 1 to her repository "my_repo"
			gh project link 1 --owner monalisa --repo my_repo

			# link monalisa's organization's project 1 to her team "my_team"
			gh project link 1 --owner my_organization --team my_team

			# link monalisa's project 1 to the repository of current directory if neither --repo nor --team is specified
			gh project link 1
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			queryClient, err := client.New(f)
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

			if err := cmdutil.MutuallyExclusive("specify only one of `--repo` or `--team`", opts.repo != "", opts.teamSlug != ""); err != nil {
				return err
			}

			config := linkConfig{
				io:          f.IOStreams,
				httpClient:  f.HttpClient,
				config:      f.Config,
				baseRepo:    f.BaseRepo,
				queryClient: queryClient,
				opts:        opts,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runLink(config)
		},
	}

	cmdutil.EnableRepoOverrideVar(linkCmd, f, &opts.repo)
	linkCmd.Flags().StringVar(&opts.owner, "owner", "", "Login of the project owner. Use \"@me\" for the current user.")
	linkCmd.Flags().StringVarP(&opts.teamSlug, "team", "T", "", "The team to be linked to this project")

	return linkCmd
}

func runLink(config linkConfig) error {
	canPrompt := config.io.CanPrompt()
	owner, err := config.queryClient.NewOwner(canPrompt, config.opts.owner)
	if err != nil {
		return err
	}

	project, err := config.queryClient.NewProject(canPrompt, owner, config.opts.number, false)
	if err != nil {
		return err
	}

	httpClient, err := config.httpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	cfg, err := config.config()
	if err != nil {
		return err
	}
	host, _ := cfg.Authentication().DefaultHost()

	// If we have a team slug, we'll link the project and team together
	if config.opts.teamSlug != "" {
		if err != linkTeam(apiClient, host, config.queryClient, project, config.opts.teamSlug) {
			return err
		}

		if config.io.IsStdoutTTY() {
			fmt.Fprintf(config.io.Out, "Linked '%s/%s' to project #%d '%s'\n", project.OwnerLogin(), config.opts.teamSlug, project.Number, project.Title)
		}

		return nil
	}

	// Otherwise, we'll see if a repo arg was provided, and if not then we'll try to use
	// whichever repo we happen to be in (or the value of GH_HOST)
	var repo ghrepo.Interface
	if config.opts.repo != "" {
		if repo, err = ghrepo.FromFullName(config.opts.repo); err != nil {
			return err
		}
	} else {
		if repo, err = config.baseRepo(); err != nil {
			return err
		}
	}

	// If the repo points at a different host than the other clients (which use GH_HOST or DefaultHost)
	// then consider this an error. This could be fixed up later with some refactoring.
	if repo.RepoHost() != host {
		return fmt.Errorf("the provided repo's host cannot be different than the default host")
	}

	if repo.RepoOwner() != config.opts.owner {
		return fmt.Errorf("the repo owner and project owner must be the same")
	}

	if err := linkRepo(apiClient, config.queryClient, project, repo); err != nil {
		return err
	}

	if config.io.IsStdoutTTY() {
		fmt.Fprintf(config.io.Out, "Linked '%s' to project #%d '%s'\n", ghrepo.FullName(repo), project.Number, project.Title)
	}

	return nil
}

func linkTeam(apiClient *api.Client, host string, queryClient *queries.Client, project *queries.Project, targetTeam string) error {
	team, err := api.OrganizationTeam(apiClient, host, project.OwnerLogin(), targetTeam)
	if err != nil {
		return err
	}

	err = queryClient.LinkProjectToTeam(project.ID, team.ID)
	if err != nil {
		return err
	}

	return nil
}

func linkRepo(apiClient *api.Client, queryClient *queries.Client, project *queries.Project, targetRepo ghrepo.Interface) error {
	apiHydratedRepo, err := api.GitHubRepo(apiClient, targetRepo)
	if err != nil {
		return err
	}

	err = queryClient.LinkProjectToRepository(project.ID, apiHydratedRepo.ID)
	if err != nil {
		return err
	}

	return nil
}
