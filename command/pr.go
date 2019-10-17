package command

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"

	"github.com/AlecAivazis/survey/v2"
	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/context"
	"github.com/github/gh-cli/git"
	"github.com/github/gh-cli/github"
	"github.com/github/gh-cli/ui"
	"github.com/github/gh-cli/utils"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(prCmd)
	prCmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List pull requests",
			RunE:  prList,
		},
		&cobra.Command{
			Use:   "view [pr-number]",
			Short: "Open a pull request in the browser",
			Long: `Opens the pull request in the web browser.

When <pr-number> is not given, the pull request that belongs to the current
branch is opened.
`,
			RunE: prView,
		},
		&cobra.Command{
			Use:   "create",
			Short: "Create a pull request",
			Run: func(cmd *cobra.Command, args []string) {
				createPr(args...)
			},
		},
		&cobra.Command{
			Use:   "checkout <pr-number>",
			Short: "Check out a pull request in git",
			RunE: func(cmd *cobra.Command, args []string) error {
				prNumber := ""
				if len(args) > 0 {
					prNumber = args[0]
				}
				return checkoutPr(prNumber)
			},
		},
	)
}

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Work with pull requests",
	Long: `Interact with pull requests for this repository.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return interactiveList()
	},
}

type prFilter int

type prCreateInput struct {
	Title string
	Body  string
}

const (
	createdByViewer prFilter = iota
	reviewRequested
)

func determineEditor() string {
	// TODO THIS IS PROBABLY GROSS
	// I copied this from survey because i wanted to use the same logic as them
	// for now.
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	} else if e := os.Getenv("EDITOR"); e != "" {
		return e
	}

	return "nano"
}

func createPrSurvey(inProgress prCreateInput) (prCreateInput, error) {
	editor := determineEditor()
	qs := []*survey.Question{
		{
			Name: "title",
			Prompt: &survey.Input{
				Message: "PR Title",
				Default: inProgress.Title,
			},
		},
		{
			Name: "body",
			Prompt: &survey.Editor{
				Message:       fmt.Sprintf("PR Body (%s)", editor),
				FileName:      "*.md",
				Default:       inProgress.Body,
				AppendDefault: true,
				Editor:        editor,
			},
		},
	}

	err := survey.Ask(qs, &inProgress)
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		return inProgress, err
	}

	return inProgress, nil
}

func createPr(...string) {
	// TODO i wanted some information here:
	// - whether current branch was pushed yet
	// - whether PR was already open for this branch
	// - if working directory was dirty
	// - what branch is targeted
	// and also wanted to take some flags:
	// - --target for a different place to land a PR
	// - --no-push to disable the initial push

	confirmed := false

	inProgress := prCreateInput{}
	for !confirmed {
		inProgress, _ = createPrSurvey(inProgress)

		ui.Println(inProgress.Body)

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
						"Cancel and discard",
					},
				},
			},
		}

		err := survey.Ask(confirmQs, &confirmAnswers)
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
			return
		}

		switch confirmAnswers.Confirmation {
		case "Yes":
			confirmed = true
		case "Edit":
			continue
		case "Cancel and discard":
			ui.Println("Discarding PR.")
			return
		}
	}

	// TODO this is quite silly for now; but i expect that survey will intake
	// slightly different data than the CPR API wants sooner than later
	prParams := github.PullRequestParams{
		Title: inProgress.Title,
		Body:  inProgress.Body,
	}

	err := github.CreatePullRequest(prParams)
	if err != nil {
		// I'd like this to go even higher but this works for now
		utils.Check(err)
	}
}

func interactiveList() error {
	payload, err := api.PullRequests()
	if err != nil {
		return err
	}

	prs := []api.PullRequest{}
	if payload.CurrentPR != nil {
		prs = append(prs, *payload.CurrentPR)
	}
	prs = append(prs, payload.ViewerCreated...)
	prs = append(prs, payload.ReviewRequested...)

	const openAction = "open in browser"
	const checkoutAction = "checkout PR locally"
	const cancelAction = "cancel"

	prOptions := []string{}
	seen := map[int]bool{}
	for _, pr := range prs {
		if seen[pr.Number] {
			continue
		}
		prOptions = append(prOptions, fmt.Sprintf("[%v] %s", pr.Number, pr.Title))
		seen[pr.Number] = true
	}

	// TODO figure out how to visually seperate the PR list
	qs := []*survey.Question{
		{
			Name: "pr",
			Prompt: &survey.Select{
				Message: "PRs you might be interested in",
				Options: prOptions,
			},
		},
		{
			Name: "action",
			Prompt: &survey.Select{
				Message: "What would you like to do?",
				Options: []string{
					openAction,
					checkoutAction,
					cancelAction,
				},
			},
		},
	}

	answers := struct {
		Pr     int
		Action string
	}{}

	err = survey.Ask(qs, &answers)
	if err != nil {
		return err
	}

	actions := map[string]func() error{}

	actions[cancelAction] = func() error { return nil }
	actions[openAction] = func() error {
		launcher, err := utils.BrowserLauncher()
		if err != nil {
			return err
		}
		exec.Command(launcher[0], prs[answers.Pr].URL).Run()
		return nil
	}
	actions[checkoutAction] = func() error {
		pr := prs[answers.Pr]
		return checkoutPr(fmt.Sprintf("%v", pr.Number))
	}

	return actions[answers.Action]()

}

func prList(cmd *cobra.Command, args []string) error {
	prPayload, err := api.PullRequests()
	if err != nil {
		return err
	}

	printHeader("Current branch")
	if prPayload.CurrentPR != nil {
		printPrs(*prPayload.CurrentPR)
	} else {
		currentBranch, err := context.Current().Branch()
		if err != nil {
			return err
		}
		message := fmt.Sprintf("  There is no pull request associated with %s", utils.Cyan("["+currentBranch+"]"))
		printMessage(message)
	}
	fmt.Println()

	printHeader("Created by you")
	if len(prPayload.ViewerCreated) > 0 {
		printPrs(prPayload.ViewerCreated...)
	} else {
		printMessage("  You have no open pull requests")
	}
	fmt.Println()

	printHeader("Requesting a code review from you")
	if len(prPayload.ReviewRequested) > 0 {
		printPrs(prPayload.ReviewRequested...)
	} else {
		printMessage("  You have no pull requests to review")
	}
	fmt.Println()

	return nil
}

func prView(cmd *cobra.Command, args []string) error {
	baseRepo, err := context.Current().BaseRepo()
	if err != nil {
		return err
	}
	var openURL string
	if len(args) > 0 {
		if prNumber, err := strconv.Atoi(args[0]); err == nil {
			// TODO: move URL generation into GitHubRepository
			openURL = fmt.Sprintf("https://github.com/%s/%s/pull/%d", baseRepo.Owner, baseRepo.Name, prNumber)
		} else {
			return fmt.Errorf("invalid pull request number: '%s'", args[0])
		}
	} else {
		prPayload, err := api.PullRequests()
		if err != nil || prPayload.CurrentPR == nil {
			branch, err := context.Current().Branch()
			if err != nil {
				return err
			}
			return fmt.Errorf("The [%s] branch has no open PRs", branch)
		}
		openURL = prPayload.CurrentPR.URL
	}

	fmt.Printf("Opening %s in your browser.\n", openURL)
	return openInBrowser(openURL)
}

// TODO: pullRequests(first: $per_page, states: OPEN, orderBy: {field: CREATED_AT, direction: DESC})
func openPullRequests() ([]api.PullRequest, error) {
	return []api.PullRequest{}, nil
}

func printPrs(prs ...api.PullRequest) {
	for _, pr := range prs {
		fmt.Printf("  #%d %s %s\n", pr.Number, truncateTitle(pr.Title), utils.Cyan("["+pr.HeadRefName+"]"))
	}
}

func printHeader(s string) {
	fmt.Println(utils.Bold(s))
}

func printMessage(s string) {
	fmt.Println(utils.Gray(s))
}

func truncateTitle(title string) string {
	const maxLength = 50

	if len(title) > maxLength {
		return title[0:maxLength-3] + "..."
	}
	return title
}

func openInBrowser(url string) error {
	launcher, err := utils.BrowserLauncher()
	if err != nil {
		return err
	}
	endingArgs := append(launcher[1:], url)
	return exec.Command(launcher[0], endingArgs...).Run()
}

func checkoutPr(number string) error {
	if number == "" {
		return checkoutMenu()
	}

	_, err := strconv.Atoi(number)
	if err != nil {
		return err
	}

	baseRepo, err := context.Current().BaseRepo()
	if err != nil {
		return err
	}
	client := github.NewClient("github.com")
	pullRequest, err := client.PullRequest(github.NewProject(baseRepo.Owner, baseRepo.Name, ""), number)
	if err != nil {
		return err
	}

	repo, err := github.LocalRepo()
	if err != nil {
		return err
	}

	baseRemote, err := repo.RemoteForRepo(pullRequest.Base.Repo)
	if err != nil {
		return err
	}

	var headRemote *github.Remote
	if pullRequest.IsSameRepo() {
		headRemote = baseRemote
	} else if pullRequest.Head.Repo != nil {
		headRemote, _ = repo.RemoteForRepo(pullRequest.Head.Repo)
	}

	newBranchName := ""
	if headRemote != nil {
		if newBranchName == "" {
			newBranchName = pullRequest.Head.Ref
		}
		remoteBranch := fmt.Sprintf("%s/%s", headRemote.Name, pullRequest.Head.Ref)
		refSpec := fmt.Sprintf("+refs/heads/%s:refs/remotes/%s", pullRequest.Head.Ref, remoteBranch)

		utils.Check(git.Run("fetch", headRemote.Name, refSpec))

		if git.HasFile("refs", "heads", newBranchName) {
			utils.Check(git.Run("checkout", newBranchName))
			utils.Check(git.Run("merge", "--ff-only", fmt.Sprintf("refs/remotes/%s", remoteBranch)))
		} else {
			utils.Check(git.Run("checkout", "-b", newBranchName, "--no-track", remoteBranch))
			utils.Check(git.Run("config", fmt.Sprintf("branch.%s.remote", newBranchName), headRemote.Name))
			utils.Check(git.Run("config", fmt.Sprintf("branch.%s.merge", newBranchName), "refs/heads/"+pullRequest.Head.Ref))
		}
	} else {
		if newBranchName == "" {
			newBranchName = pullRequest.Head.Ref
			if pullRequest.Head.Repo != nil && newBranchName == pullRequest.Head.Repo.DefaultBranch {
				newBranchName = fmt.Sprintf("%s-%s", pullRequest.Head.Repo.Owner.Login, newBranchName)
			}
		}

		ref := fmt.Sprintf("refs/pull/%d/head", pullRequest.Number)
		utils.Check(git.Run("fetch", baseRemote.Name, fmt.Sprintf("%s:%s", ref, newBranchName)))
		utils.Check(git.Run("checkout", newBranchName))

		remote := baseRemote.Name
		mergeRef := ref
		if pullRequest.MaintainerCanModify && pullRequest.Head.Repo != nil {
			headRepo := pullRequest.Head.Repo
			headProject := github.NewProject(headRepo.Owner.Login, headRepo.Name, "")
			remote = headProject.GitURL("", "", true)
			mergeRef = fmt.Sprintf("refs/heads/%s", pullRequest.Head.Ref)
		}
		utils.Check(git.Run("config", fmt.Sprintf("branch.%s.remote", newBranchName), remote))
		utils.Check(git.Run("config", fmt.Sprintf("branch.%s.merge", newBranchName), mergeRef))
	}
	return nil
}

func checkoutMenu() error {
	prs, err := openPullRequests()
	if err != nil {
		return err
	}

	currentBranch, err := context.Current().Branch()
	if err != nil {
		return err
	}
	prOptions := []string{}
	for _, pr := range prs {
		if pr.HeadRefName == currentBranch {
			continue
		}
		prOptions = append(prOptions, fmt.Sprintf("#%d - %s [%s]", pr.Number, pr.Title, pr.HeadRefName))
	}

	if len(prOptions) == 0 {
		return fmt.Errorf("no open pull requests found")
	}

	qs := []*survey.Question{
		{
			Name: "pr",
			Prompt: &survey.Select{
				Message: "Select the pull request to check out",
				Options: prOptions,
			},
		},
	}

	answers := struct {
		Pr int
	}{}

	err = survey.Ask(qs, &answers)
	if err != nil {
		return err
	}

	return checkoutPr(strconv.Itoa(prs[answers.Pr].Number))
}
