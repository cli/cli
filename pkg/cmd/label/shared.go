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

type label struct {
	Color       string    `json:"color"`
	CreatedAt   time.Time `json:"createdAt"`
	Description string    `json:"description"`
	ID          string    `json:"node_id"`
	IsDefault   bool      `json:"isDefault"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	UpdatedAt   time.Time `json:"updatedAt"`
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
