package project

import (
	"github.com/MakeNowJust/heredoc"
	cmdClose "github.com/cli/cli/v2/pkg/cmd/project/close"
	cmdCopy "github.com/cli/cli/v2/pkg/cmd/project/copy"
	cmdCreate "github.com/cli/cli/v2/pkg/cmd/project/create"
	cmdDelete "github.com/cli/cli/v2/pkg/cmd/project/delete"
	cmdEdit "github.com/cli/cli/v2/pkg/cmd/project/edit"
	cmdFieldCreate "github.com/cli/cli/v2/pkg/cmd/project/field-create"
	cmdFieldDelete "github.com/cli/cli/v2/pkg/cmd/project/field-delete"
	cmdFieldList "github.com/cli/cli/v2/pkg/cmd/project/field-list"
	cmdItemAdd "github.com/cli/cli/v2/pkg/cmd/project/item-add"
	cmdItemArchive "github.com/cli/cli/v2/pkg/cmd/project/item-archive"
	cmdItemCreate "github.com/cli/cli/v2/pkg/cmd/project/item-create"
	cmdItemDelete "github.com/cli/cli/v2/pkg/cmd/project/item-delete"
	cmdItemEdit "github.com/cli/cli/v2/pkg/cmd/project/item-edit"
	cmdItemList "github.com/cli/cli/v2/pkg/cmd/project/item-list"
	cmdLink "github.com/cli/cli/v2/pkg/cmd/project/link"
	cmdList "github.com/cli/cli/v2/pkg/cmd/project/list"
	cmdTemplate "github.com/cli/cli/v2/pkg/cmd/project/mark-template"
	cmdUnlink "github.com/cli/cli/v2/pkg/cmd/project/unlink"
	cmdView "github.com/cli/cli/v2/pkg/cmd/project/view"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdProject(f *cmdutil.Factory) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "project <command>",
		Short: "Work with GitHub Projects.",
		Long:  "Work with GitHub Projects. Note that the token you are using must have 'project' scope, which is not set by default. You can verify your token scope by running 'gh auth status' and add the project scope by running 'gh auth refresh -s project'.",
		Example: heredoc.Doc(`
			$ gh project create --owner monalisa --title "Roadmap"
			$ gh project view 1 --owner cli --web
			$ gh project field-list 1 --owner cli
			$ gh project item-list 1 --owner cli
		`),
		GroupID: "core",
	}

	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	cmd.AddCommand(cmdCreate.NewCmdCreate(f, nil))
	cmd.AddCommand(cmdCopy.NewCmdCopy(f, nil))
	cmd.AddCommand(cmdClose.NewCmdClose(f, nil))
	cmd.AddCommand(cmdDelete.NewCmdDelete(f, nil))
	cmd.AddCommand(cmdEdit.NewCmdEdit(f, nil))
	cmd.AddCommand(cmdLink.NewCmdLink(f, nil))
	cmd.AddCommand(cmdView.NewCmdView(f, nil))
	cmd.AddCommand(cmdTemplate.NewCmdMarkTemplate(f, nil))
	cmd.AddCommand(cmdUnlink.NewCmdUnlink(f, nil))

	// items
	cmd.AddCommand(cmdItemList.NewCmdList(f, nil))
	cmd.AddCommand(cmdItemCreate.NewCmdCreateItem(f, nil))
	cmd.AddCommand(cmdItemAdd.NewCmdAddItem(f, nil))
	cmd.AddCommand(cmdItemEdit.NewCmdEditItem(f, nil))
	cmd.AddCommand(cmdItemArchive.NewCmdArchiveItem(f, nil))
	cmd.AddCommand(cmdItemDelete.NewCmdDeleteItem(f, nil))

	// fields
	cmd.AddCommand(cmdFieldList.NewCmdList(f, nil))
	cmd.AddCommand(cmdFieldCreate.NewCmdCreateField(f, nil))
	cmd.AddCommand(cmdFieldDelete.NewCmdDeleteField(f, nil))

	return cmd
}
