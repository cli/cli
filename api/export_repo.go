package api

import (
	"reflect"
)

func (repo *Repository) ExportData(fields []string) map[string]interface{} {
	v := reflect.ValueOf(repo).Elem()
	data := map[string]interface{}{}

	for _, f := range fields {
		switch f {
		case "parent":
			data[f] = miniRepoExport(repo.Parent)
		case "templateRepository":
			data[f] = miniRepoExport(repo.TemplateRepository)
		case "languages":
			data[f] = repo.Languages.Edges
		case "labels":
			data[f] = repo.Labels.Nodes
		case "assignableUsers":
			data[f] = repo.AssignableUsers.Nodes
		case "mentionableUsers":
			data[f] = repo.MentionableUsers.Nodes
		case "milestones":
			data[f] = repo.Milestones.Nodes
		case "projects":
			data[f] = repo.Projects.Nodes
		case "repositoryTopics":
			var topics []RepositoryTopic
			for _, n := range repo.RepositoryTopics.Nodes {
				topics = append(topics, n.Topic)
			}
			data[f] = topics
		default:
			sf := fieldByName(v, f)
			data[f] = sf.Interface()
		}
	}

	return data
}

func miniRepoExport(r *Repository) map[string]interface{} {
	if r == nil {
		return nil
	}
	return map[string]interface{}{
		"id":    r.ID,
		"name":  r.Name,
		"owner": r.Owner,
	}
}
