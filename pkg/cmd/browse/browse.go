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
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	FlagAmount  int
	SelectorArg string

	Branch       string
	ProjectsFlag bool
	RepoFlag     bool
	SettingsFlag bool
	WikiFlag     bool
}

func NewCmdBrowse(f *cmdutil.Factory) *cobra.Command {

	opts := &BrowseOptions{
		Browser:    f.Browser,
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Long:  "Open the GitHub repository in the web browser.",
		Short: "Open the repository in the browser",
		Use:   "browse {<number> | <path>}",
		Args:  cobra.RangeArgs(0, 1),
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
				To configure a web browser other than the default, use the BROWSER environment variable. 
			`),
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo
			opts.FlagAmount = cmd.Flags().NFlag()

			if opts.FlagAmount > 2 {
				return &cmdutil.FlagError{Err: fmt.Errorf("cannot have more than two flags, %d were recieved", opts.FlagAmount)}
			}
			if repoOverride, _ := cmd.Flags().GetString("repo"); repoOverride != "" {
				opts.RepoFlag = true
			}
			if opts.FlagAmount > 1 && !opts.RepoFlag {
				return &cmdutil.FlagError{Err: fmt.Errorf("these two flags are incompatible, see below for instructions")}
			}

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

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("unable to determine base repository: %w\nUse 'gh browse --help' for more information about browse\n", err)
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("unable to create an http client: %w\nUse 'gh browse --help' for more information about browse\n", err)
	}

	apiClient := api.NewClientFromHTTP(httpClient)
	branchName, err := api.RepoDefaultBranch(apiClient, baseRepo)
	// if err != nil { //TODO resolve errors related to using this with tests
	// 	return fmt.Errorf("unable to determine default branch for %s: %w", ghrepo.FullName(baseRepo), err)
	// }

	repoUrl := ghrepo.GenerateRepoURL(baseRepo, "")

	hasArg := opts.SelectorArg != ""
	hasFlag := opts.FlagAmount > 0

	if opts.Branch != "" {
		err, repoUrl = addBranch(opts, repoUrl)
		if err != nil {
			return err
		}
	} else if hasArg && (!hasFlag || opts.RepoFlag) {
		err, repoUrl = addArg(opts, repoUrl, branchName)
		if err != nil {
			return err
		}
	} else if !hasArg && hasFlag {
		repoUrl = addFlag(opts, repoUrl)
	} else {
		fmt.Fprintf(opts.IO.Out, "now opening %s in browser . . .\n", repoUrl)
	}
	opts.Browser.Browse(repoUrl)
	return nil
}

func addBranch(opts *BrowseOptions, url string) (error, string) {
	if opts.SelectorArg == "" {
		url += "/tree/" + opts.Branch
	} else {
		err, arr := parseFileArg(opts.SelectorArg)
		if err != nil {
			return err, url
		}
		if len(arr) > 1 {
			url += "/tree/" + opts.Branch + "/" + arr[0] + "#L" + arr[1]
		} else {
			url += "/tree/" + opts.Branch + "/" + arr[0]
		}
	}
	fmt.Fprintf(opts.IO.Out, "now opening %s in browser . . .\n", url)
	return nil, url
}

func addFlag(opts *BrowseOptions, url string) string {
	if opts.ProjectsFlag {
		url += "/projects"
	} else if opts.SettingsFlag {
		url += "/settings"
	} else if opts.WikiFlag {
		url += "/wiki"
	}
	fmt.Fprintf(opts.IO.Out, "now opening %s in browser . . .\n", url)
	return url
}

func addArg(opts *BrowseOptions, url string, branchName string) (error, string) {
	if isNumber(opts.SelectorArg) {
		url += "/issues/" + opts.SelectorArg
		fmt.Fprintf(opts.IO.Out, "now opening issue/pr in browser . . .\n")
	} else {
		err, arr := parseFileArg(opts.SelectorArg)
		if err != nil {
			return err, url
		}
		if len(arr) > 1 {
			url += "/tree/" + branchName + "/" + arr[0] + "#L" + arr[1]
		} else {
			url += "/tree/" + branchName + "/" + arr[0]
		}
		fmt.Fprintf(opts.IO.Out, "now opening %s in browser . . .\n", url)
	}
	return nil, url
}

func parseFileArg(fileArg string) (error, []string) {
	arr := strings.Split(fileArg, ":")
	if len(arr) > 2 {
		return fmt.Errorf("invalid use of colon\nUse 'gh browse --help' for more information about browse\n"), arr
	}
	if len(arr) > 1 && !isNumber(arr[1]) {
		return fmt.Errorf("invalid line number after colon\nUse 'gh browse --help' for more information about browse\n"), arr
	}
	return nil, arr
}

func isNumber(arg string) bool {
	_, err := strconv.Atoi(arg)
	return err == nil
}
