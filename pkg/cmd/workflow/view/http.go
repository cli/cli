package view

import (
	"encoding/base64"
	"fmt"
	"net/url"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	runShared "github.com/cli/cli/pkg/cmd/run/shared"
	"github.com/cli/cli/pkg/cmd/workflow/shared"
)

type workflowRuns struct {
	Total int
	Runs  []runShared.Run
}

func getWorkflowContent(client *api.Client, repo ghrepo.Interface, ref string, workflow *shared.Workflow) (string, error) {
	path := fmt.Sprintf("repos/%s/contents/%s", ghrepo.FullName(repo), workflow.Path)
	if ref != "" {
		q := fmt.Sprintf("?ref=%s", url.QueryEscape(ref))
		path = path + q
	}

	type Result struct {
		Content string
	}

	var result Result
	err := client.REST(repo.RepoHost(), "GET", path, nil, &result)
	if err != nil {
		return "", err
	}

	decoded, err := base64.StdEncoding.DecodeString(result.Content)
	if err != nil {
		return "", fmt.Errorf("failed to decode workflow file: %w", err)
	}

	return string(decoded), nil
}

func getWorkflowRuns(client *api.Client, repo ghrepo.Interface, workflow *shared.Workflow) (workflowRuns, error) {
	var wr workflowRuns
	var result runShared.RunsPayload
	path := fmt.Sprintf("repos/%s/actions/workflows/%d/runs?per_page=%d&page=%d", ghrepo.FullName(repo), workflow.ID, 5, 1)

	err := client.REST(repo.RepoHost(), "GET", path, nil, &result)
	if err != nil {
		return wr, err
	}

	wr.Total = result.TotalCount
	wr.Runs = append(wr.Runs, result.WorkflowRuns...)

	return wr, nil
}
