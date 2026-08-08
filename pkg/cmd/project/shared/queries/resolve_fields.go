package queries

import (
	"fmt"
	"sort"
	"strings"
)

// This file holds the ProjectV2 field-by-name and field-by-ID resolvers plus
// their error types. It is intentionally self-contained (only fmt/sort/strings
// and ProjectField.Name()/ID()) and is shared verbatim by the item-edit and
// item-list commands, which ship in separate pull requests. Keeping this file
// byte-identical in both lets Git auto-merge it cleanly (an "added by both"
// identical file dedupes to a single copy), so the two PRs can land in any
// order without a duplicate-symbol conflict.

// FieldIDNotFoundError is returned when no project field matches the given node
// ID. Candidates lists the available fields as "name (id)" pairs so the caller
// can self-correct, keeping the ID path as actionable as the name path.
type FieldIDNotFoundError struct {
	ID         string
	Candidates []string
}

func (e *FieldIDNotFoundError) Error() string {
	if len(e.Candidates) == 0 {
		return fmt.Sprintf("field with ID %q not found in project; the project has no fields", e.ID)
	}
	return fmt.Sprintf("field with ID %q not found in project; available fields: %s", e.ID, strings.Join(e.Candidates, ", "))
}

// FieldNotFoundError is returned when no project field matches the given name.
// Candidates lists the available field names so the caller can self-correct.
type FieldNotFoundError struct {
	Name       string
	Candidates []string
}

func (e *FieldNotFoundError) Error() string {
	if len(e.Candidates) == 0 {
		return fmt.Sprintf("field %q not found in project; the project has no fields", e.Name)
	}
	return fmt.Sprintf("field %q not found in project; available fields: %s", e.Name, strings.Join(e.Candidates, ", "))
}

// FieldAmbiguousError is returned when more than one field shares the given
// name. Candidates lists the node IDs of the matching fields.
type FieldAmbiguousError struct {
	Name       string
	Candidates []string
}

func (e *FieldAmbiguousError) Error() string {
	return fmt.Sprintf("field %q is ambiguous; multiple fields share this name (use --field-id with one of: %s)", e.Name, strings.Join(e.Candidates, ", "))
}

// ResolveFieldByName finds the single project field whose name matches name,
// case-insensitively. Matching is case-insensitive to mirror the Projects v2
// platform API, which normalizes field names via a name_slug
// (name.downcase.gsub(" ","-")) and so guarantees field names are
// case-insensitively unique; EqualFold can therefore match at most one field.
// It returns a *FieldNotFoundError when nothing matches and a
// *FieldAmbiguousError when more than one field shares the name, each carrying
// candidates so the error is self-correctable.
func ResolveFieldByName(fields []ProjectField, name string) (ProjectField, error) {
	var matches []ProjectField
	for _, f := range fields {
		if strings.EqualFold(f.Name(), name) {
			matches = append(matches, f)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		names := make([]string, 0, len(fields))
		for _, f := range fields {
			names = append(names, f.Name())
		}
		sort.Strings(names)
		return ProjectField{}, &FieldNotFoundError{Name: name, Candidates: names}
	default:
		ids := make([]string, 0, len(matches))
		for _, f := range matches {
			ids = append(ids, f.ID())
		}
		sort.Strings(ids)
		return ProjectField{}, &FieldAmbiguousError{Name: name, Candidates: ids}
	}
}

// ResolveFieldByID finds the project field whose node ID matches id, validating
// it against the fields returned with the project so an unknown ID fails with a
// candidate-carrying *FieldIDNotFoundError instead of silently rendering an
// empty column. Node IDs are matched exactly (they are case-sensitive).
func ResolveFieldByID(fields []ProjectField, id string) (ProjectField, error) {
	for _, f := range fields {
		if f.ID() == id {
			return f, nil
		}
	}
	candidates := make([]string, 0, len(fields))
	for _, f := range fields {
		candidates = append(candidates, fmt.Sprintf("%s (%s)", f.Name(), f.ID()))
	}
	sort.Strings(candidates)
	return ProjectField{}, &FieldIDNotFoundError{ID: id, Candidates: candidates}
}
