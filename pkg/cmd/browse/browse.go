package browse

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type browser interface {
	Browse(string) error
}

type BrowseOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser

	SelectorArg string
	FileArg     string // Used for storing the file path
	NumberArg   int    // Used for storing pull request number
}

func NewCmdBrowse(f *cmdutil.Factory) *cobra.Command {

	opts := &BrowseOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Browser:    f.Browser,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Long:  "Work with GitHub in the browser", // displays when you are on the help page of this command
		Short: "Open GitHub in the browser",      // displays in the gh root help
		Use:   "browse",                          // necessary!!! This is the cmd that gets passed on the prompt
		Args:  cobra.RangeArgs(0, 1),             // make sure only one arg at most is passed
		Run: func(cmd *cobra.Command, args []string) {

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}
			openInBrowser(cmd, opts) // run gets rid of the usage / runs function
		},
	}

	return cmd
}

func openInBrowser(cmd *cobra.Command, opts *BrowseOptions) {
	// args := cmd.Args

	baseRepo, err := opts.BaseRepo()
	w := opts.IO.ErrOut
	cs := opts.IO.ColorScheme()
	help := "Use 'gh browse --help' for more information about browse\n"
	if err != nil {
		fmt.Fprintf(w, "%s Navigate to a repository to open in browser\n%s",
			cs.Red("x"),
			help)
		return
	}

	// ATTN: add into the empty string where you want to go within the repo
	repoUrl := ghrepo.GenerateRepoURL(baseRepo, "")
	opts.Browser.Browse(repoUrl)
	fmt.Fprintf(w, "%s Now opening %s in browser . . .\n",
		cs.Green("✓"),
		cs.Bold(ghrepo.FullName(baseRepo)))
}

func parseArgs(opts *BrowseOptions) {
	if opts.SelectorArg != "" {

		convertedArg, err := strconv.Atoi(opts.SelectorArg)
		if err != nil { //It's not a number, but a file name
			opts.FileArg = opts.SelectorArg

		} else { // It's a number, open issue or pull request
			opts.NumberArg = convertedArg
		}
	}
}

func parseFlags(opts *BrowseOptions) {

}

//hello

//making pull request
