package autolink

import (
	"github.com/MakeNowJust/heredoc"
	cmdCreate "github.com/cli/cli/v2/pkg/cmd/repo/autolink/create"
	cmdList "github.com/cli/cli/v2/pkg/cmd/repo/autolink/list"
	cmdView "github.com/cli/cli/v2/pkg/cmd/repo/autolink/view"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdAutolink(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "autolink <command>",
		Short: "Manage autolink references",
		Long: heredoc.Docf(`
			Autolinks link issues, pull requests, commit messages, and release descriptions to external third-party services.

			Autolinks require %[1]sadmin%[1]s role to view or manage.

			For more information, see <https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/managing-repository-settings/configuring-autolinks-to-reference-external-resources>
		`, "`"),
	}
	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	cmd.AddCommand(cmdCreate.NewCmdCreate(f, nil))
	cmd.AddCommand(cmdView.NewCmdView(f, nil))

	return cmd
}
