package view

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"text/template"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/markdown"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type browser interface {
	Browse(string) error
}

type ViewOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser
	Exporter   cmdutil.Exporter

	RepoArg string
	Web     bool
	Branch  string
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
		Browser:    f.Browser,
	}

	cmd := &cobra.Command{
		Use:   "view [<repository>]",
		Short: "View a repository",
		Long: `Display the description and the README of a GitHub repository.

With no argument, the repository for the current directory is displayed.

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
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, api.RepositoryFields)

	return cmd
}

var defaultFields = []string{"name", "owner", "description"}

func viewRun(opts *ViewOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	var toView ghrepo.Interface
	apiClient := api.NewClientFromHTTP(httpClient)
	if opts.RepoArg == "" {
		var err error
		toView, err = opts.BaseRepo()
		if err != nil {
			return err
		}
	} else {
		var err error
		viewURL := opts.RepoArg
		if !strings.Contains(viewURL, "/") {
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

	var readme *RepoReadme
	fields := defaultFields
	if opts.Exporter != nil {
		fields = opts.Exporter.Fields()
	}

	repo, err := fetchRepository(apiClient, toView, fields)
	if err != nil {
		return err
	}

	if !opts.Web && opts.Exporter == nil {
		readme, err = RepositoryReadme(httpClient, toView, opts.Branch)
		if err != nil && !errors.Is(err, NotFoundError) {
			return err
		}
	}

	openURL := generateBranchURL(toView, opts.Branch)
	if opts.Web {
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(openURL))
		}
		return opts.Browser.Browse(openURL)
	}

	opts.IO.DetectTerminalTheme()
	if err := opts.IO.StartPager(); err != nil {
		return err
	}
	defer opts.IO.StopPager()

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO.Out, repo, opts.IO.ColorEnabled())
	}

	fullName := ghrepo.FullName(toView)
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
		{{.Description}}

		{{.Readme}}

		{{.View}}
	`)

	tmpl, err := template.New("repo").Parse(repoTmpl)
	if err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()

	var readmeContent string
	if readme == nil {
		readmeContent = cs.Gray("This repository does not have a README")
	} else if isMarkdownFile(readme.Filename) {
		var err error
		style := markdown.GetStyle(opts.IO.TerminalTheme())
		readmeContent, err = markdown.RenderWithBaseURL(readme.Content, style, readme.BaseURL)
		if err != nil {
			return fmt.Errorf("error rendering markdown: %w", err)
		}
	} else {
		readmeContent = readme.Content
	}

	description := repo.Description
	if description == "" {
		description = cs.Gray("No description provided")
	}

	repoData := struct {
		FullName    string
		Description string
		Readme      string
		View        string
	}{
		FullName:    cs.Bold(fullName),
		Description: description,
		Readme:      readmeContent,
		View:        cs.Gray(fmt.Sprintf("View this repository on GitHub: %s", openURL)),
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

func generateBranchURL(r ghrepo.Interface, branch string) string {
	if branch == "" {
		return ghrepo.GenerateRepoURL(r, "")
	}

	return ghrepo.GenerateRepoURL(r, "tree/%s", url.QueryEscape(branch))
}
