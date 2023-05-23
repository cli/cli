package itemdelete

import (
	"fmt"
	"strconv"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type deleteItemOpts struct {
	userOwner string
	orgOwner  string
	number    int32
	itemID    string
	projectID string
	format    string
}

type deleteItemConfig struct {
	tp     *tableprinter.TablePrinter
	client *queries.Client
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
		Use:   "item-delete [<number>]",
		Example: heredoc.Doc(`
			# delete an item in the current user's project "1"
			gh project item-delete 1 --user "@me" --id <item-ID>
		`),
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
			config := deleteItemConfig{
				tp:     t,
				client: client,
				opts:   opts,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runDeleteItem(config)
		},
	}

	deleteItemCmd.Flags().StringVar(&opts.userOwner, "user", "", "Login of the user owner. Use \"@me\" for the current user.")
	deleteItemCmd.Flags().StringVar(&opts.orgOwner, "org", "", "Login of the organization owner")
	deleteItemCmd.Flags().StringVar(&opts.itemID, "id", "", "ID of the item to delete")
	cmdutil.StringEnumFlag(deleteItemCmd, &opts.format, "format", "", "", []string{"json"}, "Output format")

	_ = deleteItemCmd.MarkFlagRequired("id")

	return deleteItemCmd
}

func runDeleteItem(config deleteItemConfig) error {
	owner, err := config.client.NewOwner(config.opts.userOwner, config.opts.orgOwner)
	if err != nil {
		return err
	}

	project, err := config.client.NewProject(owner, config.opts.number, false)
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
