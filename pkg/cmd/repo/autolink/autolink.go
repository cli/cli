package autolink

import (
	"github.com/MakeNowJust/heredoc"
	cmdList "github.com/cli/cli/v2/pkg/cmd/repo/autolink/list"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdAutolink(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "autolink <command>",
		Short: "Manage autolink references",
		Long: heredoc.Docf(`
		Work with GitHub autolink references.
		
		GitHub autolinks require admin access to configure and can be found at
		https://github.com/{owner}/{repo}/settings/key_links.
		Use %[1]sgh repo autolink list --web%[1]s to open this page for the current repository.
		
		For more information about GitHub autolinks, see https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/managing-repository-settings/configuring-autolinks-to-reference-external-resources
	`, "`"),
	}
	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(cmdList.NewCmdList(f, nil))

	return cmd
}
