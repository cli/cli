package label

import (
	"time"

	"github.com/cli/cli/v2/pkg/cmdutil"
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
	ID          string    `json:"id"`
	IsDefault   bool      `json:"isDefault"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (l *label) ExportData(fields []string) map[string]interface{} {
	return cmdutil.StructExportData(l, fields)
}
