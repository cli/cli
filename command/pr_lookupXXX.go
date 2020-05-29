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

func prFromArgsXXX(ctx context.Context, apiClient *api.Client, repo ghrepo.Interface, args []string, pr interface{}) (bool, error) {
	if len(args) == 0 {
		return prForCurrentBranchXXX(ctx, apiClient, repo, pr)
	}

	// // First check to see if the prString is a url
	prString := args[0]
	found, err := prFromURLXXX(ctx, apiClient, repo, prString, pr)
	if found || err != nil {
		return found, err
	}

	// Next see if the prString is a number and use that to look up the url
	found, err = prFromNumberStringXXX(ctx, apiClient, repo, prString, pr)
	if found || err != nil {
		return found, err
	}

	// // Last see if it is a branch name
	found, err = api.PullRequestForBranchXXX(apiClient, repo, "", prString, pr)
	return found, err
}

func prFromNumberStringXXX(ctx context.Context, apiClient *api.Client, repo ghrepo.Interface, s string, pr interface{}) (bool, error) {
	if prNumber, err := strconv.Atoi(strings.TrimPrefix(s, "#")); err == nil {
		return api.PullRequestByNumberXXX(apiClient, repo, prNumber, pr)
	}

	return false, nil
}

func prFromURLXXX(ctx context.Context, apiClient *api.Client, repo ghrepo.Interface, s string, pr interface{}) (bool, error) {
	r := regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/pull/(\d+)`)
	if m := r.FindStringSubmatch(s); m != nil {
		prNumberString := m[3]
		return prFromNumberStringXXX(ctx, apiClient, repo, prNumberString, pr)
	}

	return false, nil
}

func prForCurrentBranchXXX(ctx context.Context, apiClient *api.Client, repo ghrepo.Interface, pr interface{}) (bool, error) {
	prHeadRef, err := ctx.Branch()
	if err != nil {
		return false, err
	}

	branchConfig := git.ReadBranchConfig(prHeadRef)

	// the branch is configured to merge a special PR head ref
	prHeadRE := regexp.MustCompile(`^refs/pull/(\d+)/head$`)
	if m := prHeadRE.FindStringSubmatch(branchConfig.MergeRef); m != nil {
		found, err := prFromNumberStringXXX(ctx, apiClient, repo, m[1], pr)
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

	return api.PullRequestForBranchXXX(apiClient, repo, "", prHeadRef, pr)
}
