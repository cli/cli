package fieldlist

import (
	"fmt"
	"strconv"

	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/cli/cli/v2/pkg/cmd/project/shared/format"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/cli/go-gh/v2/pkg/term"
	"github.com/spf13/cobra"
)

type listOpts struct {
	limit     string
	userOwner string
	orgOwner  string
	number    int
	format    string
}

type listConfig struct {
	tp     tableprinter.TablePrinter
	client *api.GraphQLClient
	opts   listOpts
}

func parseLimit(limit string) (int, error) {
	if limit == "" {
		return queries.LimitMax, nil
	} else if limit == "all" {
		return 0, nil
	}

	v, err := strconv.Atoi(limit)
	if err != nil {
		return 0, fmt.Errorf("invalid value '%s' for limit", limit)
	}
	return v, nil
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
					return err
				}
			}

			terminal := term.FromEnv()
			termWidth, _, err := terminal.Size()
			if err != nil {
				// set a static width in case of error
				termWidth = 80
			}
			t := tableprinter.New(terminal.Out(), terminal.IsTerminalOutput(), termWidth)

			config := listConfig{
				tp:     t,
				client: client,
				opts:   opts,
			}
			return runList(config)
		},
	}

	listCmd.Flags().StringVar(&opts.userOwner, "user", "", "Login of the user owner. Use \"@me\" for the current user.")
	listCmd.Flags().StringVar(&opts.orgOwner, "org", "", "Login of the organization owner.")
	listCmd.Flags().StringVar(&opts.format, "format", "", "Output format, must be 'json'.")
	listCmd.Flags().StringVar(&opts.limit, "limit", "", "Maximum number of fields. Defaults to 100. Set to 'all' to list all fields.")

	// owner can be a user or an org
	listCmd.MarkFlagsMutuallyExclusive("user", "org")

	return listCmd
}

func runList(config listConfig) error {
	if config.opts.format != "" && config.opts.format != "json" {
		return fmt.Errorf("format must be 'json'")
	}

	limit, err := parseLimit(config.opts.limit)
	if err != nil {
		return err
	}

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

	project, err := queries.ProjectFields(config.client, owner, config.opts.number, limit)
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
		config.tp.AddField(fmt.Sprintf("Project %d for login %s has no fields", config.opts.number, login))
		config.tp.EndRow()
		return config.tp.Render()
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
