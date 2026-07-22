package fieldcreate

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

type createFieldOpts struct {
	name                string
	dataType            string
	owner               string
	singleSelectOptions []string
	issueFieldID        string
	number              int32
	projectID           string
	exporter            cmdutil.Exporter
}

type createFieldConfig struct {
	client *queries.Client
	opts   createFieldOpts
	io     *iostreams.IOStreams
}

type createProjectV2FieldMutation struct {
	CreateProjectV2Field struct {
		Field queries.ProjectField `graphql:"projectV2Field"`
	} `graphql:"createProjectV2Field(input:$input)"`
}

// createProjectV2IssueFieldMutation attaches an existing issue field to a project as a new column.
type createProjectV2IssueFieldMutation struct {
	CreateProjectV2IssueField struct {
		Field queries.ProjectField `graphql:"projectV2Field"`
	} `graphql:"createProjectV2IssueField(input:$input)"`
}

// CreateProjectV2IssueFieldInput is defined locally because the vendored githubv4 lacks the issue-field types.
type CreateProjectV2IssueFieldInput struct {
	ProjectID    string `json:"projectId"`
	IssueFieldID string `json:"issueFieldId"`
}

func NewCmdCreateField(f *cmdutil.Factory, runF func(config createFieldConfig) error) *cobra.Command {
	opts := createFieldOpts{}
	createFieldCmd := &cobra.Command{
		Short: "Create a field in a project",
		Use:   "field-create [<number>]",
		Example: heredoc.Doc(`
			# Create a field in the current user's project "1"
			$ gh project field-create 1 --owner "@me" --name "new field" --data-type "text"

			# Create a field with three options to select from for owner monalisa
			$ gh project field-create 1 --owner monalisa --name "new field" --data-type "SINGLE_SELECT" --single-select-options "one,two,three"

			# Attach an existing organization issue field to project 1 as a new column
			$ gh project field-create 1 --owner monalisa --issue-field-id <issue-field-id>
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

			config := createFieldConfig{
				client: client,
				opts:   opts,
				io:     f.IOStreams,
			}

			if config.opts.issueFieldID != "" {
				if config.opts.name != "" || config.opts.dataType != "" || len(config.opts.singleSelectOptions) > 0 {
					return cmdutil.FlagErrorf("`--issue-field-id` cannot be combined with `--name`, `--data-type`, or `--single-select-options`")
				}
			} else {
				if config.opts.name == "" || config.opts.dataType == "" {
					return cmdutil.FlagErrorf("`--name` and `--data-type` are required unless attaching an issue field with `--issue-field-id`")
				}
				if config.opts.dataType == "SINGLE_SELECT" && len(config.opts.singleSelectOptions) == 0 {
					return fmt.Errorf("passing `--single-select-options` is required for SINGLE_SELECT data type")
				}
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runCreateField(config)
		},
	}

	createFieldCmd.Flags().StringVar(&opts.owner, "owner", "", "Login of the owner. Use \"@me\" for the current user.")
	createFieldCmd.Flags().StringVar(&opts.name, "name", "", "Name of the new field")
	cmdutil.StringEnumFlag(createFieldCmd, &opts.dataType, "data-type", "", "", []string{"TEXT", "SINGLE_SELECT", "DATE", "NUMBER"}, "DataType of the new field.")
	createFieldCmd.Flags().StringSliceVar(&opts.singleSelectOptions, "single-select-options", []string{}, "Options for SINGLE_SELECT data type")
	createFieldCmd.Flags().StringVar(&opts.issueFieldID, "issue-field-id", "", "ID of an existing issue field to attach to the project as a new column")
	cmdutil.AddFormatFlags(createFieldCmd, &opts.exporter)

	return createFieldCmd
}

func runCreateField(config createFieldConfig) error {
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

	if config.opts.issueFieldID != "" {
		return attachIssueField(config)
	}

	query, variables := createFieldArgs(config)

	err = config.client.Mutate("CreateField", query, variables)
	if err != nil {
		return err
	}

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, query.CreateProjectV2Field.Field)
	}

	return printResults(config, query.CreateProjectV2Field.Field)
}

// attachIssueField surfaces an existing issue field on the project as a new column.
func attachIssueField(config createFieldConfig) error {
	mutation := &createProjectV2IssueFieldMutation{}
	variables := map[string]interface{}{
		"input": CreateProjectV2IssueFieldInput{
			ProjectID:    config.opts.projectID,
			IssueFieldID: config.opts.issueFieldID,
		},
	}

	if err := config.client.Mutate("CreateProjectV2IssueField", mutation, variables); err != nil {
		return err
	}

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, mutation.CreateProjectV2IssueField.Field)
	}

	if config.io.IsStdoutTTY() {
		if _, err := fmt.Fprintf(config.io.Out, "Attached issue field\n"); err != nil {
			return err
		}
	}
	return nil
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
	if !config.io.IsStdoutTTY() {
		return nil
	}

	_, err := fmt.Fprintf(config.io.Out, "Created field\n")
	return err
}
