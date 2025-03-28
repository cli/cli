package cmdutil

import (
	"os"
)

func AdvancedIssueSearchEnabled(f *Factory) bool {
	envVar, set := os.LookupEnv("GH_ADVANCED_ISSUE_SEARCH")
	if set {
		switch envVar {
		case "", "0", "disabled", "false":
			return false
		default:
			return true
		}
	}

	var configValue string
	config, err := f.Config()
	if err != nil {
		configValue = "disabled"
	} else {
		configValue = config.AdvancedIssueSearch("").Value
	}

	return configValue == "enabled"
}
