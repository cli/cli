package format

import (
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
)

// JSONProject serializes a Project to JSON.
func JSONProject(project queries.Project) map[string]interface{} {
	return project.ExportData(nil)
}

// JSONProjects serializes a slice of Projects to JSON.
// JSON fields are `totalCount` and `projects`.
func JSONProjects(projects []queries.Project, totalCount int) map[string]interface{} {
	return queries.Projects{
		Nodes:      projects,
		TotalCount: totalCount,
	}.ExportData(nil)
}

// JSONProjectField serializes a ProjectField to JSON.
func JSONProjectField(field queries.ProjectField) map[string]interface{} {
	return field.ExportData(nil)
}

// JSONProjectFields serializes a slice of ProjectFields to JSON.
// JSON fields are `totalCount` and `fields`.
func JSONProjectFields(project *queries.Project) map[string]interface{} {
	return project.Fields.ExportData(nil)
}

// JSONProjectItem serializes a ProjectItem to JSON.
func JSONProjectItem(item queries.ProjectItem) map[string]interface{} {
	return item.ExportData(nil)
}

// JSONProjectDraftIssue serializes a DraftIssue to JSON.
// This is needed because the field for
// https://docs.github.com/en/graphql/reference/mutations#updateprojectv2draftissue
// is a DraftIssue https://docs.github.com/en/graphql/reference/objects#draftissue
// and not a ProjectV2Item https://docs.github.com/en/graphql/reference/objects#projectv2item
func JSONProjectDraftIssue(item queries.DraftIssue) map[string]interface{} {
	return item.ExportData(nil)
}

// JSONProjectWithItems returns a detailed JSON representation of project items.
// JSON fields are `totalCount` and `items`.
func JSONProjectDetailedItems(project *queries.Project) map[string]interface{} {
	return project.DetailedItems()
}
