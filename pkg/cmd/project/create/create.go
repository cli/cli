package create

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/client"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type createOpts struct {
	title    string
	owner    string
	ownerID  string
	exporter cmdutil.Exporter
}

type createConfig struct {
	client *queries.Client
	opts   createOpts
	io     *iostreams.IOStreams
}

type createProjectMutation struct {
	CreateProjectV2 struct {
		ProjectV2 queries.Project `graphql:"projectV2"`
	} `graphql:"createProjectV2(input:$input)"`
}

func NewCmdCreate(f *cmdutil.Factory, runF func(config createConfig) error) *cobra.Command {
	opts := createOpts{}
	createCmd := &cobra.Command{
		Short: "Create a project",
		Use:   "create",
		Example: heredoc.Doc(`
			# create a new project owned by login monalisa
			gh project create --owner monalisa --title "a new project"
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client.New(f)
			if err != nil {
				return err
			}

			config := createConfig{
				client: client,
				opts:   opts,
				io:     f.IOStreams,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runCreate(config)
		},
	}

	createCmd.Flags().StringVar(&opts.title, "title", "", "Title for the project")
	createCmd.Flags().StringVar(&opts.owner, "owner", "", "Login of the owner. Use \"@me\" for the current user.")
	cmdutil.AddFormatFlags(createCmd, &opts.exporter)

	_ = createCmd.MarkFlagRequired("title")

	return createCmd
}

func runCreate(config createConfig) error {
	canPrompt := config.io.CanPrompt()
	owner, err := config.client.NewOwner(canPrompt, config.opts.owner)
	if err != nil {
		return err
	}

	config.opts.ownerID = owner.ID
	query, variables := createArgs(config)

	err = config.client.Mutate("CreateProjectV2", query, variables)
	if err != nil {
		return err
	}

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, query.CreateProjectV2.ProjectV2)
	}

	return printResults(config, query.CreateProjectV2.ProjectV2)
}

func createArgs(config createConfig) (*createProjectMutation, map[string]interface{}) {
	return &createProjectMutation{}, map[string]interface{}{
		"input": githubv4.CreateProjectV2Input{
			OwnerID: githubv4.ID(config.opts.ownerID),
			Title:   githubv4.String(config.opts.title),
		},
		"firstItems":  githubv4.Int(0),
		"afterItems":  (*githubv4.String)(nil),
		"firstFields": githubv4.Int(0),
		"afterFields": (*githubv4.String)(nil),
	}
}

func printResults(config createConfig, project queries.Project) error {
	if !config.io.IsStdoutTTY() {
		return nil
	}

	_, err := fmt.Fprintf(config.io.Out, "%s\n", project.URL)
	return err
}
