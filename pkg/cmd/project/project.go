package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/cli/cli/v2/pkg/cmd/factory"
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
	cmdList "github.com/cli/cli/v2/pkg/cmd/project/list"
	cmdView "github.com/cli/cli/v2/pkg/cmd/project/view"
	"github.com/spf13/cobra"
)

// analogous to cli/pkg/cmd/pr.go in cli/cli
func main() {
	var rootCmd = &cobra.Command{
		Use:           "project",
		Short:         "Work with GitHub Projects.",
		Long:          "Work with GitHub Projects. Note that the token you are using must have 'project' scope, which is not set by default. You can verify your token scope by running 'gh auth status' and add the project scope by running 'gh auth refresh -s project'.",
		SilenceErrors: true,
	}

	cmdFactory := factory.New("0.1.0") // will be replaced by buildVersion := build.Version

	rootCmd.AddCommand(cmdList.NewCmdList(cmdFactory, nil))
	rootCmd.AddCommand(cmdCreate.NewCmdCreate(cmdFactory, nil))
	rootCmd.AddCommand(cmdCopy.NewCmdCopy(cmdFactory, nil))
	rootCmd.AddCommand(cmdClose.NewCmdClose(cmdFactory, nil))
	rootCmd.AddCommand(cmdDelete.NewCmdDelete(cmdFactory, nil))
	rootCmd.AddCommand(cmdEdit.NewCmdEdit(cmdFactory, nil))
	rootCmd.AddCommand(cmdView.NewCmdView(cmdFactory, nil))

	// items
	rootCmd.AddCommand(cmdItemList.NewCmdList(cmdFactory, nil))
	rootCmd.AddCommand(cmdItemCreate.NewCmdCreateItem(cmdFactory, nil))
	rootCmd.AddCommand(cmdItemAdd.NewCmdAddItem(cmdFactory, nil))
	rootCmd.AddCommand(cmdItemEdit.NewCmdEditItem(cmdFactory, nil))
	rootCmd.AddCommand(cmdItemArchive.NewCmdArchiveItem(cmdFactory, nil))
	rootCmd.AddCommand(cmdItemDelete.NewCmdDeleteItem(cmdFactory, nil))

	// fields
	rootCmd.AddCommand(cmdFieldList.NewCmdList(cmdFactory, nil))
	rootCmd.AddCommand(cmdFieldCreate.NewCmdCreateField(cmdFactory, nil))
	rootCmd.AddCommand(cmdFieldDelete.NewCmdDeleteField(cmdFactory, nil))

	if err := rootCmd.Execute(); err != nil {
		if strings.HasPrefix(err.Error(), "Message: Your token has not been granted the required scopes to execute this query") {
			fmt.Println("Your token has not been granted the required scopes to execute this query.\nRun 'gh auth refresh -s project' to add the 'project' scope.\nRun 'gh auth status' to see your current token scopes.")
			os.Exit(1)
		}
		fmt.Println(err)
		os.Exit(1)
	}
}
