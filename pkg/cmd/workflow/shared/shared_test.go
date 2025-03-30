package shared

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ghAPI "github.com/cli/go-gh/v2/pkg/api"
)

func TestFindWorkflow(t *testing.T) {
	badRequestURL, err := url.Parse("https://api.github.com/repos/OWNER/REPO/actions/workflows/nonexistentWorkflow.yml")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name              string
		workflowSelector  string
		repo              ghrepo.Interface
		httpStubs         func(*httpmock.Registry)
		states            []WorkflowState
		expectedWorkflow  Workflow
		expectedHTTPError *api.HTTPError
		expectedError     error
	}{
		{
			name:             "When the workflow selector is empty, it returns an error",
			workflowSelector: "",
			repo:             ghrepo.New("OWNER", "REPO"),
			expectedError:    errors.New("empty workflow selector"),
		},
		{
			name:             "When the workflow selector is a number, it returns the workflow with that ID",
			workflowSelector: "1",
			repo:             ghrepo.New("OWNER", "REPO"),
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/1"),
					httpmock.StatusJSONResponse(200, Workflow{
						ID: 1,
					}),
				)
			},
			expectedWorkflow: Workflow{
				ID: 1,
			},
		},
		{
			name:             "When the workflow selector is a file, it returns the workflow with that path",
			workflowSelector: "workflowFile.yml",
			repo:             ghrepo.New("OWNER", "REPO"),
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/workflowFile.yml"),
					httpmock.StatusJSONResponse(200, Workflow{
						ID:   1,
						Name: "Some Workflow",
						Path: ".github/workflows/workflowFile.yml",
					}),
				)
			},
			expectedWorkflow: Workflow{
				ID:   1,
				Name: "Some Workflow",
				Path: ".github/workflows/workflowFile.yml",
			},
		},
		{
			name:             "When the workflow selector is a workflow that doesn't exist, it returns the workflow not found error",
			workflowSelector: "nonexistentWorkflow.yml",
			repo:             ghrepo.New("OWNER", "REPO"),
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", strings.TrimPrefix(badRequestURL.Path, "/")),
					httpmock.StatusJSONResponse(404, Workflow{}),
				)
			},
			expectedHTTPError: &api.HTTPError{
				HTTPError: &ghAPI.HTTPError{
					Message:    "workflow nonexistentWorkflow.yml not found on the default branch",
					StatusCode: 404,
					RequestURL: badRequestURL,
				},
			},
		},
		{
			name:             "When the workflow selector is a file but the server errors, it returns that error",
			workflowSelector: "nonexistentWorkflow.yml",
			repo:             ghrepo.New("OWNER", "REPO"),
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", strings.TrimPrefix(badRequestURL.Path, "/")),
					httpmock.StatusStringResponse(500, "server error"),
				)
			},
			expectedHTTPError: &api.HTTPError{
				HTTPError: &ghAPI.HTTPError{
					Errors: []ghAPI.HTTPErrorItem{
						{
							Message: "server error",
						},
					},
					StatusCode: 500,
					RequestURL: badRequestURL,
				},
			},
		},
		{
			name:             "When the workflow selector is a name and the state is active, it returns that workflow",
			workflowSelector: "Workflow Name",
			repo:             ghrepo.New("OWNER", "REPO"),
			states:           []WorkflowState{Active},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.StatusJSONResponse(200, WorkflowsPayload{
						Workflows: []Workflow{
							{
								ID:    1,
								Name:  "Workflow Name",
								State: Active,
							},
						}}),
				)
			},
			expectedWorkflow: Workflow{
				ID:    1,
				Name:  "Workflow Name",
				State: "active",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			client := api.NewClientFromHTTP(&http.Client{Transport: reg})

			workflow, err := FindWorkflow(client, tt.repo, tt.workflowSelector, tt.states)
			if tt.expectedError != nil {
				require.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
			} else if tt.expectedHTTPError != nil {
				var httpErr api.HTTPError
				require.ErrorAs(t, err, &httpErr)
				assert.Equal(t, tt.expectedHTTPError.Error(), httpErr.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedWorkflow, workflow[0])
			}
		})
	}
}

type ErrorTransport struct {
	Err error
}

func (t *ErrorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, t.Err
}

func TestFindWorkflow_nonHTTPError(t *testing.T) {
	t.Run("When the client fails to instantiate, it returns the error", func(t *testing.T) {
		client := api.NewClientFromHTTP(&http.Client{Transport: &ErrorTransport{Err: errors.New("non-HTTP error")}})
		repo := ghrepo.New("OWNER", "REPO")
		workflow, err := FindWorkflow(client, repo, "1", nil)

		require.Error(t, err)
		assert.ErrorContains(t, err, "non-HTTP error")
		assert.Nil(t, workflow)
	})
}

func Test_getWorkflowsByName_filtering(t *testing.T) {
	tests := []struct {
		name              string
		workflowName      string
		repo              ghrepo.Interface
		states            []WorkflowState
		httpStubs         func(*httpmock.Registry)
		expectedWorkflows []Workflow
		expectedErrorMsg  string
	}{
		{
			name:         "When no workflows match, no workflows are returned",
			workflowName: "Unmatched Workflow Name",
			repo:         ghrepo.New("OWNER", "REPO"),
			states:       []WorkflowState{Active},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.StatusJSONResponse(200, WorkflowsPayload{
						Workflows: []Workflow{
							{
								ID:    1,
								Name:  "Workflow Name",
								State: Active,
							},
							{
								ID:    2,
								Name:  "Workflow Name",
								State: DisabledInactivity,
							},
							{
								ID:    3,
								Name:  "Workflow Name",
								State: Active,
							},
						},
					}),
				)
			},
			expectedWorkflows: []Workflow(nil),
		},
		{
			name:         "When there are more than one workflow with the same name, only the ones matching the provided state are returned",
			workflowName: "Workflow Name",
			repo:         ghrepo.New("OWNER", "REPO"),
			states:       []WorkflowState{Active},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.StatusJSONResponse(200, WorkflowsPayload{
						Workflows: []Workflow{
							{
								ID:    1,
								Name:  "Workflow Name",
								State: Active,
							},
							{
								ID:    2,
								Name:  "Workflow Name",
								State: DisabledInactivity,
							},
							{
								ID:    3,
								Name:  "Workflow Name",
								State: Active,
							},
						},
					}),
				)
			},
			expectedWorkflows: []Workflow{
				{
					ID:    1,
					Name:  "Workflow Name",
					State: Active,
				},
				{
					ID:    3,
					Name:  "Workflow Name",
					State: Active,
				},
			},
		},
		{
			name: "When GetWorkflows errors",
			repo: ghrepo.New("OWNER", "REPO"),
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.StatusStringResponse(500, ""),
				)
			},
			expectedErrorMsg: "couldn't fetch workflows for OWNER/REPO",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			client := api.NewClientFromHTTP(&http.Client{Transport: reg})

			workflows, err := getWorkflowsByName(client, tt.repo, tt.workflowName, tt.states)
			if tt.expectedErrorMsg != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.expectedErrorMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedWorkflows, workflows)
			}
		})
	}
}

func TestGetWorkflows(t *testing.T) {
	tests := []struct {
		name              string
		repo              ghrepo.Interface
		limit             int
		httpStubs         func(*httpmock.Registry)
		expectedWorkflows []Workflow
		expectedError     error
	}{
		{
			name: "When the repo has no workflows, it returns an empty slice",
			repo: ghrepo.New("OWNER", "REPO"),
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.StatusJSONResponse(200, WorkflowsPayload{
						Workflows: []Workflow{},
					}),
				)
			},
			expectedWorkflows: []Workflow{},
		},
		{
			name: "When the api returns workflows, it returns those workflows",
			repo: ghrepo.New("OWNER", "REPO"),
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.StatusJSONResponse(200, WorkflowsPayload{
						Workflows: []Workflow{
							{
								Name: "Workflow 1",
							},
							{
								Name: "Workflow 2",
							},
						},
					}),
				)
			},
			expectedWorkflows: []Workflow{
				{
					Name: "Workflow 1",
				},
				{
					Name: "Workflow 2",
				},
			},
		},
		{
			name:  "When the api return paginates, it returns the workflows from all the pages",
			repo:  ghrepo.New("OWNER", "REPO"),
			limit: 0,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.StatusJSONResponse(200, WorkflowsPayload{
						Workflows: generateWorkflows(t, 100, 1),
					}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.StatusJSONResponse(200, WorkflowsPayload{
						Workflows: generateWorkflows(t, 50, 2),
					}),
				)
			},
			expectedWorkflows: append(generateWorkflows(t, 100, 1), generateWorkflows(t, 50, 2)...),
		},
		{
			name:  "When the limit is set to fewer workflows than the api returns, it returns the number of workflows specified by the limit",
			repo:  ghrepo.New("OWNER", "REPO"),
			limit: 2,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.StatusJSONResponse(200, WorkflowsPayload{
						Workflows: generateWorkflows(t, 100, 1),
					}),
				)
			},
			expectedWorkflows: generateWorkflows(t, 2, 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			client := api.NewClientFromHTTP(&http.Client{Transport: reg})

			workflows, err := GetWorkflows(client, tt.repo, tt.limit)
			if tt.expectedError != nil {
				require.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedWorkflows, workflows)
			}
		})
	}
}

// generateWorkflows returns an slice of workflows with the given count, labeled
// with the page number of testing pagination.
// The page number is used to generate unique Names and IDs for each workflow.
func generateWorkflows(t *testing.T, workflowCount int, pageNum int) []Workflow {
	t.Helper()
	workflows := []Workflow{}
	for i := 0; i < workflowCount; i++ {
		workflows = append(workflows, Workflow{
			Name: fmt.Sprintf("Workflow-%d-%d", pageNum, i),
			ID:   int64(i) + int64(pageNum-1)*100,
		})
	}
	return workflows
}
