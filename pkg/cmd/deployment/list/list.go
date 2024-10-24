package list

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type listOptions struct {
	BaseRepo   func() (ghrepo.Interface, error)
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Now        time.Time

	Limit       int
	Environment string
}

type deployment struct {
	ID          int
	Ref         *string
	Environment string
	Status      *string
	CreatedAt   time.Time
}

type deploymentsQuery struct {
	Repository struct {
		Deployments struct {
			Nodes []struct {
				DatabaseID  int
				Environment string
				Ref         *struct {
					Name string
				}
				LatestStatus *struct {
					State     githubv4.DeploymentStatusState
					CreatedAt time.Time
				}
				CreatedAt time.Time
			}
			PageInfo struct {
				HasNextPage bool
				EndCursor   string
			}
		} `graphql:"deployments(first: $per_page, environments: $environments, after: $endCursor, orderBy: {field: CREATED_AT, direction: DESC})"`
	} `graphql:"repository(owner: $owner, name: $name)"`
}

func NewCmdList(f *cmdutil.Factory, runF func(*listOptions) error) *cobra.Command {
	opts := listOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List deployments",
		Long: heredoc.Docf(`
			List deployments.
		`, "`"),
		Example: heredoc.Doc(`
			# List deployments in the current repository
			$ gh deployment list

			# List deployments in a specific repository
			$ gh deployment list --repo owner/repo

			# List deployments in a specific environment
			$ gh deployment list --environment production

			# List more than 10 deployments
			$ gh deployment list --limit 100
		`),
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo

			if runF != nil {
				return runF(&opts)
			}
			return listRun(&opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 10, "Maximum number of deployments to fetch")
	cmd.Flags().StringVarP(&opts.Environment, "environment", "e", "", "Filter deployments by environment")

	return cmd
}

func listRun(opts *listOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not build http client: %w", err)
	}
	client := api.NewClientFromHTTP(httpClient)

	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicator()
	deployments, err := listDeployments(client, repo, opts.Limit, opts.Environment)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("could not list deployments: %w", err)
	}

	if len(deployments) == 0 {
		return cmdutil.NewNoResultsError("no deployments found")
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	cs := opts.IO.ColorScheme()
	tp := tableprinter.New(opts.IO, tableprinter.WithHeader("ID", "ENVIRONMENT", "STATUS", "REF", "CREATED"))

	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}

	for _, deployment := range deployments {
		tp.AddField(strconv.Itoa(deployment.ID))
		tp.AddField(deployment.Environment, tableprinter.WithColor(cs.Bold))

		status := "none"
		if deployment.Status != nil {
			status = *deployment.Status
		}
		switch status {
		case "error", "failure":
			tp.AddField(status, tableprinter.WithColor(cs.Red))
		case "queued", "pending":
			tp.AddField(status, tableprinter.WithColor(cs.Yellow))
		case "in_progress":
			tp.AddField(status, tableprinter.WithColor(cs.Blue))
		case "success":
			tp.AddField(status, tableprinter.WithColor(cs.Green))
		default:
			tp.AddField(status, tableprinter.WithColor(cs.Gray))
		}

		ref := "none"
		if deployment.Ref != nil {
			ref = *deployment.Ref
		}
		tp.AddField(ref)

		tp.AddTimeField(opts.Now, deployment.CreatedAt, cs.Gray)
		tp.EndRow()
	}

	return tp.Render()
}

func listDeployments(client *api.Client, repo ghrepo.Interface, limit int, environment string) ([]deployment, error) {
	perPage := limit
	if perPage > 100 {
		perPage = 100
	}

	variables := map[string]interface{}{
		"owner":        githubv4.String(repo.RepoOwner()),
		"name":         githubv4.String(repo.RepoName()),
		"per_page":     githubv4.Int(perPage),
		"environments": (*[]githubv4.String)(nil),
		"endCursor":    (*githubv4.String)(nil),
	}

	if environment != "" {
		variables["environments"] = []githubv4.String{githubv4.String(environment)}
	}

	deployments := []deployment{}

pagination:
	for {
		var result deploymentsQuery
		err := client.Query(repo.RepoHost(), "RepositoryDeployments", &result, variables)
		if err != nil {
			return nil, err
		}

		for _, node := range result.Repository.Deployments.Nodes {
			deployment := deployment{
				ID:          node.DatabaseID,
				Environment: node.Environment,
				CreatedAt:   node.CreatedAt,
			}

			if node.Ref != nil {
				ref := node.Ref.Name
				deployment.Ref = &ref
			}

			if node.LatestStatus != nil {
				state := strings.ToLower(string(node.LatestStatus.State))
				deployment.Status = &state
			}

			deployments = append(deployments, deployment)

			if len(deployments) == limit {
				break pagination
			}
		}

		if !result.Repository.Deployments.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(result.Repository.Deployments.PageInfo.EndCursor)
	}

	return deployments, nil
}
