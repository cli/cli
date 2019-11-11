package command

import (
	"fmt"
	"os"
	"runtime"

	"github.com/AlecAivazis/survey/v2"
	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/context"
	"github.com/github/gh-cli/git"
	"github.com/spf13/cobra"
)

var (
	_draftF bool
	_titleF string
	_bodyF  string
	_baseF  string
)

func prCreate(cmd *cobra.Command, _ []string) error {
	ctx := contextForCommand(cmd)

	ucc, err := git.UncommittedChangeCount()
	if err != nil {
		return err
	}
	if ucc > 0 {
		noun := "change"
		if ucc > 1 {
			// TODO: use pluralize helper
			noun = noun + "s"
		}

		cmd.Printf("Warning: %d uncommitted %s\n", ucc, noun)
	}

	head, err := ctx.Branch()
	if err != nil {
		return fmt.Errorf("could not determine current branch: %s", err)
	}

	remote, err := guessRemote(ctx)
	if err != nil {
		return err
	}

	if err = git.Push(remote, fmt.Sprintf("HEAD:%s", head)); err != nil {
		return fmt.Errorf("was not able to push to remote '%s': %s", remote, err)
	}

	interactive := _titleF == "" || _bodyF == ""

	inProgress := struct {
		Body  string
		Title string
	}{}

	if interactive {
		confirmed := false
		editor := determineEditor()

		for !confirmed {
			titleQuestion := &survey.Question{
				Name: "title",
				Prompt: &survey.Input{
					Message: "PR Title",
					Default: inProgress.Title,
				},
			}
			bodyQuestion := &survey.Question{
				Name: "body",
				Prompt: &survey.Editor{
					Message:       fmt.Sprintf("PR Body (%s)", editor),
					FileName:      "*.md",
					Default:       inProgress.Body,
					AppendDefault: true,
					Editor:        editor,
				},
			}

			qs := []*survey.Question{}
			if _titleF == "" {
				qs = append(qs, titleQuestion)
			}
			if _bodyF == "" {
				qs = append(qs, bodyQuestion)
			}

			err := survey.Ask(qs, &inProgress)
			if err != nil {
				return fmt.Errorf("could not prompt: %s", err)
			}
			confirmAnswers := struct {
				Confirmation string
			}{}
			confirmQs := []*survey.Question{
				{
					Name: "confirmation",
					Prompt: &survey.Select{
						Message: "Submit?",
						Options: []string{
							"Yes",
							"Edit",
							"Cancel",
						},
					},
				},
			}

			err = survey.Ask(confirmQs, &confirmAnswers)
			if err != nil {
				return fmt.Errorf("could not prompt: %s", err)
			}

			switch confirmAnswers.Confirmation {
			case "Yes":
				confirmed = true
			case "Edit":
				continue
			case "Cancel":
				cmd.Println("Discarding pull request")
				return nil
			}
		}
	}

	title := _titleF
	if title == "" {
		title = inProgress.Title
	}
	body := _bodyF
	if body == "" {
		body = inProgress.Body
	}
	base := _baseF
	if base == "" {
		base = "master"
	}

	client, err := apiClientForContext(ctx)
	if err != nil {
		return fmt.Errorf("could not initialize api client: %s", err)
	}

	repo, err := ctx.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine GitHub repo: %s", err)
	}

	params := map[string]interface{}{
		"title":       title,
		"body":        body,
		"draft":       _draftF,
		"baseRefName": base,
		"headRefName": head,
	}

	pr, err := api.CreatePullRequest(client, repo, params)
	if err != nil {
		return fmt.Errorf("failed to create PR: %s", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), pr.URL)
	return nil
}

func guessRemote(ctx context.Context) (string, error) {
	remotes, err := ctx.Remotes()
	if err != nil {
		return "", fmt.Errorf("could not determine suitable remote: %s", err)
	}

	// TODO: consolidate logic with fsContext.BaseRepo
	// TODO: check if the GH repo that the remote points to is writeable
	remote, err := remotes.FindByName("upstream", "github", "origin", "*")
	if err != nil {
		return "", fmt.Errorf("could not determine suitable remote: %s", err)
	}

	return remote.Name, nil
}

func determineEditor() string {
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "nano"
}

var prCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a pull request",
	RunE:  prCreate,
}

func init() {
	prCreateCmd.Flags().BoolVarP(&_draftF, "draft", "d", false,
		"Mark PR as a draft")
	prCreateCmd.Flags().StringVarP(&_titleF, "title", "t", "",
		"Supply a title. Will prompt for one otherwise.")
	prCreateCmd.Flags().StringVarP(&_bodyF, "body", "b", "",
		"Supply a body. Will prompt for one otherwise.")
	prCreateCmd.Flags().StringVarP(&_baseF, "base", "T", "",
		"The branch into which you want your code merged")

}
