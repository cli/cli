package create

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/format"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type createOpts struct {
	title     string
	userOwner string
	orgOwner  string
	ownerID   string
	format    string
}

type createConfig struct {
	tp     *tableprinter.TablePrinter
	client *queries.Client
	opts   createOpts
}

type createProjectMutation struct {
	CreateProjectV2 struct {
		ProjectV2 queries.Project `graphql:"projectV2"`
	} `graphql:"createProjectV2(input:$input)"`
}

func NewCmdCreate(f *cmdutil.Factory, runF func(config createConfig) error) *cobra.Command {
	opts := createOpts{}
	createCmd := &cobra.Command{
		Short: "Create a project",
		Use:   "create",
		Example: heredoc.Doc(`
			# create a new project owned by user monalisa
			gh project create --user monalisa --title "a new project"
		`),
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

			t := tableprinter.New(f.IOStreams)
			config := createConfig{
				tp:     t,
				client: client,
				opts:   opts,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runCreate(config)
		},
	}

	createCmd.Flags().StringVar(&opts.title, "title", "", "Title for the project")
	createCmd.Flags().StringVar(&opts.userOwner, "user", "", "Login of the user owner. Use \"@me\" for the current user.")
	createCmd.Flags().StringVar(&opts.orgOwner, "org", "", "Login of the organization owner")
	cmdutil.StringEnumFlag(createCmd, &opts.format, "format", "", "", []string{"json"}, "Output format")

	_ = createCmd.MarkFlagRequired("title")

	return createCmd
}

func runCreate(config createConfig) error {
	owner, err := config.client.NewOwner(config.opts.userOwner, config.opts.orgOwner)
	if err != nil {
		return err
	}

	config.opts.ownerID = owner.ID
	query, variables := createArgs(config)

	err = config.client.Mutate("CreateProjectV2", query, variables)
	if err != nil {
		return err
	}

	if config.opts.format == "json" {
		return printJSON(config, query.CreateProjectV2.ProjectV2)
	}

	return printResults(config, query.CreateProjectV2.ProjectV2)
}

func createArgs(config createConfig) (*createProjectMutation, map[string]interface{}) {
	return &createProjectMutation{}, map[string]interface{}{
		"input": githubv4.CreateProjectV2Input{
			OwnerID: githubv4.ID(config.opts.ownerID),
			Title:   githubv4.String(config.opts.title),
		},
		"firstItems":  githubv4.Int(0),
		"afterItems":  (*githubv4.String)(nil),
		"firstFields": githubv4.Int(0),
		"afterFields": (*githubv4.String)(nil),
	}
}

func printResults(config createConfig, project queries.Project) error {
	// using table printer here for consistency in case it ends up being needed in the future
	config.tp.AddField(fmt.Sprintf("Created project '%s'", project.Title))
	config.tp.EndRow()
	config.tp.AddField(project.URL)
	config.tp.EndRow()
	return config.tp.Render()
}

func printJSON(config createConfig, project queries.Project) error {
	b, err := format.JSONProject(project)
	if err != nil {
		return err
	}
	config.tp.AddField(string(b))
	return config.tp.Render()
}
