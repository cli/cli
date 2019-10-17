package github

import (
	"fmt"
	"strings"

	"github.com/github/gh-cli/git"
	"github.com/github/gh-cli/ui"
)

type PullRequestParams struct {
	// TODO git stuff, maybe, once reliance on localrepo phased out
	Body    string
	Title   string
	Draft   bool
	Target  string
	PrePush bool
}

func CreatePullRequest(prParams PullRequestParams) error {
	// TODO understand head/target stuff
	localRepo, err := LocalRepo()
	if err != nil {
		return err
	}

	currentBranch, cberr := localRepo.CurrentBranch()
	if cberr != nil {
		return cberr
	}

	baseProject, mperr := localRepo.MainProject()
	if mperr != nil {
		return err
	}

	host, err := CurrentConfig().PromptForHost(baseProject.Host)
	if err != nil {
		return FormatError("creating pull request", err)
	}
	client := NewClientWithHost(host)

	trackedBranch, headProject, _ := localRepo.RemoteBranchAndProject(host.User, false)
	if headProject == nil {
		return fmt.Errorf("could not determine project for head branch")
	}

	fmt.Printf("%+v\n", localRepo)
	fmt.Printf("%+v\n", host)
	fmt.Printf("%+v\n", currentBranch)
	fmt.Printf("%+v\n", baseProject)
	fmt.Printf("%+v\n", trackedBranch)
	fmt.Printf("%+v\n", headProject)

	var (
		base, head string
		baseRemote *Remote
	)

	if prParams.Target != "" {
		split := strings.SplitN(prParams.Target, ":", 2)
		baseRemote, err = localRepo.RemoteByName(split[0])
		if err != nil {
			return err
		}
		base = split[1]
	} else {
		baseRemote, _ = localRepo.RemoteForProject(baseProject)
		if base == "" && baseRemote != nil {
			base = localRepo.DefaultBranch(baseRemote).ShortName()
		}
	}

	if head == "" && trackedBranch != nil {
		if !trackedBranch.IsRemote() {
			// the current branch tracking another branch
			// pretend there's no upstream at all
			trackedBranch = nil
		} else {
			if baseProject.SameAs(headProject) && base == trackedBranch.ShortName() {
				e := fmt.Errorf(`Aborted: head branch is the same as base ("%s")`, base)
				e = fmt.Errorf("%s\n(use `-h <branch>` to specify an explicit pull request head)", e)
				if e != nil {
					return e
				}
			}
		}
	}

	if head == "" {
		if trackedBranch == nil {
			head = currentBranch.ShortName()
		} else {
			head = trackedBranch.ShortName()
		}
	}

	if headRepo, err := client.Repository(headProject); err == nil {
		headProject.Owner = headRepo.Owner.Login
		headProject.Name = headRepo.Name
	}

	fullHead := fmt.Sprintf("%s:%s", headProject.Owner, head)

	remote := baseRemote
	if remote == nil || !baseProject.SameAs(headProject) {
		remote, _ = localRepo.RemoteForProject(headProject)
	}

	if remote == nil {
		return fmt.Errorf("Can't find remote for %s", head)
	}

	fmt.Printf("%+v\n", trackedBranch)
	fmt.Printf("%+v\n", head)
	fmt.Printf("%+v\n", headProject)
	//fmt.Printf("%+v\n", headRepo)
	fmt.Printf("%+v\n", fullHead)
	fmt.Printf("%+v\n", remote)

	if prParams.PrePush {
		err = git.Run("push", "--set-upstream", remote.Name, fmt.Sprintf("HEAD:%s", head))

		if err != nil {
			return err
		}
	}

	// TODO use graphql
	headers := map[string]string{
		"Accept": "application/vnd.github.shadow-cat-preview+json",
	}
	repoIdQuery := `
query FindRepoID($owner:String!, $name:String!) {
	repository(owner:$owner, name:$name) {
		id
	}
}`
	variables := map[string]interface{}{
		"owner": baseProject.Owner,
		"name":  baseProject.Name,
	}
	data := map[string]interface{}{
		"variables": variables,
		"query":     repoIdQuery,
	}

	response, err := client.GenericAPIRequest("POST", "graphql", data, headers, 0)
	if err != nil {
		return err
	}

	type repoResult struct {
		Data struct {
			Repository struct {
				Id string
			}
		}
	}

	result := repoResult{}
	err = response.Unmarshal(&result)
	if err != nil {
		return err
	}
	repoId := result.Data.Repository.Id
	ui.Println(repoId)

	//	mutation := `
	//	mutation CreatePullRequest($input: CreatePullRequestInput!) {
	//	  createPullRequest(input: $input) {
	//		  pullRequest {
	//		    url
	//			}
	//		}
	//	}`
	//
	//	variables = map[string]interface{}{
	//		"input": map[string]interface{}{
	//			"repositoryId": repoId,
	//			"baseRefName":  base,
	//			"headRefName":  fullHead,
	//			"title":        prParams.Title,
	//			"body":         prParams.Body,
	//			"draft":        prParams.Draft,
	//		},
	//	}
	//	data = map[string]interface{}{
	//		"variables": variables,
	//		"query":     mutation,
	//	}
	//
	//	response, err = client.GenericAPIRequest("POST", "graphql", data, headers, 0)
	//	if err != nil {
	//		return err
	//	}
	//
	//	type createResult struct {
	//		Data struct {
	//			CreatePullRequest struct {
	//				PullRequest struct {
	//					Url string
	//				}
	//			}
	//		}
	//	}
	//
	//	cresult := createResult{}
	//	err = response.Unmarshal(&cresult)
	//	if err != nil {
	//		return err
	//	}
	//	pullRequestURL := cresult.Data.CreatePullRequest.PullRequest.Url
	//	ui.Println(pullRequestURL)

	return nil
}
