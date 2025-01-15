package itemarchive

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

type archiveItemOpts struct {
	owner     string
	number    int32
	undo      bool
	itemID    string
	projectID string
	exporter  cmdutil.Exporter
}

type archiveItemConfig struct {
	client *queries.Client
	opts   archiveItemOpts
	io     *iostreams.IOStreams
}

type archiveProjectItemMutation struct {
	ArchiveProjectItem struct {
		ProjectV2Item queries.ProjectItem `graphql:"item"`
	} `graphql:"archiveProjectV2Item(input:$input)"`
}

type unarchiveProjectItemMutation struct {
	UnarchiveProjectItem struct {
		ProjectV2Item queries.ProjectItem `graphql:"item"`
	} `graphql:"unarchiveProjectV2Item(input:$input)"`
}

func NewCmdArchiveItem(f *cmdutil.Factory, runF func(config archiveItemConfig) error) *cobra.Command {
	opts := archiveItemOpts{}
	archiveItemCmd := &cobra.Command{
		Short: "Archive an item in a project",
		Use:   "item-archive [<number>]",
		Example: heredoc.Doc(`
			# archive an item in the current user's project "1"
			gh project item-archive 1 --owner "@me" --id <item-ID>
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

			config := archiveItemConfig{
				client: client,
				opts:   opts,
				io:     f.IOStreams,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runArchiveItem(config)
		},
	}

	archiveItemCmd.Flags().StringVar(&opts.owner, "owner", "", "Login of the owner. Use \"@me\" for the current user.")
	archiveItemCmd.Flags().StringVar(&opts.itemID, "id", "", "ID of the item to archive")
	archiveItemCmd.Flags().BoolVar(&opts.undo, "undo", false, "Unarchive an item")
	cmdutil.AddFormatFlags(archiveItemCmd, &opts.exporter)

	_ = archiveItemCmd.MarkFlagRequired("id")

	return archiveItemCmd
}

func runArchiveItem(config archiveItemConfig) error {
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

	if config.opts.undo {
		query, variables := unarchiveItemArgs(config, config.opts.itemID)
		err = config.client.Mutate("UnarchiveProjectItem", query, variables)
		if err != nil {
			return err
		}

		if config.opts.exporter != nil {
			return config.opts.exporter.Write(config.io, query.UnarchiveProjectItem.ProjectV2Item)
		}

		return printResults(config, query.UnarchiveProjectItem.ProjectV2Item)
	}
	query, variables := archiveItemArgs(config)
	err = config.client.Mutate("ArchiveProjectItem", query, variables)
	if err != nil {
		return err
	}

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, query.ArchiveProjectItem.ProjectV2Item)
	}

	return printResults(config, query.ArchiveProjectItem.ProjectV2Item)
}

func archiveItemArgs(config archiveItemConfig) (*archiveProjectItemMutation, map[string]interface{}) {
	return &archiveProjectItemMutation{}, map[string]interface{}{
		"input": githubv4.ArchiveProjectV2ItemInput{
			ProjectID: githubv4.ID(config.opts.projectID),
			ItemID:    githubv4.ID(config.opts.itemID),
		},
	}
}

func unarchiveItemArgs(config archiveItemConfig, itemID string) (*unarchiveProjectItemMutation, map[string]interface{}) {
	return &unarchiveProjectItemMutation{}, map[string]interface{}{
		"input": githubv4.UnarchiveProjectV2ItemInput{
			ProjectID: githubv4.ID(config.opts.projectID),
			ItemID:    githubv4.ID(itemID),
		},
	}
}

func printResults(config archiveItemConfig, item queries.ProjectItem) error {
	if !config.io.IsStdoutTTY() {
		return nil
	}

	if config.opts.undo {
		_, err := fmt.Fprintf(config.io.Out, "Unarchived item\n")
		return err
	}

	_, err := fmt.Fprintf(config.io.Out, "Archived item\n")
	return err
}
