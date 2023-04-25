package itemdelete

import (
	"fmt"
	"strconv"

	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/cli/go-gh/v2/pkg/term"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type deleteItemOpts struct {
	userOwner string
	orgOwner  string
	number    int
	itemID    string
	projectID string
	format    string
}

type deleteItemConfig struct {
	tp     tableprinter.TablePrinter
	client *api.GraphQLClient
	opts   deleteItemOpts
}

type deleteProjectItemMutation struct {
	DeleteProjectItem struct {
		DeletedItemId githubv4.ID `graphql:"deletedItemId"`
	} `graphql:"deleteProjectV2Item(input:$input)"`
}

func NewCmdDeleteItem(f *cmdutil.Factory, runF func(config deleteItemConfig) error) *cobra.Command {
	opts := deleteItemOpts{}
	deleteItemCmd := &cobra.Command{
		Short: "Delete an item from a project by ID",
		Use:   "item-delete [number]",
		Example: `
# delete an item in the current user's project 1
gh projects item-delete 1 --user "@me" --id ID

# delete an item in the monalisa user project 1
gh projects item-delete 1 --user monalisa --id ID

# delete an item in the github org project 1
gh projects item-delete 1 --org github --id ID

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
					return err
				}
			}

			terminal := term.FromEnv()
			termWidth, _, err := terminal.Size()
			if err != nil {
				// set a static width in case of error
				termWidth = 80
			}
			t := tableprinter.New(terminal.Out(), terminal.IsTerminalOutput(), termWidth)

			config := deleteItemConfig{
				tp:     t,
				client: client,
				opts:   opts,
			}
			return runDeleteItem(config)
		},
	}

	deleteItemCmd.Flags().StringVar(&opts.userOwner, "user", "", "Login of the user owner. Use \"@me\" for the current user.")
	deleteItemCmd.Flags().StringVar(&opts.orgOwner, "org", "", "Login of the organization owner.")
	deleteItemCmd.Flags().StringVar(&opts.itemID, "id", "", "Global ID of the item to delete from the project.")
	deleteItemCmd.Flags().StringVar(&opts.format, "format", "", "Output format, must be 'json'.")

	deleteItemCmd.MarkFlagsMutuallyExclusive("user", "org")
	_ = deleteItemCmd.MarkFlagRequired("id")

	return deleteItemCmd
}

func runDeleteItem(config deleteItemConfig) error {
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

	query, variables := deleteItemArgs(config)
	err = config.client.Mutate("DeleteProjectItem", query, variables)
	if err != nil {
		return err
	}

	if config.opts.format == "json" {
		return printJSON(config, query.DeleteProjectItem.DeletedItemId)
	}

	return printResults(config)

}

func deleteItemArgs(config deleteItemConfig) (*deleteProjectItemMutation, map[string]interface{}) {
	return &deleteProjectItemMutation{}, map[string]interface{}{
		"input": githubv4.DeleteProjectV2ItemInput{
			ProjectID: githubv4.ID(config.opts.projectID),
			ItemID:    githubv4.ID(config.opts.itemID),
		},
	}
}

func printResults(config deleteItemConfig) error {
	// using table printer here for consistency in case it ends up being needed in the future
	config.tp.AddField("Deleted item")
	config.tp.EndRow()
	return config.tp.Render()
}

func printJSON(config deleteItemConfig, ID githubv4.ID) error {
	config.tp.AddField(fmt.Sprintf(`{"id": "%s"}`, ID))
	return config.tp.Render()
}
