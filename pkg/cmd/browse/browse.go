package browse

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

type browser interface {
	Browse(string) error
}

type BrowseOptions struct {
	BaseRepo         func() (ghrepo.Interface, error)
	Browser          browser
	HttpClient       func() (*http.Client, error)
	IO               *iostreams.IOStreams
	PathFromRepoRoot func() string
	GitClient        gitClient

	SelectorArg string

	Branch        string
	CommitFlag    bool
	ProjectsFlag  bool
	SettingsFlag  bool
	WikiFlag      bool
	NoBrowserFlag bool
}

func NewCmdBrowse(f *cmdutil.Factory, runF func(*BrowseOptions) error) *cobra.Command {
	opts := &BrowseOptions{
		Browser:          f.Browser,
		HttpClient:       f.HttpClient,
		IO:               f.IOStreams,
		PathFromRepoRoot: git.PathFromRepoRoot,
		GitClient:        &localGitClient{},
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
				"specify only one of `--branch`, `--commit`, `--projects`, `--wiki`, or `--settings`",
				opts.Branch != "",
				opts.CommitFlag,
				opts.WikiFlag,
				opts.SettingsFlag,
				opts.ProjectsFlag,
			); err != nil {
				return err
			}
			if cmd.Flags().Changed("repo") {
				opts.GitClient = &remoteGitClient{opts.BaseRepo, opts.HttpClient}
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
	cmd.Flags().BoolVarP(&opts.CommitFlag, "commit", "c", false, "Open the last commit")
	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "Select another branch by passing in the branch name")

	return cmd
}

func runBrowse(opts *BrowseOptions) error {
	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("unable to determine base repository: %w", err)
	}

	if opts.CommitFlag {
		commit, err := opts.GitClient.LastCommit()
		if err != nil {
			return err
		}
		opts.Branch = commit.Sha
	}

	section, err := parseSection(baseRepo, opts)
	if err != nil {
		return err
	}
	url := ghrepo.GenerateRepoURL(baseRepo, "%s", section)

	if opts.NoBrowserFlag {
		_, err := fmt.Fprintln(opts.IO.Out, url)
		return err
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", utils.DisplayURL(url))
	}
	return opts.Browser.Browse(url)
}

func parseSection(baseRepo ghrepo.Interface, opts *BrowseOptions) (string, error) {
	if opts.SelectorArg == "" {
		if opts.ProjectsFlag {
			return "projects", nil
		} else if opts.SettingsFlag {
			return "settings", nil
		} else if opts.WikiFlag {
			return "wiki", nil
		} else if opts.Branch == "" {
			return "", nil
		}
	}

	if isNumber(opts.SelectorArg) {
		return fmt.Sprintf("issues/%s", opts.SelectorArg), nil
	}

	filePath, rangeStart, rangeEnd, err := parseFile(*opts, opts.SelectorArg)
	if err != nil {
		return "", err
	}

	branchName := opts.Branch
	if branchName == "" {
		httpClient, err := opts.HttpClient()
		if err != nil {
			return "", err
		}
		apiClient := api.NewClientFromHTTP(httpClient)
		branchName, err = api.RepoDefaultBranch(apiClient, baseRepo)
		if err != nil {
			return "", fmt.Errorf("error determining the default branch: %w", err)
		}
	}

	if rangeStart > 0 {
		var rangeFragment string
		if rangeEnd > 0 && rangeStart != rangeEnd {
			rangeFragment = fmt.Sprintf("L%d-L%d", rangeStart, rangeEnd)
		} else {
			rangeFragment = fmt.Sprintf("L%d", rangeStart)
		}
		return fmt.Sprintf("blob/%s/%s?plain=1#%s", escapePath(branchName), escapePath(filePath), rangeFragment), nil
	}
	return strings.TrimSuffix(fmt.Sprintf("tree/%s/%s", escapePath(branchName), escapePath(filePath)), "/"), nil
}

// escapePath URL-encodes special characters but leaves slashes unchanged
func escapePath(p string) string {
	return strings.ReplaceAll(url.PathEscape(p), "%2F", "/")
}

func parseFile(opts BrowseOptions, f string) (p string, start int, end int, err error) {
	if f == "" {
		return
	}

	parts := strings.SplitN(f, ":", 3)
	if len(parts) > 2 {
		err = fmt.Errorf("invalid file argument: %q", f)
		return
	}

	p = filepath.ToSlash(parts[0])
	if !path.IsAbs(p) {
		p = path.Join(opts.PathFromRepoRoot(), p)
		if p == "." || strings.HasPrefix(p, "..") {
			p = ""
		}
	}
	if len(parts) < 2 {
		return
	}

	if idx := strings.IndexRune(parts[1], '-'); idx >= 0 {
		start, err = strconv.Atoi(parts[1][:idx])
		if err != nil {
			err = fmt.Errorf("invalid file argument: %q", f)
			return
		}
		end, err = strconv.Atoi(parts[1][idx+1:])
		if err != nil {
			err = fmt.Errorf("invalid file argument: %q", f)
		}
		return
	}

	start, err = strconv.Atoi(parts[1])
	if err != nil {
		err = fmt.Errorf("invalid file argument: %q", f)
	}
	end = start
	return
}

func isNumber(arg string) bool {
	_, err := strconv.Atoi(arg)
	return err == nil
}

// gitClient is used to implement functions that can be performed on both local and remote git repositories
type gitClient interface {
	LastCommit() (*git.Commit, error)
}

type localGitClient struct{}

type remoteGitClient struct {
	repo       func() (ghrepo.Interface, error)
	httpClient func() (*http.Client, error)
}

func (gc *localGitClient) LastCommit() (*git.Commit, error) { return git.LastCommit() }

func (gc *remoteGitClient) LastCommit() (*git.Commit, error) {
	httpClient, err := gc.httpClient()
	if err != nil {
		return nil, err
	}
	repo, err := gc.repo()
	if err != nil {
		return nil, err
	}
	commit, err := api.LastCommit(api.NewClientFromHTTP(httpClient), repo)
	if err != nil {
		return nil, err
	}
	return &git.Commit{Sha: commit.OID}, nil
}
