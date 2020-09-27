package view

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"text/template"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/enescakir/emoji"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	RepoArg string
	Web     bool
	Branch  string
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "view [<repository> | <.>]",
		Short: "View a repository",
		Long: `Display the description and the README of a GitHub repository.

With no argument, the repository for the current directory is displayed.

With '.', view the current repository's branch, if the branch is not remote then shows the local readme file (overrides branch flag)

With '--web', open the repository in a web browser instead.

With '--branch', view a specific branch of the repository.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RepoArg = args[0]
			}
			if runF != nil {
				return runF(&opts)
			}
			return viewRun(&opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open a repository in the browser")
	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "View a specific branch of the repository")

	return cmd
}

func viewRun(opts *ViewOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	var toView, baseRepo ghrepo.Interface
	apiClient := api.NewClientFromHTTP(httpClient)
	branchName := opts.Branch
	if opts.RepoArg == "" {
		var err error
		toView, err = opts.BaseRepo()

		if err != nil {
			return err
		}
	} else {
		var err error
		viewURL := opts.RepoArg

		if opts.RepoArg == "." {
			currentUser, err := api.CurrentLoginName(apiClient, ghinstance.Default())
			if err != nil {
				return err
			}

			baseRepo, err = opts.BaseRepo()
			if err != nil {
				return err
			}

			branchName, err = git.CurrentBranch()
			if err != nil {
				return err
			}

			viewURL = currentUser + "/" + baseRepo.RepoName()
		} else if !strings.Contains(viewURL, "/") {
			currentUser, err := api.CurrentLoginName(apiClient, ghinstance.Default())
			if err != nil {
				return err
			}

			viewURL = currentUser + "/" + viewURL
		}

		toView, err = ghrepo.FromFullName(viewURL)
		if err != nil {
			return fmt.Errorf("argument error: %w", err)
		}
	}

	repo, err := api.GitHubRepo(apiClient, toView)
	if err != nil {
		return err
	}

	openURL := ghrepo.GenerateRepoURL(toView, "")
	if opts.Web {
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(openURL))
		}
		return utils.OpenInBrowser(openURL)
	}

	fullName := ghrepo.FullName(toView)

	isRemoteReadme := true
	readme, err := RepositoryReadme(httpClient, toView, branchName)

	var readmePath string
	if readme == nil && opts.RepoArg == "." {
		topLevelDir, _ := git.ToplevelDir()
		var files string
		filepath.Walk(topLevelDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			files = files + " " + path
			return nil
		})
		re := regexp.MustCompile(regexp.QuoteMeta(topLevelDir) + `/(?i)readme\.md`)
		readmePath = re.FindString(files)
		readmeContent, err := ioutil.ReadFile(readmePath)

		if err != nil {
			return err
		}

		isRemoteReadme = false
		readme = &RepoReadme{
			Filename: readmePath,
			Content:  string(readmeContent),
		}
	}

	if err != nil && err != NotFoundError {
		return err
	}

	err = opts.IO.StartPager()
	if err != nil {
		return err
	}
	defer opts.IO.StopPager()

	stdout := opts.IO.Out

	if !opts.IO.IsStdoutTTY() {
		fmt.Fprintf(stdout, "name:\t%s\n", fullName)
		fmt.Fprintf(stdout, "description:\t%s\n", repo.Description)
		if readme != nil {
			fmt.Fprintln(stdout, "--")
			fmt.Fprintf(stdout, readme.Content)
			fmt.Fprintln(stdout)
		}

		return nil
	}

	repoTmpl := heredoc.Doc(`
		{{.FullName}}
		{{.ReadmeFile}}
		{{.Description}}
		
		{{.Readme}}
		
		{{.View}}
	`)

	tmpl, err := template.New("repo").Parse(repoTmpl)
	if err != nil {
		return err
	}

	var readmeContent string
	if readme == nil {
		readmeContent = utils.Gray("This repository does not have a README")
	} else if isMarkdownFile(readme.Filename) {
		var err error
		readmeContent, err = utils.RenderMarkdown(readme.Content)
		if err != nil {
			return fmt.Errorf("error rendering markdown: %w", err)
		}
		readmeContent = emoji.Parse(readmeContent)
	} else {
		readmeContent = emoji.Parse(readme.Content)
	}

	description := repo.Description
	if description == "" {
		description = utils.Gray("No description provided")
	}

	branchPattern := ""
	if branchName != "" {
		branchPattern = "tree/%s"
	}

	readmeUrl := ghrepo.GenerateRepoURL(toView, branchPattern, branchName)
	readmeFileDescription := readmeUrl + " (remote)"

	if !isRemoteReadme {
		readmeFileDescription = readmePath + " (local)"
	}

	repoData := struct {
		FullName    string
		ReadmeFile  string
		Description string
		Readme      string
		View        string
	}{
		FullName:    utils.Bold(fullName),
		ReadmeFile:  readmeFileDescription,
		Description: description,
		Readme:      readmeContent,
		View:        utils.Gray(fmt.Sprintf("View this repository on GitHub: %s", openURL)),
	}

	err = tmpl.Execute(stdout, repoData)
	if err != nil && !errors.Is(err, syscall.EPIPE) {
		return err
	}

	return nil
}

func isMarkdownFile(filename string) bool {
	// kind of gross, but i'm assuming that 90% of the time the suffix will just be .md. it didn't
	// seem worth executing a regex for this given that assumption.
	return strings.HasSuffix(filename, ".md") ||
		strings.HasSuffix(filename, ".markdown") ||
		strings.HasSuffix(filename, ".mdown") ||
		strings.HasSuffix(filename, ".mkdown")
}
