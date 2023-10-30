package format

import (
	"encoding/json"
	"strings"

	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
)

// JSONProject serializes a Project to JSON.
func JSONProject(project queries.Project) ([]byte, error) {
	return json.Marshal(projectJSON{
		Number:           project.Number,
		URL:              project.URL,
		ShortDescription: project.ShortDescription,
		Public:           project.Public,
		Closed:           project.Closed,
		// The Template field is commented out due to https://github.com/cli/cli/issues/8103.
		// The Template field does not exist on GHES 3.8 and older, once GHES 3.8 gets
		// deprecated on 2024-03-07 we can start populating this field again.
		// Template: project.Template,
		Title:  project.Title,
		ID:     project.ID,
		Readme: project.Readme,
		Items: struct {
			TotalCount int `json:"totalCount"`
		}{
			TotalCount: project.Items.TotalCount,
		},
		Fields: struct {
			TotalCount int `json:"totalCount"`
		}{
			TotalCount: project.Fields.TotalCount,
		},
		Owner: struct {
			Type  string `json:"type"`
			Login string `json:"login"`
		}{
			Type:  project.OwnerType(),
			Login: project.OwnerLogin(),
		},
	})
}

// JSONProjects serializes a slice of Projects to JSON.
// JSON fields are `totalCount` and `projects`.
func JSONProjects(projects []queries.Project, totalCount int) ([]byte, error) {
	var result []projectJSON
	for _, p := range projects {
		result = append(result, projectJSON{
			Number:           p.Number,
			URL:              p.URL,
			ShortDescription: p.ShortDescription,
			Public:           p.Public,
			Closed:           p.Closed,
			// The Template field is commented out due to https://github.com/cli/cli/issues/8103.
			// The Template field does not exist on GHES 3.8 and older, once GHES 3.8 gets
			// deprecated on 2024-03-07 we can start populating this field again.
			// Template: p.Template,
			Title:  p.Title,
			ID:     p.ID,
			Readme: p.Readme,
			Items: struct {
				TotalCount int `json:"totalCount"`
			}{
				TotalCount: p.Items.TotalCount,
			},
			Fields: struct {
				TotalCount int `json:"totalCount"`
			}{
				TotalCount: p.Fields.TotalCount,
			},
			Owner: struct {
				Type  string `json:"type"`
				Login string `json:"login"`
			}{
				Type:  p.OwnerType(),
				Login: p.OwnerLogin(),
			},
		})
	}

	return json.Marshal(struct {
		Projects   []projectJSON `json:"projects"`
		TotalCount int           `json:"totalCount"`
	}{
		Projects:   result,
		TotalCount: totalCount,
	})
}

type projectJSON struct {
	Number           int32  `json:"number"`
	URL              string `json:"url"`
	ShortDescription string `json:"shortDescription"`
	Public           bool   `json:"public"`
	Closed           bool   `json:"closed"`
	// The Template field is commented out due to https://github.com/cli/cli/issues/8103.
	// The Template field does not exist on GHES 3.8 and older, once GHES 3.8 gets
	// deprecated on 2024-03-07 we can start populating this field again.
	// Template         bool   `json:"template"`
	Title  string `json:"title"`
	ID     string `json:"id"`
	Readme string `json:"readme"`
	Items  struct {
		TotalCount int `json:"totalCount"`
	} `graphql:"items(first: 100)" json:"items"`
	Fields struct {
		TotalCount int `json:"totalCount"`
	} `graphql:"fields(first:100)" json:"fields"`
	Owner struct {
		Type  string `json:"type"`
		Login string `json:"login"`
	} `json:"owner"`
}

// JSONProjectField serializes a ProjectField to JSON.
func JSONProjectField(field queries.ProjectField) ([]byte, error) {
	val := projectFieldJSON{
		ID:   field.ID(),
		Name: field.Name(),
		Type: field.Type(),
	}
	for _, o := range field.Options() {
		val.Options = append(val.Options, singleSelectOptionJSON{
			Name: o.Name,
			ID:   o.ID,
		})
	}

	return json.Marshal(val)
}

// JSONProjectFields serializes a slice of ProjectFields to JSON.
// JSON fields are `totalCount` and `fields`.
func JSONProjectFields(project *queries.Project) ([]byte, error) {
	var result []projectFieldJSON
	for _, f := range project.Fields.Nodes {
		val := projectFieldJSON{
			ID:   f.ID(),
			Name: f.Name(),
			Type: f.Type(),
		}
		for _, o := range f.Options() {
			val.Options = append(val.Options, singleSelectOptionJSON{
				Name: o.Name,
				ID:   o.ID,
			})
		}

		result = append(result, val)
	}

	return json.Marshal(struct {
		Fields     []projectFieldJSON `json:"fields"`
		TotalCount int                `json:"totalCount"`
	}{
		Fields:     result,
		TotalCount: project.Fields.TotalCount,
	})
}

type projectFieldJSON struct {
	ID      string                   `json:"id"`
	Name    string                   `json:"name"`
	Type    string                   `json:"type"`
	Options []singleSelectOptionJSON `json:"options,omitempty"`
}

type singleSelectOptionJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// JSONProjectItem serializes a ProjectItem to JSON.
func JSONProjectItem(item queries.ProjectItem) ([]byte, error) {
	return json.Marshal(projectItemJSON{
		ID:    item.ID(),
		Title: item.Title(),
		Body:  item.Body(),
		Type:  item.Type(),
		URL:   item.URL(),
	})
}

type projectItemJSON struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
	Type  string `json:"type"`
	URL   string `json:"url,omitempty"`
}

// JSONProjectDraftIssue serializes a DraftIssue to JSON.
// This is needed because the field for
// https://docs.github.com/en/graphql/reference/mutations#updateprojectv2draftissue
// is a DraftIssue https://docs.github.com/en/graphql/reference/objects#draftissue
// and not a ProjectV2Item https://docs.github.com/en/graphql/reference/objects#projectv2item
func JSONProjectDraftIssue(item queries.DraftIssue) ([]byte, error) {

	return json.Marshal(draftIssueJSON{
		ID:    item.ID,
		Title: item.Title,
		Body:  item.Body,
		Type:  "DraftIssue",
	})
}

type draftIssueJSON struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
	Type  string `json:"type"`
}

func projectItemContent(p queries.ProjectItem) any {
	switch p.Content.TypeName {
	case "DraftIssue":
		return struct {
			Type  string `json:"type"`
			Body  string `json:"body"`
			Title string `json:"title"`
		}{
			Type:  p.Type(),
			Body:  p.Body(),
			Title: p.Title(),
		}
	case "Issue":
		return struct {
			Type       string `json:"type"`
			Body       string `json:"body"`
			Title      string `json:"title"`
			Number     int    `json:"number"`
			Repository string `json:"repository"`
			URL        string `json:"url"`
		}{
			Type:       p.Type(),
			Body:       p.Body(),
			Title:      p.Title(),
			Number:     p.Number(),
			Repository: p.Repo(),
			URL:        p.URL(),
		}
	case "PullRequest":
		return struct {
			Type       string `json:"type"`
			Body       string `json:"body"`
			Title      string `json:"title"`
			Number     int    `json:"number"`
			Repository string `json:"repository"`
			URL        string `json:"url"`
		}{
			Type:       p.Type(),
			Body:       p.Body(),
			Title:      p.Title(),
			Number:     p.Number(),
			Repository: p.Repo(),
			URL:        p.URL(),
		}
	}

	return nil
}

func projectFieldValueData(v queries.FieldValueNodes) any {
	switch v.Type {
	case "ProjectV2ItemFieldDateValue":
		return v.ProjectV2ItemFieldDateValue.Date
	case "ProjectV2ItemFieldIterationValue":
		return struct {
			Title     string `json:"title"`
			StartDate string `json:"startDate"`
			Duration  int    `json:"duration"`
		}{
			Title:     v.ProjectV2ItemFieldIterationValue.Title,
			StartDate: v.ProjectV2ItemFieldIterationValue.StartDate,
			Duration:  v.ProjectV2ItemFieldIterationValue.Duration,
		}
	case "ProjectV2ItemFieldNumberValue":
		return v.ProjectV2ItemFieldNumberValue.Number
	case "ProjectV2ItemFieldSingleSelectValue":
		return v.ProjectV2ItemFieldSingleSelectValue.Name
	case "ProjectV2ItemFieldTextValue":
		return v.ProjectV2ItemFieldTextValue.Text
	case "ProjectV2ItemFieldMilestoneValue":
		return struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			DueOn       string `json:"dueOn"`
		}{
			Title:       v.ProjectV2ItemFieldMilestoneValue.Milestone.Title,
			Description: v.ProjectV2ItemFieldMilestoneValue.Milestone.Description,
			DueOn:       v.ProjectV2ItemFieldMilestoneValue.Milestone.DueOn,
		}
	case "ProjectV2ItemFieldLabelValue":
		names := make([]string, 0)
		for _, p := range v.ProjectV2ItemFieldLabelValue.Labels.Nodes {
			names = append(names, p.Name)
		}
		return names
	case "ProjectV2ItemFieldPullRequestValue":
		urls := make([]string, 0)
		for _, p := range v.ProjectV2ItemFieldPullRequestValue.PullRequests.Nodes {
			urls = append(urls, p.Url)
		}
		return urls
	case "ProjectV2ItemFieldRepositoryValue":
		return v.ProjectV2ItemFieldRepositoryValue.Repository.Url
	case "ProjectV2ItemFieldUserValue":
		logins := make([]string, 0)
		for _, p := range v.ProjectV2ItemFieldUserValue.Users.Nodes {
			logins = append(logins, p.Login)
		}
		return logins
	case "ProjectV2ItemFieldReviewerValue":
		names := make([]string, 0)
		for _, p := range v.ProjectV2ItemFieldReviewerValue.Reviewers.Nodes {
			if p.Type == "Team" {
				names = append(names, p.Team.Name)
			} else if p.Type == "User" {
				names = append(names, p.User.Login)
			}
		}
		return names

	}

	return nil
}

// serialize creates a map from field to field values
func serializeProjectWithItems(project *queries.Project) []map[string]any {
	fields := make(map[string]string)

	// make a map of fields by ID
	for _, f := range project.Fields.Nodes {
		fields[f.ID()] = CamelCase(f.Name())
	}
	itemsSlice := make([]map[string]any, 0)

	// for each value, look up the name by ID
	// and set the value to the field value
	for _, i := range project.Items.Nodes {
		o := make(map[string]any)
		o["id"] = i.Id
		o["content"] = projectItemContent(i)
		for _, v := range i.FieldValues.Nodes {
			id := v.ID()
			value := projectFieldValueData(v)

			o[fields[id]] = value
		}
		itemsSlice = append(itemsSlice, o)
	}
	return itemsSlice
}

// JSONProjectWithItems returns a detailed JSON representation of project items.
// JSON fields are `totalCount` and `items`.
func JSONProjectDetailedItems(project *queries.Project) ([]byte, error) {
	items := serializeProjectWithItems(project)
	return json.Marshal(struct {
		Items      []map[string]any `json:"items"`
		TotalCount int              `json:"totalCount"`
	}{
		Items:      items,
		TotalCount: project.Items.TotalCount,
	})
}

// CamelCase converts a string to camelCase, which is useful for turning Go field names to JSON keys.
func CamelCase(s string) string {
	if len(s) == 0 {
		return ""
	}

	if len(s) == 1 {
		return strings.ToLower(s)
	}
	return strings.ToLower(s[0:1]) + s[1:]
}
