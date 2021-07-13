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
			fileArg, err := parseFileArg(opts.SelectorArg)
			if err != nil {
				return err
			}
			if opts.Branch != "" {
				url += "/tree/" + opts.Branch + "/"
			} else {
				apiClient := api.NewClientFromHTTP(httpClient)
				branchName, err := api.RepoDefaultBranch(apiClient, baseRepo)
				if err != nil {
					return err
				}
				url += "/tree/" + branchName + "/"
			}
			url += fileArg
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

func parseFileArg(fileArg string) (string, error) {
	arr := strings.Split(fileArg, ":")
	if len(arr) > 2 {
		return "", fmt.Errorf("invalid use of colon\nUse 'gh browse --help' for more information about browse\n")
	}
	if len(arr) > 1 {
		if !isNumber(arr[1]) {
			return "", fmt.Errorf("invalid line number after colon\nUse 'gh browse --help' for more information about browse\n")
		}
		return arr[0] + "#L" + arr[1], nil
	}
	return arr[0], nil
}

func isNumber(arg string) bool {
	_, err := strconv.Atoi(arg)
	return err == nil
}
