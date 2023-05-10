package itemarchive

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

type archiveItemOpts struct {
	userOwner string
	orgOwner  string
	number    int
	undo      bool
	itemID    string
	projectID string
	format    string
}

type archiveItemConfig struct {
	tp     *tableprinter.TablePrinter
	client *api.GraphQLClient
	opts   archiveItemOpts
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
		Example: `
# archive an item in the current user's project 1
gh project item-archive 1 --user "@me" --id ID

# archive an item in the monalisa user's project 1
gh project item-archive 1 --user monalisa --id ID

# archive an item in the org github's project 1
gh project item-archive 1 --org github --id ID

# unarchive an item
gh project item-archive 1 --user "@me" --id ID --undo

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
			config := archiveItemConfig{
				tp:     t,
				client: client,
				opts:   opts,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runArchiveItem(config)
		},
	}

	archiveItemCmd.Flags().StringVar(&opts.userOwner, "user", "", "Login of the user owner. Use \"@me\" for the current user.")
	archiveItemCmd.Flags().StringVar(&opts.orgOwner, "org", "", "Login of the organization owner.")
	archiveItemCmd.Flags().StringVar(&opts.itemID, "id", "", "Global ID of the item to archive from the project.")
	archiveItemCmd.Flags().BoolVar(&opts.undo, "undo", false, "Undo archive (unarchive) of an item.")
	archiveItemCmd.Flags().StringVar(&opts.format, "format", "", "Output format, must be 'json'.")

	archiveItemCmd.MarkFlagsMutuallyExclusive("user", "org")
	_ = archiveItemCmd.MarkFlagRequired("id")

	return archiveItemCmd
}

func runArchiveItem(config archiveItemConfig) error {
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

	if config.opts.undo {
		query, variables := unarchiveItemArgs(config, config.opts.itemID)
		err = config.client.Mutate("UnarchiveProjectItem", query, variables)
		if err != nil {
			return err
		}

		if config.opts.format == "json" {
			return printJSON(config, query.UnarchiveProjectItem.ProjectV2Item)
		}

		return printResults(config, query.UnarchiveProjectItem.ProjectV2Item)
	}
	query, variables := archiveItemArgs(config)
	err = config.client.Mutate("ArchiveProjectItem", query, variables)
	if err != nil {
		return err
	}

	if config.opts.format == "json" {
		return printJSON(config, query.ArchiveProjectItem.ProjectV2Item)
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
	// using table printer here for consistency in case it ends up being needed in the future
	if config.opts.undo {
		config.tp.AddField("Unarchived item")
	} else {
		config.tp.AddField("Archived item")
	}
	config.tp.EndRow()
	return config.tp.Render()
}

func printJSON(config archiveItemConfig, item queries.ProjectItem) error {
	b, err := format.JSONProjectItem(item)
	if err != nil {
		return err
	}
	config.tp.AddField(string(b))
	return config.tp.Render()
}
