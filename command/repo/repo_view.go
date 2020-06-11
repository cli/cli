package repo

import (
	"fmt"
	"net/url"
	"text/template"

	"github.com/cli/cli/api"
	"github.com/cli/cli/command"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

func RepoViewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view [<repository>]",
		Short: "View a repository",
		Long: `Display the description and the README of a GitHub repository.
With no argument, the repository for the current directory is displayed.
With '--web', open the repository in a web browser instead.`,
		RunE: repoView,
	}

	cmd.Flags().BoolP("web", "w", false, "Open a repository in the browser")

	return cmd
}

func repoView(cmd *cobra.Command, args []string) error {
	ctx := command.ContextForCommand(cmd)
	apiClient, err := command.ApiClientForContext(ctx)
	if err != nil {
		return err
	}

	var toView ghrepo.Interface
	if len(args) == 0 {
		var err error
		toView, err = command.DetermineBaseRepo(apiClient, cmd, ctx)
		if err != nil {
			return err
		}
	} else {
		repoArg := args[0]
		if isURL(repoArg) {
			parsedURL, err := url.Parse(repoArg)
			if err != nil {
				return fmt.Errorf("did not understand argument: %w", err)
			}

			toView, err = ghrepo.FromURL(parsedURL)
			if err != nil {
				return fmt.Errorf("did not understand argument: %w", err)
			}
		} else {
			var err error
			toView, err = ghrepo.FromFullName(repoArg)
			if err != nil {
				return fmt.Errorf("argument error: %w", err)
			}
		}
	}

	repo, err := api.GitHubRepo(apiClient, toView)
	if err != nil {
		return err
	}

	web, err := cmd.Flags().GetBool("web")
	if err != nil {
		return err
	}

	fullName := ghrepo.FullName(toView)

	openURL := fmt.Sprintf("https://github.com/%s", fullName)
	if web {
		fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in your browser.\n", command.DisplayURL(openURL))
		return utils.OpenInBrowser(openURL)
	}

	repoTmpl := `
{{.FullName}}
{{.Description}}

{{.Readme}}

{{.View}}
`

	tmpl, err := template.New("repo").Parse(repoTmpl)
	if err != nil {
		return err
	}

	readmeContent, _ := api.RepositoryReadme(apiClient, fullName)

	if readmeContent == "" {
		readmeContent = utils.Gray("No README provided")
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

	out := command.ColorableOut(cmd)

	err = tmpl.Execute(out, repoData)
	if err != nil {
		return err
	}

	return nil
}
