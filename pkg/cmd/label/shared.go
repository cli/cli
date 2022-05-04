package label

import (
	"reflect"
	"strings"
	"time"
)

var labelFields = []string{
	"color",
	"createdAt",
	"description",
	"id",
	"isDefault",
	"name",
	"updatedAt",
	"url",
}

// TODO: Should this be moved the api package like other exportables?
type label struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description,omitempty"`

	ID        string
	CreatedAt time.Time
	UpdatedAt time.Time
	IsDefault bool
	URL       string
}

// ExportData implements cmdutil.exportable
func (l *label) ExportData(fields []string) map[string]interface{} {
	v := reflect.ValueOf(l).Elem()
	data := map[string]interface{}{}

	for _, f := range fields {
		switch f {
		default:
			sf := fieldByName(v, f)
			data[f] = sf.Interface()
		}
	}

	return data
}

func fieldByName(v reflect.Value, field string) reflect.Value {
	return v.FieldByNameFunc(func(s string) bool {
		return strings.EqualFold(field, s)
	})
}
