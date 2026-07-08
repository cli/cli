package queries

import (
	"fmt"
	"strconv"
	"strings"
)

// DisplayValue returns a single-line, human-readable rendering of the field
// value suitable for a table cell. Multi-value fields (labels, assignees, etc.)
// are joined with commas. It is used by item-list to show a named field's value
// without a separate field-ID preflight lookup.
func (v FieldValueNodes) DisplayValue() string {
	switch data := projectFieldValueData(v).(type) {
	case nil:
		return ""
	case string:
		return data
	case float64:
		return strconv.FormatFloat(data, 'f', -1, 64)
	case []string:
		return strings.Join(data, ", ")
	case map[string]interface{}:
		if title, ok := data["title"].(string); ok {
			return title
		}
		return ""
	default:
		return fmt.Sprintf("%v", data)
	}
}
