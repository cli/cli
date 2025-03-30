package cmdutil

import (
	"reflect"

	"github.com/cli/cli/v2/internal/gh"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const skipAuthCheckAnnotation = "skipAuthCheck"

func DisableAuthCheck(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}

	cmd.Annotations[skipAuthCheckAnnotation] = "true"
}

func DisableAuthCheckFlag(flag *pflag.Flag) {
	if flag.Annotations == nil {
		flag.Annotations = map[string][]string{}
	}

	flag.Annotations[skipAuthCheckAnnotation] = []string{"true"}
}

func CheckAuth(cfg gh.Config) bool {
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
		// Check whether any command marked as DisableAuthCheck is set
		if c.Annotations != nil && c.Annotations[skipAuthCheckAnnotation] == "true" {
			return false
		}

		// Check whether any flag marked as DisableAuthCheckFlag is set
		var skipAuthCheck bool
		c.Flags().Visit(func(f *pflag.Flag) {
			if f.Annotations != nil && reflect.DeepEqual(f.Annotations[skipAuthCheckAnnotation], []string{"true"}) {
				skipAuthCheck = true
			}
		})
		if skipAuthCheck {
			return false
		}
	}

	return true
}
