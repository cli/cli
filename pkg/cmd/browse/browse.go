package browse

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"

	"github.com/cli/cli/utils"
)

type browser interface {
	Browse(string) error
}

type BrowseOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser

	SelectorArg   string
	AdditionalArg string
	// FileArg     string // Used for storing the file path
	IsNumberArg   bool    // Used for storing pull request number

	ProjectsFlag  bool
	WikiFlag      bool
	SettingsFlag  bool
}

type exitCode int

const (
	exitSuccess      exitCode = 0
	exitNotInRepo    exitCode = 1
	exitTooManyFlags exitCode = 2
	exitInvalidUrl   exitCode = 3
	exitHttpError    exitCode = 4
	exitError        exitCode = 5
)

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
		Args:  cobra.RangeArgs(0, 2),             // make sure only one arg at most is passed

		Run: func(cmd *cobra.Command, args []string) {

			if len(args) > 1 {
				
			}

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}
			openInBrowser(cmd, opts) // run gets rid of the usage / runs function
		},
	}

	cmd.Flags().BoolVarP(&opts.ProjectsFlag, "projects", "p", false, "Open projects tab in browser")
	cmd.Flags().BoolVarP(&opts.WikiFlag, "wiki", "w", false, "Opens the wiki in browser")
	cmd.Flags().BoolVarP(&opts.SettingsFlag, "settings", "s", false, "Opens the settings in browse")

	return cmd
}

func openInBrowser(cmd *cobra.Command, opts *BrowseOptions) {

	baseRepo, err := opts.BaseRepo()

	if !inRepo(err) { // must be in a repo to execute
		printExit(exitNotInRepo, cmd, opts, "")
		return
	}

	if getFlagAmount(cmd) > 1 { // command can't have more than one flag
		printExit(exitTooManyFlags, cmd, opts, "")
		return
	}

	repoUrl := ghrepo.GenerateRepoURL(baseRepo, "")
	parseArgs(opts)

	if !hasArgs(opts) && hasFlags(cmd) {
		repoUrl += addFlags(opts)
	} else if hasArgs(opts) && !hasFlags(cmd) {
		repoUrl += addArgs(opts)
	} else if hasArgs(opts) && hasFlags(cmd) {

	}	

	

	// if !hasArgs(opts) { // handle flags without args
	// 	repoUrl += addFlags(opts) // add flags, if any
	// 	response := getHttpResponse(opts, repoUrl)
	// 	if response != exitSuccess { // test the connection to see if the url is valid
	// 		printExit(response, cmd, opts, repoUrl) // if not, print error, and return
	// 		return
	// 	}
	// 	opts.Browser.Browse(repoUrl)            // otherwise open repo
	// 	printExit(response, cmd, opts, repoUrl) // print success
	// 	return
	// }

	printExit(exitError, cmd, opts, "")
}

func parseArgs(opts *BrowseOptions) {
	if opts.SelectorArg != "" {
		_, err := strconv.Atoi(opts.SelectorArg)
		if err != nil { //It's not a number, but a file name
			opts.IsNumberArg = false
		} else { // It's a number, open issue or pull request
			opts.IsNumberArg = true
		}
	}
}

func addFlags(opts *BrowseOptions) string {
	if opts.ProjectsFlag {
		return "/projects"
	} else if opts.SettingsFlag {
		return "/settings"
	} else if opts.WikiFlag {
		return "/wiki"
	}
	return ""
}

func addArgs(opts *BrowseOptions) string {
	if opts.IsNumberArg {
		return "/issues/" + opts.SelectorArg
	}
	return "/" + opts.SelectorArg
}

func printExit(errorCode exitCode, cmd *cobra.Command, opts *BrowseOptions, url string) {
	w := opts.IO.ErrOut
	cs := opts.IO.ColorScheme()
	help := "Use 'gh browse --help' for more information about browse\n"

	switch errorCode {
	case exitSuccess:
		fmt.Fprintf(w, "%s Now opening %s in browser . . .\n",
			cs.Green("✓"), cs.Bold(url))
		break
	case exitNotInRepo:
		fmt.Fprintf(w, "%s Change directory to a repository to open in browser\n%s",
			cs.Red("x"), help)
		break
	case exitTooManyFlags:
		fmt.Fprintf(w, "%s accepts 1 flag, %d flags were recieved\n%s",
			cs.Red("x"), getFlagAmount(cmd), help)
		break
	case exitInvalidUrl:
		fmt.Fprintf(w, "%s Invalid url format, could not open %s\n%s",
			cs.Red("x"), cs.Bold(url), help)
		break
	case exitHttpError:
		fmt.Fprintf(w, "%s Could not successfully create an http request\n%s",
			cs.Red("x"), help)
		break
	case exitError:
		fmt.Fprintf(w, "%s Could not successfully find %s\n%s",
			cs.Red("x"), cs.Bold(url), help)
		break
	}
}

func getHttpResponse(opts *BrowseOptions, url string) exitCode {

	if !utils.IsURL(url) || !utils.ValidURL(url) {
		return exitInvalidUrl
	}

	resp, err := http.Get(url)

	if err != nil {
		return exitHttpError
	}

	statusCode := resp.StatusCode

	if 200 <= statusCode && statusCode < 300 {
		return exitSuccess
	}

	return exitError
}

func hasFlags(cmd *cobra.Command) bool {
	return getFlagAmount(cmd) > 0
}

func hasArgs(opts *BrowseOptions) bool {
	return opts.SelectorArg != ""
}

func getFlagAmount(cmd *cobra.Command) int {
	return cmd.Flags().NFlag()
}

func inRepo(err error) bool {
	return err == nil
}
