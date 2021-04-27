package api

import (
	"reflect"
	"strings"
)

func (issue *Issue) ExportData(fields []string) *map[string]interface{} {
	v := reflect.ValueOf(issue).Elem()
	data := map[string]interface{}{}

	for _, f := range fields {
		switch f {
		case "milestone":
			if issue.Milestone.Title != "" {
				data[f] = &issue.Milestone
			} else {
				data[f] = nil
			}
		case "comments":
			data[f] = issue.Comments.Nodes
		case "assignees":
			data[f] = issue.Assignees.Nodes
		case "labels":
			data[f] = issue.Labels.Nodes
		case "projectCards":
			data[f] = issue.ProjectCards.Nodes
		default:
			sf := fieldByName(v, f)
			data[f] = sf.Interface()
		}
	}

	return &data
}

func (pr *PullRequest) ExportData(fields []string) *map[string]interface{} {
	v := reflect.ValueOf(pr).Elem()
	data := map[string]interface{}{}

	for _, f := range fields {
		switch f {
		case "headRepository":
			data[f] = map[string]string{"name": pr.HeadRepository.Name}
		case "milestone":
			if pr.Milestone.Title != "" {
				data[f] = &pr.Milestone
			} else {
				data[f] = nil
			}
		case "statusCheckRollup":
			if n := pr.Commits.Nodes; len(n) > 0 {
				data[f] = n[0].Commit.StatusCheckRollup.Contexts.Nodes
			} else {
				data[f] = nil
			}
		case "comments":
			data[f] = pr.Comments.Nodes
		case "assignees":
			data[f] = pr.Assignees.Nodes
		case "labels":
			data[f] = pr.Labels.Nodes
		case "projectCards":
			data[f] = pr.ProjectCards.Nodes
		case "reviews":
			data[f] = pr.Reviews.Nodes
		case "files":
			data[f] = pr.Files.Nodes
		case "reviewRequests":
			requests := make([]interface{}, 0, len(pr.ReviewRequests.Nodes))
			for _, req := range pr.ReviewRequests.Nodes {
				if req.RequestedReviewer.TypeName == "" {
					continue
				}
				requests = append(requests, req.RequestedReviewer)
			}
			data[f] = &requests
		default:
			sf := fieldByName(v, f)
			data[f] = sf.Interface()
		}
	}

	return &data
}

func ExportIssues(issues []Issue, fields []string) *[]interface{} {
	data := make([]interface{}, len(issues))
	for i, issue := range issues {
		data[i] = issue.ExportData(fields)
	}
	return &data
}

func ExportPRs(prs []PullRequest, fields []string) *[]interface{} {
	data := make([]interface{}, len(prs))
	for i, pr := range prs {
		data[i] = pr.ExportData(fields)
	}
	return &data
}

func fieldByName(v reflect.Value, field string) reflect.Value {
	return v.FieldByNameFunc(func(s string) bool {
		return strings.EqualFold(field, s)
	})
}
