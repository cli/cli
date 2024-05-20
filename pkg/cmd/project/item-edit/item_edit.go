package itemedit

import (
	"fmt"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/client"
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
	clear                bool
	// format
	exporter cmdutil.Exporter
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

type ClearProjectV2FieldValue struct {
	Clear struct {
		Item queries.ProjectItem `graphql:"projectV2Item"`
	} `graphql:"clearProjectV2ItemFieldValue(input:$input)"`
}

func NewCmdEditItem(f *cmdutil.Factory, runF func(config editItemConfig) error) *cobra.Command {
	opts := editItemOpts{}
	editItemCmd := &cobra.Command{
		Use:   "item-edit",
		Short: "Edit an item in a project",
		Long: heredoc.Docf(`
			Edit either a draft issue or a project item. Both usages require the ID of the item to edit.
			
			For non-draft issues, the ID of the project is also required, and only a single field value can be updated per invocation.

			Remove project item field value using %[1]s--clear%[1]s flag.
		`, "`"),
		Example: heredoc.Doc(`
			# edit an item's text field value
			gh project item-edit --id <item-ID> --field-id <field-ID> --project-id <project-ID> --text "new text"

			# clear an item's field value
			gh project item-edit --id <item-ID> --field-id <field-ID> --project-id <project-ID> --clear
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

			if err := cmdutil.MutuallyExclusive(
				"cannot use `--text`, `--number`, `--date`, `--single-select-option-id` or `--iteration-id` in conjunction with `--clear`",
				opts.text != "" || opts.number != 0 || opts.date != "" || opts.singleSelectOptionID != "" || opts.iterationID != "",
				opts.clear,
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
	cmdutil.AddFormatFlags(editItemCmd, &opts.exporter)

	editItemCmd.Flags().StringVar(&opts.title, "title", "", "Title of the draft issue item")
	editItemCmd.Flags().StringVar(&opts.body, "body", "", "Body of the draft issue item")

	editItemCmd.Flags().StringVar(&opts.fieldID, "field-id", "", "ID of the field to update")
	editItemCmd.Flags().StringVar(&opts.projectID, "project-id", "", "ID of the project to which the field belongs to")
	editItemCmd.Flags().StringVar(&opts.text, "text", "", "Text value for the field")
	editItemCmd.Flags().Float32Var(&opts.number, "number", 0, "Number value for the field")
	editItemCmd.Flags().StringVar(&opts.date, "date", "", "Date value for the field (YYYY-MM-DD)")
	editItemCmd.Flags().StringVar(&opts.singleSelectOptionID, "single-select-option-id", "", "ID of the single select option value to set on the field")
	editItemCmd.Flags().StringVar(&opts.iterationID, "iteration-id", "", "ID of the iteration value to set on the field")
	editItemCmd.Flags().BoolVar(&opts.clear, "clear", false, "Remove field value")

	_ = editItemCmd.MarkFlagRequired("id")

	return editItemCmd
}

func runEditItem(config editItemConfig) error {
	// when clear flag is used, remove value set to the corresponding field ID
	if config.opts.clear {
		return clearItemFieldValue(config)
	}

	// update draft issue
	if config.opts.title != "" || config.opts.body != "" {
		return updateDraftIssue(config)
	}

	// update item values
	if config.opts.text != "" || config.opts.number != 0 || config.opts.date != "" || config.opts.singleSelectOptionID != "" || config.opts.iterationID != "" {
		return updateItemValues(config)
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

func buildClearItem(config editItemConfig) (*ClearProjectV2FieldValue, map[string]interface{}) {
	return &ClearProjectV2FieldValue{}, map[string]interface{}{
		"input": githubv4.ClearProjectV2ItemFieldValueInput{
			ProjectID: githubv4.ID(config.opts.projectID),
			ItemID:    githubv4.ID(config.opts.itemID),
			FieldID:   githubv4.ID(config.opts.fieldID),
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

func printItemResults(config editItemConfig, item *queries.ProjectItem) error {
	if !config.io.IsStdoutTTY() {
		return nil
	}
	_, err := fmt.Fprintf(config.io.Out, "Edited item %q\n", item.Title())
	return err
}

func clearItemFieldValue(config editItemConfig) error {
	if err := fieldIdAndProjectIdPresence(config); err != nil {
		return err
	}
	query, variables := buildClearItem(config)
	err := config.client.Mutate("ClearItemFieldValue", query, variables)
	if err != nil {
		return err
	}

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, &query.Clear.Item)
	}

	return printItemResults(config, &query.Clear.Item)
}

func updateDraftIssue(config editItemConfig) error {
	if !strings.HasPrefix(config.opts.itemID, "DI_") {
		return cmdutil.FlagErrorf("ID must be the ID of the draft issue content which is prefixed with `DI_`")
	}

	query, variables := buildEditDraftIssue(config)

	err := config.client.Mutate("EditDraftIssueItem", query, variables)
	if err != nil {
		return err
	}

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, query.UpdateProjectV2DraftIssue.DraftIssue)
	}

	return printDraftIssueResults(config, query.UpdateProjectV2DraftIssue.DraftIssue)
}

func updateItemValues(config editItemConfig) error {
	if err := fieldIdAndProjectIdPresence(config); err != nil {
		return err
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

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, &query.Update.Item)
	}

	return printItemResults(config, &query.Update.Item)
}

func fieldIdAndProjectIdPresence(config editItemConfig) error {
	if config.opts.fieldID == "" && config.opts.projectID == "" {
		return cmdutil.FlagErrorf("field-id and project-id must be provided")
	}
	if config.opts.fieldID == "" {
		return cmdutil.FlagErrorf("field-id must be provided")
	}
	if config.opts.projectID == "" {
		// TODO: offer to fetch interactively
		return cmdutil.FlagErrorf("project-id must be provided")
	}
	return nil
}
