package command

import (
	"fmt"

	"github.com/github/gh/git"
	"github.com/github/gh/github"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(prCmd)
	prCmd.AddCommand(prListCmd)
}

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Work with pull requests",
	Long: `This command allows you to
work with pull requests.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("pr")
	},
}

var prListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pull requests",
	Run: func(cmd *cobra.Command, args []string) {
		ExecutePr()
	},
}

type prFilter int

const (
	createdByViewer prFilter = iota
	reviewRequested
)

func ExecutePr() {
	// prsForCurrentBranch := pullRequestsForCurrentBranch()
	prsCreatedByViewer := pullRequests(createdByViewer)
	// prsRequestingReview := pullRequests(reviewRequested)

	fmt.Printf("🌭 count! %d\n", len(prsCreatedByViewer))
}

type searchBody struct {
	Items []github.PullRequest `json:"items"`
}

func pullRequestsForCurrentBranch() []github.PullRequest {
	project := project()
	client := github.NewClient(project.Host)
	currentBranch, error := git.Head()
	if error != nil {
		panic(error)
	}

	headWithOwner := fmt.Sprintf("%s:%s", project.Owner, currentBranch)
	filterParams := map[string]interface{}{"headWithOwner": headWithOwner}
	prs, error := client.FetchPullRequests(&project, filterParams, 10, nil)
	if error != nil {
		panic(error)
	}

	return prs
}

func pullRequests(filter prFilter) []github.PullRequest {
	project := project()
	client := github.NewClient(project.Host)
	owner := project.Owner
	name := project.Name
	user, error := client.CurrentUser()
	if error != nil {
		panic(error)
	}

	var headers map[string]string
	var q string
	if filter == createdByViewer {
		q = fmt.Sprintf("user:%s repo:%s state:open is:pr author:%s", owner, name, user.Login)
	} else if filter == reviewRequested {
		q = fmt.Sprintf("user:%s repo:%s state:open review-requested:%s", owner, name, user.Login)
	} else {
		panic("This is not a fitler")
	}

	data := map[string]interface{}{"q": q}

	response, error := client.GenericAPIRequest("GET", "search/issues", data, headers, 60)
	if error != nil {
		panic(fmt.Sprintf("GenericAPIRequest failed %+v", error))
	}
	searchBody := searchBody{}
	error = response.Unmarshal(&searchBody)
	if error != nil {
		panic(fmt.Sprintf("Unmarshal failed %+v", error))
	}

	return searchBody.Items
}

func project() github.Project {
	remotes, error := github.Remotes()
	if error != nil {
		panic(error)
	}

	for _, remote := range remotes {
		if project, error := remote.Project(); error == nil {
			return *project
		}
	}

	panic("Could not get the project. What is a project? I don't know, it's kind of like a git repository I think?")
}
