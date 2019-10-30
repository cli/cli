package command

import (
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/context"
	"github.com/github/gh-cli/git"
	"github.com/github/gh-cli/utils"
	"github.com/spf13/cobra"
	"os"
	"runtime"
)

var (
	_draftF bool
	_titleF string
	_bodyF  string
	_baseF  string
)

func prCreate(ctx context.Context) error {
	ucc, err := git.UncommittedChangeCount()
	if err != nil {
		return fmt.Errorf(
			"could not determine state of working directory: %v", err)
	}
	if ucc > 0 {
		noun := "change"
		if ucc > 1 {
			noun = noun + "s"
		}

		fmt.Printf("%s %d uncommitted %s %s", utils.Red("!!!"),
			ucc, noun, utils.Red("!!!"))
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
				fmt.Println(utils.Red("Discarding PR."))
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

	payload, err := api.CreatePullRequest(client, repo, title, body, _draftF, base, head)
	if err != nil {
		return fmt.Errorf("failed to create PR: %s", err)
	}

	fmt.Println(payload)

	return nil
}

func guessRemote(ctx context.Context) (string, error) {
	remotes, err := ctx.Remotes()
	if err != nil {
		return "", fmt.Errorf("could not determine suitable remote: %s", err)
	}

	remote, err := remotes.FindByName("origin", "github")
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
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := contextForCommand(cmd)
		return prCreate(ctx)
	},
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
