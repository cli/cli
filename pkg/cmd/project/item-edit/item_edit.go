package itemedit

import (
	"fmt"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/client"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/format"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type editItemOpts struct {
	// updateDraftIssue
	title  string
	body   string
	itemID string
	// updateItem
	fieldID              string
	projectID            string
	text                 string
	number               float32
	date                 string
	singleSelectOptionID string
	iterationID          string
	// format
	format string
}

type editItemConfig struct {
	io     *iostreams.IOStreams
	client *queries.Client
	opts   editItemOpts
}

type EditProjectDraftIssue struct {
	UpdateProjectV2DraftIssue struct {
		DraftIssue queries.DraftIssue `graphql:"draftIssue"`
	} `graphql:"updateProjectV2DraftIssue(input:$input)"`
}

type UpdateProjectV2FieldValue struct {
	Update struct {
		Item queries.ProjectItem `graphql:"projectV2Item"`
	} `graphql:"updateProjectV2ItemFieldValue(input:$input)"`
}

func NewCmdEditItem(f *cmdutil.Factory, runF func(config editItemConfig) error) *cobra.Command {
	opts := editItemOpts{}
	editItemCmd := &cobra.Command{
		Use:   "item-edit",
		Short: "Edit an item in a project",
		Long: heredoc.Doc(`
			Edit either a draft issue or a project item. Both usages require the ID of the item to edit.
			
			For non-draft issues, the ID of the project is also required, and only a single field value can be updated per invocation.
		`),
		Example: heredoc.Doc(`
			# edit an item's text field value
			gh project item-edit --id <item-ID> --field-id <field-ID> --project-id <project-ID> --text "new text"
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmdutil.MutuallyExclusive(
				"only one of `--text`, `--number`, `--date`, `--single-select-option-id` or `--iteration-id` may be used",
				opts.text != "",
				opts.number != 0,
				opts.date != "",
				opts.singleSelectOptionID != "",
				opts.iterationID != "",
			); err != nil {
				return err
			}

			client, err := client.New(f)
			if err != nil {
				return err
			}

			config := editItemConfig{
				io:     f.IOStreams,
				client: client,
				opts:   opts,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runEditItem(config)
		},
	}

	editItemCmd.Flags().StringVar(&opts.itemID, "id", "", "ID of the item to edit")
	cmdutil.StringEnumFlag(editItemCmd, &opts.format, "format", "", "", []string{"json"}, "Output format")

	editItemCmd.Flags().StringVar(&opts.title, "title", "", "Title of the draft issue item")
	editItemCmd.Flags().StringVar(&opts.body, "body", "", "Body of the draft issue item")

	editItemCmd.Flags().StringVar(&opts.fieldID, "field-id", "", "ID of the field to update")
	editItemCmd.Flags().StringVar(&opts.projectID, "project-id", "", "ID of the project to which the field belongs to")
	editItemCmd.Flags().StringVar(&opts.text, "text", "", "Text value for the field")
	editItemCmd.Flags().Float32Var(&opts.number, "number", 0, "Number value for the field")
	editItemCmd.Flags().StringVar(&opts.date, "date", "", "Date value for the field (YYYY-MM-DD)")
	editItemCmd.Flags().StringVar(&opts.singleSelectOptionID, "single-select-option-id", "", "ID of the single select option value to set on the field")
	editItemCmd.Flags().StringVar(&opts.iterationID, "iteration-id", "", "ID of the iteration value to set on the field")

	_ = editItemCmd.MarkFlagRequired("id")

	return editItemCmd
}

func runEditItem(config editItemConfig) error {
	// update draft issue
	if config.opts.title != "" || config.opts.body != "" {
		if !strings.HasPrefix(config.opts.itemID, "DI_") {
			return cmdutil.FlagErrorf("ID must be the ID of the draft issue content which is prefixed with `DI_`")
		}

		query, variables := buildEditDraftIssue(config)

		err := config.client.Mutate("EditDraftIssueItem", query, variables)
		if err != nil {
			return err
		}

		if config.opts.format == "json" {
			return printDraftIssueJSON(config, query.UpdateProjectV2DraftIssue.DraftIssue)
		}

		return printDraftIssueResults(config, query.UpdateProjectV2DraftIssue.DraftIssue)
	}

	// update item values
	if config.opts.text != "" || config.opts.number != 0 || config.opts.date != "" || config.opts.singleSelectOptionID != "" || config.opts.iterationID != "" {
		if config.opts.fieldID == "" {
			return cmdutil.FlagErrorf("field-id must be provided")
		}
		if config.opts.projectID == "" {
			// TODO: offer to fetch interactively
			return cmdutil.FlagErrorf("project-id must be provided")
		}

		var parsedDate time.Time
		if config.opts.date != "" {
			date, err := time.Parse("2006-01-02", config.opts.date)
			if err != nil {
				return err
			}
			parsedDate = date
		}

		query, variables := buildUpdateItem(config, parsedDate)
		err := config.client.Mutate("UpdateItemValues", query, variables)
		if err != nil {
			return err
		}

		if config.opts.format == "json" {
			return printItemJSON(config, &query.Update.Item)
		}

		return printItemResults(config, &query.Update.Item)
	}

	if _, err := fmt.Fprintln(config.io.ErrOut, "error: no changes to make"); err != nil {
		return err
	}
	return cmdutil.SilentError
}

func buildEditDraftIssue(config editItemConfig) (*EditProjectDraftIssue, map[string]interface{}) {
	return &EditProjectDraftIssue{}, map[string]interface{}{
		"input": githubv4.UpdateProjectV2DraftIssueInput{
			Body:         githubv4.NewString(githubv4.String(config.opts.body)),
			DraftIssueID: githubv4.ID(config.opts.itemID),
			Title:        githubv4.NewString(githubv4.String(config.opts.title)),
		},
	}
}

func buildUpdateItem(config editItemConfig, date time.Time) (*UpdateProjectV2FieldValue, map[string]interface{}) {
	var value githubv4.ProjectV2FieldValue
	if config.opts.text != "" {
		value = githubv4.ProjectV2FieldValue{
			Text: githubv4.NewString(githubv4.String(config.opts.text)),
		}
	} else if config.opts.number != 0 {
		value = githubv4.ProjectV2FieldValue{
			Number: githubv4.NewFloat(githubv4.Float(config.opts.number)),
		}
	} else if config.opts.date != "" {
		value = githubv4.ProjectV2FieldValue{
			Date: githubv4.NewDate(githubv4.Date{Time: date}),
		}
	} else if config.opts.singleSelectOptionID != "" {
		value = githubv4.ProjectV2FieldValue{
			SingleSelectOptionID: githubv4.NewString(githubv4.String(config.opts.singleSelectOptionID)),
		}
	} else if config.opts.iterationID != "" {
		value = githubv4.ProjectV2FieldValue{
			IterationID: githubv4.NewString(githubv4.String(config.opts.iterationID)),
		}
	}

	return &UpdateProjectV2FieldValue{}, map[string]interface{}{
		"input": githubv4.UpdateProjectV2ItemFieldValueInput{
			ProjectID: githubv4.ID(config.opts.projectID),
			ItemID:    githubv4.ID(config.opts.itemID),
			FieldID:   githubv4.ID(config.opts.fieldID),
			Value:     value,
		},
	}
}

func printDraftIssueResults(config editItemConfig, item queries.DraftIssue) error {
	if !config.io.IsStdoutTTY() {
		return nil
	}
	_, err := fmt.Fprintf(config.io.Out, "Edited draft issue %q\n", item.Title)
	return err
}

func printDraftIssueJSON(config editItemConfig, item queries.DraftIssue) error {
	b, err := format.JSONProjectDraftIssue(item)
	if err != nil {
		return err
	}
	_, err = config.io.Out.Write(b)
	return err
}

func printItemResults(config editItemConfig, item *queries.ProjectItem) error {
	if !config.io.IsStdoutTTY() {
		return nil
	}
	_, err := fmt.Fprintf(config.io.Out, "Edited item %q\n", item.Title())
	return err
}

func printItemJSON(config editItemConfig, item *queries.ProjectItem) error {
	b, err := format.JSONProjectItem(*item)
	if err != nil {
		return err
	}
	_, err = config.io.Out.Write(b)
	return err

}
