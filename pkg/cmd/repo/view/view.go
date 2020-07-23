package view

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	HttpClient       func() (*http.Client, error)
	IO               *iostreams.IOStreams
	ResolvedBaseRepo func(*http.Client) (ghrepo.Interface, error)

	RepoArg string
	Web     bool
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := ViewOptions{
		IO:               f.IOStreams,
		HttpClient:       f.HttpClient,
		ResolvedBaseRepo: f.ResolvedBaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "view [<repository>]",
		Short: "View a repository",
		Long: `Display the description and the README of a GitHub repository.

With no argument, the repository for the current directory is displayed.

With '--web', open the repository in a web browser instead.`,
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

	return cmd
}

func viewRun(opts *ViewOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	var toView ghrepo.Interface
	if opts.RepoArg == "" {
		var err error
		toView, err = opts.ResolvedBaseRepo(httpClient)
		if err != nil {
			return err
		}
	} else {
		if isURL(opts.RepoArg) {
			parsedURL, err := url.Parse(opts.RepoArg)
			if err != nil {
				return fmt.Errorf("did not understand argument: %w", err)
			}

			toView, err = ghrepo.FromURL(parsedURL)
			if err != nil {
				return fmt.Errorf("did not understand argument: %w", err)
			}
		} else {
			var err error
			toView, err = ghrepo.FromFullName(opts.RepoArg)
			if err != nil {
				return fmt.Errorf("argument error: %w", err)
			}
		}
	}

	apiClient := api.NewClientFromHTTP(httpClient)

	repo, err := api.GitHubRepo(apiClient, toView)
	if err != nil {
		return err
	}

	openURL := generateRepoURL(toView, "")
	if opts.Web {
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", displayURL(openURL))
		}
		return utils.OpenInBrowser(openURL)
	}

	fullName := ghrepo.FullName(toView)

	readme, err := RepositoryReadme(httpClient, toView)
	var notFound *api.NotFoundError
	if err != nil && !errors.As(err, &notFound) {
		return err
	}

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

	var readmeContent string
	if readme == nil {
		readmeContent = utils.Gray("This repository does not have a README")
	} else if isMarkdownFile(readme.Filename) {
		var err error
		readmeContent, err = utils.RenderMarkdown(readme.Content)
		if err != nil {
			return fmt.Errorf("error rendering markdown: %w", err)
		}
	} else {
		readmeContent = readme.Content
	}

	description := repo.Description
	if description == "" {
		description = utils.Gray("No description provided")
	}

	repoData := struct {
		FullName    string
		Description string
		Readme      string
		View        string
	}{
		FullName:    utils.Bold(fullName),
		Description: description,
		Readme:      readmeContent,
		View:        utils.Gray(fmt.Sprintf("View this repository on GitHub: %s", openURL)),
	}

	err = tmpl.Execute(stdout, repoData)
	if err != nil {
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

// TODO COPYPASTA FROM command; CONSIDER FOR cmdutil?
func isURL(arg string) bool {
	return strings.HasPrefix(arg, "http:/") || strings.HasPrefix(arg, "https:/")
}

func generateRepoURL(repo ghrepo.Interface, p string, args ...interface{}) string {
	baseURL := fmt.Sprintf("https://%s/%s/%s", repo.RepoHost(), repo.RepoOwner(), repo.RepoName())
	if p != "" {
		return baseURL + "/" + fmt.Sprintf(p, args...)
	}
	return baseURL
}

func displayURL(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return urlStr
	}
	return u.Hostname() + u.Path
}
