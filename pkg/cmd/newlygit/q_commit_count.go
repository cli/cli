package newlygit

import (
	"fmt"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/utils"
)

func commit_count(client *http.Client, baseRepo ghrepo.Interface) (bool, error) {
	cmd := git.GitCommand("shortlog", "-s", "-n", "--no-merges", "trunk")
	output, err := run.PrepareCmd(cmd).Output()
	if err != nil {
		return false, fmt.Errorf("unknown git command")
	}

	type cont struct {
		Name  string
		Count string
	}
	counts := []cont{}
	for _, line := range strings.Split(string(output), "\n") {
		r, _ := regexp.Compile(`^\s+(\d+)\s+(.*)`)
		matches := r.FindStringSubmatch(line)
		if len(matches) > 2 {
			counts = append(counts, cont{matches[2], matches[1]})
		}
	}

	a := counts[rand.Intn(len(counts))]

	questionMessage := fmt.Sprintf("How many commits has %s made to %s/%s?", a.Name, baseRepo.RepoOwner(), baseRepo.RepoName())
	var qs = []*survey.Question{
		{
			Name: "q",
			Prompt: &survey.Select{
				Message: questionMessage,
				Options: randomizedAnswers(a.Count),
			},
		},
	}

	answers := struct{ Q string }{}

	err = survey.Ask(qs, &answers)
	if err != nil {
		return false, err
	}

	correct := false
	if answers.Q == a.Count {
		correct = true
		fmt.Printf("%s", utils.Green("\n🎉 CORRECT 🎉\n"))
	} else {
		fmt.Printf("%s", utils.Red("\n😫 WRONG 😫\n"))
	}

	if a.Count == "1" {
		fmt.Printf("%s has made %s commit\n\n", a.Name, utils.Green(a.Count))
	} else {
		fmt.Printf("%s has made %s commits\n\n", a.Name, utils.Green(a.Count))
	}

	return correct, nil

	return true, nil
}

func randomizedAnswers(aString string) []string {
	aInt, _ := strconv.Atoi(aString)
	o := []float32{}
	if aInt < 8 {
		o = []float32{2, 4, 8, 16}
	} else {
		o = []float32{1 / 4, 1 / 2, 1 / 3, 2, 3, 4}
	}

	rand.Shuffle(len(o), func(i, j int) {
		o[i], o[j] = o[j], o[i]
	})

	answers := []string{aString}
	for _, x := range o[0:3] {
		answers = append(answers, strconv.Itoa(int(float32(aInt)*x)))
	}
	rand.Shuffle(len(answers), func(i, j int) {
		answers[i], answers[j] = answers[j], answers[i]
	})
	return answers
}
