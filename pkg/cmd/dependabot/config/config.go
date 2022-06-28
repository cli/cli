package config

import (
	"github.com/cli/cli/v2/pkg/cmd/dependabot/config/create"
	"github.com/cli/cli/v2/pkg/cmd/dependabot/config/validate"
	"github.com/cli/cli/v2/pkg/cmd/pr/view"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdConfig(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config <command>",
		Short: "Manage Dependabot Configuration",
		Long:  `Create and validate configuration options for Dependabot.`,
	}
	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(validate.NewCmdValidate(f, nil))
	cmd.AddCommand(create.NewCmdCreate(f, nil))
	cmd.AddCommand(view.NewCmdView(f, nil))

	return cmd
}
