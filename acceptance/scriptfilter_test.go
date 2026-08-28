package acceptance_test

import (
	"os"
	"path"
	"strings"
)

// parseScriptFilter splits a comma-separated GH_ACCEPTANCE_SCRIPT value into
// individual script names, trimming whitespace and ignoring empty entries.
func parseScriptFilter(raw string) []string {
	var scripts []string
	for s := range strings.SplitSeq(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			scripts = append(scripts, s)
		}
	}
	return scripts
}

// selectScripts returns the script files under testdata/command that match the
// requested names, and reports whether a filter was applied (i.e. scripts is
// non-empty). A named script not found in the directory is silently ignored
// because it belongs to another command directory in the same run.
func selectScripts(command string, scripts []string) (files []string, filtered bool) {
	if len(scripts) == 0 {
		return nil, false
	}
	for _, script := range scripts {
		p := path.Join("testdata", command, script)
		if _, err := os.Stat(p); err == nil {
			files = append(files, p)
		}
	}
	return files, true
}
