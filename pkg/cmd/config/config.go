package config

import (
	"fmt"
	"strings"

	"github.com/cli/cli/v2/internal/config"
	cmdClearCache "github.com/cli/cli/v2/pkg/cmd/config/clear-cache"
	cmdGet "github.com/cli/cli/v2/pkg/cmd/config/get"
	cmdList "github.com/cli/cli/v2/pkg/cmd/config/list"
	cmdSet "github.com/cli/cli/v2/pkg/cmd/config/set"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdConfig(f *cmdutil.Factory) *cobra.Command {
	longDoc := strings.Builder{}
	longDoc.WriteString("Display or change configuration settings for gh.\n\n")
	longDoc.WriteString("Current respected settings:\n")
	for _, co := range config.Options {
		longDoc.WriteString(fmt.Sprintf("- `%s`: %s", co.Key, co.Description))
		if len(co.AllowedValues) > 0 {
			longDoc.WriteString(fmt.Sprintf(" `{%s}`", strings.Join(co.AllowedValues, " | ")))
		}
		if co.DefaultValue != "" {
			longDoc.WriteString(fmt.Sprintf(" (default `%s`)", co.DefaultValue))
		}
		longDoc.WriteRune('\n')
	}

	longDoc.WriteString(strings.TrimLeft(`
Context-scoped account selection:

The `+"`account_rules`"+` key lets gh automatically choose which authenticated account
to act as, based on the current working directory or the repository owner, instead of
the single globally active account. This is useful when multiple accounts share a host
(for example an Enterprise Managed User and a personal account on github.com). Rules are
edited directly in `+"`config.yml`"+` and take the following shape:

    account_rules:
      gitdir:
        ~/work/: octocat_acme@github.com     # any repo under ~/work uses this account
        ~/personal/: octocat@github.com
      owner:
        acme-corp: octocat_acme@github.com   # any repo owned by acme-corp uses this account
        octocat: octocat@github.com

Account values are `+"`user`"+` or `+"`user@host`"+`. When several rules apply, an owner rule wins
over a gitdir rule, and the longest matching directory prefix wins. Resolution never
changes the globally active account. Environment tokens (GH_TOKEN etc.) take precedence
over rules, followed by the `+"`--account`"+` flag / GH_ACCOUNT, then rules, then the active
account. Run `+"`gh auth status`"+` to see which account resolves for the current directory.
`, "\n"))

	cmd := &cobra.Command{
		Use:   "config <command>",
		Short: "Manage configuration for gh",
		Long:  longDoc.String(),
	}

	cmdutil.DisableAuthCheck(cmd)

	cmd.AddCommand(cmdGet.NewCmdConfigGet(f, nil))
	cmd.AddCommand(cmdSet.NewCmdConfigSet(f, nil))
	cmd.AddCommand(cmdList.NewCmdConfigList(f, nil))
	cmd.AddCommand(cmdClearCache.NewCmdConfigClearCache(f, nil))

	return cmd
}
