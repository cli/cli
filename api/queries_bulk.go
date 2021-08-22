package api

import (
	"fmt"
	"strings"

	"github.com/cli/cli/internal/ghrepo"
)

type BulkInput struct {
	Number     int
	Repository ghrepo.Interface

	Label        string
	RepositoryID string
}

type ResolveResult struct {
	ID    string
	Issue struct {
		ID string
	} `json:"issueOrPullRequest"`
	Label struct {
		ID string
	}
}

func ResolveNodeIDs(client *Client, inputs []BulkInput) ([]ResolveResult, error) {
	var queries []string
	for i, input := range inputs {
		if input.Number > 0 {
			queries = append(queries, fmt.Sprintf(`
				r%03d: repository(owner:%q, name:%q) {
					id
					issueOrPullRequest(number:%d) {
						...on Issue { id }
						...on PullRequest { id }
					}
				}
			`, i, input.Repository.RepoOwner(), input.Repository.RepoName(), input.Number))
		} else if input.Label != "" {
			queries = append(queries, fmt.Sprintf(`
				r%03d: node(id:%q) {
					...on Repository {
						id
						label(name:%q) { id }
					}
				}
			`, i, input.RepositoryID, input.Label))
		} else {
			queries = append(queries, fmt.Sprintf(`
				r%03d: repository(owner:%q, name:%q) { id }
			`, i, input.Repository.RepoOwner(), input.Repository.RepoName()))
		}
	}

	result := make(map[string]ResolveResult)
	var ids []ResolveResult

	err := client.GraphQL(fmt.Sprintf(`{%s}`, strings.Join(queries, "")), nil, &result)
	if err != nil {
		return ids, err
	}

	// NOTE: iteration is not ordered
	for _, v := range result {
		ids = append(ids, v)
	}

	return ids, nil
}

func BulkAddLabels(client *Client, inputs []BulkInput, labelNames []string) error {
	ids, err := ResolveNodeIDs(client, inputs)
	if err != nil {
		return err
	}

	var mutations []string
	cache := make(map[string][]ResolveResult)

	for i, id := range ids {
		labelIDs, ok := cache[id.ID]
		if !ok {
			var labelInputs []BulkInput
			for _, l := range labelNames {
				labelInputs = append(labelInputs, BulkInput{
					RepositoryID: id.ID,
					Label:        l,
				})
			}
			labelIDs, err = ResolveNodeIDs(client, labelInputs)
			if err != nil {
				return err
			}
			cache[id.ID] = labelIDs
		}

		var labelIDsSerialized []string
		for _, l := range labelIDs {
			labelIDsSerialized = append(labelIDsSerialized, fmt.Sprintf("%q", l.Label.ID))
		}

		mutations = append(mutations, fmt.Sprintf(`
			m%03d: addLabelsToLabelable(input: {
				labelableId: %q
				labelIds: [%s]
			}) { clientMutationId }
		`, i, id.Issue.ID, strings.Join(labelIDsSerialized, ",")))
	}

	err = client.GraphQL(fmt.Sprintf(`mutation{%s}`, strings.Join(mutations, "")), nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func BulkChangeState(client *Client, inputs []BulkInput, state string) error {
	ids, err := ResolveNodeIDs(client, inputs)
	if err != nil {
		return err
	}

	var mutations []string
	var newState string

	switch state {
	case "open":
		newState = "OPEN"
	case "close":
		newState = "CLOSED"
	default:
		return fmt.Errorf("unrecognized state: %q", state)
	}

	for i, id := range ids {
		mutations = append(mutations, fmt.Sprintf(`
			m%03d: updateIssue(input: {
				id: %q
				state: %s
			}) { clientMutationId }
		`, i, id.Issue.ID, newState))
	}

	err = client.GraphQL(fmt.Sprintf(`mutation{%s}`, strings.Join(mutations, "")), nil, nil)
	if err != nil {
		return err
	}

	return nil
}
