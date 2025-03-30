package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
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
	if err == nil {
		t.Fatal("GitHubRepo did not return an error")
	}
	if wants := "GraphQL: Could not resolve to a Repository with the name 'OWNER/REPO'."; err.Error() != wants {
		t.Errorf("GitHubRepo error: want %q, got %q", wants, err.Error())
	}
	if repo != nil {
		t.Errorf("GitHubRepo: expected nil repo, got %v", repo)
	}
}

func Test_RepoMetadata(t *testing.T) {
	http := &httpmock.Registry{}
	client := newTestClient(http)

	repo, _ := ghrepo.FromFullName("OWNER/REPO")
	input := RepoMetadataInput{
		Assignees:  true,
		Reviewers:  true,
		Labels:     true,
		Projects:   true,
		Milestones: true,
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
	if !sliceEqual(memberIDs, expectedMemberIDs) {
		t.Errorf("expected members %v, got %v", expectedMemberIDs, memberIDs)
	}

	expectedTeamIDs := []string{"COREID", "OWNERSID"}
	teamIDs, err := result.TeamsToIDs([]string{"OWNER/core", "/owners"})
	if err != nil {
		t.Errorf("error resolving teams: %v", err)
	}
	if !sliceEqual(teamIDs, expectedTeamIDs) {
		t.Errorf("expected teams %v, got %v", expectedTeamIDs, teamIDs)
	}

	expectedLabelIDs := []string{"BUGID", "TODOID"}
	labelIDs, err := result.LabelsToIDs([]string{"bug", "todo"})
	if err != nil {
		t.Errorf("error resolving labels: %v", err)
	}
	if !sliceEqual(labelIDs, expectedLabelIDs) {
		t.Errorf("expected labels %v, got %v", expectedLabelIDs, labelIDs)
	}

	expectedProjectIDs := []string{"TRIAGEID", "ROADMAPID"}
	expectedProjectV2IDs := []string{"TRIAGEV2ID", "ROADMAPV2ID", "MONALISAV2ID"}
	projectIDs, projectV2IDs, err := result.ProjectsToIDs([]string{"triage", "roadmap", "triagev2", "roadmapv2", "monalisav2"})
	if err != nil {
		t.Errorf("error resolving projects: %v", err)
	}
	if !sliceEqual(projectIDs, expectedProjectIDs) {
		t.Errorf("expected projects %v, got %v", expectedProjectIDs, projectIDs)
	}
	if !sliceEqual(projectV2IDs, expectedProjectV2IDs) {
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

func Test_ProjectsToPaths(t *testing.T) {
	expectedProjectPaths := []string{"OWNER/REPO/PROJECT_NUMBER", "ORG/PROJECT_NUMBER", "OWNER/REPO/PROJECT_NUMBER_2"}
	projects := []RepoProject{
		{ID: "id1", Name: "My Project", ResourcePath: "/OWNER/REPO/projects/PROJECT_NUMBER"},
		{ID: "id2", Name: "Org Project", ResourcePath: "/orgs/ORG/projects/PROJECT_NUMBER"},
		{ID: "id3", Name: "Project", ResourcePath: "/orgs/ORG/projects/PROJECT_NUMBER_2"},
	}
	projectsV2 := []ProjectV2{
		{ID: "id4", Title: "My Project V2", ResourcePath: "/OWNER/REPO/projects/PROJECT_NUMBER_2"},
		{ID: "id5", Title: "Org Project V2", ResourcePath: "/orgs/ORG/projects/PROJECT_NUMBER_3"},
	}
	projectNames := []string{"My Project", "Org Project", "My Project V2"}

	projectPaths, err := ProjectsToPaths(projects, projectsV2, projectNames)
	if err != nil {
		t.Errorf("error resolving projects: %v", err)
	}
	if !sliceEqual(projectPaths, expectedProjectPaths) {
		t.Errorf("expected projects %v, got %v", expectedProjectPaths, projectPaths)
	}
}

func Test_ProjectNamesToPaths(t *testing.T) {
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

	projectPaths, err := ProjectNamesToPaths(client, repo, []string{"Triage", "Roadmap", "TriageV2", "RoadmapV2", "MonalisaV2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedProjectPaths := []string{"ORG/1", "OWNER/REPO/2", "ORG/2", "OWNER/REPO/4", "MONALISA/5"}
	if !sliceEqual(projectPaths, expectedProjectPaths) {
		t.Errorf("expected projects paths %v, got %v", expectedProjectPaths, projectPaths)
	}
}

func Test_RepoResolveMetadataIDs(t *testing.T) {
	http := &httpmock.Registry{}
	client := newTestClient(http)

	repo, _ := ghrepo.FromFullName("OWNER/REPO")
	input := RepoResolveInput{
		Assignees: []string{"monalisa", "hubot"},
		Reviewers: []string{"monalisa", "octocat", "OWNER/core", "/robots"},
		Labels:    []string{"bug", "help wanted"},
	}

	expectedQuery := `query RepositoryResolveMetadataIDs {
u000: user(login:"monalisa"){id,login}
u001: user(login:"hubot"){id,login}
u002: user(login:"octocat"){id,login}
repository(owner:"OWNER",name:"REPO"){
l000: label(name:"bug"){id,name}
l001: label(name:"help wanted"){id,name}
}
organization(login:"OWNER"){
t000: team(slug:"core"){id,slug}
t001: team(slug:"robots"){id,slug}
}
}
`
	responseJSON := `
	{ "data": {
		"u000": { "login": "MonaLisa", "id": "MONAID" },
		"u001": { "login": "hubot", "id": "HUBOTID" },
		"u002": { "login": "octocat", "id": "OCTOID" },
		"repository": {
			"l000": { "name": "bug", "id": "BUGID" },
			"l001": { "name": "Help Wanted", "id": "HELPID" }
		},
		"organization": {
			"t000": { "slug": "core", "id": "COREID" },
			"t001": { "slug": "Robots", "id": "ROBOTID" }
		}
	} }
	`

	http.Register(
		httpmock.GraphQL(`query RepositoryResolveMetadataIDs\b`),
		httpmock.GraphQLQuery(responseJSON, func(q string, _ map[string]interface{}) {
			if q != expectedQuery {
				t.Errorf("expected query %q, got %q", expectedQuery, q)
			}
		}))

	result, err := RepoResolveMetadataIDs(client, repo, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedMemberIDs := []string{"MONAID", "HUBOTID", "OCTOID"}
	memberIDs, err := result.MembersToIDs([]string{"monalisa", "hubot", "octocat"})
	if err != nil {
		t.Errorf("error resolving members: %v", err)
	}
	if !sliceEqual(memberIDs, expectedMemberIDs) {
		t.Errorf("expected members %v, got %v", expectedMemberIDs, memberIDs)
	}

	expectedTeamIDs := []string{"COREID", "ROBOTID"}
	teamIDs, err := result.TeamsToIDs([]string{"/core", "/robots"})
	if err != nil {
		t.Errorf("error resolving teams: %v", err)
	}
	if !sliceEqual(teamIDs, expectedTeamIDs) {
		t.Errorf("expected members %v, got %v", expectedTeamIDs, teamIDs)
	}

	expectedLabelIDs := []string{"BUGID", "HELPID"}
	labelIDs, err := result.LabelsToIDs([]string{"bug", "help wanted"})
	if err != nil {
		t.Errorf("error resolving labels: %v", err)
	}
	if !sliceEqual(labelIDs, expectedLabelIDs) {
		t.Errorf("expected members %v, got %v", expectedLabelIDs, labelIDs)
	}
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
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
		assignee RepoAssignee
		want     string
	}{
		{
			name:     "assignee with name",
			assignee: RepoAssignee{"123", "octocat123", "Octavious Cath"},
			want:     "octocat123 (Octavious Cath)",
		},
		{
			name:     "assignee without name",
			assignee: RepoAssignee{"123", "octocat123", ""},
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

func TestForkRepoReturnsErrorWhenForkIsNotPossible(t *testing.T) {
	// Given our API returns 202 with a Fork that is the same as
	// the repo we provided
	repoName := "test-repo"
	ownerLogin := "test-owner"
	stubbedForkResponse := repositoryV3{
		Name: repoName,
		Owner: struct{ Login string }{
			Login: ownerLogin,
		},
	}

	reg := &httpmock.Registry{}
	reg.Register(
		httpmock.REST("POST", fmt.Sprintf("repos/%s/%s/forks", ownerLogin, repoName)),
		httpmock.StatusJSONResponse(202, stubbedForkResponse),
	)

	client := newTestClient(reg)

	// When we fork the repo
	_, err := ForkRepo(client, ghrepo.New(ownerLogin, repoName), ownerLogin, "", false)

	// Then it provides a useful error message
	require.Equal(t, fmt.Errorf("%s/%s cannot be forked. A single user account cannot own both a parent and fork.", ownerLogin, repoName), err)
}

func TestListLicenseTemplatesReturnsLicenses(t *testing.T) {
	hostname := "api.github.com"
	httpStubs := func(reg *httpmock.Registry) {
		reg.Register(
			httpmock.REST("GET", "licenses"),
			httpmock.StringResponse(`[
						{
							"key": "mit",
							"name": "MIT License",
							"spdx_id": "MIT",
							"url": "https://api.github.com/licenses/mit",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						},
						{
							"key": "lgpl-3.0",
							"name": "GNU Lesser General Public License v3.0",
							"spdx_id": "LGPL-3.0",
							"url": "https://api.github.com/licenses/lgpl-3.0",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						},
						{
							"key": "mpl-2.0",
							"name": "Mozilla Public License 2.0",
							"spdx_id": "MPL-2.0",
							"url": "https://api.github.com/licenses/mpl-2.0",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						},
						{
							"key": "agpl-3.0",
							"name": "GNU Affero General Public License v3.0",
							"spdx_id": "AGPL-3.0",
							"url": "https://api.github.com/licenses/agpl-3.0",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						},
						{
							"key": "unlicense",
							"name": "The Unlicense",
							"spdx_id": "Unlicense",
							"url": "https://api.github.com/licenses/unlicense",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						},
						{
							"key": "apache-2.0",
							"name": "Apache License 2.0",
							"spdx_id": "Apache-2.0",
							"url": "https://api.github.com/licenses/apache-2.0",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						},
						{
							"key": "gpl-3.0",
							"name": "GNU General Public License v3.0",
							"spdx_id": "GPL-3.0",
							"url": "https://api.github.com/licenses/gpl-3.0",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						}
						]`,
			))
	}
	wantLicenses := []License{
		{
			Key:            "mit",
			Name:           "MIT License",
			SPDXID:         "MIT",
			URL:            "https://api.github.com/licenses/mit",
			NodeID:         "MDc6TGljZW5zZW1pdA==",
			HTMLURL:        "",
			Description:    "",
			Implementation: "",
			Permissions:    nil,
			Conditions:     nil,
			Limitations:    nil,
			Body:           "",
		},
		{
			Key:            "lgpl-3.0",
			Name:           "GNU Lesser General Public License v3.0",
			SPDXID:         "LGPL-3.0",
			URL:            "https://api.github.com/licenses/lgpl-3.0",
			NodeID:         "MDc6TGljZW5zZW1pdA==",
			HTMLURL:        "",
			Description:    "",
			Implementation: "",
			Permissions:    nil,
			Conditions:     nil,
			Limitations:    nil,
			Body:           "",
		},
		{
			Key:            "mpl-2.0",
			Name:           "Mozilla Public License 2.0",
			SPDXID:         "MPL-2.0",
			URL:            "https://api.github.com/licenses/mpl-2.0",
			NodeID:         "MDc6TGljZW5zZW1pdA==",
			HTMLURL:        "",
			Description:    "",
			Implementation: "",
			Permissions:    nil,
			Conditions:     nil,
			Limitations:    nil,
			Body:           "",
		},
		{
			Key:            "agpl-3.0",
			Name:           "GNU Affero General Public License v3.0",
			SPDXID:         "AGPL-3.0",
			URL:            "https://api.github.com/licenses/agpl-3.0",
			NodeID:         "MDc6TGljZW5zZW1pdA==",
			HTMLURL:        "",
			Description:    "",
			Implementation: "",
			Permissions:    nil,
			Conditions:     nil,
			Limitations:    nil,
			Body:           "",
		},
		{
			Key:            "unlicense",
			Name:           "The Unlicense",
			SPDXID:         "Unlicense",
			URL:            "https://api.github.com/licenses/unlicense",
			NodeID:         "MDc6TGljZW5zZW1pdA==",
			HTMLURL:        "",
			Description:    "",
			Implementation: "",
			Permissions:    nil,
			Conditions:     nil,
			Limitations:    nil,
			Body:           "",
		},
		{
			Key:            "apache-2.0",
			Name:           "Apache License 2.0",
			SPDXID:         "Apache-2.0",
			URL:            "https://api.github.com/licenses/apache-2.0",
			NodeID:         "MDc6TGljZW5zZW1pdA==",
			HTMLURL:        "",
			Description:    "",
			Implementation: "",
			Permissions:    nil,
			Conditions:     nil,
			Limitations:    nil,
			Body:           "",
		},
		{
			Key:            "gpl-3.0",
			Name:           "GNU General Public License v3.0",
			SPDXID:         "GPL-3.0",
			URL:            "https://api.github.com/licenses/gpl-3.0",
			NodeID:         "MDc6TGljZW5zZW1pdA==",
			HTMLURL:        "",
			Description:    "",
			Implementation: "",
			Permissions:    nil,
			Conditions:     nil,
			Limitations:    nil,
			Body:           "",
		},
	}

	reg := &httpmock.Registry{}
	httpStubs(reg)

	httpClient := func() (*http.Client, error) {
		return &http.Client{Transport: reg}, nil
	}
	client, _ := httpClient()
	defer reg.Verify(t)

	gotLicenses, err := RepoLicenses(client, hostname)

	assert.NoError(t, err, "Expected no error while fetching /licenses")
	assert.Equal(t, wantLicenses, gotLicenses, "Licenses fetched is not as expected")
}

func TestLicenseTemplateReturnsLicense(t *testing.T) {
	licenseTemplateName := "mit"
	hostname := "api.github.com"
	httpStubs := func(reg *httpmock.Registry) {
		reg.Register(
			httpmock.REST("GET", fmt.Sprintf("licenses/%v", licenseTemplateName)),
			httpmock.StringResponse(`{
						"key": "mit",
						"name": "MIT License",
						"spdx_id": "MIT",
						"url": "https://api.github.com/licenses/mit",
						"node_id": "MDc6TGljZW5zZTEz",
						"html_url": "http://choosealicense.com/licenses/mit/",
						"description": "A short and simple permissive license with conditions only requiring preservation of copyright and license notices. Licensed works, modifications, and larger works may be distributed under different terms and without source code.",
						"implementation": "Create a text file (typically named LICENSE or LICENSE.txt) in the root of your source code and copy the text of the license into the file. Replace [year] with the current year and [fullname] with the name (or names) of the copyright holders.",
						"permissions": [
							"commercial-use",
							"modifications",
							"distribution",
							"private-use"
						],
						"conditions": [
							"include-copyright"
						],
						"limitations": [
							"liability",
							"warranty"
						],
						"body": "MIT License\n\nCopyright (c) [year] [fullname]\n\nPermission is hereby granted, free of charge, to any person obtaining a copy\nof this software and associated documentation files (the \"Software\"), to deal\nin the Software without restriction, including without limitation the rights\nto use, copy, modify, merge, publish, distribute, sublicense, and/or sell\ncopies of the Software, and to permit persons to whom the Software is\nfurnished to do so, subject to the following conditions:\n\nThe above copyright notice and this permission notice shall be included in all\ncopies or substantial portions of the Software.\n\nTHE SOFTWARE IS PROVIDED \"AS IS\", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR\nIMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,\nFITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE\nAUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER\nLIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,\nOUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE\nSOFTWARE.\n",
						"featured": true
						}`,
			))
	}
	wantLicense := &License{
		Key:            "mit",
		Name:           "MIT License",
		SPDXID:         "MIT",
		URL:            "https://api.github.com/licenses/mit",
		NodeID:         "MDc6TGljZW5zZTEz",
		HTMLURL:        "http://choosealicense.com/licenses/mit/",
		Description:    "A short and simple permissive license with conditions only requiring preservation of copyright and license notices. Licensed works, modifications, and larger works may be distributed under different terms and without source code.",
		Implementation: "Create a text file (typically named LICENSE or LICENSE.txt) in the root of your source code and copy the text of the license into the file. Replace [year] with the current year and [fullname] with the name (or names) of the copyright holders.",
		Permissions: []string{
			"commercial-use",
			"modifications",
			"distribution",
			"private-use",
		},
		Conditions: []string{
			"include-copyright",
		},
		Limitations: []string{
			"liability",
			"warranty",
		},
		Body:     "MIT License\n\nCopyright (c) [year] [fullname]\n\nPermission is hereby granted, free of charge, to any person obtaining a copy\nof this software and associated documentation files (the \"Software\"), to deal\nin the Software without restriction, including without limitation the rights\nto use, copy, modify, merge, publish, distribute, sublicense, and/or sell\ncopies of the Software, and to permit persons to whom the Software is\nfurnished to do so, subject to the following conditions:\n\nThe above copyright notice and this permission notice shall be included in all\ncopies or substantial portions of the Software.\n\nTHE SOFTWARE IS PROVIDED \"AS IS\", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR\nIMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,\nFITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE\nAUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER\nLIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,\nOUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE\nSOFTWARE.\n",
		Featured: true,
	}

	reg := &httpmock.Registry{}
	httpStubs(reg)

	httpClient := func() (*http.Client, error) {
		return &http.Client{Transport: reg}, nil
	}
	client, _ := httpClient()
	defer reg.Verify(t)

	gotLicenseTemplate, err := RepoLicense(client, hostname, licenseTemplateName)

	assert.NoError(t, err, fmt.Sprintf("Expected no error while fetching /licenses/%v", licenseTemplateName))
	assert.Equal(t, wantLicense, gotLicenseTemplate, fmt.Sprintf("License \"%v\" fetched is not as expected", licenseTemplateName))
}

func TestLicenseTemplateReturnsErrorWhenLicenseTemplateNotFound(t *testing.T) {
	licenseTemplateName := "invalid-license"
	hostname := "api.github.com"
	httpStubs := func(reg *httpmock.Registry) {
		reg.Register(
			httpmock.REST("GET", fmt.Sprintf("licenses/%v", licenseTemplateName)),
			httpmock.StatusStringResponse(404, heredoc.Doc(`
			{
				"message": "Not Found",
				"documentation_url": "https://docs.github.com/rest/licenses/licenses#get-a-license",
				"status": "404"
			}`)),
		)
	}

	reg := &httpmock.Registry{}
	httpStubs(reg)

	httpClient := func() (*http.Client, error) {
		return &http.Client{Transport: reg}, nil
	}
	client, _ := httpClient()
	defer reg.Verify(t)

	_, err := RepoLicense(client, hostname, licenseTemplateName)

	assert.Error(t, err, fmt.Sprintf("Expected error while fetching /licenses/%v", licenseTemplateName))
}

func TestListGitIgnoreTemplatesReturnsGitIgnoreTemplates(t *testing.T) {
	hostname := "api.github.com"
	httpStubs := func(reg *httpmock.Registry) {
		reg.Register(
			httpmock.REST("GET", "gitignore/templates"),
			httpmock.StringResponse(`[
						"AL",
						"Actionscript",
						"Ada",
						"Agda",
						"Android",
						"AppEngine",
						"AppceleratorTitanium",
						"ArchLinuxPackages",
						"Autotools",
						"Ballerina",
						"C",
						"C++",
						"CFWheels",
						"CMake",
						"CUDA",
						"CakePHP",
						"ChefCookbook",
						"Clojure",
						"CodeIgniter",
						"CommonLisp",
						"Composer",
						"Concrete5",
						"Coq",
						"CraftCMS",
						"D"
						]`,
			))
	}
	wantGitIgnoreTemplates := []string{
		"AL",
		"Actionscript",
		"Ada",
		"Agda",
		"Android",
		"AppEngine",
		"AppceleratorTitanium",
		"ArchLinuxPackages",
		"Autotools",
		"Ballerina",
		"C",
		"C++",
		"CFWheels",
		"CMake",
		"CUDA",
		"CakePHP",
		"ChefCookbook",
		"Clojure",
		"CodeIgniter",
		"CommonLisp",
		"Composer",
		"Concrete5",
		"Coq",
		"CraftCMS",
		"D",
	}

	reg := &httpmock.Registry{}
	httpStubs(reg)

	httpClient := func() (*http.Client, error) {
		return &http.Client{Transport: reg}, nil
	}
	client, _ := httpClient()
	defer reg.Verify(t)

	gotGitIgnoreTemplates, err := RepoGitIgnoreTemplates(client, hostname)

	assert.NoError(t, err, "Expected no error while fetching /gitignore/templates")
	assert.Equal(t, wantGitIgnoreTemplates, gotGitIgnoreTemplates, "GitIgnore templates fetched is not as expected")
}

func TestGitIgnoreTemplateReturnsGitIgnoreTemplate(t *testing.T) {
	gitIgnoreTemplateName := "Go"
	httpStubs := func(reg *httpmock.Registry) {
		reg.Register(
			httpmock.REST("GET", fmt.Sprintf("gitignore/templates/%v", gitIgnoreTemplateName)),
			httpmock.StringResponse(`{
						"name": "Go",
						"source": "# If you prefer the allow list template instead of the deny list, see community template:\n# https://github.com/github/gitignore/blob/main/community/Golang/Go.AllowList.gitignore\n#\n# Binaries for programs and plugins\n*.exe\n*.exe~\n*.dll\n*.so\n*.dylib\n\n# Test binary, built with go test -c\n*.test\n\n# Output of the go coverage tool, specifically when used with LiteIDE\n*.out\n\n# Dependency directories (remove the comment below to include it)\n# vendor/\n\n# Go workspace file\ngo.work\ngo.work.sum\n\n# env file\n.env\n"
						}`,
			))
	}
	wantGitIgnoreTemplate := &GitIgnore{
		Name:   "Go",
		Source: "# If you prefer the allow list template instead of the deny list, see community template:\n# https://github.com/github/gitignore/blob/main/community/Golang/Go.AllowList.gitignore\n#\n# Binaries for programs and plugins\n*.exe\n*.exe~\n*.dll\n*.so\n*.dylib\n\n# Test binary, built with go test -c\n*.test\n\n# Output of the go coverage tool, specifically when used with LiteIDE\n*.out\n\n# Dependency directories (remove the comment below to include it)\n# vendor/\n\n# Go workspace file\ngo.work\ngo.work.sum\n\n# env file\n.env\n",
	}

	reg := &httpmock.Registry{}
	httpStubs(reg)

	httpClient := func() (*http.Client, error) {
		return &http.Client{Transport: reg}, nil
	}
	client, _ := httpClient()
	defer reg.Verify(t)

	gotGitIgnoreTemplate, err := RepoGitIgnoreTemplate(client, "api.github.com", gitIgnoreTemplateName)

	assert.NoError(t, err, fmt.Sprintf("Expected no error while fetching /gitignore/templates/%v", gitIgnoreTemplateName))
	assert.Equal(t, wantGitIgnoreTemplate, gotGitIgnoreTemplate, fmt.Sprintf("GitIgnore template \"%v\" fetched is not as expected", gitIgnoreTemplateName))
}

func TestGitIgnoreTemplateReturnsErrorWhenGitIgnoreTemplateNotFound(t *testing.T) {
	gitIgnoreTemplateName := "invalid-gitignore"
	httpStubs := func(reg *httpmock.Registry) {
		reg.Register(
			httpmock.REST("GET", fmt.Sprintf("gitignore/templates/%v", gitIgnoreTemplateName)),
			httpmock.StatusStringResponse(404, heredoc.Doc(`
			{
				"message": "Not Found",
				"documentation_url": "https://docs.github.com/v3/gitignore",
				"status": "404"
			}`)),
		)
	}

	reg := &httpmock.Registry{}
	httpStubs(reg)

	httpClient := func() (*http.Client, error) {
		return &http.Client{Transport: reg}, nil
	}
	client, _ := httpClient()
	defer reg.Verify(t)

	_, err := RepoGitIgnoreTemplate(client, "api.github.com", gitIgnoreTemplateName)

	assert.Error(t, err, fmt.Sprintf("Expected error while fetching /gitignore/templates/%v", gitIgnoreTemplateName))
}
