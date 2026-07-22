package editfield

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type EditFieldOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	IssueNumber int

	FieldID              string
	Text                 string
	Number               float64
	NumberChanged        bool
	Date                 string
	SingleSelectOptionID string
	MultiSelectOptionIDs []string
	Clear                bool
}

// UpdateIssueFieldValueInput is defined locally because the vendored githubv4 lacks the issue-field types.
type UpdateIssueFieldValueInput struct {
	IssueID    string                        `json:"issueId"`
	IssueField IssueFieldCreateOrUpdateInput `json:"issueField"`
}

type IssueFieldCreateOrUpdateInput struct {
	FieldID              string    `json:"fieldId"`
	TextValue            *string   `json:"textValue,omitempty"`
	NumberValue          *float64  `json:"numberValue,omitempty"`
	DateValue            *string   `json:"dateValue,omitempty"`
	SingleSelectOptionID *string   `json:"singleSelectOptionId,omitempty"`
	MultiSelectOptionIDs *[]string `json:"multiSelectOptionIds,omitempty"`
	Delete               *bool     `json:"delete,omitempty"`
}

func NewCmdEditField(f *cmdutil.Factory, runF func(*EditFieldOptions) error) *cobra.Command {
	opts := &EditFieldOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "edit-field {<number> | <url>} --field-id <id>",
		Short: "Set an issue field value",
		Long: heredoc.Docf(`
			Set or clear a custom issue field value on an issue.

			Issue field values are stored on the issue itself, not on a project item, so
			they are edited here rather than with %[1]sgh project item-edit%[1]s (just like labels and
			assignees). Reading these values is still available from a project with
			%[1]sgh project item-list%[1]s.

			Identify the issue field with %[1]s--field-id%[1]s (the ID of the issue field) and
			provide exactly one typed value (%[1]s--text%[1]s, %[1]s--number%[1]s, %[1]s--date%[1]s,
			%[1]s--single-select-option-id%[1]s, or %[1]s--multi-select-option-ids%[1]s), or
			%[1]s--clear%[1]s to remove the value.

			Editing issue fields requires authorization with the %[1]sproject%[1]s scope.
			To authorize, run %[1]sgh auth refresh -s project%[1]s.
		`, "`"),
		Example: heredoc.Doc(`
			# Set a text issue field value
			$ gh issue edit-field 23 --field-id <field-id> --text "Platform"

			# Set a single-select issue field value by option ID
			$ gh issue edit-field 23 --field-id <field-id> --single-select-option-id <option-id>

			# Set a multi-select issue field value by option IDs
			$ gh issue edit-field 23 --field-id <field-id> --multi-select-option-ids <id1>,<id2>

			# Clear an issue field value
			$ gh issue edit-field 23 --field-id <field-id> --clear
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.NumberChanged = cmd.Flags().Changed("number")

			issueNumber, baseRepo, err := shared.ParseIssueFromArg(args[0])
			if err != nil {
				return err
			}

			// If the args provided the base repo then use that directly.
			if baseRepo, present := baseRepo.Value(); present {
				opts.BaseRepo = func() (ghrepo.Interface, error) {
					return baseRepo, nil
				}
			} else {
				// support `-R, --repo` override
				opts.BaseRepo = f.BaseRepo
			}

			opts.IssueNumber = issueNumber

			if opts.FieldID == "" {
				return cmdutil.FlagErrorf("`--field-id` (the ID of the issue field) is required")
			}

			n := 0
			if opts.Text != "" {
				n++
			}
			if opts.NumberChanged {
				n++
			}
			if opts.Date != "" {
				n++
			}
			if opts.SingleSelectOptionID != "" {
				n++
			}
			if len(opts.MultiSelectOptionIDs) > 0 {
				n++
			}
			if opts.Clear {
				n++
			}
			if n != 1 {
				return cmdutil.FlagErrorf("provide exactly one of `--text`, `--number`, `--date`, `--single-select-option-id`, `--multi-select-option-ids`, or `--clear`")
			}

			if runF != nil {
				return runF(opts)
			}
			return editFieldRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.FieldID, "field-id", "", "ID of the issue field to set")
	cmd.Flags().StringVar(&opts.Text, "text", "", "Text value for the field")
	cmd.Flags().Float64Var(&opts.Number, "number", 0, "Number value for the field")
	cmd.Flags().StringVar(&opts.Date, "date", "", "Date value for the field (YYYY-MM-DD)")
	cmd.Flags().StringVar(&opts.SingleSelectOptionID, "single-select-option-id", "", "ID of the single select option value to set")
	cmd.Flags().StringSliceVar(&opts.MultiSelectOptionIDs, "multi-select-option-ids", nil, "IDs of the multi select option values to set")
	cmd.Flags().BoolVar(&opts.Clear, "clear", false, "Remove the field value")

	return cmd
}

func editFieldRun(opts *EditFieldOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	issue, err := shared.FindIssueOrPR(httpClient, baseRepo, opts.IssueNumber, []string{"id", "number", "title"})
	if err != nil {
		return err
	}

	field := IssueFieldCreateOrUpdateInput{FieldID: opts.FieldID}
	switch {
	case opts.Clear:
		del := true
		field.Delete = &del
	case opts.Text != "":
		field.TextValue = &opts.Text
	case opts.NumberChanged:
		field.NumberValue = &opts.Number
	case opts.Date != "":
		field.DateValue = &opts.Date
	case opts.SingleSelectOptionID != "":
		field.SingleSelectOptionID = &opts.SingleSelectOptionID
	case len(opts.MultiSelectOptionIDs) > 0:
		ids := opts.MultiSelectOptionIDs
		field.MultiSelectOptionIDs = &ids
	}

	var mutation struct {
		UpdateIssueFieldValue struct {
			ClientMutationID string
		} `graphql:"updateIssueFieldValue(input: $input)"`
	}
	variables := map[string]interface{}{
		"input": UpdateIssueFieldValueInput{
			IssueID:    issue.ID,
			IssueField: field,
		},
	}

	gql := api.NewClientFromHTTP(httpClient)
	if err := gql.Mutate(baseRepo.RepoHost(), "UpdateIssueFieldValue", &mutation, variables); err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()
	action := "Set"
	if opts.Clear {
		action = "Cleared"
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s %s issue field value on %s#%d\n", cs.SuccessIcon(), action, ghrepo.FullName(baseRepo), issue.Number)
	return nil
}
