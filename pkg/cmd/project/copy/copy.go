package copy

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

type copyOpts struct {
	includeDraftIssues bool
	number             int32
	ownerID            string
	projectID          string
	sourceOwner        string
	targetOwner        string
	title              string
	exporter           cmdutil.Exporter
}

type copyConfig struct {
	io     *iostreams.IOStreams
	client *queries.Client
	opts   copyOpts
}

type copyProjectMutation struct {
	CopyProjectV2 struct {
		ProjectV2 queries.Project `graphql:"projectV2"`
	} `graphql:"copyProjectV2(input:$input)"`
}

func NewCmdCopy(f *cmdutil.Factory, runF func(config copyConfig) error) *cobra.Command {
	opts := copyOpts{}
	copyCmd := &cobra.Command{
		Short: "Copy a project",
		Use:   "copy [<number>]",
		Example: heredoc.Doc(`
			# copy project "1" owned by monalisa to github
			gh project copy 1 --source-owner monalisa --target-owner github --title "a new project"
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

			config := copyConfig{
				io:     f.IOStreams,
				client: client,
				opts:   opts,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runCopy(config)
		},
	}

	copyCmd.Flags().StringVar(&opts.sourceOwner, "source-owner", "", "Login of the source owner. Use \"@me\" for the current user.")
	copyCmd.Flags().StringVar(&opts.targetOwner, "target-owner", "", "Login of the target owner. Use \"@me\" for the current user.")
	copyCmd.Flags().StringVar(&opts.title, "title", "", "Title for the new project")
	copyCmd.Flags().BoolVar(&opts.includeDraftIssues, "drafts", false, "Include draft issues when copying")
	cmdutil.AddFormatFlags(copyCmd, &opts.exporter)

	_ = copyCmd.MarkFlagRequired("title")

	return copyCmd
}

func runCopy(config copyConfig) error {
	canPrompt := config.io.CanPrompt()
	sourceOwner, err := config.client.NewOwner(canPrompt, config.opts.sourceOwner)
	if err != nil {
		return err
	}

	targetOwner, err := config.client.NewOwner(canPrompt, config.opts.targetOwner)
	if err != nil {
		return err
	}

	project, err := config.client.NewProject(canPrompt, sourceOwner, config.opts.number, false)
	if err != nil {
		return err
	}

	config.opts.projectID = project.ID
	config.opts.ownerID = targetOwner.ID

	query, variables := copyArgs(config)

	err = config.client.Mutate("CopyProjectV2", query, variables)
	if err != nil {
		return err
	}

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, query.CopyProjectV2.ProjectV2)
	}

	return printResults(config, query.CopyProjectV2.ProjectV2)
}

func copyArgs(config copyConfig) (*copyProjectMutation, map[string]interface{}) {
	return &copyProjectMutation{}, map[string]interface{}{
		"input": githubv4.CopyProjectV2Input{
			OwnerID:            githubv4.ID(config.opts.ownerID),
			ProjectID:          githubv4.ID(config.opts.projectID),
			Title:              githubv4.String(config.opts.title),
			IncludeDraftIssues: githubv4.NewBoolean(githubv4.Boolean(config.opts.includeDraftIssues)),
		},
		"firstItems":  githubv4.Int(0),
		"afterItems":  (*githubv4.String)(nil),
		"firstFields": githubv4.Int(0),
		"afterFields": (*githubv4.String)(nil),
	}
}

func printResults(config copyConfig, project queries.Project) error {
	if !config.io.IsStdoutTTY() {
		return nil
	}
	_, err := fmt.Fprintf(config.io.Out, "%s\n", project.URL)
	return err
}
