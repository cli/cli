package browse

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
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

	SelectorArg   string
	AdditionalArg string

	ProjectsFlag bool
	WikiFlag     bool
	SettingsFlag bool
	BranchFlag   bool
	//LineFlag     bool
}

type exitCode int

const (
	exitSuccess      exitCode = 0
	exitNotInRepo    exitCode = 1
	exitTooManyFlags exitCode = 2
	exitTooManyArgs  exitCode = 3
	exitExpectedArg  exitCode = 4
	exitInvalidCombo exitCode = 5
)

func NewCmdBrowse(f *cmdutil.Factory) *cobra.Command {

	opts := &BrowseOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Browser:    f.Browser,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Long:  "Open a GitHub repository in the web browser",
		Short: "Open GitHub in the browser",
		Use:   "browse {<number> | <path>}",
		Args:  cobra.RangeArgs(0, 2),
		Example: heredoc.Doc(`
			$ gh browse
			#=> Open current repository to the default branch

			$ gh browse 217
			#=> Open issue or pull request 217

			$ gh browse --settings
			#=> Open repository settings

			$ gh browse main.go:312
			#=> Open main.go at line 312

			$ gh browse main.go --branch main
			#=> Open main.go in the main branch
		`),
		Annotations: map[string]string{
			"IsCore": "true",
			"help:arguments": heredoc.Doc(`
				A browser location can be specified using arguments in the following format:
				- by number for issue or pull request, e.g. "123"; or
				- by path for opening folders and files, e.g. "cmd/gh/main.go"
			`),
			"help:environment": heredoc.Doc(`
				To configure a web browser other than the default, use the BROWSER environment variable 
			`),
		},
		Run: func(cmd *cobra.Command, args []string) {

			if len(args) > 1 {
				opts.AdditionalArg = args[1]
			}

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}
			openInBrowser(cmd, opts)
		},
	}
	cmdutil.EnableRepoOverride(cmd, f)
	cmd.Flags().BoolVarP(&opts.ProjectsFlag, "projects", "p", false, "Open repository projects")
	cmd.Flags().BoolVarP(&opts.WikiFlag, "wiki", "w", false, "Open repository wiki")
	cmd.Flags().BoolVarP(&opts.SettingsFlag, "settings", "s", false, "Open repository settings")
	cmd.Flags().BoolVarP(&opts.BranchFlag, "branch", "b", false, "Select another branch by passing in the branch name")

	return cmd
}

func openInBrowser(cmd *cobra.Command, opts *BrowseOptions) (exitCode, string) {

	baseRepo, err := opts.BaseRepo()
	httpClient, _ := opts.HttpClient()
	apiClient := api.NewClientFromHTTP(httpClient)
	branchName, err := api.RepoDefaultBranch(apiClient, baseRepo)

	if !inRepo(err) { // must be in a repo to execute
		printExit(exitNotInRepo, cmd, opts, "")
		return exitNotInRepo, ""
	}

	if getFlagAmount(cmd) > 1 { // command can't have more than one flag
		printExit(exitTooManyFlags, cmd, opts, "")
		return exitTooManyFlags, ""
	}

	repoUrl := ghrepo.GenerateRepoURL(baseRepo, "")
	response := exitSuccess

	if !hasArg(opts) && hasFlag(cmd) {
		response, repoUrl = addFlag(opts, repoUrl)
	} else if hasArg(opts) && !hasFlag(cmd) {
		response, repoUrl = addArg(opts, repoUrl, branchName)
	} else if hasArg(opts) && hasFlag(cmd) {
		response, repoUrl = addCombined(opts, repoUrl, branchName)
	}

	if response == exitSuccess {
		opts.Browser.Browse(repoUrl) // otherwise open repo
	}

	printExit(response, cmd, opts, repoUrl) // print success
	return response, repoUrl
}

func addCombined(opts *BrowseOptions, url string, branchName string) (exitCode, string) {

	if !opts.BranchFlag { // gh browse --settings main.go
		return exitInvalidCombo, ""
	}

	if opts.AdditionalArg == "" {
		return exitSuccess, url + "/tree/" + opts.SelectorArg
	}

	arr := parseFileArg(opts)
	if len(arr) > 1 {
		return exitSuccess, url + "/tree/" + opts.AdditionalArg + "/" + arr[0] + "#L" + arr[1]
	}

	return exitSuccess, url + "/tree/" + opts.AdditionalArg + "/" + arr[0]

}

func addFlag(opts *BrowseOptions, url string) (exitCode, string) {
	if opts.ProjectsFlag {
		return exitSuccess, url + "/projects"
	} else if opts.SettingsFlag {
		return exitSuccess, url + "/settings"
	} else if opts.WikiFlag {
		return exitSuccess, url + "/wiki"
	}
	return exitExpectedArg, "" // Flag is a branch and needs an argument
}

func addArg(opts *BrowseOptions, url string, branchName string) (exitCode, string) {

	if opts.AdditionalArg != "" {
		return exitTooManyArgs, ""
	}

	if isNumber(opts.SelectorArg) {
		url += "/issues/" + opts.SelectorArg
		return exitSuccess, url
	}

	arr := parseFileArg(opts)
	if len(arr) > 1 {
		return exitSuccess, url + "/tree/" + branchName + "/" + arr[0] + "#L" + arr[1]
	}

	return exitSuccess, url + "/tree/" + branchName + "/" + arr[0]
}

func parseFileArg(opts *BrowseOptions) []string {
	arr := strings.Split(opts.SelectorArg, ":")
	return arr
}

func printExit(exit exitCode, cmd *cobra.Command, opts *BrowseOptions, url string) {
	w := opts.IO.ErrOut
	cs := opts.IO.ColorScheme()
	help := "Use 'gh browse --help' for more information about browse\n"

	switch exit {
	case exitSuccess:
		fmt.Fprintf(w, "%s now opening %s in browser . . .\n",
			cs.Green("✓"), cs.Bold(url))
		break
	case exitNotInRepo:
		fmt.Fprintf(w, "%s change directory to a repository to open in browser\n%s",
			cs.Red("x"), help)
		break
	case exitTooManyFlags:
		fmt.Fprintf(w, "%s accepts 1 flag, %d flag(s) were recieved\n%s",
			cs.Red("x"), getFlagAmount(cmd), help)
		break
	case exitTooManyArgs:
		fmt.Fprintf(w, "%s accepts 1 arg, 2 arg(s) were received \n%s",
			cs.Red("x"), help)
		break
	case exitExpectedArg:
		fmt.Fprintf(w, "%s expected argument with this flag %s\n%s",
			cs.Red("x"), cs.Bold(url), help)
		break
	case exitInvalidCombo:
		fmt.Fprintf(w, "%s invalid use of flag and argument\n%s",
			cs.Red("x"), help)
		break
	}
}

func hasFlag(cmd *cobra.Command) bool {
	return getFlagAmount(cmd) > 0
}

func hasArg(opts *BrowseOptions) bool {
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
