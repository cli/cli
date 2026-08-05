package api

import (
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubRepo_notFound(t *testing.T) {
	httpReg := &httpmock.Registry{}
	defer httpReg.Verify(t)

	httpReg.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`{ "data": { "repository": null } }`))

	client := newTestClient(httpReg)
	repo, err := GitHubRepo(client, ghrepo.New("OWNER", "REPO"))
	require.EqualError(t, err, "GraphQL: Could not resolve to a Repository with the name 'OWNER/REPO'.")
	assert.Nil(t, repo)
}

func TestGitHubRepo_success(t *testing.T) {
	httpReg := &httpmock.Registry{}
	defer httpReg.Verify(t)

	httpReg.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": {
			"id": "REPOID",
			"name": "REPO",
			"owner": {"login": "OWNER"},
			"hasIssuesEnabled": true,
			"description": "a cool repo",
			"hasWikiEnabled": true,
			"viewerPermission": "ADMIN",
			"defaultBranchRef": {"name": "main"},
			"parent": null,
			"mergeCommitAllowed": true,
			"rebaseMergeAllowed": true,
			"squashMergeAllowed": false
		} } }`))

	client := newTestClient(httpReg)
	repo, err := GitHubRepo(client, ghrepo.New("OWNER", "REPO"))
	require.NoError(t, err)
	assert.Equal(t, &Repository{
		ID:                 "REPOID",
		Name:               "REPO",
		Owner:              RepositoryOwner{Login: "OWNER"},
		HasIssuesEnabled:   true,
		Description:        "a cool repo",
		HasWikiEnabled:     true,
		ViewerPermission:   "ADMIN",
		DefaultBranchRef:   BranchRef{Name: "main"},
		MergeCommitAllowed: true,
		RebaseMergeAllowed: true,
		hostname:           "github.com",
	}, repo)
	assert.True(t, repo.ViewerCanPush())
	assert.True(t, repo.ViewerCanTriage())
}

func TestGitHubRepo_withParent(t *testing.T) {
	httpReg := &httpmock.Registry{}
	defer httpReg.Verify(t)

	httpReg.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": {
			"id": "REPOID",
			"name": "REPO",
			"owner": {"login": "OWNER"},
			"hasIssuesEnabled": true,
			"description": "",
			"hasWikiEnabled": false,
			"viewerPermission": "READ",
			"defaultBranchRef": {"name": "main"},
			"parent": {
				"id": "PARENTID",
				"name": "PARENT-REPO",
				"owner": {"login": "PARENT-OWNER"},
				"hasIssuesEnabled": true,
				"description": "parent repo",
				"hasWikiEnabled": true,
				"viewerPermission": "READ",
				"defaultBranchRef": {"name": "develop"}
			},
			"mergeCommitAllowed": false,
			"rebaseMergeAllowed": false,
			"squashMergeAllowed": true
		} } }`))

	client := newTestClient(httpReg)
	repo, err := GitHubRepo(client, ghrepo.New("OWNER", "REPO"))
	require.NoError(t, err)
	wantParent := &Repository{
		ID:               "PARENTID",
		Name:             "PARENT-REPO",
		Owner:            RepositoryOwner{Login: "PARENT-OWNER"},
		HasIssuesEnabled: true,
		Description:      "parent repo",
		HasWikiEnabled:   true,
		ViewerPermission: "READ",
		DefaultBranchRef: BranchRef{Name: "develop"},
		hostname:         "github.com",
	}
	assert.Equal(t, &Repository{
		ID:                 "REPOID",
		Name:               "REPO",
		Owner:              RepositoryOwner{Login: "OWNER"},
		HasIssuesEnabled:   true,
		ViewerPermission:   "READ",
		DefaultBranchRef:   BranchRef{Name: "main"},
		Parent:             wantParent,
		SquashMergeAllowed: true,
		hostname:           "github.com",
	}, repo)
	assert.False(t, repo.ViewerCanPush())
	assert.False(t, repo.ViewerCanTriage())
}

func TestIssueRepoInfo_notFound(t *testing.T) {
	httpReg := &httpmock.Registry{}
	defer httpReg.Verify(t)

	httpReg.Register(
		httpmock.GraphQL(`query IssueRepositoryInfo\b`),
		httpmock.StringResponse(`{ "data": { "repository": null } }`))

	client := newTestClient(httpReg)
	repo, err := IssueRepoInfo(client, ghrepo.New("OWNER", "REPO"))
	require.EqualError(t, err, "GraphQL: Could not resolve to a Repository with the name 'OWNER/REPO'.")
	assert.Nil(t, repo)
}

func TestIssueRepoInfo_success(t *testing.T) {
	httpReg := &httpmock.Registry{}
	defer httpReg.Verify(t)

	httpReg.Register(
		httpmock.GraphQL(`query IssueRepositoryInfo\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": {
			"id": "REPOID",
			"name": "REPO",
			"owner": {"login": "OWNER"},
			"hasIssuesEnabled": true,
			"viewerPermission": "WRITE"
		} } }`))

	client := newTestClient(httpReg)
	repo, err := IssueRepoInfo(client, ghrepo.New("OWNER", "REPO"))
	require.NoError(t, err)
	assert.Equal(t, &Repository{
		ID:               "REPOID",
		Name:             "REPO",
		Owner:            RepositoryOwner{Login: "OWNER"},
		HasIssuesEnabled: true,
		ViewerPermission: "WRITE",
		hostname:         "github.com",
	}, repo)
	assert.True(t, repo.ViewerCanTriage())
}

func TestIssueRepoInfo_issuesDisabled(t *testing.T) {
	httpReg := &httpmock.Registry{}
	defer httpReg.Verify(t)

	httpReg.Register(
		httpmock.GraphQL(`query IssueRepositoryInfo\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": {
			"id": "REPOID",
			"name": "REPO",
			"owner": {"login": "OWNER"},
			"hasIssuesEnabled": false,
			"viewerPermission": "READ"
		} } }`))

	client := newTestClient(httpReg)
	repo, err := IssueRepoInfo(client, ghrepo.New("OWNER", "REPO"))
	require.NoError(t, err)
	assert.Equal(t, &Repository{
		ID:               "REPOID",
		Name:             "REPO",
		Owner:            RepositoryOwner{Login: "OWNER"},
		ViewerPermission: "READ",
		hostname:         "github.com",
	}, repo)
	assert.False(t, repo.ViewerCanTriage())
}

func Test_RepoMetadata(t *testing.T) {
	http := &httpmock.Registry{}
	client := newTestClient(http)

	repo, _ := ghrepo.FromFullName("OWNER/REPO")
	input := RepoMetadataInput{
		Assignees:     true,
		Reviewers:     true,
		TeamReviewers: true,
		Labels:        true,
		ProjectsV1:    true,
		ProjectsV2:    true,
		Milestones:    true,
	}

	http.Register(
		httpmock.GraphQL(`query RepositoryAssignableUsers\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "assignableUsers": {
			"nodes": [
				{ "login": "hubot", "id": "HUBOTID" },
				{ "login": "MonaLisa", "id": "MONAID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query RepositoryLabelList\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "labels": {
			"nodes": [
				{ "name": "feature", "id": "FEATUREID" },
				{ "name": "TODO", "id": "TODOID" },
				{ "name": "bug", "id": "BUGID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query RepositoryMilestoneList\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "milestones": {
			"nodes": [
				{ "title": "GA", "id": "GAID" },
				{ "title": "Big One.oh", "id": "BIGONEID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query RepositoryProjectList\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "projects": {
			"nodes": [
				{ "name": "Cleanup", "id": "CLEANUPID" },
				{ "name": "Roadmap", "id": "ROADMAPID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query RepositoryProjectV2List\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "projectsV2": {
			"nodes": [
				{ "title": "CleanupV2", "id": "CLEANUPV2ID" },
				{ "title": "RoadmapV2", "id": "ROADMAPV2ID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query OrganizationProjectList\b`),
		httpmock.StringResponse(`
		{ "data": { "organization": { "projects": {
			"nodes": [
				{ "name": "Triage", "id": "TRIAGEID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query OrganizationProjectV2List\b`),
		httpmock.StringResponse(`
		{ "data": { "organization": { "projectsV2": {
			"nodes": [
				{ "title": "TriageV2", "id": "TRIAGEV2ID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query UserProjectV2List\b`),
		httpmock.StringResponse(`
		{ "data": { "viewer": { "projectsV2": {
			"nodes": [
				{ "title": "MonalisaV2", "id": "MONALISAV2ID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query OrganizationTeamList\b`),
		httpmock.StringResponse(`
		{ "data": { "organization": { "teams": {
			"nodes": [
				{ "slug": "owners", "id": "OWNERSID" },
				{ "slug": "Core", "id": "COREID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`
		  { "data": { "viewer": { "login": "monalisa" } } }
		`))

	result, err := RepoMetadata(client, repo, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedMemberIDs := []string{"MONAID", "HUBOTID"}
	memberIDs, err := result.MembersToIDs([]string{"monalisa", "hubot"})
	if err != nil {
		t.Errorf("error resolving members: %v", err)
	}
	if !slices.Equal(memberIDs, expectedMemberIDs) {
		t.Errorf("expected members %v, got %v", expectedMemberIDs, memberIDs)
	}

	expectedTeamIDs := []string{"COREID", "OWNERSID"}
	teamIDs, err := result.TeamsToIDs([]string{"OWNER/core", "/owners"})
	if err != nil {
		t.Errorf("error resolving teams: %v", err)
	}
	if !slices.Equal(teamIDs, expectedTeamIDs) {
		t.Errorf("expected teams %v, got %v", expectedTeamIDs, teamIDs)
	}

	expectedLabelIDs := []string{"BUGID", "TODOID"}
	labelIDs, err := result.LabelsToIDs([]string{"bug", "todo"})
	if err != nil {
		t.Errorf("error resolving labels: %v", err)
	}
	if !slices.Equal(labelIDs, expectedLabelIDs) {
		t.Errorf("expected labels %v, got %v", expectedLabelIDs, labelIDs)
	}

	expectedProjectIDs := []string{"TRIAGEID", "ROADMAPID"}
	expectedProjectV2IDs := []string{"TRIAGEV2ID", "ROADMAPV2ID", "MONALISAV2ID"}
	projectIDs, projectV2IDs, err := result.ProjectsTitlesToIDs([]string{"triage", "roadmap", "triagev2", "roadmapv2", "monalisav2"})
	if err != nil {
		t.Errorf("error resolving projects: %v", err)
	}
	if !slices.Equal(projectIDs, expectedProjectIDs) {
		t.Errorf("expected projects %v, got %v", expectedProjectIDs, projectIDs)
	}
	if !slices.Equal(projectV2IDs, expectedProjectV2IDs) {
		t.Errorf("expected projectsV2 %v, got %v", expectedProjectV2IDs, projectV2IDs)
	}

	expectedMilestoneID := "BIGONEID"
	milestoneID, err := result.MilestoneToID("big one.oh")
	if err != nil {
		t.Errorf("error resolving milestone: %v", err)
	}
	if milestoneID != expectedMilestoneID {
		t.Errorf("expected milestone %v, got %v", expectedMilestoneID, milestoneID)
	}

	expectedCurrentLogin := "monalisa"
	if result.CurrentLogin != expectedCurrentLogin {
		t.Errorf("expected current user %v, got %v", expectedCurrentLogin, result.CurrentLogin)
	}
}

// Test that RepoMetadata only fetches teams if the input specifies it
func Test_RepoMetadata_TeamsAreConditionallyFetched(t *testing.T) {
	http := &httpmock.Registry{}
	client := newTestClient(http)
	repo, _ := ghrepo.FromFullName("OWNER/REPO")
	input := RepoMetadataInput{
		Reviewers:     true,
		TeamReviewers: false, // Do not fetch teams
	}

	http.Register(
		httpmock.GraphQL(`query RepositoryAssignableUsers\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "assignableUsers": {
			"nodes": [
				{ "login": "hubot", "id": "HUBOTID" },
				{ "login": "MonaLisa", "id": "MONAID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))

	http.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`
		  { "data": { "viewer": { "login": "monalisa" } } }
		`))

	http.Exclude(
		t,
		httpmock.GraphQL(`query OrganizationTeamList\b`),
	)

	_, err := RepoMetadata(client, repo, input)
	require.NoError(t, err)
}

func Test_ProjectNamesToPaths(t *testing.T) {
	t.Run("when projectsV1 is supported, requests them", func(t *testing.T) {
		http := &httpmock.Registry{}
		client := newTestClient(http)

		repo, _ := ghrepo.FromFullName("OWNER/REPO")

		http.Register(
			httpmock.GraphQL(`query RepositoryProjectList\b`),
			httpmock.StringResponse(`
		{ "data": { "repository": { "projects": {
			"nodes": [
				{ "name": "Cleanup", "id": "CLEANUPID", "resourcePath": "/OWNER/REPO/projects/1" },
				{ "name": "Roadmap", "id": "ROADMAPID", "resourcePath": "/OWNER/REPO/projects/2" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
		http.Register(
			httpmock.GraphQL(`query OrganizationProjectList\b`),
			httpmock.StringResponse(`
		{ "data": { "organization": { "projects": {
			"nodes": [
				{ "name": "Triage", "id": "TRIAGEID", "resourcePath": "/orgs/ORG/projects/1"  }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
		http.Register(
			httpmock.GraphQL(`query RepositoryProjectV2List\b`),
			httpmock.StringResponse(`
		{ "data": { "repository": { "projectsV2": {
			"nodes": [
				{ "title": "CleanupV2", "id": "CLEANUPV2ID", "resourcePath": "/OWNER/REPO/projects/3" },
				{ "title": "RoadmapV2", "id": "ROADMAPV2ID", "resourcePath": "/OWNER/REPO/projects/4" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
		http.Register(
			httpmock.GraphQL(`query OrganizationProjectV2List\b`),
			httpmock.StringResponse(`
		{ "data": { "organization": { "projectsV2": {
			"nodes": [
				{ "title": "TriageV2", "id": "TRIAGEV2ID", "resourcePath": "/orgs/ORG/projects/2"  }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
		http.Register(
			httpmock.GraphQL(`query UserProjectV2List\b`),
			httpmock.StringResponse(`
		{ "data": { "viewer": { "projectsV2": {
			"nodes": [
				{ "title": "MonalisaV2", "id": "MONALISAV2ID", "resourcePath": "/users/MONALISA/projects/5"  }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))

		projectPaths, err := ProjectTitlesToPaths(client, repo, []string{"Triage", "Roadmap", "TriageV2", "RoadmapV2", "MonalisaV2"}, gh.ProjectsV1Supported)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expectedProjectPaths := []string{"ORG/1", "OWNER/REPO/2", "ORG/2", "OWNER/REPO/4", "MONALISA/5"}
		if !slices.Equal(projectPaths, expectedProjectPaths) {
			t.Errorf("expected projects paths %v, got %v", expectedProjectPaths, projectPaths)
		}
	})

	t.Run("when projectsV1 is not supported, does not request them", func(t *testing.T) {
		http := &httpmock.Registry{}
		client := newTestClient(http)

		repo, _ := ghrepo.FromFullName("OWNER/REPO")

		http.Exclude(
			t,
			httpmock.GraphQL(`query RepositoryProjectList\b`),
		)
		http.Exclude(
			t,
			httpmock.GraphQL(`query OrganizationProjectList\b`),
		)

		http.Register(
			httpmock.GraphQL(`query RepositoryProjectV2List\b`),
			httpmock.StringResponse(`
		{ "data": { "repository": { "projectsV2": {
			"nodes": [
				{ "title": "CleanupV2", "id": "CLEANUPV2ID", "resourcePath": "/OWNER/REPO/projects/3" },
				{ "title": "RoadmapV2", "id": "ROADMAPV2ID", "resourcePath": "/OWNER/REPO/projects/4" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
		http.Register(
			httpmock.GraphQL(`query OrganizationProjectV2List\b`),
			httpmock.StringResponse(`
		{ "data": { "organization": { "projectsV2": {
			"nodes": [
				{ "title": "TriageV2", "id": "TRIAGEV2ID", "resourcePath": "/orgs/ORG/projects/2"  }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
		http.Register(
			httpmock.GraphQL(`query UserProjectV2List\b`),
			httpmock.StringResponse(`
		{ "data": { "viewer": { "projectsV2": {
			"nodes": [
				{ "title": "MonalisaV2", "id": "MONALISAV2ID", "resourcePath": "/users/MONALISA/projects/5"  }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))

		projectPaths, err := ProjectTitlesToPaths(client, repo, []string{"TriageV2", "RoadmapV2", "MonalisaV2"}, gh.ProjectsV1Unsupported)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expectedProjectPaths := []string{"ORG/2", "OWNER/REPO/4", "MONALISA/5"}
		if !slices.Equal(projectPaths, expectedProjectPaths) {
			t.Errorf("expected projects paths %v, got %v", expectedProjectPaths, projectPaths)
		}
	})

	t.Run("when a project is not found, returns an error", func(t *testing.T) {
		http := &httpmock.Registry{}
		client := newTestClient(http)

		repo, _ := ghrepo.FromFullName("OWNER/REPO")

		// No projects found
		http.Register(
			httpmock.GraphQL(`query RepositoryProjectV2List\b`),
			httpmock.StringResponse(`
		{ "data": { "repository": { "projectsV2": {
			"nodes": [],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
		http.Register(
			httpmock.GraphQL(`query OrganizationProjectV2List\b`),
			httpmock.StringResponse(`
		{ "data": { "organization": { "projectsV2": {
			"nodes": [],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
		http.Register(
			httpmock.GraphQL(`query UserProjectV2List\b`),
			httpmock.StringResponse(`
		{ "data": { "viewer": { "projectsV2": {
			"nodes": [],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))

		_, err := ProjectTitlesToPaths(client, repo, []string{"TriageV2"}, gh.ProjectsV1Unsupported)
		require.Equal(t, err, fmt.Errorf("'TriageV2' not found"))
	})
}

func TestMembersToIDs(t *testing.T) {
	t.Parallel()

	t.Run("finds ids in assignable users", func(t *testing.T) {
		t.Parallel()

		repoMetadataResult := RepoMetadataResult{
			AssignableUsers: []AssignableUser{
				NewAssignableUser("MONAID", "monalisa", ""),
				NewAssignableUser("MONAID2", "monalisa2", ""),
			},
			AssignableActors: []AssignableActor{
				NewAssignableBot("HUBOTID", "hubot"),
			},
		}
		ids, err := repoMetadataResult.MembersToIDs([]string{"monalisa"})
		require.NoError(t, err)
		require.Equal(t, []string{"MONAID"}, ids)
	})

	t.Run("finds ids by assignable actor logins", func(t *testing.T) {
		t.Parallel()

		repoMetadataResult := RepoMetadataResult{
			AssignableActors: []AssignableActor{
				NewAssignableBot("HUBOTID", "hubot"),
				NewAssignableUser("MONAID", "monalisa", ""),
			},
		}
		ids, err := repoMetadataResult.MembersToIDs([]string{"monalisa"})
		require.NoError(t, err)
		require.Equal(t, []string{"MONAID"}, ids)
	})

	t.Run("finds ids by assignable actor display names", func(t *testing.T) {
		t.Parallel()

		repoMetadataResult := RepoMetadataResult{
			AssignableActors: []AssignableActor{
				NewAssignableUser("MONAID", "monalisa", "mona"),
			},
		}
		ids, err := repoMetadataResult.MembersToIDs([]string{"monalisa (mona)"})
		require.NoError(t, err)
		require.Equal(t, []string{"MONAID"}, ids)
	})

	t.Run("when a name appears in both assignable users and actors, the id is only returned once", func(t *testing.T) {
		t.Parallel()

		repoMetadataResult := RepoMetadataResult{
			AssignableUsers: []AssignableUser{
				NewAssignableUser("MONAID", "monalisa", ""),
			},
			AssignableActors: []AssignableActor{
				NewAssignableUser("MONAID", "monalisa", ""),
			},
		}
		ids, err := repoMetadataResult.MembersToIDs([]string{"monalisa"})
		require.NoError(t, err)
		require.Equal(t, []string{"MONAID"}, ids)
	})

	t.Run("when id is not found, returns an error", func(t *testing.T) {
		t.Parallel()

		repoMetadataResult := RepoMetadataResult{}
		_, err := repoMetadataResult.MembersToIDs([]string{"monalisa"})
		require.Error(t, err)
	})
}

func Test_RepoMilestones(t *testing.T) {
	tests := []struct {
		state   string
		want    string
		wantErr bool
	}{
		{
			state: "open",
			want:  `"states":["OPEN"]`,
		},
		{
			state: "closed",
			want:  `"states":["CLOSED"]`,
		},
		{
			state: "all",
			want:  `"states":["OPEN","CLOSED"]`,
		},
		{
			state:   "invalid state",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		var query string
		reg := &httpmock.Registry{}
		reg.Register(httpmock.MatchAny, func(req *http.Request) (*http.Response, error) {
			buf := new(strings.Builder)
			_, err := io.Copy(buf, req.Body)
			if err != nil {
				return nil, err
			}
			query = buf.String()
			return httpmock.StringResponse("{}")(req)
		})
		client := newTestClient(reg)

		_, err := RepoMilestones(client, ghrepo.New("OWNER", "REPO"), tt.state)
		if (err != nil) != tt.wantErr {
			t.Errorf("RepoMilestones() error = %v, wantErr %v", err, tt.wantErr)
			return
		}
		if !strings.Contains(query, tt.want) {
			t.Errorf("query does not contain %v", tt.want)
		}
	}
}

func TestDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		assignee AssignableUser
		want     string
	}{
		{
			name:     "assignee with name",
			assignee: AssignableUser{"123", "octocat123", "Octavious Cath"},
			want:     "octocat123 (Octavious Cath)",
		},
		{
			name:     "assignee without name",
			assignee: AssignableUser{"123", "octocat123", ""},
			want:     "octocat123",
		},
	}
	for _, tt := range tests {
		actual := tt.assignee.DisplayName()
		if actual != tt.want {
			t.Errorf("display name was %s wanted %s", actual, tt.want)
		}
	}
}

func TestActorDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		login    string
		actName  string
		want     string
	}{
		{name: "copilot reviewer", typeName: "Bot", login: "copilot-pull-request-reviewer", want: "Copilot (AI)"},
		{name: "copilot assignee", typeName: "Bot", login: "copilot-swe-agent", want: "Copilot (AI)"},
		{name: "copilot without typename", typeName: "", login: "copilot-pull-request-reviewer", want: "Copilot (AI)"},
		{name: "copilot actor name login", typeName: "", login: "Copilot", want: "Copilot (AI)"},
		{name: "regular bot", typeName: "Bot", login: "dependabot", want: "dependabot"},
		{name: "user with name", typeName: "User", login: "octocat", actName: "Mona Lisa", want: "octocat (Mona Lisa)"},
		{name: "user without name", typeName: "User", login: "octocat", want: "octocat"},
		{name: "unknown type with name", typeName: "", login: "octocat", actName: "Mona Lisa", want: "octocat (Mona Lisa)"},
		{name: "empty login", typeName: "", login: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, actorDisplayName(tt.typeName, tt.login, tt.actName))
		})
	}
}

func TestRepoExists(t *testing.T) {
	tests := []struct {
		name       string
		httpStub   func(*httpmock.Registry)
		repo       ghrepo.Interface
		existCheck bool
		wantErrMsg string
	}{
		{
			name: "repo exists",
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.REST("HEAD", "repos/OWNER/REPO"),
					httpmock.StringResponse("{}"),
				)
			},
			repo:       ghrepo.New("OWNER", "REPO"),
			existCheck: true,
			wantErrMsg: "",
		},
		{
			name: "repo does not exists",
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.REST("HEAD", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(404, "Not Found"),
				)
			},
			repo:       ghrepo.New("OWNER", "REPO"),
			existCheck: false,
			wantErrMsg: "",
		},
		{
			name: "http error",
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.REST("HEAD", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(500, "Internal Server Error"),
				)
			},
			repo:       ghrepo.New("OWNER", "REPO"),
			existCheck: false,
			wantErrMsg: "HTTP 500 (https://api.github.com/repos/OWNER/REPO)",
		},
	}
	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.httpStub != nil {
			tt.httpStub(reg)
		}

		client := newTestClient(reg)

		t.Run(tt.name, func(t *testing.T) {
			exist, err := RepoExists(client, ghrepo.New("OWNER", "REPO"))
			if tt.wantErrMsg != "" {
				assert.Equal(t, tt.wantErrMsg, err.Error())
			} else {
				assert.NoError(t, err)
			}

			if exist != tt.existCheck {
				t.Errorf("RepoExists() returns %v, expected %v", exist, tt.existCheck)
				return
			}
		})
	}
}
