package httpmock

import (
	"fmt"
)

// TODO: clean up methods in this file when there are no more callers

func (r *Registry) StubRepoInfoResponse(owner, repo, branch string) {
	r.Register(
		GraphQL(`query RepositoryInfo\b`),
		StringResponse(fmt.Sprintf(`
		{ "data": { "repository": {
			"id": "REPOID",
			"name": "%s",
			"owner": {"login": "%s"},
			"description": "",
			"defaultBranchRef": {"name": "%s"},
			"hasIssuesEnabled": true,
			"viewerPermission": "WRITE"
		} } }
		`, repo, owner, branch)))
}

func (r *Registry) StubRepoResponse(owner, repo string) {
	r.StubRepoResponseWithPermission(owner, repo, "WRITE")
}

func (r *Registry) StubRepoResponseWithPermission(owner, repo, permission string) {
	r.Register(GraphQL(`query RepositoryNetwork\b`), StringResponse(RepoNetworkStubResponse(owner, repo, "master", permission)))
}

func RepoNetworkStubResponse(owner, repo, defaultBranch, permission string) string {
	return fmt.Sprintf(`
		{ "data": { "repo_000": {
			"id": "REPOID",
			"name": "%s",
			"owner": {"login": "%s"},
			"defaultBranchRef": {
				"name": "%s"
			},
			"viewerPermission": "%s"
		} } }
	`, repo, owner, defaultBranch, permission)
}
