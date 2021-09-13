package browse

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
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

	SelectorArg string

	Branch        string
	ProjectsFlag  bool
	SettingsFlag  bool
	WikiFlag      bool
	NoBrowserFlag bool
}

func NewCmdBrowse(f *cmdutil.Factory, runF func(*BrowseOptions) error) *cobra.Command {
	opts := &BrowseOptions{
		Browser:    f.Browser,
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Long:  "Open the GitHub repository in the web browser.",
		Short: "Open the repository in the browser",
		Use:   "browse [<number> | <path>]",
		Args:  cobra.MaximumNArgs(1),
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

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if err := cmdutil.MutuallyExclusive(
				"specify only one of `--branch`, `--projects`, `--wiki`, or `--settings`",
				opts.Branch != "",
				opts.WikiFlag,
				opts.SettingsFlag,
				opts.ProjectsFlag,
			); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}
			return runBrowse(opts)
		},
	}

	cmdutil.EnableRepoOverride(cmd, f)
	cmd.Flags().BoolVarP(&opts.ProjectsFlag, "projects", "p", false, "Open repository projects")
	cmd.Flags().BoolVarP(&opts.WikiFlag, "wiki", "w", false, "Open repository wiki")
	cmd.Flags().BoolVarP(&opts.SettingsFlag, "settings", "s", false, "Open repository settings")
	cmd.Flags().BoolVarP(&opts.NoBrowserFlag, "no-browser", "n", false, "Print destination URL instead of opening the browser")
	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "Select another branch by passing in the branch name")

	return cmd
}

func runBrowse(opts *BrowseOptions) error {
	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("unable to determine base repository: %w", err)
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("unable to create an http client: %w", err)
	}
	url := ghrepo.GenerateRepoURL(baseRepo, "")

	if opts.SelectorArg == "" {
		if opts.ProjectsFlag {
			url += "/projects"
		}
		if opts.SettingsFlag {
			url += "/settings"
		}
		if opts.WikiFlag {
			url += "/wiki"
		}
		if opts.Branch != "" {
			url += "/tree/" + opts.Branch + "/"
		}
	} else {
		if isNumber(opts.SelectorArg) {
			url += "/issues/" + opts.SelectorArg
		} else {

			var branchName string
			if opts.Branch != "" {
				branchName = opts.Branch
			} else {
				apiClient := api.NewClientFromHTTP(httpClient)
				branchName, err = api.RepoDefaultBranch(apiClient, baseRepo)
				if err != nil {
					return err
				}
			}

			path, err := genPath(opts.SelectorArg, branchName)
			if err != nil {
				return err
			}
			url += path
		}
	}

	if opts.NoBrowserFlag {
		fmt.Fprintf(opts.IO.Out, "%s\n", url)
		return nil
	} else {
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.Out, "now opening %s in browser\n", url)
		}
		return opts.Browser.Browse(url)
	}
}

// genPath constructs the path portion of the url with leading "/" for the browser to open.
func genPath(fileArg, branchName string) (string, error) {
	sb := strings.Builder{} // for building the path string return value

	arr := strings.Split(fileArg, ":") // look for line number or range by splitting on delimiter
	if len(arr) > 2 {
		return "", fmt.Errorf("invalid use of colon\nUse 'gh browse --help' for more information about browse\n")
	}

	filename := arr[0]
	if len(arr) > 1 { // expecting a line number and possibly range
		// build path with "blob" instead of "tree" so "plain" param will work
		sb.WriteString("/blob/")
		sb.WriteString(branchName)
		sb.WriteString("/")
		sb.WriteString(filename)
		sb.WriteString("?plain=1") // param to disable markdown rendering so we can highlight line number

		// build fragment for line number and possibly range
		sb.WriteString("#L")
		lines := strings.Split(arr[1], "-") // look for line range by splitting on delimiter
		if len(lines) > 0 {
			if !isNumber(lines[0]) {
				return "", fmt.Errorf("invalid line number after colon\nUse 'gh browse --help' for more information about browse\n")
			}
			sb.WriteString(lines[0]) // build line number
		}

		if len(lines) > 1 {
			if !isNumber(lines[1]) {
				return "", fmt.Errorf("invalid line range after colon\nUse 'gh browse --help' for more information about browse\n")
			}
			// build line range
			sb.WriteString("-L")
			sb.WriteString(lines[1])
		}
	} else { // no line number or range found in fileArg
		sb.WriteString("/tree/")
		sb.WriteString(branchName)
		sb.WriteString("/")
		sb.WriteString(filename)
	}

	return sb.String(), nil
}

func isNumber(arg string) bool {
	_, err := strconv.Atoi(arg)
	return err == nil
}
