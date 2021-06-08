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

	SelectorArg string
	FlagAmount  int

	ProjectsFlag bool
	WikiFlag     bool
	SettingsFlag bool
	Branch       string
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
			opts.FlagAmount = cmd.Flags().NFlag()
			if opts.FlagAmount > 1 {
				return &cmdutil.FlagError{Err: fmt.Errorf("accepts 1 flag, %d flag(s) were recieved", opts.FlagAmount)}
			}
			// TODO test whether you can set cobra command arg range to 1 with a branch accepting an arg1
			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			return runBrowse(opts)
		},
	}
	cmdutil.EnableRepoOverride(cmd, f)
	cmd.Flags().BoolVarP(&opts.ProjectsFlag, "projects", "p", false, "Open repository projects")
	cmd.Flags().BoolVarP(&opts.WikiFlag, "wiki", "w", false, "Open repository wiki")
	cmd.Flags().BoolVarP(&opts.SettingsFlag, "settings", "s", false, "Open repository settings")
	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "Select another branch by passing in the branch name")

	return cmd
}

func runBrowse(opts *BrowseOptions) error {
	help := "Use 'gh browse --help' for more information about browse\n"

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("unable to determine base repository: %w\n%s", err, help)
	}

	httpClient, _ := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("unable to create an http client: %w\n%s", err, help)
	}

	apiClient := api.NewClientFromHTTP(httpClient)
	branchName, err := api.RepoDefaultBranch(apiClient, baseRepo)
	if err != nil {
		return fmt.Errorf("unable to determine default branch for %s: %w", ghrepo.FullName(baseRepo), err)
	}

	repoUrl := ghrepo.GenerateRepoURL(baseRepo, "")

	hasArg := hasArg(opts)
	hasFlag := hasFlag(opts)

	if hasArg && !hasFlag {
		repoUrl = addArg(opts, repoUrl, branchName)
	} else if !hasArg && hasFlag {
		err, repoUrl = addFlag(opts, repoUrl)
		if err != nil {
			return err
		}
	} else if hasArg && hasFlag {
		err, repoUrl = addBranch(opts, repoUrl)
		if err != nil {
			return err
		}
	} else {
		fmt.Fprintf(opts.IO.Out, "now opening %s in browser . . .\n", repoUrl)
	}

	opts.Browser.Browse(repoUrl)

	return nil
}

func addBranch(opts *BrowseOptions, url string) (error, string) {
	help := "Use 'gh browse --help' for more information about browse\n"

	if opts.Branch == "" {
		return fmt.Errorf("invalid use of flag and argument\n%s", help), ""
	}

	if opts.SelectorArg == "" {
		url += "/tree/" + opts.Branch
		fmt.Fprintf(opts.IO.Out, "now opening %s in browser . . .\n", url)
		return nil, url
	}

	arr := parseFileArg(opts)
	if len(arr) > 1 {
		return nil, url + "/tree/" + opts.Branch + "/" + arr[0] + "#L" + arr[1]
	}
	return nil, url + "/tree/" + opts.Branch + "/" + arr[0]
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

func addArg(opts *BrowseOptions, url string, branchName string) string {
	if isNumber(opts.SelectorArg) {
		url += "/issues/" + opts.SelectorArg
		fmt.Fprintf(opts.IO.Out, "now opening issue/pr in browser . . .\n")
		return url
	}

	arr := parseFileArg(opts)
	if len(arr) > 1 {
		fmt.Fprintf(opts.IO.Out, "now opening %s in browser . . .\n", url)
		return url + "/tree/" + branchName + "/" + arr[0] + "#L" + arr[1]
	}

	fmt.Fprintf(opts.IO.Out, "now opening %s in browser . . .\n", url)
	return url + "/tree/" + branchName + "/" + arr[0]
}

func parseFileArg(opts *BrowseOptions) []string {
	arr := strings.Split(opts.SelectorArg, ":")
	return arr
}

func hasFlag(opts *BrowseOptions) bool {
	return opts.FlagAmount > 0
}

func hasArg(opts *BrowseOptions) bool {
	return opts.SelectorArg != ""
}

func isNumber(arg string) bool {
	_, err := strconv.Atoi(arg)
	return err == nil
}
