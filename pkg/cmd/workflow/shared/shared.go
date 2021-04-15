package shared

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
)

const (
	Active           WorkflowState = "active"
	DisabledManually WorkflowState = "disabled_manually"
)

type WorkflowState string

type Workflow struct {
	Name  string
	ID    int64
	Path  string
	State WorkflowState
}

type WorkflowsPayload struct {
	Workflows []Workflow
}

func (w *Workflow) Disabled() bool {
	return w.State != Active
}

func (w *Workflow) Base() string {
	return path.Base(w.Path)
}

func GetWorkflows(client *api.Client, repo ghrepo.Interface, limit int) ([]Workflow, error) {
	perPage := limit
	page := 1
	if limit > 100 || limit == 0 {
		perPage = 100
	}

	workflows := []Workflow{}

	for {
		if limit > 0 && len(workflows) == limit {
			break
		}
		var result WorkflowsPayload

		path := fmt.Sprintf("repos/%s/actions/workflows?per_page=%d&page=%d", ghrepo.FullName(repo), perPage, page)

		err := client.REST(repo.RepoHost(), "GET", path, nil, &result)
		if err != nil {
			return nil, err
		}

		for _, workflow := range result.Workflows {
			workflows = append(workflows, workflow)
			if limit > 0 && len(workflows) == limit {
				break
			}
		}

		if len(result.Workflows) < perPage {
			break
		}

		page++
	}

	return workflows, nil
}

type FilteredAllError struct {
	error
}

func SelectWorkflow(workflows []Workflow, promptMsg string, states []WorkflowState) (*Workflow, error) {
	filtered := []Workflow{}
	candidates := []string{}
	for _, workflow := range workflows {
		for _, state := range states {
			if workflow.State == state {
				filtered = append(filtered, workflow)
				candidates = append(candidates, fmt.Sprintf("%s (%s)", workflow.Name, workflow.Base()))
				break
			}
		}
	}

	if len(candidates) == 0 {
		return nil, FilteredAllError{errors.New("")}
	}

	var selected int

	err := prompt.SurveyAskOne(&survey.Select{
		Message:  promptMsg,
		Options:  candidates,
		PageSize: 15,
	}, &selected)
	if err != nil {
		return nil, err
	}

	return &filtered[selected], nil
}

func FindWorkflow(client *api.Client, repo ghrepo.Interface, workflowSelector string, states []WorkflowState) ([]Workflow, error) {
	if workflowSelector == "" {
		return nil, errors.New("empty workflow selector")
	}

	workflow, err := getWorkflowByID(client, repo, workflowSelector)
	if err == nil {
		return []Workflow{*workflow}, nil
	} else {
		var httpErr api.HTTPError
		if !errors.As(err, &httpErr) || httpErr.StatusCode != 404 {
			return nil, err
		}
	}

	return getWorkflowsByName(client, repo, workflowSelector, states)
}

func getWorkflowByID(client *api.Client, repo ghrepo.Interface, ID string) (*Workflow, error) {
	var workflow Workflow

	err := client.REST(repo.RepoHost(), "GET",
		fmt.Sprintf("repos/%s/actions/workflows/%s", ghrepo.FullName(repo), ID),
		nil, &workflow)

	if err != nil {
		return nil, err
	}

	return &workflow, nil
}

func getWorkflowsByName(client *api.Client, repo ghrepo.Interface, name string, states []WorkflowState) ([]Workflow, error) {
	workflows, err := GetWorkflows(client, repo, 0)
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch workflows for %s: %w", ghrepo.FullName(repo), err)
	}
	filtered := []Workflow{}

	for _, workflow := range workflows {
		desiredState := false
		for _, state := range states {
			if workflow.State == state {
				desiredState = true
				break
			}
		}

		if !desiredState {
			continue
		}

		// TODO consider fuzzy or prefix match
		if strings.EqualFold(workflow.Name, name) {
			filtered = append(filtered, workflow)
		}
	}

	return filtered, nil
}

func ResolveWorkflow(io *iostreams.IOStreams, client *api.Client, repo ghrepo.Interface, prompt bool, workflowSelector string, states []WorkflowState) (*Workflow, error) {
	if prompt {
		workflows, err := GetWorkflows(client, repo, 0)
		if len(workflows) == 0 {
			err = errors.New("no workflows are enabled")
		}

		if err != nil {
			var httpErr api.HTTPError
			if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
				err = errors.New("no workflows are enabled")
			}

			return nil, fmt.Errorf("could not fetch workflows for %s: %w", ghrepo.FullName(repo), err)
		}

		return SelectWorkflow(workflows, "Select a workflow", states)
	}

	workflows, err := FindWorkflow(client, repo, workflowSelector, states)
	if err != nil {
		return nil, err
	}

	if len(workflows) == 0 {
		return nil, fmt.Errorf("could not find any workflows named %s", workflowSelector)
	}

	if len(workflows) == 1 {
		return &workflows[0], nil
	}

	if !io.CanPrompt() {
		errMsg := "could not resolve to a unique workflow; found:"
		for _, workflow := range workflows {
			errMsg += fmt.Sprintf(" %s", workflow.Base())
		}
		return nil, errors.New(errMsg)
	}

	return SelectWorkflow(workflows, "Which workflow do you mean?", states)
}

func GetWorkflowContent(client *api.Client, repo ghrepo.Interface, workflow Workflow, ref string) ([]byte, error) {
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
		return nil, err
	}

	decoded, err := base64.StdEncoding.DecodeString(result.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to decode workflow file: %w", err)
	}

	return decoded, nil
}
