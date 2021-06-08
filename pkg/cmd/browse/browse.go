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
}

func NewCmdBrowse(f *cmdutil.Factory) *cobra.Command {

	opts := &BrowseOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Browser:    f.Browser,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Long:  "Open the GitHub repository in the web browser.",
		Short: "Open the repository in the browser",
		Use:   "browse {<number> | <path>}",
		Args:  cobra.RangeArgs(0, 2),
		Example: heredoc.Doc(`
			$ gh browse
			#=> Open the home page of the current repository

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
		RunE: func(cmd *cobra.Command, args []string) error {

			if len(args) > 1 {
				opts.AdditionalArg = args[1]
			}

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}
			return openInBrowser(cmd, opts)
		},
	}
	cmdutil.EnableRepoOverride(cmd, f)
	cmd.Flags().BoolVarP(&opts.ProjectsFlag, "projects", "p", false, "Open repository projects")
	cmd.Flags().BoolVarP(&opts.WikiFlag, "wiki", "w", false, "Open repository wiki")
	cmd.Flags().BoolVarP(&opts.SettingsFlag, "settings", "s", false, "Open repository settings")
	cmd.Flags().BoolVarP(&opts.BranchFlag, "branch", "b", false, "Select another branch by passing in the branch name")

	return cmd
}

func openInBrowser(cmd *cobra.Command, opts *BrowseOptions) error {
	help := "Use 'gh browse --help' for more information about browse\n"
	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("unable to determine base repository: %w\n%s", err, help)
	}

	httpClient, _ := opts.HttpClient()
	apiClient := api.NewClientFromHTTP(httpClient)
	branchName, _ := api.RepoDefaultBranch(apiClient, baseRepo)

	if getFlagAmount(cmd) > 1 {
		return fmt.Errorf("accepts 1 flag, %d flag(s) were recieved\n%s", getFlagAmount(cmd), help)
	}

	repoUrl := ghrepo.GenerateRepoURL(baseRepo, "")

	if !hasArg(opts) && hasFlag(cmd) {
		err, repoUrl = addFlag(opts, repoUrl)
		if err != nil {
			return err
		}
	} else if hasArg(opts) && !hasFlag(cmd) {
		err, repoUrl = addArg(opts, repoUrl, branchName)
		if err != nil {
			return err
		}
	} else if hasArg(opts) && hasFlag(cmd) {
		err, repoUrl = addCombined(opts, repoUrl, branchName)
		if err != nil {
			return err
		}
	}

	if repoUrl == ghrepo.GenerateRepoURL(baseRepo, "") {
		fmt.Fprintf(opts.IO.Out, "now opening %s in browser . . .\n", repoUrl)
	}
	opts.Browser.Browse(repoUrl)

	return nil
}

func addCombined(opts *BrowseOptions, url string, branchName string) (error, string) {
	help := "Use 'gh browse --help' for more information about browse\n"

	if !opts.BranchFlag {
		return fmt.Errorf("invalid use of flag and argument\n%s", help), ""
	}

	if opts.AdditionalArg == "" {
		url += "/tree/" + opts.SelectorArg
		fmt.Fprintf(opts.IO.Out, "now opening %s in browser . . .\n", url)
		return nil, url
	}

	arr := parseFileArg(opts)
	if len(arr) > 1 {
		url += "/tree/" + opts.SelectorArg
		fmt.Fprintf(opts.IO.Out, "now opening %s in browser . . .\n", url)
		return nil, url
	}

	url += "/tree/" + opts.SelectorArg
	fmt.Fprintf(opts.IO.Out, "now opening %s in browser . . .\n", url)
	return nil, url
}

func addFlag(opts *BrowseOptions, url string) (error, string) {
	if opts.ProjectsFlag {
		url += "/projects"
	} else if opts.SettingsFlag {
		url += "/settings"
	} else if opts.WikiFlag {
		url += "/wiki"
	}
	fmt.Fprintf(opts.IO.Out, "now opening %s in browser . . .\n", url)
	return nil, url
}

func addArg(opts *BrowseOptions, url string, branchName string) (error, string) {
	help := "Use 'gh browse --help' for more information about browse\n"

	if opts.AdditionalArg != "" {
		return fmt.Errorf("accepts 1 arg, 2 arg(s) were recieved\n%s", help), ""
	}

	if isNumber(opts.SelectorArg) {
		url += "/issues/" + opts.SelectorArg
		fmt.Fprintf(opts.IO.Out, "now opening issue/pr in browser . . .\n")
		return nil, url
	}

	arr := parseFileArg(opts)
	if len(arr) > 1 {
		fmt.Fprintf(opts.IO.Out, "now opening %s in browser . . .\n", url)
		return nil, url + "/tree/" + branchName + "/" + arr[0] + "#L" + arr[1]
	}

	fmt.Fprintf(opts.IO.Out, "now opening %s in browser . . .\n", url)
	return nil, url + "/tree/" + branchName + "/" + arr[0]
}

func parseFileArg(opts *BrowseOptions) []string {
	arr := strings.Split(opts.SelectorArg, ":")
	return arr
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

func isNumber(arg string) bool {
	_, err := strconv.Atoi(arg)
	if err != nil {
		return false
	} else {
		return true
	}
}
