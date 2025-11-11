package capi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const defaultEventType = "gh_cli"

// Job represents a coding agent's task. Used to request a new session.
type Job struct {
	ID                string          `json:"job_id,omitempty"`
	SessionID         string          `json:"session_id,omitempty"`
	ProblemStatement  string          `json:"problem_statement,omitempty"`
	CustomAgent       string          `json:"custom_agent,omitempty"`
	EventType         string          `json:"event_type,omitempty"`
	ContentFilterMode string          `json:"content_filter_mode,omitempty"`
	Status            string          `json:"status,omitempty"`
	Result            string          `json:"result,omitempty"`
	Actor             *JobActor       `json:"actor,omitempty"`
	CreatedAt         time.Time       `json:"created_at,omitempty"`
	UpdatedAt         time.Time       `json:"updated_at,omitempty"`
	PullRequest       *JobPullRequest `json:"pull_request,omitempty"`
	WorkflowRun       *struct {
		ID string `json:"id"`
	} `json:"workflow_run,omitempty"`
	ErrorInfo *JobError `json:"error,omitempty"`
}

type JobActor struct {
	ID    int    `json:"id"`
	Login string `json:"login"`
}

type JobPullRequest struct {
	ID      int    `json:"id"`
	Number  int    `json:"number"`
	BaseRef string `json:"base_ref,omitempty"`
}

type JobError struct {
	Message            string `json:"message"`
	ResponseStatusCode int    `json:"response_status_code,string"`
	Service            string `json:"service"`
}

const jobsBasePathV1 = baseCAPIURL + "/agents/swe/v1/jobs"

// CreateJob queues a new job using the v1 Jobs API. It may or may not
// return Pull Request information. If Pull Request information is required
// following up by polling GetJob with the job ID is necessary.
func (c *CAPIClient) CreateJob(ctx context.Context, owner, repo, problemStatement, baseBranch, customAgent string) (*Job, error) {
	if owner == "" || repo == "" {
		return nil, errors.New("owner and repo are required")
	}
	if problemStatement == "" {
		return nil, errors.New("problem statement is required")
	}

	url := fmt.Sprintf("%s/%s/%s", jobsBasePathV1, url.PathEscape(owner), url.PathEscape(repo))

	prOpts := JobPullRequest{}
	if baseBranch != "" {
		prOpts.BaseRef = "refs/heads/" + baseBranch
	}

	payload := &Job{
		ProblemStatement: problemStatement,
		CustomAgent:      customAgent,
		EventType:        defaultEventType,
		PullRequest:      &prOpts,
	}

	b, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)

	var j Job
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&j); err != nil {
		if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK { // accept 201 or 200
			// This happens when there's an error like unauthorized (401).
			statusText := fmt.Sprintf("%d %s", res.StatusCode, http.StatusText(res.StatusCode))
			return nil, fmt.Errorf("failed to create job: %s", statusText)
		}
		return nil, fmt.Errorf("failed to decode create job response: %w", err)
	}

	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK { // accept 201 or 200
		statusText := fmt.Sprintf("%d %s", res.StatusCode, http.StatusText(res.StatusCode))

		// If the response has error embeded, we can use that.
		// TODO: Does this really ever happen?
		if j.ErrorInfo != nil {
			return nil, fmt.Errorf("failed to create job: %s: %s", statusText, j.ErrorInfo.Message)
		}

		// If the response doesn't have error embedded,
		// try to decode the response itself as a jobError.
		var errInfo JobError
		if err := json.NewDecoder(bytes.NewReader(body)).Decode(&errInfo); err != nil {
			return nil, fmt.Errorf("failed to create job: %s", statusText)
		}

		return nil, fmt.Errorf("failed to create job: %s: %s", statusText, errInfo.Message)
	}

	return &j, nil
}

// GetJob retrieves a agent job
func (c *CAPIClient) GetJob(ctx context.Context, owner, repo, jobID string) (*Job, error) {
	if owner == "" || repo == "" || jobID == "" {
		return nil, errors.New("owner, repo, and jobID are required")
	}
	url := fmt.Sprintf("%s/%s/%s/%s", jobsBasePathV1, url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(jobID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		// Normalize to "<code> <text>" form
		statusText := fmt.Sprintf("%d %s", res.StatusCode, http.StatusText(res.StatusCode))
		return nil, fmt.Errorf("failed to get job: %s", statusText)
	}
	var j Job
	if err := json.NewDecoder(res.Body).Decode(&j); err != nil {
		return nil, fmt.Errorf("failed to decode get job response: %w", err)
	}
	return &j, nil
}
