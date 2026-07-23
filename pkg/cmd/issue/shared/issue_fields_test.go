package shared

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveIssueFieldValueInput(t *testing.T) {
	fields := []issueFieldDefinition{
		{ID: "IF_text", Name: "Team", DataType: "TEXT"},
		{ID: "IF_num", Name: "Story Points", DataType: "NUMBER"},
		{ID: "IF_date", Name: "Due", DataType: "DATE"},
		{ID: "IF_ss", Name: "Priority", DataType: "SINGLE_SELECT", Options: []issueFieldOption{
			{ID: "opt_hi", Name: "High"},
			{ID: "opt_lo", Name: "Low"},
		}},
		{ID: "IF_ms", Name: "Platforms", DataType: "MULTI_SELECT", Options: []issueFieldOption{
			{ID: "opt_ios", Name: "iOS"},
			{ID: "opt_web", Name: "Web"},
		}},
	}

	t.Run("text by name", func(t *testing.T) {
		got, err := resolveIssueFieldValueInput(fields, "Team", "", "Platform")
		require.NoError(t, err)
		assert.Equal(t, "IF_text", got.FieldID)
		require.NotNil(t, got.TextValue)
		assert.Equal(t, "Platform", *got.TextValue)
	})

	t.Run("number by name", func(t *testing.T) {
		got, err := resolveIssueFieldValueInput(fields, "Story Points", "", "5")
		require.NoError(t, err)
		require.NotNil(t, got.NumberValue)
		assert.Equal(t, float64(5), *got.NumberValue)
	})

	t.Run("date by name", func(t *testing.T) {
		got, err := resolveIssueFieldValueInput(fields, "Due", "", "2026-01-02")
		require.NoError(t, err)
		require.NotNil(t, got.DateValue)
		assert.Equal(t, "2026-01-02", *got.DateValue)
	})

	t.Run("single-select resolves option name case-insensitively", func(t *testing.T) {
		got, err := resolveIssueFieldValueInput(fields, "Priority", "", "high")
		require.NoError(t, err)
		require.NotNil(t, got.SingleSelectOptionID)
		assert.Equal(t, "opt_hi", *got.SingleSelectOptionID)
	})

	t.Run("multi-select resolves comma-separated option names in order", func(t *testing.T) {
		got, err := resolveIssueFieldValueInput(fields, "Platforms", "", "iOS, Web")
		require.NoError(t, err)
		require.NotNil(t, got.MultiSelectOptionIDs)
		assert.Equal(t, []string{"opt_ios", "opt_web"}, *got.MultiSelectOptionIDs)
	})

	t.Run("field by id", func(t *testing.T) {
		got, err := resolveIssueFieldValueInput(fields, "", "IF_text", "hi")
		require.NoError(t, err)
		assert.Equal(t, "IF_text", got.FieldID)
		require.NotNil(t, got.TextValue)
		assert.Equal(t, "hi", *got.TextValue)
	})

	t.Run("field not found lists candidates", func(t *testing.T) {
		_, err := resolveIssueFieldValueInput(fields, "Nope", "", "x")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `issue field "Nope" not found`)
		assert.Contains(t, err.Error(), "Team")
	})

	t.Run("unknown field id", func(t *testing.T) {
		_, err := resolveIssueFieldValueInput(fields, "", "IF_missing", "x")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `no issue field found with ID "IF_missing"`)
	})

	t.Run("option not found lists candidates", func(t *testing.T) {
		_, err := resolveIssueFieldValueInput(fields, "Priority", "", "Nope")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `option "Nope" not found`)
		assert.Contains(t, err.Error(), "High, Low")
	})

	t.Run("invalid number", func(t *testing.T) {
		_, err := resolveIssueFieldValueInput(fields, "Story Points", "", "abc")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid number value")
	})
}
