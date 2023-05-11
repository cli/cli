package itemlist

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
	limit     string
	userOwner string
	orgOwner  string
	number    int
	format    string
}

type listConfig struct {
	tp     *tableprinter.TablePrinter
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
		Short: "List the items in a project",
		Use:   "item-list [<number>]",
		Example: `
The default output is a column format with a subset of system defined fields.
To list all of the fields, use the --format directive.

# list the items in the current users's project number 1
gh project item-list 1 --user "@me"

# list the items in user monalisa's project number 1
gh project item-list 1 --user monalisa

# list the items in org github's project number 1
gh project item-list 1 --org github

# add --format=json to output in JSON format
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
				opts.number, err = strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("invalid number: %v", args[0])
				}
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
	listCmd.Flags().StringVar(&opts.orgOwner, "org", "", "Login of the organization owner.")
	listCmd.Flags().StringVar(&opts.format, "format", "", "Output format, must be 'json'.")
	listCmd.Flags().StringVar(&opts.limit, "limit", "", "Maximum number of items. Defaults to 30. Set to 'all' to list all items.")

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

	project, err := queries.ProjectItems(config.client, owner, config.opts.number, limit)
	if err != nil {
		return err
	}

	if config.opts.format == "json" {
		return printJSON(config, project)
	}

	return printResults(config, project.Items.Nodes, owner.Login)
}

func printResults(config listConfig, items []queries.ProjectItem, login string) error {
	if len(items) == 0 {
		config.tp.AddField(fmt.Sprintf("Project %d for login %s has no items", config.opts.number, login))
		config.tp.EndRow()
		return config.tp.Render()
	}

	config.tp.AddField("Type")
	config.tp.AddField("Title")
	config.tp.AddField("Number")
	config.tp.AddField("Repository")
	config.tp.AddField("ID")
	config.tp.EndRow()

	for _, i := range items {
		config.tp.AddField(i.Type())
		config.tp.AddField(i.Title())
		if i.Number() == 0 {
			config.tp.AddField(" - ")
		} else {
			config.tp.AddField(fmt.Sprintf("%d", i.Number()))
		}
		if i.Repo() == "" {
			config.tp.AddField(" - ")
		} else {
			config.tp.AddField(i.Repo())
		}
		config.tp.AddField(i.ID())
		config.tp.EndRow()
	}

	return config.tp.Render()
}

func printJSON(config listConfig, project *queries.Project) error {
	b, err := format.JSONProjectDetailedItems(project)
	if err != nil {
		return err
	}
	config.tp.AddField(string(b))
	config.tp.EndRow()
	return config.tp.Render()

}
