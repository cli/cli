package itemdelete

import (
	"fmt"
	"strconv"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/client"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type deleteItemOpts struct {
	owner     string
	number    int32
	itemID    string
	projectID string
	exporter  cmdutil.Exporter
}

type deleteItemConfig struct {
	client *queries.Client
	opts   deleteItemOpts
	io     *iostreams.IOStreams
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
			gh project item-delete 1 --owner "@me" --id <item-ID>
		`),
		Args: cobra.MaximumNArgs(1),
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

			config := deleteItemConfig{
				client: client,
				opts:   opts,
				io:     f.IOStreams,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runDeleteItem(config)
		},
	}

	deleteItemCmd.Flags().StringVar(&opts.owner, "owner", "", "Login of the owner. Use \"@me\" for the current user.")
	deleteItemCmd.Flags().StringVar(&opts.itemID, "id", "", "ID of the item to delete")
	cmdutil.AddFormatFlags(deleteItemCmd, &opts.exporter)

	_ = deleteItemCmd.MarkFlagRequired("id")

	return deleteItemCmd
}

func runDeleteItem(config deleteItemConfig) error {
	canPrompt := config.io.CanPrompt()
	owner, err := config.client.NewOwner(canPrompt, config.opts.owner)
	if err != nil {
		return err
	}

	project, err := config.client.NewProject(canPrompt, owner, config.opts.number, false)
	if err != nil {
		return err
	}
	config.opts.projectID = project.ID

	query, variables := deleteItemArgs(config)
	err = config.client.Mutate("DeleteProjectItem", query, variables)
	if err != nil {
		return err
	}

	if config.opts.exporter != nil {
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
	if !config.io.IsStdoutTTY() {
		return nil
	}

	_, err := fmt.Fprintf(config.io.Out, "Deleted item\n")
	return err
}

func printJSON(config deleteItemConfig, id githubv4.ID) error {
	m := map[string]interface{}{
		"id": id,
	}
	return config.opts.exporter.Write(config.io, m)
}
