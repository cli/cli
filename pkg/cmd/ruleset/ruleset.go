package ruleset

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdRuleset(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ruleset <command>",
		Short: "Manage repository and organization rulesets",
		Long: heredoc.Doc(`
			TODO
		`),
		Aliases: []string{"rs"},
		Example: "TODO",
	}

	cmdutil.EnableRepoOverride(cmd, f)
	// cmd.AddCommand(cmdList.NewCmdList(f, nil)
	// cmd.AddCommand(cmdList.NewCmdView(f, nil)
	// cmd.AddCommand(cmdCheck.NewCmdCheck(f, nil)

	return cmd
}
