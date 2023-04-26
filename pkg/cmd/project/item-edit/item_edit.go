package itemedit

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/cli/cli/v2/pkg/cmd/project/shared/format"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"

	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/go-gh/v2/pkg/api"
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
	tp     *tableprinter.TablePrinter
	client *api.GraphQLClient
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
		Short: "Edit an item in a project by ID",
		Long: `
Edit one of a draft issue or a project item. Both require the ID of the item to edit. For non-draft issues, the ID of the project is also required, and only a single field value can be updated per invocation. See the flags for more details.`,
		Example: `
# add --format=json to output in JSON format

# edit a draft issue title and body
gh project item-edit --id DRAFT_ISSUE_CONTENT_ID --title "a new title" --body "a new body"

# edit an item's text field value
gh project item-edit --id ITEM_ID --field-id FIELD_ID --project-id PROJECT_ID --text "new text"

# edit an item's number field value
gh project item-edit --id ITEM_ID --field-id FIELD_ID --project-id PROJECT_ID --number 1

# edit an item' date field value
gh project item-edit --id ITEM_ID --field-id FIELD_ID --project-id PROJECT_ID --date "2023-01-01"

# edit an item's single-select field value
# you can retrieve the option ID from the output of the 'field-list --format=json' command
gh project item-edit --id ITEM_ID --field-id FIELD_ID --project-id PROJECT_ID --single-select-option-id OPTION_ID

# edit an item's iteration field value
gh project item-edit --id ITEM_ID --field-id FIELD_ID --project-id PROJECT_ID --iteration-id ITERATION_ID
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := queries.NewClient()
			if err != nil {
				return err
			}

			t := tableprinter.New(f.IOStreams)
			config := editItemConfig{
				tp:     t,
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

	editItemCmd.Flags().StringVar(&opts.itemID, "id", "", "ID of the item to edit (required). For draft issues, the ID is for the draft issue content which is prefixed with `DI_`. For other issues, it is the ID of the project item.")
	editItemCmd.Flags().StringVar(&opts.format, "format", "", "Output format, must be 'json'.")

	editItemCmd.Flags().StringVar(&opts.title, "title", "", "DRAFT ISSUE - Title of the draft issue item to edit.")
	editItemCmd.Flags().StringVar(&opts.body, "body", "", "DRAFT ISSUE - Body of the draft issue item to edit.")

	editItemCmd.Flags().StringVar(&opts.fieldID, "field-id", "", "ID of the field to update.")
	editItemCmd.Flags().StringVar(&opts.projectID, "project-id", "", "ID of the project to which the field belongs to.")
	editItemCmd.Flags().StringVar(&opts.text, "text", "", "Text value to set on the field.")
	editItemCmd.Flags().Float32Var(&opts.number, "number", 0, "Number value to set on the field.")
	editItemCmd.Flags().StringVar(&opts.date, "date", "", "The ISO 8601 (YYYY-MM-DD) date value to set on the field.")
	editItemCmd.Flags().StringVar(&opts.singleSelectOptionID, "single-select-option-id", "", "ID of the single select option value to set on the field.")
	editItemCmd.Flags().StringVar(&opts.iterationID, "iteration-id", "", "ID of the iteration value to set on the field.")

	editItemCmd.MarkFlagsMutuallyExclusive("text", "number", "date", "single-select-option-id", "iteration-id")
	_ = editItemCmd.MarkFlagRequired("id")

	return editItemCmd
}

func runEditItem(config editItemConfig) error {
	// update draft issue
	if config.opts.title != "" || config.opts.body != "" {
		if !strings.HasPrefix(config.opts.itemID, "DI_") {
			return errors.New("ID must be the ID of the draft issue content which is prefixed with `DI_`")
		}

		if config.opts.format != "" && config.opts.format != "json" {
			return fmt.Errorf("format must be 'json'")
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
			return errors.New("field-id must be provided")
		}
		if config.opts.projectID == "" {
			// TODO: offer to fetch interactively
			return errors.New("project-id must be provided")
		}

		var parsedDate time.Time
		if config.opts.date != "" {
			date, error := time.Parse("2006-01-02", config.opts.date)
			if error != nil {
				return error
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

	config.tp.AddField("No changes to make")
	return config.tp.Render()

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
	// using table printer here for consistency in case it ends up being needed in the future
	config.tp.AddField("Title")
	config.tp.AddField("Body")
	config.tp.EndRow()
	config.tp.AddField(item.Title)
	config.tp.AddField(item.Body)
	config.tp.EndRow()
	return config.tp.Render()
}

func printDraftIssueJSON(config editItemConfig, item queries.DraftIssue) error {
	b, err := format.JSONProjectDraftIssue(item)
	if err != nil {
		return err
	}
	config.tp.AddField(string(b))
	return config.tp.Render()
}

func printItemResults(config editItemConfig, item *queries.ProjectItem) error {
	config.tp.AddField("Type")
	config.tp.AddField("Title")
	config.tp.AddField("Number")
	config.tp.AddField("Repository")
	config.tp.AddField("ID")
	config.tp.EndRow()

	config.tp.AddField(item.Type())
	config.tp.AddField(item.Title())
	if item.Number() == 0 {
		config.tp.AddField(" - ")
	} else {
		config.tp.AddField(fmt.Sprintf("%d", item.Number()))
	}
	if item.Repo() == "" {
		config.tp.AddField(" - ")
	} else {
		config.tp.AddField(item.Repo())
	}
	config.tp.AddField(item.ID())
	config.tp.EndRow()

	return config.tp.Render()
}

func printItemJSON(config editItemConfig, item *queries.ProjectItem) error {
	b, err := format.JSONProjectItem(*item)
	if err != nil {
		return err
	}
	config.tp.AddField(string(b))
	config.tp.EndRow()
	return config.tp.Render()

}
