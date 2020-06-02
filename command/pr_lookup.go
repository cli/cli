package command

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
)

func prFromArgs(ctx context.Context, apiClient *api.Client, repo ghrepo.Interface, args []string, pr interface{}) error {
	notFoundErr := fmt.Errorf("pull request not found")
	if len(args) == 0 {
		found, err := prForCurrentBranch(ctx, apiClient, repo, pr)
		if err != nil {
			return err
		} else if !found {
			return notFoundErr
		}

		return nil
	}

	// First check to see if the prString is a url
	prString := args[0]
	found, err := prFromURL(ctx, apiClient, repo, prString, pr)
	if found || err != nil {
		return err
	}

	// Next see if the prString is a number and use that to look up the url
	found, err = prFromNumberString(ctx, apiClient, repo, prString, pr)
	if found || err != nil {
		return err
	}

	// Last see if it is a branch name
	found, err = api.PullRequestForBranch(apiClient, repo, "", prString, pr)
	if err != nil {
		return err
	} else if !found {
		return notFoundErr
	}

	return nil
}

func prFromNumberString(ctx context.Context, apiClient *api.Client, repo ghrepo.Interface, s string, pr interface{}) (bool, error) {
	if prNumber, err := strconv.Atoi(strings.TrimPrefix(s, "#")); err == nil {
		return api.PullRequestByNumber(apiClient, repo, prNumber, pr)
	}

	return false, nil
}

func prFromURL(ctx context.Context, apiClient *api.Client, repo ghrepo.Interface, s string, pr interface{}) (bool, error) {
	r := regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/pull/(\d+)`)
	if m := r.FindStringSubmatch(s); m != nil {
		prNumberString := m[3]
		return prFromNumberString(ctx, apiClient, repo, prNumberString, pr)
	}

	return false, nil
}

func prForCurrentBranch(ctx context.Context, apiClient *api.Client, repo ghrepo.Interface, pr interface{}) (bool, error) {
	prHeadRef, err := ctx.Branch()
	if err != nil {
		return false, err
	}

	branchConfig := git.ReadBranchConfig(prHeadRef)

	// the branch is configured to merge a special PR head ref
	prHeadRE := regexp.MustCompile(`^refs/pull/(\d+)/head$`)
	if m := prHeadRE.FindStringSubmatch(branchConfig.MergeRef); m != nil {
		found, err := prFromNumberString(ctx, apiClient, repo, m[1], pr)
		if found || err != nil {
			return found, err
		}
	}

	var branchOwner string
	if branchConfig.RemoteURL != nil {
		// the branch merges from a remote specified by URL
		if r, err := ghrepo.FromURL(branchConfig.RemoteURL); err == nil {
			branchOwner = r.RepoOwner()
		}
	} else if branchConfig.RemoteName != "" {
		// the branch merges from a remote specified by name
		rem, _ := ctx.Remotes()
		if r, err := rem.FindByName(branchConfig.RemoteName); err == nil {
			branchOwner = r.RepoOwner()
		}
	}

	if branchOwner != "" {
		if strings.HasPrefix(branchConfig.MergeRef, "refs/heads/") {
			prHeadRef = strings.TrimPrefix(branchConfig.MergeRef, "refs/heads/")
		}
		// prepend `OWNER:` if this branch is pushed to a fork
		if !strings.EqualFold(branchOwner, repo.RepoOwner()) {
			prHeadRef = fmt.Sprintf("%s:%s", branchOwner, prHeadRef)
		}
	}

	return api.PullRequestForBranch(apiClient, repo, "", prHeadRef, pr)
}
