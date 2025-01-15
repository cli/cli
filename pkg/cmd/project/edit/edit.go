package edit

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

type editOpts struct {
	number           int32
	owner            string
	title            string
	readme           string
	visibility       string
	shortDescription string
	projectID        string
	exporter         cmdutil.Exporter
}

type editConfig struct {
	client *queries.Client
	opts   editOpts
	io     *iostreams.IOStreams
}

type updateProjectMutation struct {
	UpdateProjectV2 struct {
		ProjectV2 queries.Project `graphql:"projectV2"`
	} `graphql:"updateProjectV2(input:$input)"`
}

const projectVisibilityPublic = "PUBLIC"
const projectVisibilityPrivate = "PRIVATE"

func NewCmdEdit(f *cmdutil.Factory, runF func(config editConfig) error) *cobra.Command {
	opts := editOpts{}
	editCmd := &cobra.Command{
		Short: "Edit a project",
		Use:   "edit [<number>]",
		Example: heredoc.Doc(`
			# edit the title of monalisa's project "1"
			gh project edit 1 --owner monalisa --title "New title"
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

			config := editConfig{
				client: client,
				opts:   opts,
				io:     f.IOStreams,
			}

			if config.opts.title == "" && config.opts.shortDescription == "" && config.opts.readme == "" && config.opts.visibility == "" {
				return fmt.Errorf("no fields to edit")
			}
			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runEdit(config)
		},
	}

	editCmd.Flags().StringVar(&opts.owner, "owner", "", "Login of the owner. Use \"@me\" for the current user.")
	cmdutil.StringEnumFlag(editCmd, &opts.visibility, "visibility", "", "", []string{projectVisibilityPublic, projectVisibilityPrivate}, "Change project visibility")
	editCmd.Flags().StringVar(&opts.title, "title", "", "New title for the project")
	editCmd.Flags().StringVar(&opts.readme, "readme", "", "New readme for the project")
	editCmd.Flags().StringVarP(&opts.shortDescription, "description", "d", "", "New description of the project")
	cmdutil.AddFormatFlags(editCmd, &opts.exporter)

	return editCmd
}

func runEdit(config editConfig) error {
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

	query, variables := editArgs(config)
	err = config.client.Mutate("UpdateProjectV2", query, variables)
	if err != nil {
		return err
	}

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, query.UpdateProjectV2.ProjectV2)
	}

	return printResults(config, query.UpdateProjectV2.ProjectV2)
}

func editArgs(config editConfig) (*updateProjectMutation, map[string]interface{}) {
	variables := githubv4.UpdateProjectV2Input{ProjectID: githubv4.ID(config.opts.projectID)}
	if config.opts.title != "" {
		variables.Title = githubv4.NewString(githubv4.String(config.opts.title))
	}
	if config.opts.shortDescription != "" {
		variables.ShortDescription = githubv4.NewString(githubv4.String(config.opts.shortDescription))
	}
	if config.opts.readme != "" {
		variables.Readme = githubv4.NewString(githubv4.String(config.opts.readme))
	}
	if config.opts.visibility != "" {
		if config.opts.visibility == projectVisibilityPublic {
			variables.Public = githubv4.NewBoolean(githubv4.Boolean(true))
		} else if config.opts.visibility == projectVisibilityPrivate {
			variables.Public = githubv4.NewBoolean(githubv4.Boolean(false))
		}
	}

	return &updateProjectMutation{}, map[string]interface{}{
		"input":       variables,
		"firstItems":  githubv4.Int(0),
		"afterItems":  (*githubv4.String)(nil),
		"firstFields": githubv4.Int(0),
		"afterFields": (*githubv4.String)(nil),
	}
}

func printResults(config editConfig, project queries.Project) error {
	if !config.io.IsStdoutTTY() {
		return nil
	}

	_, err := fmt.Fprintf(config.io.Out, "%s\n", project.URL)
	return err
}
