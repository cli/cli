package acceptance_test

import "strings"

// parseScriptFilter splits a comma-separated GH_ACCEPTANCE_SCRIPT value into
// individual script names, trimming whitespace and ignoring empty entries.
func parseScriptFilter(raw string) []string {
	var scripts []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			scripts = append(scripts, s)
		}
	}
	return scripts
}
