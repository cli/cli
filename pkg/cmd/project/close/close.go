package close

import (
	"fmt"
	"strconv"

	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/format"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type closeOpts struct {
	number    int32
	userOwner string
	orgOwner  string
	reopen    bool
	projectID string
	format    string
}

type closeConfig struct {
	tp     *tableprinter.TablePrinter
	client *api.GraphQLClient
	opts   closeOpts
}

// the close command relies on the updateProjectV2 mutation
type updateProjectMutation struct {
	UpdateProjectV2 struct {
		ProjectV2 queries.Project `graphql:"projectV2"`
	} `graphql:"updateProjectV2(input:$input)"`
}

func NewCmdClose(f *cmdutil.Factory, runF func(config closeConfig) error) *cobra.Command {
	opts := closeOpts{}
	closeCmd := &cobra.Command{
		Short: "Close a project",
		Use:   "close [<number>]",
		Example: `
# close project 1 owned by user monalisa
gh project close 1 --user monalisa

# close project 1 owned by org github
gh project close 1 --org github

# reopen closed project 1 owned by org github
gh project close 1 --org github --undo
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmdutil.MutuallyExclusive(
				"only one of `--user` or `--org` may be used",
				opts.userOwner != "",
				opts.orgOwner != "",
			); err != nil {
				return err
			}

			client, err := queries.NewClient()
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

			t := tableprinter.New(f.IOStreams)
			config := closeConfig{
				tp:     t,
				client: client,
				opts:   opts,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runClose(config)
		},
	}

	closeCmd.Flags().StringVar(&opts.userOwner, "user", "", "Login of the user owner. Use \"@me\" for the current user.")
	closeCmd.Flags().StringVar(&opts.orgOwner, "org", "", "Login of the organization owner")
	closeCmd.Flags().BoolVar(&opts.reopen, "undo", false, "Reopen a closed project")
	closeCmd.Flags().StringVar(&opts.format, "format", "", "Output format, must be 'json'")

	return closeCmd
}

func runClose(config closeConfig) error {
	if config.opts.format != "" && config.opts.format != "json" {
		return fmt.Errorf("format must be 'json'")
	}

	owner, err := queries.NewOwner(config.client, config.opts.userOwner, config.opts.orgOwner)
	if err != nil {
		return err
	}

	project, err := queries.NewProject(config.client, owner, config.opts.number, false)
	if err != nil {
		return err
	}
	config.opts.projectID = project.ID

	query, variables := closeArgs(config)

	err = config.client.Mutate("CloseProjectV2", query, variables)
	if err != nil {
		return err
	}

	if config.opts.format == "json" {
		return printJSON(config, *project)
	}

	return printResults(config, query.UpdateProjectV2.ProjectV2)
}

func closeArgs(config closeConfig) (*updateProjectMutation, map[string]interface{}) {
	closed := !config.opts.reopen
	return &updateProjectMutation{}, map[string]interface{}{
		"input": githubv4.UpdateProjectV2Input{
			ProjectID: githubv4.ID(config.opts.projectID),
			Closed:    githubv4.NewBoolean(githubv4.Boolean(closed)),
		},
		"firstItems":  githubv4.Int(0),
		"afterItems":  (*githubv4.String)(nil),
		"firstFields": githubv4.Int(0),
		"afterFields": (*githubv4.String)(nil),
	}
}

func printResults(config closeConfig, project queries.Project) error {
	// using table printer here for consistency in case it ends up being needed in the future
	var action string
	if config.opts.reopen {
		action = "Reopened"
	} else {
		action = "Closed"
	}
	config.tp.AddField(fmt.Sprintf("%s project %s", action, project.URL))
	config.tp.EndRow()
	return config.tp.Render()
}

func printJSON(config closeConfig, project queries.Project) error {
	b, err := format.JSONProject(project)
	if err != nil {
		return err
	}
	config.tp.AddField(string(b))
	return config.tp.Render()
}
