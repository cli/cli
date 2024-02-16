package marktemplate

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

type markTemplateOpts struct {
	owner     string
	undo      bool
	number    int32
	projectID string
	exporter  cmdutil.Exporter
}

type markTemplateConfig struct {
	client *queries.Client
	opts   markTemplateOpts
	io     *iostreams.IOStreams
}

type markProjectTemplateMutation struct {
	TemplateProject struct {
		Project queries.Project `graphql:"projectV2"`
	} `graphql:"markProjectV2AsTemplate(input:$input)"`
}
type unmarkProjectTemplateMutation struct {
	TemplateProject struct {
		Project queries.Project `graphql:"projectV2"`
	} `graphql:"unmarkProjectV2AsTemplate(input:$input)"`
}

func NewCmdMarkTemplate(f *cmdutil.Factory, runF func(config markTemplateConfig) error) *cobra.Command {
	opts := markTemplateOpts{}
	markTemplateCmd := &cobra.Command{
		Short: "Mark a project as a template",
		Use:   "mark-template [<number>]",
		Example: heredoc.Doc(`
			# mark the github org's project "1" as a template
			gh project mark-template 1 --owner "github"

			# unmark the github org's project "1" as a template
			gh project mark-template 1 --owner "github" --undo
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

			config := markTemplateConfig{
				client: client,
				opts:   opts,
				io:     f.IOStreams,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runMarkTemplate(config)
		},
	}

	markTemplateCmd.Flags().StringVar(&opts.owner, "owner", "", "Login of the org owner.")
	markTemplateCmd.Flags().BoolVar(&opts.undo, "undo", false, "Unmark the project as a template.")
	cmdutil.AddFormatFlags(markTemplateCmd, &opts.exporter)

	return markTemplateCmd
}

func runMarkTemplate(config markTemplateConfig) error {
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
		query, variables := unmarkTemplateArgs(config)
		err = config.client.Mutate("UnmarkProjectTemplate", query, variables)
		if err != nil {
			return err
		}

		if config.opts.exporter != nil {
			return config.opts.exporter.Write(config.io, *project)
		}

		return printResults(config, query.TemplateProject.Project)

	}
	query, variables := markTemplateArgs(config)
	err = config.client.Mutate("MarkProjectTemplate", query, variables)
	if err != nil {
		return err
	}

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, query.TemplateProject.Project)
	}

	return printResults(config, query.TemplateProject.Project)
}

func markTemplateArgs(config markTemplateConfig) (*markProjectTemplateMutation, map[string]interface{}) {
	return &markProjectTemplateMutation{}, map[string]interface{}{
		"input": githubv4.MarkProjectV2AsTemplateInput{
			ProjectID: githubv4.ID(config.opts.projectID),
		},
		"firstItems":  githubv4.Int(0),
		"afterItems":  (*githubv4.String)(nil),
		"firstFields": githubv4.Int(0),
		"afterFields": (*githubv4.String)(nil),
	}
}

func unmarkTemplateArgs(config markTemplateConfig) (*unmarkProjectTemplateMutation, map[string]interface{}) {
	return &unmarkProjectTemplateMutation{}, map[string]interface{}{
		"input": githubv4.UnmarkProjectV2AsTemplateInput{
			ProjectID: githubv4.ID(config.opts.projectID),
		},
		"firstItems":  githubv4.Int(0),
		"afterItems":  (*githubv4.String)(nil),
		"firstFields": githubv4.Int(0),
		"afterFields": (*githubv4.String)(nil),
	}
}

func printResults(config markTemplateConfig, project queries.Project) error {
	if !config.io.IsStdoutTTY() {
		return nil
	}

	if config.opts.undo {
		_, err := fmt.Fprintf(config.io.Out, "Unmarked project %d as a template.\n", project.Number)
		return err
	}

	_, err := fmt.Fprintf(config.io.Out, "Marked project %d as a template.\n", project.Number)
	return err
}
