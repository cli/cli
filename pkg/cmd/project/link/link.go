package link

import (
	"fmt"
	"strconv"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
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

type linkable interface {
	LinkWith(project project) error
}

type linkCfg struct {
	owner          string
	projectNumber  int32
	projectFetcher projectFetcher
	linkable       linkable
}

func NewCmdLink(f *cmdutil.Factory, runF func(config linkCfg) error) *cobra.Command {
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
			// Argument validation
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

			// Collect all the various client and config we need from the context
			ghCfg, err := f.Config()
			if err != nil {
				return err
			}
			defaultHost, _ := ghCfg.Authentication().DefaultHost()

			httpClient, err := f.HttpClient()
			if err != nil {
				return err
			}
			apiClient := api.NewClientFromHTTP(httpClient)
			projectQueryClient := queries.NewClient(httpClient, defaultHost, f.IOStreams)

			// And now we start slotting lego pieces together. You can find the lego pieces below.
			var linkable linkable
			if opts.teamSlug != "" {
				// If a team slug was provided we know we're linking a project to a team
				linkable = linkableTeam{
					slug: opts.teamSlug,
					teamIDFetcher: teamIDFetcher{
						apiClient: apiClient,
						host:      defaultHost,
					},
					projectTeamLinker: projectQueryClient,
				}
			} else {
				// If it wasn't provided then we know we're linking a project to a repository
				// and we'll get the repo from the --repo flag, the current directory, or GH_HOST
				repo, err := f.BaseRepo()
				if err != nil {
					return err
				}

				linkable = linkableRepo{
					repo:              repo,
					repoIDFetcher:     repoIDFetcher{apiClient: apiClient},
					projectRepoLinker: projectQueryClient,
				}
			}

			config := linkCfg{
				owner:         opts.owner,
				projectNumber: opts.number,
				projectFetcher: projectFetcher{
					ios: f.IOStreams,
					// Maybe make a new interface here but it might be overkill, it kind
					// of depends what our preferred testing method is.
					queryClient: projectQueryClient,
				},
				linkable: linkable,
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

func runLink(cfg linkCfg) error {
	project, err := cfg.projectFetcher.FetchProject(cfg.owner, cfg.projectNumber)
	if err != nil {
		return fmt.Errorf("failed to fetch project: %s", err)
	}

	return cfg.linkable.LinkWith(project)
}

// Here are all the lego pieces...

// First we deal with the concept of fetching a project...
type project struct {
	id    string
	owner string
}

type projectFetcher struct {
	ios         *iostreams.IOStreams
	queryClient *queries.Client
}

func (f *projectFetcher) FetchProject(owner string, number int32) (project, error) {
	canPrompt := f.ios.CanPrompt()
	apiHydratedOwner, err := f.queryClient.NewOwner(canPrompt, owner)
	if err != nil {
		return project{}, fmt.Errorf("failed to fetch project owner: %s", err)
	}

	apiHydratedProject, err := f.queryClient.NewProject(canPrompt, apiHydratedOwner, number, false)
	if err != nil {
		return project{}, fmt.Errorf("failed to fetch project: %s", err)
	}

	return project{
		id:    apiHydratedProject.ID,
		owner: apiHydratedProject.OwnerLogin(),
	}, nil
}

// Then we deal with the types required for linking projects to teams...
type teamIDFetcher struct {
	apiClient *api.Client
	host      string
}

func (f *teamIDFetcher) Fetch(org string, slug string) (string, error) {
	team, err := api.OrganizationTeam(f.apiClient, f.host, org, slug)
	if err != nil {
		return "", fmt.Errorf("failed to fetch team ID: %s", err)
	}

	return team.ID, nil
}

// Note that I've removed this interface but you could imagine it being used by linkableTeam
// (and similar patterns for other abstractions over the query client)
//
// type projectTeamLinker interface {
//   LinkProjectToTeam(projectID string, teamID string) error
// }
//
// Doing this makes it _extremely_ obvious what a linkableTeam depends upon, and also allows for
// very easy testing. I removed it because I don't want you to drown in indirection.
//
// I also decided to remove lots of new types like:
//   type teamID string
//
// I removed it because I also didn't want to go too far, but if you have your own wrappers for
// the parts that matter then its trivial to add type safety over bare strings.

type linkableTeam struct {
	slug              string
	teamIDFetcher     teamIDFetcher
	projectTeamLinker *queries.Client
	// projectTeamLinker projectTeamLinker (for example)
}

func (lt linkableTeam) LinkWith(project project) error {
	teamID, err := lt.teamIDFetcher.Fetch(project.owner, lt.slug)
	if err != nil {
		return err
	}

	err = lt.projectTeamLinker.LinkProjectToTeam(string(project.id), string(teamID))
	if err != nil {
		return fmt.Errorf("failed to link project to team: %s", err)
	}

	return nil
}

// Finally we deal with the types required for linking projects to repos..
type repoIDFetcher struct {
	apiClient *api.Client
}

func (f *repoIDFetcher) Fetch(ghRepo ghrepo.Interface) (string, error) {
	apiHydratedRepo, err := api.GitHubRepo(f.apiClient, ghRepo)
	if err != nil {
		return "", fmt.Errorf("failed to fetch repository ID: %s", err)
	}

	return apiHydratedRepo.ID, nil
}

type linkableRepo struct {
	repo              ghrepo.Interface
	repoIDFetcher     repoIDFetcher
	projectRepoLinker *queries.Client
}

func (lr linkableRepo) LinkWith(project project) error {
	repoID, err := lr.repoIDFetcher.Fetch(lr.repo)
	if err != nil {
		return err
	}

	err = lr.projectRepoLinker.LinkProjectToRepository(project.id, repoID)
	if err != nil {
		return fmt.Errorf("failed to link project to repo: %s", err)
	}

	return nil
}
