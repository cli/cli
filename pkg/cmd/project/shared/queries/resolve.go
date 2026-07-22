package queries

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/shurcooL/githubv4"
)

// This file holds name-based resolution for ProjectV2 fields, single-select
// options, and project items. It lets commands accept human-readable names
// (e.g. --field "Status" --value "In Progress") instead of node IDs.
//
// The resolvers mirror the semantics of the github-mcp-server Projects resolver
// so the CLI and MCP surfaces behave identically, with two deliberate
// differences documented on each type:
//   - errors are human-readable (gh convention) rather than structured JSON,
//     but still list candidate names so the message stays self-correctable;
//   - resolution yields GraphQL node IDs (PVTF_/PVTSSF_/PVTI_), not the numeric
//     databaseIDs the MCP REST path needs.
//
// Option resolution dispatches on the field's data type so that additional
// enum-like field types (MultiSelect, issue field values) can be added later
// without reworking the shared path.

// OptionNotFoundError is returned when no single-select option matches the given
// name. Candidates lists the available option names.
type OptionNotFoundError struct {
	FieldName  string
	Name       string
	Candidates []string
}

func (e *OptionNotFoundError) Error() string {
	if len(e.Candidates) == 0 {
		return fmt.Sprintf("option %q not found on field %q; the field has no options", e.Name, e.FieldName)
	}
	return fmt.Sprintf("option %q not found on field %q; available options: %s", e.Name, e.FieldName, strings.Join(e.Candidates, ", "))
}

// OptionAmbiguousError is returned when more than one single-select option
// shares the given name. Candidates lists the option IDs of the matches.
type OptionAmbiguousError struct {
	FieldName  string
	Name       string
	Candidates []string
}

func (e *OptionAmbiguousError) Error() string {
	return fmt.Sprintf("option %q is ambiguous on field %q; multiple options share this name (use --single-select-option-id with one of: %s)", e.Name, e.FieldName, strings.Join(e.Candidates, ", "))
}

// WrongFieldTypeError is returned when a field resolves by name but its data
// type does not support the requested operation, e.g. resolving an option name
// on a field that is not a single-select field.
type WrongFieldTypeError struct {
	FieldName string
	DataType  string
	Expected  string
}

func (e *WrongFieldTypeError) Error() string {
	return fmt.Sprintf("field %q has data type %q, but %q was expected", e.FieldName, e.DataType, e.Expected)
}

// ItemNotInProjectError is returned when an issue or pull request exists but is
// not an item on the target project.
type ItemNotInProjectError struct {
	URL           string
	ProjectNumber int32
}

func (e *ItemNotInProjectError) Error() string {
	return fmt.Sprintf("%s is not an item in project %d; add it first with `gh project item-add`", e.URL, e.ProjectNumber)
}

// ResolveSingleSelectOptionID resolves optionName to its option ID on a
// single-select field. It returns a *WrongFieldTypeError if the field is not a
// single-select field, a *OptionNotFoundError if no option matches, and a
// *OptionAmbiguousError if more than one option shares the name.
func ResolveSingleSelectOptionID(field ProjectField, optionName string) (string, error) {
	if field.Type() != "ProjectV2SingleSelectField" {
		return "", &WrongFieldTypeError{
			FieldName: field.Name(),
			DataType:  field.DataType(),
			Expected:  "SINGLE_SELECT",
		}
	}

	var matches []string
	for _, o := range field.Options() {
		// Matching is case-insensitive to mirror the Projects v2 platform API,
		// whose options(names:) argument downcases before matching. Unlike field
		// names, option names are not guaranteed case-insensitively unique, so
		// EqualFold may match more than one option; that still fires the
		// OptionAmbiguousError path below.
		if strings.EqualFold(o.Name, optionName) {
			matches = append(matches, o.ID)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		names := make([]string, 0, len(field.Options()))
		for _, o := range field.Options() {
			names = append(names, o.Name)
		}
		return "", &OptionNotFoundError{FieldName: field.Name(), Name: optionName, Candidates: names}
	default:
		return "", &OptionAmbiguousError{FieldName: field.Name(), Name: optionName, Candidates: matches}
	}
}

// ResolveMultiSelectOptionIDs resolves each name in optionNames to its option ID
// on a multi-select field, preserving order. It returns a *WrongFieldTypeError
// if the field is not a multi-select field, and a *OptionNotFoundError or
// *OptionAmbiguousError for the first name that does not resolve to exactly one
// option. Matching is case-insensitive, mirroring ResolveSingleSelectOptionID.
func ResolveMultiSelectOptionIDs(field ProjectField, optionNames []string) ([]string, error) {
	if field.Type() != "ProjectV2MultiSelectField" {
		return nil, &WrongFieldTypeError{
			FieldName: field.Name(),
			DataType:  field.DataType(),
			Expected:  "MULTI_SELECT",
		}
	}

	ids := make([]string, 0, len(optionNames))
	for _, name := range optionNames {
		var matches []string
		for _, o := range field.Options() {
			if strings.EqualFold(o.Name, name) {
				matches = append(matches, o.ID)
			}
		}

		switch len(matches) {
		case 1:
			ids = append(ids, matches[0])
		case 0:
			names := make([]string, 0, len(field.Options()))
			for _, o := range field.Options() {
				names = append(names, o.Name)
			}
			return nil, &OptionNotFoundError{FieldName: field.Name(), Name: name, Candidates: names}
		default:
			return nil, &OptionAmbiguousError{FieldName: field.Name(), Name: name, Candidates: matches}
		}
	}
	return ids, nil
}

// projectItemByURL resolves an issue or pull request URL to the ID of its
// corresponding item on the project identified by projectID. It is used to let
// item-edit address an item by issue/PR URL instead of a project item ID.
type projectItemByURL struct {
	Resource struct {
		Typename string `graphql:"__typename"`
		Issue    struct {
			ProjectItems struct {
				Nodes []struct {
					ID      string
					Project struct {
						ID string
					}
				}
			} `graphql:"projectItems(first: $firstItems)"`
		} `graphql:"... on Issue"`
		PullRequest struct {
			ProjectItems struct {
				Nodes []struct {
					ID      string
					Project struct {
						ID string
					}
				}
			} `graphql:"projectItems(first: $firstItems)"`
		} `graphql:"... on PullRequest"`
	} `graphql:"resource(url: $url)"`
}

// ProjectItemIDByURL returns the project item ID for the issue or pull request
// at rawURL on the project identified by projectID. projectNumber is used only
// to build a helpful error message. It returns a *ItemNotInProjectError when the
// resource exists but is not an item on the project.
//
// The projectItems connection is read with a bounded page size; very large
// boards may exceed it.
func (c *Client) ProjectItemIDByURL(rawURL, projectID string, projectNumber int32) (string, error) {
	uri, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	variables := map[string]interface{}{
		"url":        githubv4.URI{URL: uri},
		"firstItems": githubv4.Int(LimitMax),
	}

	var query projectItemByURL
	if err := c.doQueryWithProgressIndicator("GetProjectItemByURL", &query, variables); err != nil {
		return "", err
	}

	var nodes []struct {
		ID      string
		Project struct {
			ID string
		}
	}
	switch query.Resource.Typename {
	case "Issue":
		nodes = query.Resource.Issue.ProjectItems.Nodes
	case "PullRequest":
		nodes = query.Resource.PullRequest.ProjectItems.Nodes
	default:
		return "", fmt.Errorf("resource not found, please check the URL: %s", rawURL)
	}

	for _, n := range nodes {
		if n.Project.ID == projectID {
			return n.ID, nil
		}
	}

	return "", &ItemNotInProjectError{URL: rawURL, ProjectNumber: projectNumber}
}
