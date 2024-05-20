package close

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

type closeOpts struct {
	number    int32
	owner     string
	reopen    bool
	projectID string
	exporter  cmdutil.Exporter
}

type closeConfig struct {
	io     *iostreams.IOStreams
	client *queries.Client
	opts   closeOpts
}

// the close command relies on the updateProjectV2 mutation
type updateProjectMutation struct {
	UpdateProjectV2 struct {
		ProjectV2 queries.Project `graphql:"projectV2"`
	} `graphql:"updateProjectV2(input:$input)"`
}

func NewCmdClose(f *cmdutil.Factory, runF func(config closeConfig) error) *cobra.Command {
	opts := closeOpts{}
	closeCmd := &cobra.Command{
		Short: "Close a project",
		Use:   "close [<number>]",
		Example: heredoc.Doc(`
			# close project "1" owned by monalisa
			gh project close 1 --owner monalisa

			# reopen closed project "1" owned by github
			gh project close 1 --owner github --undo
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

			config := closeConfig{
				io:     f.IOStreams,
				client: client,
				opts:   opts,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runClose(config)
		},
	}

	closeCmd.Flags().StringVar(&opts.owner, "owner", "", "Login of the owner. Use \"@me\" for the current user.")
	closeCmd.Flags().BoolVar(&opts.reopen, "undo", false, "Reopen a closed project")
	cmdutil.AddFormatFlags(closeCmd, &opts.exporter)

	return closeCmd
}

func runClose(config closeConfig) error {
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

	query, variables := closeArgs(config)

	err = config.client.Mutate("CloseProjectV2", query, variables)
	if err != nil {
		return err
	}

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, query.UpdateProjectV2.ProjectV2)
	}

	return printResults(config, query.UpdateProjectV2.ProjectV2)
}

func closeArgs(config closeConfig) (*updateProjectMutation, map[string]interface{}) {
	closed := !config.opts.reopen
	return &updateProjectMutation{}, map[string]interface{}{
		"input": githubv4.UpdateProjectV2Input{
			ProjectID: githubv4.ID(config.opts.projectID),
			Closed:    githubv4.NewBoolean(githubv4.Boolean(closed)),
		},
		"firstItems":  githubv4.Int(0),
		"afterItems":  (*githubv4.String)(nil),
		"firstFields": githubv4.Int(0),
		"afterFields": (*githubv4.String)(nil),
	}
}

func printResults(config closeConfig, project queries.Project) error {
	if !config.io.IsStdoutTTY() {
		return nil
	}

	_, err := fmt.Fprintf(config.io.Out, "%s\n", project.URL)
	return err
}
