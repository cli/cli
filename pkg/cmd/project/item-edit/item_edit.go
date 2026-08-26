package itemedit

import (
	"fmt"
	"strconv"
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
	title        string
	titleChanged bool
	body         string
	bodyChanged  bool
	itemID       string
	// updateItem
	fieldID              string
	projectID            string
	text                 string
	number               float64
	numberChanged        bool
	date                 string
	singleSelectOptionID string
	iterationID          string
	clear                bool
	// name-based resolution
	owner        string
	number32     int32
	url          string
	field        string
	value        string
	valueChanged bool
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

type DraftIssueQuery struct {
	DraftIssueNode struct {
		DraftIssue queries.DraftIssue `graphql:"... on DraftIssue"`
	} `graphql:"node(id: $id)"`
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
		Use:   "item-edit [<number>]",
		Short: "Edit an item in a project",
		Long: heredoc.Docf(`
			Edit a draft issue or a project item.

			The usual way to select the item and field is by name: pass the project
			%[1]snumber%[1]s plus %[1]s--owner%[1]s, point at the item with its issue or pull
			request %[1]s--url%[1]s, and name the field with %[1]s--field%[1]s. For single-select
			fields, %[1]s--value%[1]s is the option name.

			For scripts and machine use, you can also pass GraphQL node IDs directly with
			%[1]s--id%[1]s, %[1]s--field-id%[1]s and %[1]s--project-id%[1]s (and, for single-select
			fields, %[1]s--single-select-option-id%[1]s).

			Note that %[1]s--url%[1]s is the issue or pull request URL, not a project URL, so its
			owner may differ from the project's; %[1]s--owner%[1]s selects the project.

			For non-draft issues, only a single field value can be updated per invocation.

			Remove a project item field value with %[1]s--clear%[1]s.
		`, "`"),
		Example: heredoc.Doc(`
			# Set the "Status" field to "In Progress" for an issue on monalisa's project 1
			$ gh project item-edit 1 --owner monalisa --url https://github.com/monalisa/myproject/issues/23 --field "Status" --value "In Progress"

			# Edit an item's text field value by node ID (machine / scripted use)
			$ gh project item-edit --id <item-id> --field-id <field-id> --project-id <project-id> --text "new text"

			# Clear an item's field value by node ID
			$ gh project item-edit --id <item-id> --field-id <field-id> --project-id <project-id> --clear
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.numberChanged = cmd.Flags().Changed("number")
			opts.titleChanged = cmd.Flags().Changed("title")
			opts.bodyChanged = cmd.Flags().Changed("body")
			opts.valueChanged = cmd.Flags().Changed("value")

			if len(args) == 1 {
				num, err := strconv.ParseInt(args[0], 10, 32)
				if err != nil {
					return cmdutil.FlagErrorf("invalid number: %v", args[0])
				}
				opts.number32 = int32(num)
			}

			if err := cmdutil.MutuallyExclusive(
				"only one of `--text`, `--number`, `--date`, `--single-select-option-id`, `--iteration-id` or `--value` may be used",
				opts.text != "",
				opts.numberChanged,
				opts.date != "",
				opts.singleSelectOptionID != "",
				opts.iterationID != "",
				opts.valueChanged,
			); err != nil {
				return err
			}

			if err := cmdutil.MutuallyExclusive(
				"only one of `--field` or `--field-id` may be used",
				opts.field != "",
				opts.fieldID != "",
			); err != nil {
				return err
			}

			if err := cmdutil.MutuallyExclusive(
				"only one of `--url` or `--id` may be used",
				opts.url != "",
				opts.itemID != "",
			); err != nil {
				return err
			}

			if err := cmdutil.MutuallyExclusive(
				"cannot use `--text`, `--number`, `--date`, `--single-select-option-id`, `--iteration-id` or `--value` in conjunction with `--clear`",
				opts.text != "" || opts.numberChanged || opts.date != "" || opts.singleSelectOptionID != "" || opts.iterationID != "" || opts.valueChanged,
				opts.clear,
			); err != nil {
				return err
			}

			if opts.valueChanged && opts.fieldID != "" {
				return cmdutil.FlagErrorf("`--value` cannot be used with `--field-id`; name the field with `--field` to use `--value`")
			}

			if opts.field != "" && opts.itemID != "" {
				return cmdutil.FlagErrorf("`--field` cannot be used with `--id`; use `--url` to address the item when editing by name")
			}

			if opts.valueChanged && opts.field == "" {
				return cmdutil.FlagErrorf("`--value` requires `--field`")
			}

			if opts.itemID == "" && opts.url == "" {
				return cmdutil.FlagErrorf("specify the item to edit with `--id` or `--url`")
			}

			// Name-based flags resolve the field and item within a specific
			// project, so they require the project number as an argument.
			if (opts.url != "" || opts.field != "" || opts.valueChanged) && len(args) == 0 {
				return cmdutil.FlagErrorf("provide the project number as an argument when using `--url`, `--field`, or `--value`")
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

	editItemCmd.Flags().StringVar(&opts.owner, "owner", "", "Login of the owner. Use \"@me\" for the current user.")
	editItemCmd.Flags().StringVar(&opts.url, "url", "", "URL of the issue or pull request whose project item to edit")
	editItemCmd.Flags().StringVar(&opts.field, "field", "", "Name of the field to update")
	editItemCmd.Flags().StringVar(&opts.value, "value", "", "Value to set on the field named by `--field`")

	editItemCmd.Flags().StringVar(&opts.title, "title", "", "Title of the draft issue item")
	editItemCmd.Flags().StringVar(&opts.body, "body", "", "Body of the draft issue item")

	editItemCmd.Flags().StringVar(&opts.fieldID, "field-id", "", "ID of the field to update")
	editItemCmd.Flags().StringVar(&opts.projectID, "project-id", "", "ID of the project to which the field belongs to")
	editItemCmd.Flags().StringVar(&opts.text, "text", "", "Text value for the field")
	editItemCmd.Flags().Float64Var(&opts.number, "number", 0, "Number value for the field")
	editItemCmd.Flags().StringVar(&opts.date, "date", "", "Date value for the field (YYYY-MM-DD)")
	editItemCmd.Flags().StringVar(&opts.singleSelectOptionID, "single-select-option-id", "", "ID of the single select option value to set on the field")
	editItemCmd.Flags().StringVar(&opts.iterationID, "iteration-id", "", "ID of the iteration value to set on the field")
	editItemCmd.Flags().BoolVar(&opts.clear, "clear", false, "Remove field value")

	return editItemCmd
}

func runEditItem(config editItemConfig) error {
	// resolve name-based flags (--owner/number/--url/--field/--value) into the
	// ID-typed fields the mutations use, before any write happens.
	if config.opts.url != "" || config.opts.field != "" {
		resolved, err := resolveItemEditNames(config)
		if err != nil {
			return err
		}
		config = resolved
	}

	// when clear flag is used, remove value set to the corresponding field ID
	if config.opts.clear {
		return clearItemFieldValue(config)
	}

	// update draft issue
	if config.opts.titleChanged || config.opts.bodyChanged {
		return updateDraftIssue(config)
	}

	// update item values
	if config.opts.text != "" || config.opts.numberChanged || config.opts.date != "" || config.opts.singleSelectOptionID != "" || config.opts.iterationID != "" {
		return updateItemValues(config)
	}

	if _, err := fmt.Fprintln(config.io.ErrOut, "error: no changes to make"); err != nil {
		return err
	}
	return cmdutil.SilentError
}

// resolveItemEditNames turns the name-based addressing flags into the node IDs
// used by the field-value mutations: it resolves the project from
// owner + number, the item from --url, the field from --field, and (for
// single-select fields) --value from an option name to an option ID. Resolution
// happens before the mutation so a bad name fails loudly instead of writing a
// malformed value.
func resolveItemEditNames(config editItemConfig) (editItemConfig, error) {
	canPrompt := config.io.CanPrompt()
	owner, err := config.client.NewOwner(canPrompt, config.opts.owner)
	if err != nil {
		return config, err
	}

	project, err := config.client.ProjectFields(owner, config.opts.number32, queries.LimitMax)
	if err != nil {
		return config, err
	}
	config.opts.projectID = project.ID

	// Resolve the field first so a bad field name fails before the item lookup.
	var field queries.ProjectField
	if config.opts.field != "" {
		field, err = queries.ResolveFieldByName(project.Fields.Nodes, config.opts.field)
		if err != nil {
			return config, err
		}
		config.opts.fieldID = field.ID()
	}

	if config.opts.url == "" {
		return config, cmdutil.FlagErrorf("`--url` is required to resolve the item by issue or pull request URL")
	}
	itemID, err := config.client.ProjectItemIDByURL(config.opts.url, project.ID, config.opts.number32)
	if err != nil {
		return config, err
	}
	config.opts.itemID = itemID

	if config.opts.field == "" || !config.opts.valueChanged {
		return config, nil
	}

	// Dispatch --value by the resolved field's data type. New enum-like field
	// types (e.g. MultiSelect) can be added here without reworking callers.
	switch field.DataType() {
	case "SINGLE_SELECT":
		optionID, err := queries.ResolveSingleSelectOptionID(field, config.opts.value)
		if err != nil {
			return config, err
		}
		config.opts.singleSelectOptionID = optionID
	case "TEXT":
		config.opts.text = config.opts.value
	case "NUMBER":
		n, err := strconv.ParseFloat(config.opts.value, 64)
		if err != nil {
			return config, cmdutil.FlagErrorf("invalid number value %q for field %q", config.opts.value, field.Name())
		}
		config.opts.number = n
		config.opts.numberChanged = true
	case "DATE":
		config.opts.date = config.opts.value
	case "ITERATION":
		return config, cmdutil.FlagErrorf("setting an iteration field by name is not supported; use `--iteration-id`")
	default:
		return config, cmdutil.FlagErrorf("field %q has data type %q which is not supported with `--value`", field.Name(), field.DataType())
	}

	return config, nil
}

func fetchDraftIssueByID(config editItemConfig, draftIssueID string) (*queries.DraftIssue, error) {
	var query DraftIssueQuery
	variables := map[string]any{
		"id": githubv4.ID(draftIssueID),
	}

	err := config.client.Query("DraftIssueByID", &query, variables)
	if err != nil {
		return nil, err
	}

	return &query.DraftIssueNode.DraftIssue, nil
}

func buildEditDraftIssue(config editItemConfig, currentDraftIssue *queries.DraftIssue) (*EditProjectDraftIssue, map[string]any) {
	input := githubv4.UpdateProjectV2DraftIssueInput{
		DraftIssueID: githubv4.ID(config.opts.itemID),
	}

	if config.opts.titleChanged {
		input.Title = githubv4.NewString(githubv4.String(config.opts.title))
	} else if currentDraftIssue != nil {
		// Preserve existing if title is not provided
		input.Title = githubv4.NewString(githubv4.String(currentDraftIssue.Title))
	}

	if config.opts.bodyChanged {
		input.Body = githubv4.NewString(githubv4.String(config.opts.body))
	} else if currentDraftIssue != nil {
		// Preserve existing if body is not provided
		input.Body = githubv4.NewString(githubv4.String(currentDraftIssue.Body))
	}

	return &EditProjectDraftIssue{}, map[string]any{
		"input": input,
	}
}

func buildUpdateItem(config editItemConfig, date time.Time) (*UpdateProjectV2FieldValue, map[string]any) {
	var value githubv4.ProjectV2FieldValue
	if config.opts.text != "" {
		value = githubv4.ProjectV2FieldValue{
			Text: githubv4.NewString(githubv4.String(config.opts.text)),
		}
	} else if config.opts.numberChanged {
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

	return &UpdateProjectV2FieldValue{}, map[string]any{
		"input": githubv4.UpdateProjectV2ItemFieldValueInput{
			ProjectID: githubv4.ID(config.opts.projectID),
			ItemID:    githubv4.ID(config.opts.itemID),
			FieldID:   githubv4.ID(config.opts.fieldID),
			Value:     value,
		},
	}
}

func buildClearItem(config editItemConfig) (*ClearProjectV2FieldValue, map[string]any) {
	return &ClearProjectV2FieldValue{}, map[string]any{
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

	// Fetch current draft issue to preserve fields that aren't being updated
	var currentDraftIssue *queries.DraftIssue
	var err error
	if !config.opts.titleChanged || !config.opts.bodyChanged {
		currentDraftIssue, err = fetchDraftIssueByID(config, config.opts.itemID)
		if err != nil {
			return err
		}
	}

	query, variables := buildEditDraftIssue(config, currentDraftIssue)

	err = config.client.Mutate("EditDraftIssueItem", query, variables)
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
