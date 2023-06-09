package fielddelete

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmd/project/shared/client"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/format"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type deleteFieldOpts struct {
	fieldID string
	format  string
}

type deleteFieldConfig struct {
	client *queries.Client
	opts   deleteFieldOpts
	io     *iostreams.IOStreams
}

type deleteProjectV2FieldMutation struct {
	DeleteProjectV2Field struct {
		Field queries.ProjectField `graphql:"projectV2Field"`
	} `graphql:"deleteProjectV2Field(input:$input)"`
}

func NewCmdDeleteField(f *cmdutil.Factory, runF func(config deleteFieldConfig) error) *cobra.Command {
	opts := deleteFieldOpts{}
	deleteFieldCmd := &cobra.Command{
		Short: "Delete a field in a project",
		Use:   "field-delete",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client.New(f)
			if err != nil {
				return err
			}

			config := deleteFieldConfig{
				client: client,
				opts:   opts,
				io:     f.IOStreams,
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
	if !config.io.IsStdoutTTY() {
		return nil
	}

	_, err := fmt.Fprintf(config.io.Out, "Deleted field\n")
	return err
}

func printJSON(config deleteFieldConfig, field queries.ProjectField) error {
	b, err := format.JSONProjectField(field)
	if err != nil {
		return err
	}

	_, err = config.io.Out.Write(b)
	return err
}
