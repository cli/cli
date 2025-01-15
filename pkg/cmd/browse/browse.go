package browse

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

const (
	emptyCommitFlag = "last"
)

type BrowseOptions struct {
	BaseRepo         func() (ghrepo.Interface, error)
	Browser          browser.Browser
	HttpClient       func() (*http.Client, error)
	IO               *iostreams.IOStreams
	PathFromRepoRoot func() string
	GitClient        gitClient

	SelectorArg string

	Branch          string
	Commit          string
	ProjectsFlag    bool
	ReleasesFlag    bool
	SettingsFlag    bool
	WikiFlag        bool
	NoBrowserFlag   bool
	HasRepoOverride bool
}

func NewCmdBrowse(f *cmdutil.Factory, runF func(*BrowseOptions) error) *cobra.Command {
	opts := &BrowseOptions{
		Browser:    f.Browser,
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
		PathFromRepoRoot: func() string {
			return f.GitClient.PathFromRoot(context.Background())
		},
		GitClient: &localGitClient{client: f.GitClient},
	}

	cmd := &cobra.Command{
		Short: "Open repositories, issues, pull requests, and more in the browser",
		Long: heredoc.Doc(`
			Transition from the terminal to the web browser to view and interact with:

			- Issues
			- Pull requests
			- Repository content
			- Repository home page
			- Repository settings
		`),
		Use:  "browse [<number> | <path> | <commit-SHA>]",
		Args: cobra.MaximumNArgs(1),
		Example: heredoc.Doc(`
			$ gh browse
			#=> Open the home page of the current repository

			$ gh browse script/
			#=> Open the script directory of the current repository

			$ gh browse 217
			#=> Open issue or pull request 217

			$ gh browse 77507cd94ccafcf568f8560cfecde965fcfa63
			#=> Open commit page

			$ gh browse --settings
			#=> Open repository settings

			$ gh browse main.go:312
			#=> Open main.go at line 312

			$ gh browse main.go --branch bug-fix
			#=> Open main.go with the repository at head of bug-fix branch

			$ gh browse main.go --commit=77507cd94ccafcf568f8560cfecde965fcfa63
			#=> Open main.go with the repository at commit 775007cd
		`),
		Annotations: map[string]string{
			"help:arguments": heredoc.Doc(`
				A browser location can be specified using arguments in the following format:
				- by number for issue or pull request, e.g. "123"; or
				- by path for opening folders and files, e.g. "cmd/gh/main.go"; or
				- by commit SHA
			`),
			"help:environment": heredoc.Doc(`
				To configure a web browser other than the default, use the BROWSER environment variable.
			`),
		},
		GroupID: "core",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if err := cmdutil.MutuallyExclusive(
				"arguments not supported when using `--projects`, `--releases`, `--settings`, or `--wiki`",
				opts.SelectorArg != "",
				opts.ProjectsFlag,
				opts.ReleasesFlag,
				opts.SettingsFlag,
				opts.WikiFlag,
			); err != nil {
				return err
			}

			if err := cmdutil.MutuallyExclusive(
				"specify only one of `--branch`, `--commit`, `--projects`, `--releases`, `--settings`, or `--wiki`",
				opts.Branch != "",
				opts.Commit != "",
				opts.ProjectsFlag,
				opts.ReleasesFlag,
				opts.SettingsFlag,
				opts.WikiFlag,
			); err != nil {
				return err
			}

			if (isNumber(opts.SelectorArg) || isCommit(opts.SelectorArg)) && (opts.Branch != "" || opts.Commit != "") {
				return cmdutil.FlagErrorf("%q is an invalid argument when using `--branch` or `--commit`", opts.SelectorArg)
			}

			if cmd.Flags().Changed("repo") || os.Getenv("GH_REPO") != "" {
				opts.GitClient = &remoteGitClient{opts.BaseRepo, opts.HttpClient}
				opts.HasRepoOverride = true
			}

			if runF != nil {
				return runF(opts)
			}
			return runBrowse(opts)
		},
	}

	cmdutil.EnableRepoOverride(cmd, f)
	cmd.Flags().BoolVarP(&opts.ProjectsFlag, "projects", "p", false, "Open repository projects")
	cmd.Flags().BoolVarP(&opts.ReleasesFlag, "releases", "r", false, "Open repository releases")
	cmd.Flags().BoolVarP(&opts.WikiFlag, "wiki", "w", false, "Open repository wiki")
	cmd.Flags().BoolVarP(&opts.SettingsFlag, "settings", "s", false, "Open repository settings")
	cmd.Flags().BoolVarP(&opts.NoBrowserFlag, "no-browser", "n", false, "Print destination URL instead of opening the browser")
	cmd.Flags().StringVarP(&opts.Commit, "commit", "c", "", "Select another commit by passing in the commit SHA, default is the last commit")
	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "Select another branch by passing in the branch name")

	_ = cmdutil.RegisterBranchCompletionFlags(f.GitClient, cmd, "branch")

	// Preserve backwards compatibility for when commit flag used to be a boolean flag.
	cmd.Flags().Lookup("commit").NoOptDefVal = emptyCommitFlag

	return cmd
}

func runBrowse(opts *BrowseOptions) error {
	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("unable to determine base repository: %w", err)
	}

	if opts.Commit != "" && opts.Commit == emptyCommitFlag {
		commit, err := opts.GitClient.LastCommit()
		if err != nil {
			return err
		}
		opts.Commit = commit.Sha
	}

	section, err := parseSection(baseRepo, opts)
	if err != nil {
		return err
	}
	url := ghrepo.GenerateRepoURL(baseRepo, "%s", section)

	if opts.NoBrowserFlag {
		client, err := opts.HttpClient()
		if err != nil {
			return err
		}

		exist, err := api.RepoExists(api.NewClientFromHTTP(client), baseRepo)
		if err != nil {
			return err
		}
		if !exist {
			return fmt.Errorf("%s doesn't exist", text.DisplayURL(url))
		}
		_, err = fmt.Fprintln(opts.IO.Out, url)
		return err
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", text.DisplayURL(url))
	}
	return opts.Browser.Browse(url)
}

func parseSection(baseRepo ghrepo.Interface, opts *BrowseOptions) (string, error) {
	if opts.ProjectsFlag {
		return "projects", nil
	} else if opts.ReleasesFlag {
		return "releases", nil
	} else if opts.SettingsFlag {
		return "settings", nil
	} else if opts.WikiFlag {
		return "wiki", nil
	}

	ref := opts.Branch
	if opts.Commit != "" {
		ref = opts.Commit
	}

	if ref == "" {
		if opts.SelectorArg == "" {
			return "", nil
		}
		if isNumber(opts.SelectorArg) {
			return fmt.Sprintf("issues/%s", strings.TrimPrefix(opts.SelectorArg, "#")), nil
		}
		if isCommit(opts.SelectorArg) {
			return fmt.Sprintf("commit/%s", opts.SelectorArg), nil
		}
	}

	if ref == "" {
		httpClient, err := opts.HttpClient()
		if err != nil {
			return "", err
		}
		apiClient := api.NewClientFromHTTP(httpClient)
		ref, err = api.RepoDefaultBranch(apiClient, baseRepo)
		if err != nil {
			return "", fmt.Errorf("error determining the default branch: %w", err)
		}
	}

	filePath, rangeStart, rangeEnd, err := parseFile(*opts, opts.SelectorArg)
	if err != nil {
		return "", err
	}

	if rangeStart > 0 {
		var rangeFragment string
		if rangeEnd > 0 && rangeStart != rangeEnd {
			rangeFragment = fmt.Sprintf("L%d-L%d", rangeStart, rangeEnd)
		} else {
			rangeFragment = fmt.Sprintf("L%d", rangeStart)
		}
		return fmt.Sprintf("blob/%s/%s?plain=1#%s", escapePath(ref), escapePath(filePath), rangeFragment), nil
	}

	return strings.TrimSuffix(fmt.Sprintf("tree/%s/%s", escapePath(ref), escapePath(filePath)), "/"), nil
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
	if !path.IsAbs(p) && !opts.HasRepoOverride {
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
	_, err := strconv.Atoi(strings.TrimPrefix(arg, "#"))
	return err == nil
}

// sha1 and sha256 are supported
var commitHash = regexp.MustCompile(`\A[a-f0-9]{7,64}\z`)

func isCommit(arg string) bool {
	return commitHash.MatchString(arg)
}

// gitClient is used to implement functions that can be performed on both local and remote git repositories
type gitClient interface {
	LastCommit() (*git.Commit, error)
}

type localGitClient struct {
	client *git.Client
}

type remoteGitClient struct {
	repo       func() (ghrepo.Interface, error)
	httpClient func() (*http.Client, error)
}

func (gc *localGitClient) LastCommit() (*git.Commit, error) {
	return gc.client.LastCommit(context.Background())
}

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
