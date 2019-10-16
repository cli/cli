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

	if prParams.PrePush {
		err = git.Run("push", "--set-upstream", remote.Name, fmt.Sprintf("HEAD:%s", head))

		if err != nil {
			return err
		}
	}

	var pullRequestURL string
	params := map[string]interface{}{
		"base": base,
		"head": fullHead,
	}

	params["title"] = prParams.Title
	params["body"] = prParams.Body
	params["draft"] = prParams.Draft

	var pr *PullRequest
	pr, err = client.CreatePullRequest(baseProject, params)
	if err != nil {
		return err
	}

	pullRequestURL = pr.HtmlUrl

	ui.Println(pullRequestURL)

	return nil
}
