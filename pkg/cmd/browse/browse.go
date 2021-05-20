package browse

import (
	"fmt"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdBrowse(f *cmdutil.Factory) *cobra.Command {

	cmd := &cobra.Command{
		Long:  "Work with GitHub in the browser", // displays when you are on the help page of this command
		Short: "Open GitHub in the browser",      // displays in the gh root help
		Use:   "browse",                          // necessary!!! This is the cmd that gets passed on the prompt
		Args:  cobra.RangeArgs(0, 1),             // make sure only one arg at most is passed
		Run: func(cmd *cobra.Command, args []string) {
			openInBrowser(cmd, f) // run gets rid of the usage / runs function
		},
	}

	return cmd
}

func openInBrowser(cmd *cobra.Command, f *cmdutil.Factory) {
	// args := cmd.Args
	baseRepo, err := f.BaseRepo()
	if err != nil {
		fmt.Println("error")
		return
	}
	// ATTN: add into the empty string where you want to go within the repo
	repoUrl := ghrepo.GenerateRepoURL(baseRepo, "")
	f.Browser.Browse(repoUrl)
}

// Questions for tomorrow:
// if logged in / repo
// 	go to repo
// else if logged in / !repo
// 	go to profile
// else
// 	? throw error / go to github home page
