package queries

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/h2non/gock.v1"
)

// textField builds a ProjectV2Field (TEXT) fixture with the given id and name.
func textField(id, name string) ProjectField {
	f := ProjectField{TypeName: "ProjectV2Field"}
	f.Field.ID = id
	f.Field.Name = name
	f.Field.DataType = "TEXT"
	return f
}

// singleSelectField builds a ProjectV2SingleSelectField fixture with the given
// id, name, and options (alternating id/name pairs are passed as a slice).
func singleSelectField(id, name string, options []SingleSelectFieldOptions) ProjectField {
	f := ProjectField{TypeName: "ProjectV2SingleSelectField"}
	f.SingleSelectField.ID = id
	f.SingleSelectField.Name = name
	f.SingleSelectField.DataType = "SINGLE_SELECT"
	f.SingleSelectField.Options = options
	return f
}

func TestResolveFieldByName(t *testing.T) {
	fields := []ProjectField{
		textField("PVTF_title", "Title"),
		singleSelectField("PVTSSF_status", "Status", []SingleSelectFieldOptions{
			{ID: "opt_todo", Name: "Todo"},
			{ID: "opt_inprog", Name: "In Progress"},
		}),
		textField("PVTF_prio", "Priority"),
	}

	t.Run("resolves an exact match", func(t *testing.T) {
		got, err := ResolveFieldByName(fields, "Status")
		require.NoError(t, err)
		assert.Equal(t, "PVTSSF_status", got.ID())
		assert.Equal(t, "SINGLE_SELECT", got.DataType())
	})

	t.Run("resolves case-insensitively", func(t *testing.T) {
		got, err := ResolveFieldByName(fields, "status")
		require.NoError(t, err)
		assert.Equal(t, "PVTSSF_status", got.ID())
	})

	t.Run("not found lists candidate field names", func(t *testing.T) {
		_, err := ResolveFieldByName(fields, "Statuss")
		require.Error(t, err)
		var nf *FieldNotFoundError
		require.True(t, errors.As(err, &nf))
		assert.Equal(t, "Statuss", nf.Name)
		assert.Equal(t, []string{"Priority", "Status", "Title"}, nf.Candidates)
		assert.Contains(t, err.Error(), "available fields: Priority, Status, Title")
	})

	t.Run("ambiguous name lists candidate ids", func(t *testing.T) {
		dup := append([]ProjectField{}, fields...)
		dup = append(dup, textField("PVTF_status2", "Status"))
		_, err := ResolveFieldByName(dup, "Status")
		require.Error(t, err)
		var amb *FieldAmbiguousError
		require.True(t, errors.As(err, &amb))
		assert.ElementsMatch(t, []string{"PVTSSF_status", "PVTF_status2"}, amb.Candidates)
		assert.Contains(t, err.Error(), "is ambiguous")
	})
}

func TestResolveFieldByID(t *testing.T) {
	fields := []ProjectField{
		textField("PVTF_title", "Title"),
		singleSelectField("PVTSSF_status", "Status", nil),
		textField("PVTF_prio", "Priority"),
	}

	t.Run("resolves an exact id match", func(t *testing.T) {
		got, err := ResolveFieldByID(fields, "PVTSSF_status")
		require.NoError(t, err)
		assert.Equal(t, "Status", got.Name())
	})

	t.Run("not found lists candidate name (id) pairs", func(t *testing.T) {
		_, err := ResolveFieldByID(fields, "PVTF_missing")
		require.Error(t, err)
		var nf *FieldIDNotFoundError
		require.True(t, errors.As(err, &nf))
		assert.Equal(t, "PVTF_missing", nf.ID)
		assert.Equal(t, []string{"Priority (PVTF_prio)", "Status (PVTSSF_status)", "Title (PVTF_title)"}, nf.Candidates)
		assert.Contains(t, err.Error(), "available fields: Priority (PVTF_prio), Status (PVTSSF_status), Title (PVTF_title)")
	})
}

func TestResolveSingleSelectOptionID(t *testing.T) {
	status := singleSelectField("PVTSSF_status", "Status", []SingleSelectFieldOptions{
		{ID: "opt_todo", Name: "Todo"},
		{ID: "opt_inprog", Name: "In Progress"},
		{ID: "opt_done", Name: "Done"},
	})

	t.Run("resolves an option name to its id", func(t *testing.T) {
		id, err := ResolveSingleSelectOptionID(status, "In Progress")
		require.NoError(t, err)
		assert.Equal(t, "opt_inprog", id)
	})

	t.Run("resolves an option name case-insensitively", func(t *testing.T) {
		id, err := ResolveSingleSelectOptionID(status, "in progress")
		require.NoError(t, err)
		assert.Equal(t, "opt_inprog", id)
	})

	t.Run("case-insensitive match that hits multiple options is ambiguous", func(t *testing.T) {
		dup := singleSelectField("PVTSSF_status", "Status", []SingleSelectFieldOptions{
			{ID: "opt_lower", Name: "done"},
			{ID: "opt_upper", Name: "Done"},
		})
		_, err := ResolveSingleSelectOptionID(dup, "DONE")
		require.Error(t, err)
		var amb *OptionAmbiguousError
		require.True(t, errors.As(err, &amb))
		assert.ElementsMatch(t, []string{"opt_lower", "opt_upper"}, amb.Candidates)
	})

	t.Run("not found lists candidate option names", func(t *testing.T) {
		_, err := ResolveSingleSelectOptionID(status, "In progres")
		require.Error(t, err)
		var nf *OptionNotFoundError
		require.True(t, errors.As(err, &nf))
		assert.Equal(t, "Status", nf.FieldName)
		assert.Equal(t, []string{"Todo", "In Progress", "Done"}, nf.Candidates)
		assert.Contains(t, err.Error(), "available options: Todo, In Progress, Done")
	})

	t.Run("ambiguous option lists candidate ids", func(t *testing.T) {
		dup := singleSelectField("PVTSSF_status", "Status", []SingleSelectFieldOptions{
			{ID: "opt_a", Name: "Done"},
			{ID: "opt_b", Name: "Done"},
		})
		_, err := ResolveSingleSelectOptionID(dup, "Done")
		require.Error(t, err)
		var amb *OptionAmbiguousError
		require.True(t, errors.As(err, &amb))
		assert.ElementsMatch(t, []string{"opt_a", "opt_b"}, amb.Candidates)
	})

	t.Run("wrong field type is rejected", func(t *testing.T) {
		_, err := ResolveSingleSelectOptionID(textField("PVTF_title", "Title"), "In Progress")
		require.Error(t, err)
		var wt *WrongFieldTypeError
		require.True(t, errors.As(err, &wt))
		assert.Equal(t, "TEXT", wt.DataType)
		assert.Equal(t, "SINGLE_SELECT", wt.Expected)
	})
}

func TestProjectItemIDByURL(t *testing.T) {
	t.Run("returns the item id for the matching project", func(t *testing.T) {
		defer gock.Off()
		gock.New("https://api.github.com").
			Post("/graphql").
			Reply(200).
			JSON(map[string]interface{}{
				"data": map[string]interface{}{
					"resource": map[string]interface{}{
						"__typename": "Issue",
						"projectItems": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{"id": "PVTI_other", "project": map[string]interface{}{"id": "PVT_other"}},
								{"id": "PVTI_match", "project": map[string]interface{}{"id": "PVT_target"}},
							},
						},
					},
				},
			})

		client := NewTestClient()
		id, err := client.ProjectItemIDByURL("https://github.com/monalisa/repo/issues/1", "PVT_target", 5)
		require.NoError(t, err)
		assert.Equal(t, "PVTI_match", id)
	})

	t.Run("errors when the resource is not an item on the project", func(t *testing.T) {
		defer gock.Off()
		gock.New("https://api.github.com").
			Post("/graphql").
			Reply(200).
			JSON(map[string]interface{}{
				"data": map[string]interface{}{
					"resource": map[string]interface{}{
						"__typename": "Issue",
						"projectItems": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{"id": "PVTI_other", "project": map[string]interface{}{"id": "PVT_other"}},
							},
						},
					},
				},
			})

		client := NewTestClient()
		_, err := client.ProjectItemIDByURL("https://github.com/monalisa/repo/issues/1", "PVT_target", 5)
		require.Error(t, err)
		var nip *ItemNotInProjectError
		require.True(t, errors.As(err, &nip))
		assert.Contains(t, err.Error(), "is not an item in project 5")
	})
}
