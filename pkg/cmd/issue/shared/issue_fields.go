package shared

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

// IssueFieldCreateOrUpdateInput mirrors the GraphQL input of the same name.
// It is defined locally because the vendored githubv4 lacks the issue-field types.
type IssueFieldCreateOrUpdateInput struct {
	FieldID              string    `json:"fieldId"`
	TextValue            *string   `json:"textValue,omitempty"`
	NumberValue          *float64  `json:"numberValue,omitempty"`
	DateValue            *string   `json:"dateValue,omitempty"`
	SingleSelectOptionID *string   `json:"singleSelectOptionId,omitempty"`
	MultiSelectOptionIDs *[]string `json:"multiSelectOptionIds,omitempty"`
	Delete               *bool     `json:"delete,omitempty"`
}

// issueFieldOption is a single-select or multi-select option.
type issueFieldOption struct {
	ID   string
	Name string
}

// issueFieldDefinition is a repository issue field, used to resolve a field by
// name or ID and to dispatch a value by its data type.
type issueFieldDefinition struct {
	ID       string
	Name     string
	DataType string
	Options  []issueFieldOption
}

type issueFieldsQuery struct {
	Repository struct {
		IssueFields struct {
			Nodes []struct {
				Typename string `graphql:"__typename"`
				Text     struct {
					ID       string
					Name     string
					DataType string
				} `graphql:"... on IssueFieldText"`
				Number struct {
					ID       string
					Name     string
					DataType string
				} `graphql:"... on IssueFieldNumber"`
				Date struct {
					ID       string
					Name     string
					DataType string
				} `graphql:"... on IssueFieldDate"`
				SingleSelect struct {
					ID       string
					Name     string
					DataType string
					Options  []issueFieldOption
				} `graphql:"... on IssueFieldSingleSelect"`
				MultiSelect struct {
					ID       string
					Name     string
					DataType string
					Options  []issueFieldOption
				} `graphql:"... on IssueFieldMultiSelect"`
			}
		} `graphql:"issueFields(first: 100)"`
	} `graphql:"repository(owner: $owner, name: $name)"`
}

// BuildIssueFieldValueInput resolves the issue field identified by fieldName or
// fieldID against the repository's issue fields and builds an update input with
// value dispatched by the field's data type. For single-select and multi-select
// fields, value is an option name (multi-select accepts a comma-separated list).
func BuildIssueFieldValueInput(httpClient *http.Client, repo ghrepo.Interface, fieldName, fieldID, value string) (IssueFieldCreateOrUpdateInput, error) {
	fields, err := fetchIssueFields(httpClient, repo)
	if err != nil {
		return IssueFieldCreateOrUpdateInput{}, err
	}
	return resolveIssueFieldValueInput(fields, fieldName, fieldID, value)
}

func fetchIssueFields(httpClient *http.Client, repo ghrepo.Interface) ([]issueFieldDefinition, error) {
	var query issueFieldsQuery
	variables := map[string]interface{}{
		"owner": githubv4.String(repo.RepoOwner()),
		"name":  githubv4.String(repo.RepoName()),
	}

	client := api.NewClientFromHTTP(httpClient)
	if err := client.Query(repo.RepoHost(), "RepositoryIssueFields", &query, variables); err != nil {
		return nil, err
	}

	var fields []issueFieldDefinition
	for _, n := range query.Repository.IssueFields.Nodes {
		switch n.Typename {
		case "IssueFieldText":
			fields = append(fields, issueFieldDefinition{ID: n.Text.ID, Name: n.Text.Name, DataType: n.Text.DataType})
		case "IssueFieldNumber":
			fields = append(fields, issueFieldDefinition{ID: n.Number.ID, Name: n.Number.Name, DataType: n.Number.DataType})
		case "IssueFieldDate":
			fields = append(fields, issueFieldDefinition{ID: n.Date.ID, Name: n.Date.Name, DataType: n.Date.DataType})
		case "IssueFieldSingleSelect":
			fields = append(fields, issueFieldDefinition{ID: n.SingleSelect.ID, Name: n.SingleSelect.Name, DataType: n.SingleSelect.DataType, Options: n.SingleSelect.Options})
		case "IssueFieldMultiSelect":
			fields = append(fields, issueFieldDefinition{ID: n.MultiSelect.ID, Name: n.MultiSelect.Name, DataType: n.MultiSelect.DataType, Options: n.MultiSelect.Options})
		}
	}
	return fields, nil
}

func resolveIssueFieldValueInput(fields []issueFieldDefinition, fieldName, fieldID, value string) (IssueFieldCreateOrUpdateInput, error) {
	field, err := resolveIssueField(fields, fieldName, fieldID)
	if err != nil {
		return IssueFieldCreateOrUpdateInput{}, err
	}

	input := IssueFieldCreateOrUpdateInput{FieldID: field.ID}
	switch field.DataType {
	case "TEXT":
		input.TextValue = &value
	case "DATE":
		input.DateValue = &value
	case "NUMBER":
		n, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return IssueFieldCreateOrUpdateInput{}, fmt.Errorf("invalid number value %q for field %q", value, field.Name)
		}
		input.NumberValue = &n
	case "SINGLE_SELECT":
		optionID, err := resolveOptionID(field, value)
		if err != nil {
			return IssueFieldCreateOrUpdateInput{}, err
		}
		input.SingleSelectOptionID = &optionID
	case "MULTI_SELECT":
		var ids []string
		for _, name := range splitOptionNames(value) {
			optionID, err := resolveOptionID(field, name)
			if err != nil {
				return IssueFieldCreateOrUpdateInput{}, err
			}
			ids = append(ids, optionID)
		}
		input.MultiSelectOptionIDs = &ids
	default:
		return IssueFieldCreateOrUpdateInput{}, fmt.Errorf("field %q has data type %q which is not supported", field.Name, field.DataType)
	}
	return input, nil
}

func resolveIssueField(fields []issueFieldDefinition, fieldName, fieldID string) (issueFieldDefinition, error) {
	if fieldID != "" {
		for _, f := range fields {
			if f.ID == fieldID {
				return f, nil
			}
		}
		return issueFieldDefinition{}, fmt.Errorf("no issue field found with ID %q", fieldID)
	}

	var matches []issueFieldDefinition
	for _, f := range fields {
		if strings.EqualFold(f.Name, fieldName) {
			matches = append(matches, f)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		names := make([]string, 0, len(fields))
		for _, f := range fields {
			names = append(names, f.Name)
		}
		if len(names) == 0 {
			return issueFieldDefinition{}, fmt.Errorf("issue field %q not found; the repository has no issue fields", fieldName)
		}
		return issueFieldDefinition{}, fmt.Errorf("issue field %q not found; available fields: %s", fieldName, strings.Join(names, ", "))
	default:
		return issueFieldDefinition{}, fmt.Errorf("issue field %q is ambiguous; use --field-id", fieldName)
	}
}

func resolveOptionID(field issueFieldDefinition, optionName string) (string, error) {
	var matches []string
	for _, o := range field.Options {
		if strings.EqualFold(o.Name, optionName) {
			matches = append(matches, o.ID)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		names := make([]string, 0, len(field.Options))
		for _, o := range field.Options {
			names = append(names, o.Name)
		}
		return "", fmt.Errorf("option %q not found on field %q; available options: %s", optionName, field.Name, strings.Join(names, ", "))
	default:
		return "", fmt.Errorf("option %q is ambiguous on field %q", optionName, field.Name)
	}
}

func splitOptionNames(value string) []string {
	parts := strings.Split(value, ",")
	names := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			names = append(names, p)
		}
	}
	return names
}

// UpdateIssueFieldValue writes input on the issue identified by issueID via the
// updateIssueFieldValue mutation.
func UpdateIssueFieldValue(httpClient *http.Client, repo ghrepo.Interface, issueID string, input IssueFieldCreateOrUpdateInput) error {
	var mutation struct {
		UpdateIssueFieldValue struct {
			ClientMutationID string
		} `graphql:"updateIssueFieldValue(input: $input)"`
	}
	variables := map[string]interface{}{
		"input": updateIssueFieldValueInput{
			IssueID:    issueID,
			IssueField: input,
		},
	}
	client := api.NewClientFromHTTP(httpClient)
	return client.Mutate(repo.RepoHost(), "UpdateIssueFieldValue", &mutation, variables)
}

type updateIssueFieldValueInput struct {
	IssueID    string                        `json:"issueId"`
	IssueField IssueFieldCreateOrUpdateInput `json:"issueField"`
}
