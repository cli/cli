package queries

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Regression test from before ExportData was implemented.
func TestJSONProject_User(t *testing.T) {
	project := Project{
		ID:               "123",
		Number:           2,
		URL:              "a url",
		ShortDescription: "short description",
		Public:           true,
		Readme:           "readme",
	}

	project.Items.TotalCount = 1
	project.Fields.TotalCount = 2
	project.Owner.TypeName = "User"
	project.Owner.User.Login = "monalisa"
	b, err := json.Marshal(project.ExportData(nil))
	assert.NoError(t, err)

	assert.JSONEq(t, `{"number":2,"url":"a url","shortDescription":"short description","public":true,"closed":false,"title":"","id":"123","readme":"readme","items":{"totalCount":1},"fields":{"totalCount":2},"owner":{"type":"User","login":"monalisa"}}`, string(b))
}

// Regression test from before ExportData was implemented.
func TestJSONProject_Org(t *testing.T) {
	project := Project{
		ID:               "123",
		Number:           2,
		URL:              "a url",
		ShortDescription: "short description",
		Public:           true,
		Readme:           "readme",
	}

	project.Items.TotalCount = 1
	project.Fields.TotalCount = 2
	project.Owner.TypeName = "Organization"
	project.Owner.Organization.Login = "github"
	b, err := json.Marshal(project.ExportData(nil))
	assert.NoError(t, err)

	assert.JSONEq(t, `{"number":2,"url":"a url","shortDescription":"short description","public":true,"closed":false,"title":"","id":"123","readme":"readme","items":{"totalCount":1},"fields":{"totalCount":2},"owner":{"type":"Organization","login":"github"}}`, string(b))
}

// Regression test from before ExportData was implemented.
func TestJSONProjects(t *testing.T) {
	userProject := Project{
		ID:               "123",
		Number:           2,
		URL:              "a url",
		ShortDescription: "short description",
		Public:           true,
		Readme:           "readme",
	}

	userProject.Items.TotalCount = 1
	userProject.Fields.TotalCount = 2
	userProject.Owner.TypeName = "User"
	userProject.Owner.User.Login = "monalisa"

	orgProject := Project{
		ID:               "123",
		Number:           2,
		URL:              "a url",
		ShortDescription: "short description",
		Public:           true,
		Readme:           "readme",
	}

	orgProject.Items.TotalCount = 1
	orgProject.Fields.TotalCount = 2
	orgProject.Owner.TypeName = "Organization"
	orgProject.Owner.Organization.Login = "github"

	projects := Projects{
		Nodes:      []Project{userProject, orgProject},
		TotalCount: 2,
	}
	b, err := json.Marshal(projects.ExportData(nil))
	assert.NoError(t, err)

	assert.JSONEq(
		t,
		`{"projects":[{"number":2,"url":"a url","shortDescription":"short description","public":true,"closed":false,"title":"","id":"123","readme":"readme","items":{"totalCount":1},"fields":{"totalCount":2},"owner":{"type":"User","login":"monalisa"}},{"number":2,"url":"a url","shortDescription":"short description","public":true,"closed":false,"title":"","id":"123","readme":"readme","items":{"totalCount":1},"fields":{"totalCount":2},"owner":{"type":"Organization","login":"github"}}],"totalCount":2}`,
		string(b))
}

func TestJSONProjectField_FieldType(t *testing.T) {
	field := ProjectField{}
	field.TypeName = "ProjectV2Field"
	field.Field.ID = "123"
	field.Field.Name = "name"

	b, err := json.Marshal(field.ExportData(nil))
	assert.NoError(t, err)

	assert.Equal(t, `{"id":"123","name":"name","type":"ProjectV2Field"}`, string(b))
}

func TestJSONProjectField_SingleSelectType(t *testing.T) {
	field := ProjectField{}
	field.TypeName = "ProjectV2SingleSelectField"
	field.SingleSelectField.ID = "123"
	field.SingleSelectField.Name = "name"
	field.SingleSelectField.Options = []SingleSelectFieldOptions{
		{
			ID:   "123",
			Name: "name",
		},
		{
			ID:   "456",
			Name: "name2",
		},
	}

	b, err := json.Marshal(field.ExportData(nil))
	assert.NoError(t, err)

	assert.JSONEq(t, `{"id":"123","name":"name","type":"ProjectV2SingleSelectField","options":[{"id":"123","name":"name"},{"id":"456","name":"name2"}]}`, string(b))
}

func TestJSONProjectField_ProjectV2IterationField(t *testing.T) {
	field := ProjectField{}
	field.TypeName = "ProjectV2IterationField"
	field.IterationField.ID = "123"
	field.IterationField.Name = "name"

	b, err := json.Marshal(field.ExportData(nil))
	assert.NoError(t, err)

	assert.Equal(t, `{"id":"123","name":"name","type":"ProjectV2IterationField"}`, string(b))
}

func TestJSONProjectFields(t *testing.T) {
	field := ProjectField{}
	field.TypeName = "ProjectV2Field"
	field.Field.ID = "123"
	field.Field.Name = "name"

	field2 := ProjectField{}
	field2.TypeName = "ProjectV2SingleSelectField"
	field2.SingleSelectField.ID = "123"
	field2.SingleSelectField.Name = "name"
	field2.SingleSelectField.Options = []SingleSelectFieldOptions{
		{
			ID:   "123",
			Name: "name",
		},
		{
			ID:   "456",
			Name: "name2",
		},
	}

	p := &Project{
		Fields: struct {
			TotalCount int
			Nodes      []ProjectField
			PageInfo   PageInfo
		}{
			Nodes:      []ProjectField{field, field2},
			TotalCount: 5,
		},
	}
	b, err := json.Marshal(p.Fields.ExportData(nil))
	assert.NoError(t, err)

	assert.JSONEq(t, `{"fields":[{"id":"123","name":"name","type":"ProjectV2Field"},{"id":"123","name":"name","type":"ProjectV2SingleSelectField","options":[{"id":"123","name":"name"},{"id":"456","name":"name2"}]}],"totalCount":5}`, string(b))
}

func TestJSONProjectItem_DraftIssue(t *testing.T) {
	item := ProjectItem{}
	item.Content.TypeName = "DraftIssue"
	item.Id = "123"
	item.Content.DraftIssue.Title = "title"
	item.Content.DraftIssue.Body = "a body"

	b, err := json.Marshal(item.ExportData(nil))
	assert.NoError(t, err)

	assert.JSONEq(t, `{"id":"123","title":"title","body":"a body","type":"DraftIssue"}`, string(b))
}

func TestJSONProjectItem_Issue(t *testing.T) {
	item := ProjectItem{}
	item.Content.TypeName = "Issue"
	item.Id = "123"
	item.Content.Issue.Title = "title"
	item.Content.Issue.Body = "a body"
	item.Content.Issue.URL = "a-url"

	b, err := json.Marshal(item.ExportData(nil))
	assert.NoError(t, err)

	assert.JSONEq(t, `{"id":"123","title":"title","body":"a body","type":"Issue","url":"a-url"}`, string(b))
}

func TestJSONProjectItem_PullRequest(t *testing.T) {
	item := ProjectItem{}
	item.Content.TypeName = "PullRequest"
	item.Id = "123"
	item.Content.PullRequest.Title = "title"
	item.Content.PullRequest.Body = "a body"
	item.Content.PullRequest.URL = "a-url"

	b, err := json.Marshal(item.ExportData(nil))
	assert.NoError(t, err)

	assert.JSONEq(t, `{"id":"123","title":"title","body":"a body","type":"PullRequest","url":"a-url"}`, string(b))
}

func TestJSONProjectDetailedItems(t *testing.T) {
	p := &Project{}
	p.Items.TotalCount = 5
	p.Items.Nodes = []ProjectItem{
		{
			Id: "issueId",
			Content: ProjectItemContent{
				TypeName: "Issue",
				Issue: Issue{
					Title:  "Issue title",
					Body:   "a body",
					Number: 1,
					URL:    "issue-url",
					Repository: struct {
						NameWithOwner string
					}{
						NameWithOwner: "cli/go-gh",
					},
				},
			},
		},
		{
			Id: "pullRequestId",
			Content: ProjectItemContent{
				TypeName: "PullRequest",
				PullRequest: PullRequest{
					Title:  "Pull Request title",
					Body:   "a body",
					Number: 2,
					URL:    "pr-url",
					Repository: struct {
						NameWithOwner string
					}{
						NameWithOwner: "cli/go-gh",
					},
				},
			},
		},
		{
			Id: "draftIssueId",
			Content: ProjectItemContent{
				TypeName: "DraftIssue",
				DraftIssue: DraftIssue{
					ID:    "draftIssueId",
					Title: "Pull Request title",
					Body:  "a body",
				},
			},
		},
	}

	out, err := json.Marshal(p.DetailedItems())
	assert.NoError(t, err)
	assert.JSONEq(
		t,
		`{"items":[{"content":{"type":"Issue","body":"a body","title":"Issue title","number":1,"repository":"cli/go-gh","url":"issue-url"},"id":"issueId"},{"content":{"type":"PullRequest","body":"a body","title":"Pull Request title","number":2,"repository":"cli/go-gh","url":"pr-url"},"id":"pullRequestId"},{"content":{"type":"DraftIssue","body":"a body","title":"Pull Request title","id":"draftIssueId"},"id":"draftIssueId"}],"totalCount":5}`,
		string(out))
}

func TestJSONProjectDraftIssue(t *testing.T) {
	item := DraftIssue{}
	item.ID = "123"
	item.Title = "title"
	item.Body = "a body"

	b, err := json.Marshal(item.ExportData(nil))
	assert.NoError(t, err)

	assert.JSONEq(t, `{"id":"123","title":"title","body":"a body","type":"DraftIssue"}`, string(b))
}

func TestJSONProjectItem_DraftIssue_ProjectV2ItemFieldIterationValue(t *testing.T) {
	iterationField := ProjectField{TypeName: "ProjectV2IterationField"}
	iterationField.IterationField.ID = "sprint"
	iterationField.IterationField.Name = "Sprint"

	iterationFieldValue := FieldValueNodes{Type: "ProjectV2ItemFieldIterationValue"}
	iterationFieldValue.ProjectV2ItemFieldIterationValue.Title = "Iteration Title"
	iterationFieldValue.ProjectV2ItemFieldIterationValue.Field = iterationField

	draftIssue := ProjectItem{
		Id: "draftIssueId",
		Content: ProjectItemContent{
			TypeName: "DraftIssue",
			DraftIssue: DraftIssue{
				ID:    "draftIssueId",
				Title: "Pull Request title",
				Body:  "a body",
			},
		},
	}
	draftIssue.FieldValues.Nodes = []FieldValueNodes{
		iterationFieldValue,
	}
	p := &Project{}
	p.Fields.Nodes = []ProjectField{iterationField}
	p.Items.TotalCount = 5
	p.Items.Nodes = []ProjectItem{
		draftIssue,
	}

	out, err := json.Marshal(p.DetailedItems())
	assert.NoError(t, err)
	assert.JSONEq(
		t,
		`{"items":[{"sprint":{"title":"Iteration Title","startDate":"","duration":0},"content":{"type":"DraftIssue","body":"a body","title":"Pull Request title","id":"draftIssueId"},"id":"draftIssueId"}],"totalCount":5}`,
		string(out))

}

func TestJSONProjectItem_DraftIssue_ProjectV2ItemFieldMilestoneValue(t *testing.T) {
	milestoneField := ProjectField{TypeName: "ProjectV2IterationField"}
	milestoneField.IterationField.ID = "milestone"
	milestoneField.IterationField.Name = "Milestone"

	milestoneFieldValue := FieldValueNodes{Type: "ProjectV2ItemFieldMilestoneValue"}
	milestoneFieldValue.ProjectV2ItemFieldMilestoneValue.Milestone.Title = "Milestone Title"
	milestoneFieldValue.ProjectV2ItemFieldMilestoneValue.Field = milestoneField

	draftIssue := ProjectItem{
		Id: "draftIssueId",
		Content: ProjectItemContent{
			TypeName: "DraftIssue",
			DraftIssue: DraftIssue{
				ID:    "draftIssueId",
				Title: "Pull Request title",
				Body:  "a body",
			},
		},
	}
	draftIssue.FieldValues.Nodes = []FieldValueNodes{
		milestoneFieldValue,
	}
	p := &Project{}
	p.Fields.Nodes = []ProjectField{milestoneField}
	p.Items.TotalCount = 5
	p.Items.Nodes = []ProjectItem{
		draftIssue,
	}

	out, err := json.Marshal(p.DetailedItems())
	assert.NoError(t, err)
	assert.JSONEq(
		t,
		`{"items":[{"milestone":{"title":"Milestone Title","dueOn":"","description":""},"content":{"type":"DraftIssue","body":"a body","title":"Pull Request title","id":"draftIssueId"},"id":"draftIssueId"}],"totalCount":5}`,
		string(out))

}
