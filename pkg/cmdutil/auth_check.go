package cmdutil

import (
	"github.com/cli/cli/v2/internal/config"
	"github.com/spf13/cobra"
)

func DisableAuthCheck(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}

	cmd.Annotations["skipAuthCheck"] = "true"
}

func CheckAuth(cfg config.Config) bool {
	if cfg.Authentication().HasEnvToken() {
		return true
	}

	if len(cfg.Authentication().Hosts()) > 0 {
		return true
	}

	return false
}

func IsAuthCheckEnabled(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "help", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
		return false
	}

	for c := cmd; c.Parent() != nil; c = c.Parent() {
		if c.Annotations != nil && c.Annotations["skipAuthCheck"] == "true" {
			return false
		}
	}

	return true
}
