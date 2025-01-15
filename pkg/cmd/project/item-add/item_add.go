package itemadd

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

type addItemOpts struct {
	owner     string
	number    int32
	itemURL   string
	projectID string
	itemID    string
	exporter  cmdutil.Exporter
}

type addItemConfig struct {
	client *queries.Client
	opts   addItemOpts
	io     *iostreams.IOStreams
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
		Example: heredoc.Doc(`
			# add an item to monalisa's project "1"
			gh project item-add 1 --owner monalisa --url https://github.com/monalisa/myproject/issues/23
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

			config := addItemConfig{
				client: client,
				opts:   opts,
				io:     f.IOStreams,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runAddItem(config)
		},
	}

	addItemCmd.Flags().StringVar(&opts.owner, "owner", "", "Login of the owner. Use \"@me\" for the current user.")
	addItemCmd.Flags().StringVar(&opts.itemURL, "url", "", "URL of the issue or pull request to add to the project")
	cmdutil.AddFormatFlags(addItemCmd, &opts.exporter)

	_ = addItemCmd.MarkFlagRequired("url")

	return addItemCmd
}

func runAddItem(config addItemConfig) error {
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

	itemID, err := config.client.IssueOrPullRequestID(config.opts.itemURL)
	if err != nil {
		return err
	}
	config.opts.itemID = itemID

	query, variables := addItemArgs(config)
	err = config.client.Mutate("AddItem", query, variables)
	if err != nil {
		return err
	}

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, query.CreateProjectItem.ProjectV2Item)
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
	if !config.io.IsStdoutTTY() {
		return nil
	}

	_, err := fmt.Fprintf(config.io.Out, "Added item\n")
	return err
}
