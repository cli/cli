package shared

import "github.com/cli/cli/v2/pkg/cmdutil"

type Autolink struct {
	ID             int64  `json:"id"`
	IsAlphanumeric bool   `json:"is_alphanumeric"`
	KeyPrefix      string `json:"key_prefix"`
	URLTemplate    string `json:"url_template"`
}

var AutolinkFields = []string{
	"id",
	"isAlphanumeric",
	"keyPrefix",
	"urlTemplate",
}

func (a *Autolink) ExportData(fields []string) map[string]any {
	return cmdutil.StructExportData(a, fields)
}
