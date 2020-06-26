package newlygit

import (
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/utils"
)

func pr_opened(client *http.Client, baseRepo ghrepo.Interface) (bool, error) {
	prs, err := randomPullRequests(client, baseRepo, 1)
	if err != nil {
		return false, err
	}
	pr := prs[0]

	options, err := randomizedDateOptions(pr.CreatedAt)
	if err != nil {
		return false, err
	}

	message := fmt.Sprintf(`When was the PR "%s" by %s opened?`, pr.Title, pr.Author.Login)
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
	x, _ := time.Parse(time.RFC3339, pr.CreatedAt)
	if answers.Q == f(x) {
		correct = true
		fmt.Printf("%s", utils.Green("\n🎉 CORRECT 🎉\n"))
	} else {
		fmt.Printf("%s", utils.Red("\n😫 WRONG 😫\n"))
	}

	fmt.Printf("The answer is %s\n\n", utils.Green(f(x)))
	fmt.Printf("Check out the pr at %s\n", utils.Bold(pr.URL))

	return correct, nil
}

func randomizedDateOptions(dateString string) ([]string, error) {
	fmt.Printf("🌭 %+v\n", dateString)
	correctDate, err := time.Parse(time.RFC3339, dateString)
	if err != nil {
		return nil, err
	}

	o := []int{-60 * 24, -30 * 24, -15 * 24, 15 * 24, 30 * 24, 60 * 24}

	rand.Shuffle(len(o), func(i, j int) {
		o[i], o[j] = o[j], o[i]
	})

	options := []string{f(correctDate)}
	for _, x := range o[0:3] {
		d, _ := time.ParseDuration(strconv.Itoa(x) + "h")
		options = append(options, f(correctDate.Add(d)))
	}
	rand.Shuffle(len(options), func(i, j int) {
		options[i], options[j] = options[j], options[i]
	})
	return options, nil
}

func f(t time.Time) string {
	return t.Format("Mon Jan 2, 2006")
}
