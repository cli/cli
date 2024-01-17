package itemcreate

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

type createItemOpts struct {
	title     string
	body      string
	owner     string
	number    int32
	projectID string
	exporter  cmdutil.Exporter
}

type createItemConfig struct {
	client *queries.Client
	opts   createItemOpts
	io     *iostreams.IOStreams
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
		Use:   "item-create [<number>]",
		Example: heredoc.Doc(`
			# create a draft issue in the current user's project "1"
			gh project item-create 1 --owner "@me" --title "new item" --body "new item body"
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

			config := createItemConfig{
				client: client,
				opts:   opts,
				io:     f.IOStreams,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runCreateItem(config)
		},
	}

	createItemCmd.Flags().StringVar(&opts.owner, "owner", "", "Login of the owner. Use \"@me\" for the current user.")
	createItemCmd.Flags().StringVar(&opts.title, "title", "", "Title for the draft issue")
	createItemCmd.Flags().StringVar(&opts.body, "body", "", "Body for the draft issue")
	cmdutil.AddFormatFlags(createItemCmd, &opts.exporter)

	_ = createItemCmd.MarkFlagRequired("title")

	return createItemCmd
}

func runCreateItem(config createItemConfig) error {
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

	query, variables := createDraftIssueArgs(config)

	err = config.client.Mutate("CreateDraftItem", query, variables)
	if err != nil {
		return err
	}

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, query.CreateProjectDraftItem.ProjectV2Item)
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
	if !config.io.IsStdoutTTY() {
		return nil
	}

	_, err := fmt.Fprintf(config.io.Out, "Created item\n")
	return err
}
