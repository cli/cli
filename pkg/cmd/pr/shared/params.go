package shared

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
)

func WithPrAndIssueQueryParams(baseURL, title, body string, assignees, labels, projects []string, milestones []string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	if title != "" {
		q.Set("title", title)
	}
	if body != "" {
		q.Set("body", body)
	}
	if len(assignees) > 0 {
		q.Set("assignees", strings.Join(assignees, ","))
	}
	if len(labels) > 0 {
		q.Set("labels", strings.Join(labels, ","))
	}
	if len(projects) > 0 {
		q.Set("projects", strings.Join(projects, ","))
	}
	if len(milestones) > 0 {
		q.Set("milestone", milestones[0])
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func AddMetadataToIssueParams(client *api.Client, baseRepo ghrepo.Interface, params map[string]interface{}, tb *IssueMetadataState) error {
	if !tb.HasMetadata() {
		return nil
	}

	if tb.MetadataResult == nil {
		resolveInput := api.RepoResolveInput{
			Reviewers:  tb.Reviewers,
			Assignees:  tb.Assignees,
			Labels:     tb.Labels,
			Projects:   tb.Projects,
			Milestones: tb.Milestones,
		}

		var err error
		tb.MetadataResult, err = api.RepoResolveMetadataIDs(client, baseRepo, resolveInput)
		if err != nil {
			return err
		}
	}

	assigneeIDs, err := tb.MetadataResult.MembersToIDs(tb.Assignees)
	if err != nil {
		return fmt.Errorf("could not assign user: %w", err)
	}
	params["assigneeIds"] = assigneeIDs

	labelIDs, err := tb.MetadataResult.LabelsToIDs(tb.Labels)
	if err != nil {
		return fmt.Errorf("could not add label: %w", err)
	}
	params["labelIds"] = labelIDs

	projectIDs, err := tb.MetadataResult.ProjectsToIDs(tb.Projects)
	if err != nil {
		return fmt.Errorf("could not add to project: %w", err)
	}
	params["projectIds"] = projectIDs

	if len(tb.Milestones) > 0 {
		milestoneID, err := tb.MetadataResult.MilestoneToID(tb.Milestones[0])
		if err != nil {
			return fmt.Errorf("could not add to milestone '%s': %w", tb.Milestones[0], err)
		}
		params["milestoneId"] = milestoneID
	}

	if len(tb.Reviewers) == 0 {
		return nil
	}

	var userReviewers []string
	var teamReviewers []string
	for _, r := range tb.Reviewers {
		if strings.ContainsRune(r, '/') {
			teamReviewers = append(teamReviewers, r)
		} else {
			userReviewers = append(userReviewers, r)
		}
	}

	userReviewerIDs, err := tb.MetadataResult.MembersToIDs(userReviewers)
	if err != nil {
		return fmt.Errorf("could not request reviewer: %w", err)
	}
	params["userReviewerIds"] = userReviewerIDs

	teamReviewerIDs, err := tb.MetadataResult.TeamsToIDs(teamReviewers)
	if err != nil {
		return fmt.Errorf("could not request reviewer: %w", err)
	}
	params["teamReviewerIds"] = teamReviewerIDs

	return nil
}

type FilterOptions struct {
	Entity     string
	State      string
	Assignee   string
	Labels     []string
	Author     string
	BaseBranch string
	Mention    string
	Milestone  string
}

func ListURLWithQuery(listURL string, options FilterOptions) (string, error) {
	u, err := url.Parse(listURL)
	if err != nil {
		return "", err
	}
	query := fmt.Sprintf("is:%s ", options.Entity)
	if options.State != "all" {
		query += fmt.Sprintf("is:%s ", options.State)
	}
	if options.Assignee != "" {
		query += fmt.Sprintf("assignee:%s ", options.Assignee)
	}
	for _, label := range options.Labels {
		query += fmt.Sprintf("label:%s ", quoteValueForQuery(label))
	}
	if options.Author != "" {
		query += fmt.Sprintf("author:%s ", options.Author)
	}
	if options.BaseBranch != "" {
		query += fmt.Sprintf("base:%s ", options.BaseBranch)
	}
	if options.Mention != "" {
		query += fmt.Sprintf("mentions:%s ", options.Mention)
	}
	if options.Milestone != "" {
		query += fmt.Sprintf("milestone:%s ", quoteValueForQuery(options.Milestone))
	}
	q := u.Query()
	q.Set("q", strings.TrimSuffix(query, " "))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func quoteValueForQuery(v string) string {
	if strings.ContainsAny(v, " \"\t\r\n") {
		return fmt.Sprintf("%q", v)
	}
	return v
}
