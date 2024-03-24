package shared

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestIssueFromArgWithFields(t *testing.T) {
	type args struct {
		baseRepoFn func() (ghrepo.Interface, error)
		selector   string
	}
	tests := []struct {
		name         string
		args         args
		httpStub     func(*httpmock.Registry)
		wantIssue    int
		wantRepo     string
		wantProjects string
		wantErr      bool
	}{
		{
			name: "number argument",
			args: args{
				selector: "13",
				baseRepoFn: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("OWNER/REPO")
				},
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"hasIssuesEnabled": true,
						"issue":{"number":13}
					}}}`))
			},
			wantIssue: 13,
			wantRepo:  "https://github.com/OWNER/REPO",
		},
		{
			name: "number with hash argument",
			args: args{
				selector: "#13",
				baseRepoFn: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("OWNER/REPO")
				},
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"hasIssuesEnabled": true,
						"issue":{"number":13}
					}}}`))
			},
			wantIssue: 13,
			wantRepo:  "https://github.com/OWNER/REPO",
		},
		{
			name: "URL argument",
			args: args{
				selector:   "https://example.org/OWNER/REPO/issues/13#comment-123",
				baseRepoFn: nil,
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"hasIssuesEnabled": true,
						"issue":{"number":13}
					}}}`))
			},
			wantIssue: 13,
			wantRepo:  "https://example.org/OWNER/REPO",
		},
		{
			name: "PR URL argument",
			args: args{
				selector:   "https://example.org/OWNER/REPO/pull/13#comment-123",
				baseRepoFn: nil,
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"hasIssuesEnabled": true,
						"issue":{"number":13}
					}}}`))
			},
			wantIssue: 13,
			wantRepo:  "https://example.org/OWNER/REPO",
		},
		{
			name: "project cards permission issue",
			args: args{
				selector:   "https://example.org/OWNER/REPO/issues/13",
				baseRepoFn: nil,
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`
					{
						"data": {
							"repository": {
								"hasIssuesEnabled": true,
								"issue": {
									"number": 13,
									"projectCards": {
										"nodes": [
											null,
											{
												"project": {"name": "myproject"},
												"column": {"name": "To Do"}
											},
											null,
											{
												"project": {"name": "other project"},
												"column": null
											}
										]
									}
								}
							}
						},
						"errors": [
							{
								"type": "FORBIDDEN",
								"message": "Resource not accessible by integration",
								"path": ["repository", "issue", "projectCards", "nodes", 0]
							},
							{
								"type": "FORBIDDEN",
								"message": "Resource not accessible by integration",
								"path": ["repository", "issue", "projectCards", "nodes", 2]
							}
						]
					}`))
			},
			wantErr:      true,
			wantIssue:    13,
			wantProjects: "myproject, other project",
			wantRepo:     "https://example.org/OWNER/REPO",
		},
		{
			name: "projects permission issue",
			args: args{
				selector:   "https://example.org/OWNER/REPO/issues/13",
				baseRepoFn: nil,
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`
					{
						"data": {
							"repository": {
								"hasIssuesEnabled": true,
								"issue": {
									"number": 13,
									"projectCards": {
										"nodes": null,
										"totalCount": 0
									}
								}
							}
						},
						"errors": [
							{
								"type": "FORBIDDEN",
								"message": "Resource not accessible by integration",
								"path": ["repository", "issue", "projectCards", "nodes"]
							}
						]
					}`))
			},
			wantErr:      true,
			wantIssue:    13,
			wantProjects: "",
			wantRepo:     "https://example.org/OWNER/REPO",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStub != nil {
				tt.httpStub(reg)
			}
			httpClient := &http.Client{Transport: reg}
			issue, repo, err := IssueFromArgWithFields(httpClient, tt.args.baseRepoFn, tt.args.selector, []string{"number"})
			if (err != nil) != tt.wantErr {
				t.Errorf("IssueFromArgWithFields() error = %v, wantErr %v", err, tt.wantErr)
				if issue == nil {
					return
				}
			}
			if issue.Number != tt.wantIssue {
				t.Errorf("want issue #%d, got #%d", tt.wantIssue, issue.Number)
			}
			if gotProjects := strings.Join(issue.ProjectCards.ProjectNames(), ", "); gotProjects != tt.wantProjects {
				t.Errorf("want projects %q, got %q", tt.wantProjects, gotProjects)
			}
			repoURL := ghrepo.GenerateRepoURL(repo, "")
			if repoURL != tt.wantRepo {
				t.Errorf("want repo %s, got %s", tt.wantRepo, repoURL)
			}
		})
	}
}

func TestIssuesFromArgsWithFields(t *testing.T) {
	type args struct {
		baseRepoFn func() (ghrepo.Interface, error)
		selectors  []string
	}
	tests := []struct {
		name       string
		args       args
		httpStub   func(*httpmock.Registry)
		wantIssues []int
		wantRepo   string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "multiple repos",
			args: args{
				selectors: []string{"1", "https://github.com/OWNER/OTHERREPO/issues/2"},
				baseRepoFn: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"hasIssuesEnabled": true,
						"issue":{"number":1}
					}}}`))
				r.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{
							"hasIssuesEnabled": true,
							"issue":{"number":2}
					}}}`))
			},
			wantErr:    true,
			wantErrMsg: "multiple issues must be in same repo",
		},
		{
			name: "multiple issues",
			args: args{
				selectors: []string{"1", "2"},
				baseRepoFn: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"hasIssuesEnabled": true,
						"issue":{"number":1}
					}}}`))
				r.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{
							"hasIssuesEnabled": true,
							"issue":{"number":2}
						}}}`))
			},
			wantIssues: []int{1, 2},
			wantRepo:   "https://github.com/OWNER/REPO",
		},
	}
	for _, tt := range tests {
		if !tt.wantErr && len(tt.args.selectors) != len(tt.wantIssues) {
			t.Fatal("number of selectors and issues not equal")
		}
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStub != nil {
				tt.httpStub(reg)
			}
			httpClient := &http.Client{Transport: reg}
			issues, repo, err := IssuesFromArgsWithFields(httpClient, tt.args.baseRepoFn, tt.args.selectors, []string{"number"})
			if (err != nil) != tt.wantErr {
				t.Errorf("IssuesFromArgsWithFields() error = %v, wantErr %v", err, tt.wantErr)
				if issues == nil {
					return
				}
			}
			if tt.wantErr {
				assert.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErrMsg)
				return
			}
			assert.NoError(t, err)
			for i := range issues {
				assert.Contains(t, tt.wantIssues, issues[i].Number)
			}
			if repo != nil {
				repoURL := ghrepo.GenerateRepoURL(repo, "")
				if repoURL != tt.wantRepo {
					t.Errorf("want repo %s, got %s", tt.wantRepo, repoURL)
				}
			} else if tt.wantRepo != "" {
				t.Errorf("want repo %sw, got nil", tt.wantRepo)
			}
		})
	}
}
