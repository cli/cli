package itemadd

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

type addItemOpts struct {
	userOwner string
	orgOwner  string
	number    int
	itemURL   string
	projectID string
	itemID    string
	format    string
}

type addItemConfig struct {
	tp     *tableprinter.TablePrinter
	client *api.GraphQLClient
	opts   addItemOpts
}

type addProjectItemMutation struct {
	CreateProjectItem struct {
		ProjectV2Item queries.ProjectItem `graphql:"item"`
	} `graphql:"addProjectV2ItemById(input:$input)"`
}

func NewCmdAddItem(f *cmdutil.Factory, runF func(config addItemConfig) error) *cobra.Command {
	opts := addItemOpts{}
	addItemCmd := &cobra.Command{
		Short: "Add a pull request or an issue to a project",
		Use:   "item-add [<number>]",
		Example: `
# add an item to the current user's project 1
gh project item-add 1 --user "@me" --url https://github.com/cli/go-gh/issues/1

# add an item to user monalisa's project 1
gh project item-add 1 --user monalisa --url https://github.com/cli/go-gh/issues/1

# add an item to the org github's project 1
gh project item-add 1 --org github --url https://github.com/cli/go-gh/issues/1

# add --format=json to output in JSON format
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
				opts.number, err = strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("invalid number: %v", args[0])
				}
			}

			t := tableprinter.New(f.IOStreams)
			config := addItemConfig{
				tp:     t,
				client: client,
				opts:   opts,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runAddItem(config)
		},
	}

	addItemCmd.Flags().StringVar(&opts.userOwner, "user", "", "Login of the user owner. Use \"@me\" for the current user.")
	addItemCmd.Flags().StringVar(&opts.orgOwner, "org", "", "Login of the organization owner.")
	addItemCmd.Flags().StringVar(&opts.itemURL, "url", "", "URL of the issue or pull request to add to the project. Note that the name of the owner is case sensitive, and will fail to find the item if it does not match. Must be of form https://github.com/OWNER/REPO/issues/NUMBER or https://github.com/OWNER/REPO/pull/NUMBER")
	addItemCmd.Flags().StringVar(&opts.format, "format", "", "Output format, must be 'json'.")

	_ = addItemCmd.MarkFlagRequired("url")

	return addItemCmd
}

func runAddItem(config addItemConfig) error {
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

	itemID, err := queries.IssueOrPullRequestID(config.client, config.opts.itemURL)
	if err != nil {
		return err
	}

	config.opts.itemID = itemID

	query, variables := addItemArgs(config)
	err = config.client.Mutate("AddItem", query, variables)
	if err != nil {
		return err
	}

	if config.opts.format == "json" {
		return printJSON(config, query.CreateProjectItem.ProjectV2Item)
	}

	return printResults(config, query.CreateProjectItem.ProjectV2Item)

}

func addItemArgs(config addItemConfig) (*addProjectItemMutation, map[string]interface{}) {
	return &addProjectItemMutation{}, map[string]interface{}{
		"input": githubv4.AddProjectV2ItemByIdInput{
			ProjectID: githubv4.ID(config.opts.projectID),
			ContentID: githubv4.ID(config.opts.itemID),
		},
	}
}

func printResults(config addItemConfig, item queries.ProjectItem) error {
	// using table printer here for consistency in case it ends up being needed in the future
	config.tp.AddField("Added item")
	config.tp.EndRow()
	return config.tp.Render()
}

func printJSON(config addItemConfig, item queries.ProjectItem) error {
	b, err := format.JSONProjectItem(item)
	if err != nil {
		return err
	}
	config.tp.AddField(string(b))
	return config.tp.Render()
}
