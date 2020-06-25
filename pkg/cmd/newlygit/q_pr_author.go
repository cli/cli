package newlygit

import (
	"fmt"
	"net/http"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/utils"
)

func pr_author(client *http.Client, baseRepo ghrepo.Interface) (bool, error) {
	prs, err := randomPullRequests(client, baseRepo, 1)
	if err != nil {
		return false, err
	}
	pr := prs[0]

	contributors, err := allContributors(client, baseRepo)
	if err != nil {
		return false, err
	}

	options := shuffleOptions(contributors, pr.Author.Login, 4)
	message := fmt.Sprintf("Who was the author of the PR titled \"%s\"", pr.Title)
	var qs = []*survey.Question{
		{
			Name: "q",
			Prompt: &survey.Select{
				Message: message,
				Options: options,
			},
		},
	}

	answers := struct{ Q string }{}

	err = survey.Ask(qs, &answers)
	if err != nil {
		return false, err
	}

	fmt.Printf("🌭 %q %q\n", answers.Q, pr.Author.Login)
	correct := false
	if answers.Q == pr.Author.Login {
		correct = true
		fmt.Printf("%s", utils.Green("\n🎉 CORRECT 🎉\n"))
	} else {
		fmt.Printf("%s", utils.Red("\n😫 WRONG 😫\n"))
	}

	fmt.Printf("The answer is %s\n\n", utils.Green(pr.Author.Login))
	fmt.Printf("Check out the pr at %s\n", utils.Bold(pr.URL))

	return correct, nil
}
