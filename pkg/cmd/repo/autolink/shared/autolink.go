package shared

import "github.com/cli/cli/v2/pkg/cmdutil"

type Autolink struct {
	ID             int    `json:"id"`
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

func (a *Autolink) ExportData(fields []string) map[string]interface{} {
	return cmdutil.StructExportData(a, fields)
}
