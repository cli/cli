package cmdutil

import (
	"os"
)

func AdvancedIssueSearchEnabled(f *Factory) bool {
	advancedSearch := os.Getenv("GH_ADVANCED_ISSUE_SEARCH")

	if advancedSearch == "" {
		config, err := f.Config()
		if err != nil {
			advancedSearch = "disabled"
		} else {
			advancedSearch = config.AdvancedIssueSearch("").Value
		}
	}

	return advancedSearch == "enabled"
}
