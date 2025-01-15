package shared

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/go-gh/v2/pkg/asciisanitizer"
	"golang.org/x/text/transform"
)

const (
	Active             WorkflowState = "active"
	DisabledManually   WorkflowState = "disabled_manually"
	DisabledInactivity WorkflowState = "disabled_inactivity"
)

type iprompter interface {
	Select(string, string, []string) (int, error)
}

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

func (w *Workflow) ExportData(fields []string) map[string]interface{} {
	return cmdutil.StructExportData(w, fields)
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

func selectWorkflow(p iprompter, workflows []Workflow, promptMsg string, states []WorkflowState) (*Workflow, error) {
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

	selected, err := p.Select(promptMsg, "", candidates)
	if err != nil {
		return nil, err
	}

	return &filtered[selected], nil
}

// FindWorkflow looks up a workflow either by numeric database ID, file name, or its Name field
func FindWorkflow(client *api.Client, repo ghrepo.Interface, workflowSelector string, states []WorkflowState) ([]Workflow, error) {
	if workflowSelector == "" {
		return nil, errors.New("empty workflow selector")
	}

	if _, err := strconv.Atoi(workflowSelector); err == nil || isWorkflowFile(workflowSelector) {
		workflow, err := getWorkflowByID(client, repo, workflowSelector)
		if err != nil {
			return nil, err
		}
		return []Workflow{*workflow}, nil
	}

	return getWorkflowsByName(client, repo, workflowSelector, states)
}

func GetWorkflow(client *api.Client, repo ghrepo.Interface, workflowID int64) (*Workflow, error) {
	return getWorkflowByID(client, repo, strconv.FormatInt(workflowID, 10))
}

func isWorkflowFile(f string) bool {
	name := strings.ToLower(f)
	return strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml")
}

// ID can be either a numeric database ID or the workflow file name
func getWorkflowByID(client *api.Client, repo ghrepo.Interface, ID string) (*Workflow, error) {
	var workflow Workflow

	path := fmt.Sprintf("repos/%s/actions/workflows/%s", ghrepo.FullName(repo), url.PathEscape(ID))
	if err := client.REST(repo.RepoHost(), "GET", path, nil, &workflow); err != nil {
		return nil, err
	}

	return &workflow, nil
}

func getWorkflowsByName(client *api.Client, repo ghrepo.Interface, name string, states []WorkflowState) ([]Workflow, error) {
	workflows, err := GetWorkflows(client, repo, 0)
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch workflows for %s: %w", ghrepo.FullName(repo), err)
	}

	var filtered []Workflow
	for _, workflow := range workflows {
		if !strings.EqualFold(workflow.Name, name) {
			continue
		}
		for _, state := range states {
			if workflow.State == state {
				filtered = append(filtered, workflow)
				break
			}
		}
	}

	return filtered, nil
}

func ResolveWorkflow(p iprompter, io *iostreams.IOStreams, client *api.Client, repo ghrepo.Interface, prompt bool, workflowSelector string, states []WorkflowState) (*Workflow, error) {
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

		return selectWorkflow(p, workflows, "Select a workflow", states)
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

	return selectWorkflow(p, workflows, "Which workflow do you mean?", states)
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

	sanitized, err := io.ReadAll(transform.NewReader(bytes.NewReader(decoded), &asciisanitizer.Sanitizer{}))
	if err != nil {
		return nil, err
	}

	return sanitized, nil
}
