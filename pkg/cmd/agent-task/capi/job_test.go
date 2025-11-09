package capi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetJobRequiresRepoAndJobID(t *testing.T) {
	client := &CAPIClient{}
	_, err := client.GetJob(context.Background(), "", "", "only-job-id")
	assert.EqualError(t, err, "owner, repo, and jobID are required")
	_, err = client.GetJob(context.Background(), "", "only-repo", "")
	assert.EqualError(t, err, "owner, repo, and jobID are required")
	_, err = client.GetJob(context.Background(), "only-owner", "", "")
	assert.EqualError(t, err, "owner, repo, and jobID are required")
	_, err = client.GetJob(context.Background(), "", "", "")
	assert.EqualError(t, err, "owner, repo, and jobID are required")
}

func TestGetJob(t *testing.T) {
	sampleDateString := "2025-08-29T00:00:00Z"
	sampleDate, err := time.Parse(time.RFC3339, sampleDateString)
	require.NoError(t, err)

	tests := []struct {
		name      string
		httpStubs func(*testing.T, *httpmock.Registry)
		wantErr   string
		wantOut   *Job
	}{
		{
			name: "job without PR",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/swe/v1/jobs/OWNER/REPO/job123"), "api.githubcopilot.com"),
					httpmock.StatusStringResponse(200, heredoc.Docf(`
						{
							"job_id": "job123",
							"session_id": "sess1",
							"problem_statement": "Do the thing",
							"event_type": "foo",
							"content_filter_mode": "foo",
							"status": "foo",
							"result": "foo",
							"actor": {
								"id": 1,
								"login": "octocat"
							},
							"created_at": "%[1]s",
							"updated_at": "%[1]s"
						}`,
						sampleDateString,
					)),
				)
			},
			wantOut: &Job{
				ID:                "job123",
				SessionID:         "sess1",
				ProblemStatement:  "Do the thing",
				EventType:         "foo",
				ContentFilterMode: "foo",
				Status:            "foo",
				Result:            "foo",
				Actor: &JobActor{
					ID:    1,
					Login: "octocat",
				},
				CreatedAt: sampleDate,
				UpdatedAt: sampleDate,
			},
		},
		{
			name: "job with PR",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/swe/v1/jobs/OWNER/REPO/job123"), "api.githubcopilot.com"),
					httpmock.StatusStringResponse(200, heredoc.Docf(`
						{
							"job_id": "job123",
							"session_id": "sess1",
							"problem_statement": "Do the thing",
							"event_type": "foo",
							"content_filter_mode": "foo",
							"status": "foo",
							"result": "foo",
							"actor": {
								"id": 1,
								"login": "octocat"
							},
							"created_at": "%[1]s",
							"updated_at": "%[1]s",
							"pull_request": {
								"id": 101,
								"number": 42
							}
						}`,
						sampleDateString,
					)),
				)
			},
			wantOut: &Job{
				ID:                "job123",
				SessionID:         "sess1",
				ProblemStatement:  "Do the thing",
				EventType:         "foo",
				ContentFilterMode: "foo",
				Status:            "foo",
				Result:            "foo",
				Actor: &JobActor{
					ID:    1,
					Login: "octocat",
				},
				CreatedAt: sampleDate,
				UpdatedAt: sampleDate,
				PullRequest: &JobPullRequest{
					ID:     101,
					Number: 42,
				},
			},
		},
		{
			name: "job not found",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/swe/v1/jobs/OWNER/REPO/job123"), "api.githubcopilot.com"),
					httpmock.StatusStringResponse(404, `{}`),
				)
			},
			wantErr: "failed to get job: 404 Not Found",
		},
		{
			name: "API error",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/swe/v1/jobs/OWNER/REPO/job123"), "api.githubcopilot.com"),
					httpmock.StatusStringResponse(500, `{}`),
				)
			},
			wantErr: "failed to get job: 500 Internal Server Error",
		},
		{
			name: "invalid JSON response",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/swe/v1/jobs/OWNER/REPO/job123"), "api.githubcopilot.com"),
					httpmock.StatusStringResponse(200, ``),
				)
			},
			wantErr: "failed to decode get job response: EOF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}
			defer reg.Verify(t)

			httpClient := &http.Client{Transport: reg}

			cfg := config.NewBlankConfig()
			capiClient := NewCAPIClient(httpClient, cfg.Authentication())

			job, err := capiClient.GetJob(context.Background(), "OWNER", "REPO", "job123")

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				require.Nil(t, job)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantOut, job)
		})
	}
}

func TestCreateJobRequiresRepoAndProblemStatement(t *testing.T) {
	client := &CAPIClient{}

	_, err := client.CreateJob(context.Background(), "", "only-repo", "", "", "")
	assert.EqualError(t, err, "owner and repo are required")
	_, err = client.CreateJob(context.Background(), "only-owner", "", "", "", "")
	assert.EqualError(t, err, "owner and repo are required")
	_, err = client.CreateJob(context.Background(), "", "", "", "", "")
	assert.EqualError(t, err, "owner and repo are required")

	_, err = client.CreateJob(context.Background(), "owner", "repo", "", "", "")
	assert.EqualError(t, err, "problem statement is required")
}

func TestCreateJob(t *testing.T) {
	sampleDateString := "2025-08-29T00:00:00Z"
	sampleDate, err := time.Parse(time.RFC3339, sampleDateString)
	require.NoError(t, err)

	tests := []struct {
		name        string
		baseBranch  string
		customAgent string
		httpStubs   func(*testing.T, *httpmock.Registry)
		wantErr     string
		wantOut     *Job
	}{
		{
			name: "success",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("POST", "agents/swe/v1/jobs/OWNER/REPO"), "api.githubcopilot.com"),
					httpmock.RESTPayload(201,
						heredoc.Docf(`
							{
								"job_id": "job123",
								"session_id": "sess1",
								"problem_statement": "Do the thing",
								"event_type": "foo",
								"content_filter_mode": "foo",
								"status": "foo",
								"result": "foo",
								"actor": {
									"id": 1,
									"login": "octocat"
								},
								"created_at": "%[1]s",
								"updated_at": "%[1]s"
							}
						`, sampleDateString),
						func(payload map[string]interface{}) {
							assert.Equal(t, "Do the thing", payload["problem_statement"])
							assert.Equal(t, "gh_cli", payload["event_type"])
						},
					),
				)
			},
			wantOut: &Job{
				ID:                "job123",
				SessionID:         "sess1",
				ProblemStatement:  "Do the thing",
				EventType:         "foo",
				ContentFilterMode: "foo",
				Status:            "foo",
				Result:            "foo",
				Actor: &JobActor{
					ID:    1,
					Login: "octocat",
				},
				CreatedAt: sampleDate,
				UpdatedAt: sampleDate,
			},
		},
		{
			name:       "success with base branch",
			baseBranch: "some-branch",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("POST", "agents/swe/v1/jobs/OWNER/REPO"), "api.githubcopilot.com"),
					httpmock.RESTPayload(201,
						heredoc.Docf(`
							{
								"job_id": "job123",
								"session_id": "sess1",
								"problem_statement": "Do the thing",
								"event_type": "foo",
								"content_filter_mode": "foo",
								"status": "foo",
								"result": "foo",
								"actor": {
									"id": 1,
									"login": "octocat"
								},
								"created_at": "%[1]s",
								"updated_at": "%[1]s"
							}
						`, sampleDateString),
						func(payload map[string]interface{}) {
							assert.Equal(t, "Do the thing", payload["problem_statement"])
							assert.Equal(t, "gh_cli", payload["event_type"])
							assert.Equal(t, "refs/heads/some-branch", payload["pull_request"].(map[string]interface{})["base_ref"])
						},
					),
				)
			},
			wantOut: &Job{
				ID:                "job123",
				SessionID:         "sess1",
				ProblemStatement:  "Do the thing",
				EventType:         "foo",
				ContentFilterMode: "foo",
				Status:            "foo",
				Result:            "foo",
				Actor: &JobActor{
					ID:    1,
					Login: "octocat",
				},
				CreatedAt: sampleDate,
				UpdatedAt: sampleDate,
			},
		},
		{
			name:        "Success with custom agent",
			customAgent: "my-custom-agent",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("POST", "agents/swe/v1/jobs/OWNER/REPO"), "api.githubcopilot.com"),
					httpmock.RESTPayload(201,
						heredoc.Docf(`
							{
								"job_id": "job123",
								"session_id": "sess1",
								"problem_statement": "Do the thing",
								"custom_agent": "my-custom-agent",
								"event_type": "foo",
								"content_filter_mode": "foo",
								"status": "foo",
								"result": "foo",
								"actor": {
									"id": 1,
									"login": "octocat"
								},
								"created_at": "%[1]s",
								"updated_at": "%[1]s"
							}
						`, sampleDateString),
						func(payload map[string]interface{}) {
							assert.Equal(t, "Do the thing", payload["problem_statement"])
							assert.Equal(t, "gh_cli", payload["event_type"])
							assert.Equal(t, "my-custom-agent", payload["custom_agent"])
						},
					),
				)
			},
			wantOut: &Job{
				ID:                "job123",
				SessionID:         "sess1",
				ProblemStatement:  "Do the thing",
				CustomAgent:       "my-custom-agent",
				EventType:         "foo",
				ContentFilterMode: "foo",
				Status:            "foo",
				Result:            "foo",
				Actor: &JobActor{
					ID:    1,
					Login: "octocat",
				},
				CreatedAt: sampleDate,
				UpdatedAt: sampleDate,
			},
		},
		{
			name: "API error, included in response body",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("POST", "agents/swe/v1/jobs/OWNER/REPO"), "api.githubcopilot.com"),
					httpmock.StatusStringResponse(500, heredoc.Doc(`{
						"error": {
							"message": "some error"
						}
					}`)),
				)
			},
			wantErr: "failed to create job: 500 Internal Server Error: some error",
		},
		{
			name: "API error",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("POST", "agents/swe/v1/jobs/OWNER/REPO"), "api.githubcopilot.com"),
					httpmock.StatusStringResponse(500, `{}`),
				)
			},
			wantErr: "failed to create job: 500 Internal Server Error: ",
		},
		{
			name: "invalid JSON response, non-HTTP 200",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("POST", "agents/swe/v1/jobs/OWNER/REPO"), "api.githubcopilot.com"),
					httpmock.StatusStringResponse(401, `Unauthorized`),
				)
			},
			wantErr: "failed to create job: 401 Unauthorized",
		},
		{
			name: "invalid JSON response, HTTP 200",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("POST", "agents/swe/v1/jobs/OWNER/REPO"), "api.githubcopilot.com"),
					httpmock.StatusStringResponse(200, ``),
				)
			},
			wantErr: "failed to decode create job response: EOF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}
			defer reg.Verify(t)

			httpClient := &http.Client{Transport: reg}

			cfg := config.NewBlankConfig()
			capiClient := NewCAPIClient(httpClient, cfg.Authentication())

			job, err := capiClient.CreateJob(context.Background(), "OWNER", "REPO", "Do the thing", tt.baseBranch, tt.customAgent)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				require.Nil(t, job)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantOut, job)
		})
	}
}
