package queries

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// multiSelectField builds a ProjectV2MultiSelectField fixture with the given id,
// name, and options.
func multiSelectField(id, name string, options []SingleSelectFieldOptions) ProjectField {
	f := ProjectField{TypeName: "ProjectV2MultiSelectField"}
	f.MultiSelectField.ID = id
	f.MultiSelectField.Name = name
	f.MultiSelectField.DataType = "MULTI_SELECT"
	f.MultiSelectField.Options = options
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

	t.Run("ambiguous name lists candidate ids in sorted order", func(t *testing.T) {
		dup := append([]ProjectField{}, fields...)
		dup = append(dup, textField("PVTF_status2", "Status"))
		_, err := ResolveFieldByName(dup, "Status")
		require.Error(t, err)
		var amb *FieldAmbiguousError
		require.True(t, errors.As(err, &amb))
		assert.Equal(t, []string{"PVTF_status2", "PVTSSF_status"}, amb.Candidates)
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
