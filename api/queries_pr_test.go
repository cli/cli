package api

import (
	"reflect"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/httpmock"
)

func TestBranchDeleteRemote(t *testing.T) {
	var tests = []struct {
		name           string
		responseStatus int
		responseBody   string
		expectError    bool
	}{
		{
			name:           "success",
			responseStatus: 204,
			responseBody:   "",
			expectError:    false,
		},
		{
			name:           "error",
			responseStatus: 500,
			responseBody:   `{"message": "oh no"}`,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			http := &httpmock.Registry{}
			http.Register(
				httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads/branch"),
				httpmock.StatusStringResponse(tt.responseStatus, tt.responseBody))

			client := NewClient(ReplaceTripper(http))
			repo, _ := ghrepo.FromFullName("OWNER/REPO")

			err := BranchDeleteRemote(client, repo, "branch")
			if (err != nil) != tt.expectError {
				t.Fatalf("unexpected result: %v", err)
			}
		})
	}
}

func Test_determinePullRequestFeatures(t *testing.T) {
	tests := []struct {
		name           string
		hostname       string
		queryResponse  string
		wantPrFeatures pullRequestFeature
		wantErr        bool
	}{
		{
			name:     "github.com",
			hostname: "github.com",
			wantPrFeatures: pullRequestFeature{
				HasReviewDecision:    true,
				HasStatusCheckRollup: true,
			},
			wantErr: false,
		},
		{
			name:     "GHE empty response",
			hostname: "git.my.org",
			queryResponse: heredoc.Doc(`
			{"data": {}}
			`),
			wantPrFeatures: pullRequestFeature{
				HasReviewDecision:    false,
				HasStatusCheckRollup: false,
			},
			wantErr: false,
		},
		{
			name:     "GHE has reviewDecision",
			hostname: "git.my.org",
			queryResponse: heredoc.Doc(`
			{"data": {
				"PullRequest": {
					"fields": [
						{"name": "foo"},
						{"name": "reviewDecision"}
					]
				}
			} }
			`),
			wantPrFeatures: pullRequestFeature{
				HasReviewDecision:    true,
				HasStatusCheckRollup: false,
			},
			wantErr: false,
		},
		{
			name:     "GHE has statusCheckRollup",
			hostname: "git.my.org",
			queryResponse: heredoc.Doc(`
			{"data": {
				"Commit": {
					"fields": [
						{"name": "foo"},
						{"name": "statusCheckRollup"}
					]
				}
			} }
			`),
			wantPrFeatures: pullRequestFeature{
				HasReviewDecision:    false,
				HasStatusCheckRollup: true,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeHTTP := &httpmock.Registry{}
			httpClient := NewHTTPClient(ReplaceTripper(fakeHTTP))

			if tt.queryResponse != "" {
				fakeHTTP.Register(
					httpmock.GraphQL(`query PullRequest_fields\b`),
					httpmock.StringResponse(tt.queryResponse))
			}

			gotPrFeatures, err := determinePullRequestFeatures(httpClient, tt.hostname)
			if (err != nil) != tt.wantErr {
				t.Errorf("determinePullRequestFeatures() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotPrFeatures, tt.wantPrFeatures) {
				t.Errorf("determinePullRequestFeatures() = %v, want %v", gotPrFeatures, tt.wantPrFeatures)
			}
		})
	}
}

func Test_sortPullRequestsByState(t *testing.T) {
	prs := []PullRequest{
		{
			BaseRefName: "test1",
			State:       "MERGED",
		},
		{
			BaseRefName: "test2",
			State:       "CLOSED",
		},
		{
			BaseRefName: "test3",
			State:       "OPEN",
		},
	}

	sortPullRequestsByState(prs)

	if prs[0].BaseRefName != "test3" {
		t.Errorf("prs[0]: got %s, want %q", prs[0].BaseRefName, "test3")
	}
	if prs[1].BaseRefName != "test1" {
		t.Errorf("prs[1]: got %s, want %q", prs[1].BaseRefName, "test1")
	}
	if prs[2].BaseRefName != "test2" {
		t.Errorf("prs[2]: got %s, want %q", prs[2].BaseRefName, "test2")
	}
}
