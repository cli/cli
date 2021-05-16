package browse

import (
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdBrowse(f *cmdutil.Factory) *cobra.Command {

	cmd := &cobra.Command{
		Long: "Hello world", // displays when you are on the help page of this command
		Short: "Open GitHub in the browser", // displays in the gh root help 
		Use:    "browse", // necessary!!! This is the cmd that gets passed on the prompt
		Run: func(cmd *cobra.Command, args []string) { // run gets rid of the usage / runs function
			openInBrowser(cmd, f)
 	   	},
	}

	cmdutil.EnableRepoOverride(cmd, f) // Q: what does this do???

	return cmd
}

func openInBrowser(cmd *cobra.Command, f *cmdutil.Factory) {
	const root = "www.github.com"
	f.Browser.Browse(root)
}


/*
Moving forward, since we can open in the browser we need to discuss logic of arguments

This includes:
1. Open the repo in browser with just "gh browse"
2. Make the help instruction display while opening using IOStreams 
3. Find out how "cmd" stores args within, to be parsed
4. Make a table of all the possible inputs that are valid
5. Possibly work on error handling
6. Clarify table with client before moving forward with args

*/