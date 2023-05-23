package fielddelete

import (
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/format"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type deleteFieldOpts struct {
	fieldID string
	format  string
}

type deleteFieldConfig struct {
	tp     *tableprinter.TablePrinter
	client *api.GraphQLClient
	opts   deleteFieldOpts
}

type deleteProjectV2FieldMutation struct {
	DeleteProjectV2Field struct {
		Field queries.ProjectField `graphql:"projectV2Field"`
	} `graphql:"deleteProjectV2Field(input:$input)"`
}

func NewCmdDeleteField(f *cmdutil.Factory, runF func(config deleteFieldConfig) error) *cobra.Command {
	opts := deleteFieldOpts{}
	deleteFieldCmd := &cobra.Command{
		Short: "Delete a field in a project by ID",
		Use:   "field-delete",
		Example: `
# delete a field by ID
gh project field-delete --id ID
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := queries.NewClient()
			if err != nil {
				return err
			}

			t := tableprinter.New(f.IOStreams)
			config := deleteFieldConfig{
				tp:     t,
				client: client,
				opts:   opts,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runDeleteField(config)
		},
	}

	deleteFieldCmd.Flags().StringVar(&opts.fieldID, "id", "", "ID of the field to delete")
	cmdutil.StringEnumFlag(deleteFieldCmd, &opts.format, "format", "", "", []string{"json"}, "Output format")

	_ = deleteFieldCmd.MarkFlagRequired("id")

	return deleteFieldCmd
}

func runDeleteField(config deleteFieldConfig) error {
	query, variables := deleteFieldArgs(config)

	err := config.client.Mutate("DeleteField", query, variables)
	if err != nil {
		return err
	}

	if config.opts.format == "json" {
		return printJSON(config, query.DeleteProjectV2Field.Field)
	}

	return printResults(config, query.DeleteProjectV2Field.Field)
}

func deleteFieldArgs(config deleteFieldConfig) (*deleteProjectV2FieldMutation, map[string]interface{}) {
	return &deleteProjectV2FieldMutation{}, map[string]interface{}{
		"input": githubv4.DeleteProjectV2FieldInput{
			FieldID: githubv4.ID(config.opts.fieldID),
		},
	}
}

func printResults(config deleteFieldConfig, field queries.ProjectField) error {
	// using table printer here for consistency in case it ends up being needed in the future
	config.tp.AddField("Deleted field")
	config.tp.EndRow()
	return config.tp.Render()
}

func printJSON(config deleteFieldConfig, field queries.ProjectField) error {
	b, err := format.JSONProjectField(field)
	if err != nil {
		return err
	}
	config.tp.AddField(string(b))
	return config.tp.Render()
}
