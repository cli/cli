package browse

import (
	"fmt"
	"os"

	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

func NewCmdBrowse(f *cmdutil.Factory) *cobra.Command {

	cmd := &cobra.Command{
		Long: "Hello world", // displays when you are on the help page of this command
		Short: "Open GitHub in the browser", // displays in the gh root help 
		Use:    "browse", // necessary!!! This is the cmd that gets passed on the prompt
		Run: func(cmd *cobra.Command, args []string) { // run gets rid of the usage
			openInBrowser()
 	   	},
	}

	cmdutil.EnableRepoOverride(cmd, f)

	return cmd
}

func openInBrowser() {
	fmt.Println("");
	IO,_,_,_ := iostreams.Test() // why is this called test? is it to be used for the final product?
	browser := cmdutil.NewBrowser(os.Getenv("BROWSER"), IO.Out, IO.ErrOut)
	browser.Browse("www.github.com")
	
}