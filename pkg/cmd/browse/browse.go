package browse

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"

	"github.com/cli/cli/api"
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

	ProjectsFlag bool
	WikiFlag     bool
	SettingsFlag bool
	BranchFlag   bool
	LineFlag     bool
}

type exitCode int

const (
	exitSuccess      exitCode = 0
	exitNotInRepo    exitCode = 1
	exitTooManyFlags exitCode = 2
	exitInvalidUrl   exitCode = 3
	exitHttpError    exitCode = 4
	exitInvalidCombo exitCode = 5
	exitError        exitCode = 6
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
				opts.AdditionalArg = args[1]
			}

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}
			openInBrowser(cmd, opts) // run gets rid of the usage / runs function
		},
	}

	cmd.Flags().BoolVarP(&opts.ProjectsFlag, "projects", "p", false, "Open projects tab in browser")
	cmd.Flags().BoolVarP(&opts.WikiFlag, "wiki", "w", false, "Opens the wiki in browser")
	cmd.Flags().BoolVarP(&opts.SettingsFlag, "settings", "s", false, "Opens the settings in browser")
	cmd.Flags().BoolVarP(&opts.BranchFlag, "branch", "b", false, "Opens a branch in the browser")
	cmd.Flags().BoolVarP(&opts.LineFlag, "line", "l", false, "Opens up to a line in a file in the browser")

	return cmd
}

func openInBrowser(cmd *cobra.Command, opts *BrowseOptions) {

	baseRepo, err := opts.BaseRepo()
	//baseRepo, _ := opts.BaseRepo()
	httpClient, _ := opts.HttpClient()
	apiClient := api.NewClientFromHTTP(httpClient)

	branchName, err := api.RepoDefaultBranch(apiClient, baseRepo)
	response := exitSuccess

	if !inRepo(err) { // must be in a repo to execute
		printExit(exitNotInRepo, cmd, opts, "")
		return
	}

	if getFlagAmount(cmd) > 1 { // command can't have more than one flag
		printExit(exitTooManyFlags, cmd, opts, "")
		return
	}

	repoUrl := ghrepo.GenerateRepoURL(baseRepo, "")
	//parseArgs(opts)

	if !hasArgs(opts) && hasFlags(cmd) {
		response, repoUrl = addFlags(opts, repoUrl)
	} else if hasArgs(opts) && !hasFlags(cmd) {
		response, repoUrl = addArgs(opts, repoUrl, branchName)
	} else if hasArgs(opts) && hasFlags(cmd) {
		response, repoUrl = addCombined(opts, repoUrl, branchName)
	}

	if response != exitSuccess {
		printExit(response, cmd, opts, repoUrl) // print success
		return
	}

	response = getHttpResponse(opts, repoUrl)
	if response == exitSuccess { // test the connection to see if the url is valid
		opts.Browser.Browse(repoUrl) // otherwise open repo
	}

	printExit(response, cmd, opts, repoUrl) // print success

}

func addCombined(opts *BrowseOptions, url string, branchName string) (exitCode, string) {

	if opts.SelectorArg == "" || opts.AdditionalArg == "" {
		return exitError, "" //makeing new exit error
	}

	if opts.BranchFlag {
		return exitSuccess, url + "/tree/" + opts.AdditionalArg + "/" + opts.SelectorArg
	}

	if opts.LineFlag {
		return exitSuccess, url + "/tree/" + branchName + "/" + opts.SelectorArg + "#L" + opts.AdditionalArg
	}
	return exitError, ""

}

func addFlags(opts *BrowseOptions, url string) (exitCode, string) {
	if opts.ProjectsFlag {
		return exitSuccess, url + "/projects"
	} else if opts.SettingsFlag {
		return exitSuccess, url + "/settings"
	} else if opts.WikiFlag {
		return exitSuccess, url + "/wiki"
	}
	return exitError, "" //TODO need a new exit error later (possiblly)
}

func addArgs(opts *BrowseOptions, url string, branchName string) (exitCode, string) {

	if opts.AdditionalArg != "" {
		// fmt.Println("test secodn argument")
		return exitError, ""
	}

	if isNumber(opts.SelectorArg) {
		return exitSuccess, url + "/issues/" + opts.SelectorArg
	}
	return exitSuccess, url + "/tree/" + branchName + "/" + opts.SelectorArg
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
	case exitInvalidCombo:
		fmt.Fprintf(w, "%s Invalid combination of arguments\n%s",
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

	//fmt.Println("print", url, statusCode)
	//Setting is not opening

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

func isNumber(arg string) bool {
	_, err := strconv.Atoi(arg)
	if err != nil {
		return false
	} else {
		return true
	}
}
