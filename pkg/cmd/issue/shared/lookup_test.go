package shared

import (
	"net/http"
	"testing"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/httpmock"
)

func TestIssueFromArg(t *testing.T) {
	type args struct {
		baseRepoFn func() (ghrepo.Interface, error)
		selector   string
	}
	tests := []struct {
		name      string
		args      args
		httpStub  func(*httpmock.Registry)
		wantIssue int
		wantRepo  string
		wantErr   bool
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStub != nil {
				tt.httpStub(reg)
			}
			httpClient := &http.Client{Transport: reg}
			issue, repo, err := IssueFromArg(api.NewClientFromHTTP(httpClient), tt.args.baseRepoFn, tt.args.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("IssueFromArg() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if issue.Number != tt.wantIssue {
				t.Errorf("want issue #%d, got #%d", tt.wantIssue, issue.Number)
			}
			repoURL := ghrepo.GenerateRepoURL(repo, "")
			if repoURL != tt.wantRepo {
				t.Errorf("want repo %s, got %s", tt.wantRepo, repoURL)
			}
		})
	}
}
