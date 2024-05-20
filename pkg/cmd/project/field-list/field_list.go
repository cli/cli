package fieldlist

import (
	"fmt"
	"strconv"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/client"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type listOpts struct {
	limit    int
	owner    string
	number   int32
	exporter cmdutil.Exporter
}

type listConfig struct {
	io     *iostreams.IOStreams
	client *queries.Client
	opts   listOpts
}

func NewCmdList(f *cmdutil.Factory, runF func(config listConfig) error) *cobra.Command {
	opts := listOpts{}
	listCmd := &cobra.Command{
		Short: "List the fields in a project",
		Use:   "field-list number",
		Example: heredoc.Doc(`
			# list fields in the current user's project "1"
			gh project field-list 1 --owner "@me"
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

			config := listConfig{
				io:     f.IOStreams,
				client: client,
				opts:   opts,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runList(config)
		},
	}

	listCmd.Flags().StringVar(&opts.owner, "owner", "", "Login of the owner. Use \"@me\" for the current user.")
	cmdutil.AddFormatFlags(listCmd, &opts.exporter)
	listCmd.Flags().IntVarP(&opts.limit, "limit", "L", queries.LimitDefault, "Maximum number of fields to fetch")

	return listCmd
}

func runList(config listConfig) error {
	canPrompt := config.io.CanPrompt()
	owner, err := config.client.NewOwner(canPrompt, config.opts.owner)
	if err != nil {
		return err
	}

	// no need to fetch the project if we already have the number
	if config.opts.number == 0 {
		canPrompt := config.io.CanPrompt()
		project, err := config.client.NewProject(canPrompt, owner, config.opts.number, false)
		if err != nil {
			return err
		}
		config.opts.number = project.Number
	}

	project, err := config.client.ProjectFields(owner, config.opts.number, config.opts.limit)
	if err != nil {
		return err
	}

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, project.Fields)
	}

	return printResults(config, project.Fields.Nodes, owner.Login)
}

func printResults(config listConfig, fields []queries.ProjectField, login string) error {
	if len(fields) == 0 {
		return cmdutil.NewNoResultsError(fmt.Sprintf("Project %d for owner %s has no fields", config.opts.number, login))
	}

	tp := tableprinter.New(config.io, tableprinter.WithHeader("Name", "Data type", "ID"))

	for _, f := range fields {
		tp.AddField(f.Name())
		tp.AddField(f.Type())
		tp.AddField(f.ID(), tableprinter.WithTruncate(nil))
		tp.EndRow()
	}

	return tp.Render()
}
