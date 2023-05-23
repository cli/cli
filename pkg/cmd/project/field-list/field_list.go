package fieldlist

import (
	"fmt"
	"strconv"

	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/format"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/spf13/cobra"
)

type listOpts struct {
	limit     int
	userOwner string
	orgOwner  string
	number    int32
	format    string
}

type listConfig struct {
	tp     *tableprinter.TablePrinter
	client *api.GraphQLClient
	opts   listOpts
}

func NewCmdList(f *cmdutil.Factory, runF func(config listConfig) error) *cobra.Command {
	opts := listOpts{}
	listCmd := &cobra.Command{
		Short: "List the fields in a project",
		Use:   "field-list number",
		Example: `
# list the fields in the current user's project number 1
gh project field-list 1 --user "@me"

# list the fields in user monalisa's project number 1
gh project field-list 1 --user monalisa

# list the first 30 fields in org github's project number 1
gh project field-list 1 --org github --limit 30
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmdutil.MutuallyExclusive(
				"only one of `--user` or `--org` may be used",
				opts.userOwner != "",
				opts.orgOwner != "",
			); err != nil {
				return err
			}

			client, err := queries.NewClient()
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

			t := tableprinter.New(f.IOStreams)
			config := listConfig{
				tp:     t,
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

	listCmd.Flags().StringVar(&opts.userOwner, "user", "", "Login of the user owner. Use \"@me\" for the current user.")
	listCmd.Flags().StringVar(&opts.orgOwner, "org", "", "Login of the organization owner")
	cmdutil.StringEnumFlag(listCmd, &opts.format, "format", "", "", []string{"json"}, "Output format")
	listCmd.Flags().IntVarP(&opts.limit, "limit", "L", 30, "Maximum number of fields to fetch")

	return listCmd
}

func runList(config listConfig) error {
	owner, err := queries.NewOwner(config.client, config.opts.userOwner, config.opts.orgOwner)
	if err != nil {
		return err
	}

	// no need to fetch the project if we already have the number
	if config.opts.number == 0 {
		project, err := queries.NewProject(config.client, owner, config.opts.number, false)
		if err != nil {
			return err
		}
		config.opts.number = project.Number
	}

	project, err := queries.ProjectFields(config.client, owner, config.opts.number, config.opts.limit)
	if err != nil {
		return err
	}

	if config.opts.format == "json" {
		return printJSON(config, project)
	}

	return printResults(config, project.Fields.Nodes, owner.Login)
}

func printResults(config listConfig, fields []queries.ProjectField, login string) error {
	if len(fields) == 0 {
		return cmdutil.NewNoResultsError(fmt.Sprintf("Project %d for login %s has no fields", config.opts.number, login))
	}

	config.tp.AddField("Name")
	config.tp.AddField("DataType")
	config.tp.AddField("ID")
	config.tp.EndRow()

	for _, f := range fields {
		config.tp.AddField(f.Name())
		config.tp.AddField(f.Type())
		config.tp.AddField(f.ID())
		config.tp.EndRow()
	}

	return config.tp.Render()
}

func printJSON(config listConfig, project *queries.Project) error {
	b, err := format.JSONProjectFields(project)
	if err != nil {
		return err
	}
	config.tp.AddField(string(b))

	return config.tp.Render()
}
