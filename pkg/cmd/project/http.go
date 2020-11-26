package project

import (
	"encoding/json"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
)

func getProject(client *api.Client, baseRepo ghrepo.Interface, projectID int) (*Project, error) {
	data, err := client.GetProject(baseRepo, projectID)
	if err != nil {
		return nil, err
	}

	project := &Project{}

	err = json.Unmarshal(data, project)
	if err != nil {
		return nil, err
	}

	data, err = client.GetProjectColumns(baseRepo, projectID)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, &project.Columns)
	if err != nil {
		return nil, err
	}

	for _, column := range project.Columns {
		data, err := client.GetProjectCards(baseRepo, column.ID)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal(data, &column.Cards)
		if err != nil {
			return nil, err
		}

		for _, card := range column.Cards {
			// hydrate with any linked content
			if card.Note == "" {
				data, err := client.GetProjectCardContent(card.ContentURL)
				if err != nil {
					return nil, err
				}

				err = json.Unmarshal(data, &card.Content)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return project, nil
}
