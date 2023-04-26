package itemcreate

import (
	"fmt"
	"strconv"

	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/cli/cli/v2/pkg/cmd/project/shared/format"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"

	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type createItemOpts struct {
	title     string
	body      string
	userOwner string
	orgOwner  string
	number    int
	projectID string
	format    string
}

type createItemConfig struct {
	tp     *tableprinter.TablePrinter
	client *api.GraphQLClient
	opts   createItemOpts
}

type createProjectDraftItemMutation struct {
	CreateProjectDraftItem struct {
		ProjectV2Item queries.ProjectItem `graphql:"projectItem"`
	} `graphql:"addProjectV2DraftIssue(input:$input)"`
}

func NewCmdCreateItem(f *cmdutil.Factory, runF func(config createItemConfig) error) *cobra.Command {
	opts := createItemOpts{}
	createItemCmd := &cobra.Command{
		Short: "Create a draft issue item in a project",
		Use:   "item-create [number]",
		Example: `
# create a draft issue in the current user's project 1 with title "new item" and body "new item body"
gh project item-create 1 --user "@me" --title "new item" --body "new item body"

# create a draft issue in user monalisa's project 1 with title "new item" and body "new item body"
gh project item-create 1 --user monalisa --title "new item" --body "new item body"

# create a draft issue in org github's project 1 with title "new item" and body "new item body"
gh project item-create 1 --org github --title "new item" --body "new item body"

# add --format=json to output in JSON format
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := queries.NewClient()
			if err != nil {
				return err
			}

			if len(args) == 1 {
				opts.number, err = strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("invalid number: %v", args[0])
				}
			}

			t := tableprinter.New(f.IOStreams)
			config := createItemConfig{
				tp:     t,
				client: client,
				opts:   opts,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runCreateItem(config)
		},
	}

	createItemCmd.Flags().StringVar(&opts.userOwner, "user", "", "Login of the user owner. Use \"@me\" for the current user.")
	createItemCmd.Flags().StringVar(&opts.orgOwner, "org", "", "Login of the organization owner.")
	createItemCmd.Flags().StringVar(&opts.title, "title", "", "Title of the draft issue item.")
	createItemCmd.Flags().StringVar(&opts.body, "body", "", "Body of the draft issue item.")
	createItemCmd.Flags().StringVar(&opts.format, "format", "", "Output format, must be 'json'.")

	createItemCmd.MarkFlagsMutuallyExclusive("user", "org")
	_ = createItemCmd.MarkFlagRequired("title")

	return createItemCmd
}

func runCreateItem(config createItemConfig) error {
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

	query, variables := createDraftIssueArgs(config)

	err = config.client.Mutate("CreateDraftItem", query, variables)
	if err != nil {
		return err
	}

	if config.opts.format == "json" {
		return printJSON(config, query.CreateProjectDraftItem.ProjectV2Item)
	}

	return printResults(config, query.CreateProjectDraftItem.ProjectV2Item)
}

func createDraftIssueArgs(config createItemConfig) (*createProjectDraftItemMutation, map[string]interface{}) {
	return &createProjectDraftItemMutation{}, map[string]interface{}{
		"input": githubv4.AddProjectV2DraftIssueInput{
			Body:      githubv4.NewString(githubv4.String(config.opts.body)),
			ProjectID: githubv4.ID(config.opts.projectID),
			Title:     githubv4.String(config.opts.title),
		},
	}
}

func printResults(config createItemConfig, item queries.ProjectItem) error {
	// using table printer here for consistency in case it ends up being needed in the future
	config.tp.AddField("Created item")
	config.tp.EndRow()
	return config.tp.Render()
}

func printJSON(config createItemConfig, item queries.ProjectItem) error {
	b, err := format.JSONProjectItem(item)
	if err != nil {
		return err
	}
	config.tp.AddField(string(b))
	return config.tp.Render()
}
