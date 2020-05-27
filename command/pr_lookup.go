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

func prFromArgs(ctx context.Context, apiClient *api.Client, repo ghrepo.Interface, args ...string) (*api.PullRequest, error) {
	if len(args) == 0 {
		return prForCurrentBranch(ctx, apiClient, repo)
	}

	// First check to see if the prString is a url
	prString := args[0]
	pr, err := prFromURL(ctx, apiClient, repo, prString)
	if pr != nil || err != nil {
		return pr, err
	}

	// Next see if the prString is a number and use that to look up the url
	pr, err = prFromNumberString(ctx, apiClient, repo, prString)
	if pr != nil || err != nil {
		return pr, err
	}

	// Last see if it is a branch name
	return api.PullRequestForBranch(apiClient, repo, "", prString)
}

func prFromNumberString(ctx context.Context, apiClient *api.Client, repo ghrepo.Interface, s string) (*api.PullRequest, error) {
	if prNumber, err := strconv.Atoi(strings.TrimPrefix(s, "#")); err == nil {
		return api.PullRequestByNumber(apiClient, repo, prNumber)
	}

	return nil, nil
}

func prFromURL(ctx context.Context, apiClient *api.Client, repo ghrepo.Interface, s string) (*api.PullRequest, error) {
	r := regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/pull/(\d+)`)
	if m := r.FindStringSubmatch(s); m != nil {
		prNumberString := m[3]
		return prFromNumberString(ctx, apiClient, repo, prNumberString)
	}

	return nil, nil
}

func prForCurrentBranch(ctx context.Context, apiClient *api.Client, repo ghrepo.Interface) (*api.PullRequest, error) {
	prHeadRef, err := ctx.Branch()
	if err != nil {
		return nil, err
	}

	branchConfig := git.ReadBranchConfig(prHeadRef)

	// the branch is configured to merge a special PR head ref
	prHeadRE := regexp.MustCompile(`^refs/pull/(\d+)/head$`)
	if m := prHeadRE.FindStringSubmatch(branchConfig.MergeRef); m != nil {
		return prFromNumberString(ctx, apiClient, repo, m[1])
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

	return api.PullRequestForBranch(apiClient, repo, "", prHeadRef)
}
