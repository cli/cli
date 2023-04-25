package fieldcreate

import (
	"fmt"
	"strconv"

	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/cli/cli/v2/pkg/cmd/project/shared/format"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/cli/go-gh/v2/pkg/term"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type createFieldOpts struct {
	name                string
	dataType            string
	userOwner           string
	singleSelectOptions []string
	orgOwner            string
	number              int
	projectID           string
	format              string
}

type createFieldConfig struct {
	tp     tableprinter.TablePrinter
	client *api.GraphQLClient
	opts   createFieldOpts
}

type createProjectV2FieldMutation struct {
	CreateProjectV2Field struct {
		Field queries.ProjectField `graphql:"projectV2Field"`
	} `graphql:"createProjectV2Field(input:$input)"`
}

func NewCmdCreateField(f *cmdutil.Factory, runF func(config createFieldConfig) error) *cobra.Command {
	opts := createFieldOpts{}
	createFieldCmd := &cobra.Command{
		Short: "Create a field in a project",
		Use:   "field-create [number]",
		Example: `
# create a field in the current user's project 1 with title "new item" and dataType "text"
gh projects field-create 1 --user "@me" --name "new field" --data-type "text"

# create a field in user monalisa's project 1 with title "new item" and dataType "text"
gh projects field-create 1 --user monalisa --name "new field" --data-type "text"

# create a field in org github's' project 1 with title "new item" and dataType "text"
gh projects field-create 1 --org github --name "new field" --data-type "text"

# create a field with single select options
gh projects field-create 1 --user monalisa --name "new field" --data-type "SINGLE_SELECT" --single-select-options "one,two,three"

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

			config := createFieldConfig{
				tp:     t,
				client: client,
				opts:   opts,
			}
			return runCreateField(config)
		},
	}

	createFieldCmd.Flags().StringVar(&opts.userOwner, "user", "", "Login of the user owner. Use \"@me\" for the current user.")
	createFieldCmd.Flags().StringVar(&opts.orgOwner, "org", "", "Login of the organization owner.")
	createFieldCmd.Flags().StringVar(&opts.name, "name", "", "Name of the new field.")
	createFieldCmd.Flags().StringVar(&opts.dataType, "data-type", "", "DataType of the new field. Must be one of TEXT, SINGLE_SELECT, DATE, NUMBER.")
	createFieldCmd.Flags().StringSliceVar(&opts.singleSelectOptions, "single-select-options", []string{}, "At least one option is required when data type is SINGLE_SELECT.")
	createFieldCmd.Flags().StringVar(&opts.format, "format", "", "Output format, must be 'json'.")

	createFieldCmd.MarkFlagsMutuallyExclusive("user", "org")
	_ = createFieldCmd.MarkFlagRequired("name")
	_ = createFieldCmd.MarkFlagRequired("data-type")

	return createFieldCmd
}

func runCreateField(config createFieldConfig) error {
	if config.opts.dataType == "SINGLE_SELECT" && len(config.opts.singleSelectOptions) == 0 {
		return fmt.Errorf("at least one single select options is required with data type is SINGLE_SELECT")
	}

	if config.opts.format != "" && config.opts.format != "json" {
		return fmt.Errorf("format must be 'json'")
	}

	owner, err := queries.NewOwner(config.client, config.opts.userOwner, config.opts.orgOwner)
	if err != nil {
		return err
	}

	project, err := queries.NewProject(config.client, owner, config.opts.number, false)
	if err != nil {
		return err
	}
	config.opts.projectID = project.ID

	query, variables := createFieldArgs(config)

	err = config.client.Mutate("CreateField", query, variables)
	if err != nil {
		return err
	}

	if config.opts.format == "json" {
		return printJSON(config, query.CreateProjectV2Field.Field)
	}

	return printResults(config, query.CreateProjectV2Field.Field)
}

func createFieldArgs(config createFieldConfig) (*createProjectV2FieldMutation, map[string]interface{}) {
	input := githubv4.CreateProjectV2FieldInput{
		ProjectID: githubv4.ID(config.opts.projectID),
		DataType:  githubv4.ProjectV2CustomFieldType(config.opts.dataType),
		Name:      githubv4.String(config.opts.name),
	}

	if len(config.opts.singleSelectOptions) != 0 {
		opts := make([]githubv4.ProjectV2SingleSelectFieldOptionInput, 0)
		for _, opt := range config.opts.singleSelectOptions {
			opts = append(opts, githubv4.ProjectV2SingleSelectFieldOptionInput{
				Name:  githubv4.String(opt),
				Color: githubv4.ProjectV2SingleSelectFieldOptionColor("GRAY"),
			})
		}
		input.SingleSelectOptions = &opts
	}

	return &createProjectV2FieldMutation{}, map[string]interface{}{
		"input": input,
	}
}

func printResults(config createFieldConfig, field queries.ProjectField) error {
	// using table printer here for consistency in case it ends up being needed in the future
	config.tp.AddField("Created field")
	config.tp.EndRow()
	return config.tp.Render()
}

func printJSON(config createFieldConfig, field queries.ProjectField) error {
	b, err := format.JSONProjectField(field)
	if err != nil {
		return err
	}
	config.tp.AddField(string(b))
	return config.tp.Render()
}
