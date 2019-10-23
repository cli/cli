package command

import (
	"github.com/github/gh-cli/api"
	//	"github.com/github/gh-cli/context"
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	"github.com/github/gh-cli/git"
	"github.com/github/gh-cli/utils"
	"github.com/spf13/cobra"
	"os"
	"runtime"
)

var _draftF bool
var _titleF string
var _bodyF string
var _baseF string

func prCreate() error {
	ucc, err := git.UncommittedChangeCount()
	if err != nil {
		return fmt.Errorf(
			"could not determine state of working directory: %v", err)
	}
	if ucc > 0 {
		noun := "change"
		if ucc > 1 {
			noun = "changes"
		}

		fmt.Printf(
			"%s %d uncommitted %s %s",
			utils.Red("!!!"),
			ucc,
			noun,
			utils.Red("!!!"))
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

	// TODO either use inProgress values or titleF/bodyF as needed to issue gql stuff
	// decide if i want to split up target negotiation across git/this package; figure out new gql
	// api stuff; punt on tracked branches

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
		base = "origin:master"
	}

	payload, err := api.CreatePullRequest(title, body, _draftF, base)
	if err != nil {
		return fmt.Errorf("failed to create PR: %s", err)
	}

	fmt.Println(payload)

	// TODO do something with payload (print URL)

	return nil
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
		return prCreate()
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
		"The target branch you want your PR merged into in the format remote:branch.")
}
