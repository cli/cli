package api

import "fmt"

type BranchIssueReference struct {
	ID                 int
	BranchName         string
	BranchRepositoryId int
	IssueId            int
	IssueRepositoryId  int
}

func CreateBranchIssueReference(client *Client, repo *Repository, params map[string]interface{}) (*BranchIssueReference, error) {
	query := `
		mutation CreateLinkedBranch($issueId: ID!, $oid: GitObjectID!, $name: String, $repositoryId: ID) {
			createLinkedBranch(input: {
			  issueId: $issueId,
			  name: $name,
			  oid: $oid,
			  repositoryId: $repositoryId
			}) {
				linkedBranch {
					id
				}
			}
		}
	`

	inputParams := map[string]interface{}{
		"repositoryId": repo.ID,
	}
	for key, val := range params {
		switch key {
		case "issueId", "name", "oid":
			inputParams[key] = val
		}
	}

	result := struct {
		createLinkedBranch struct {
			BranchIssueReference BranchIssueReference
		}
	}{}

	fmt.Printf("query variables %#v", inputParams)
	err := client.GraphQL(repo.RepoHost(), query, inputParams, &result)
	if err != nil {
		return nil, err
	}
	ref := &result.createLinkedBranch.BranchIssueReference
	return ref, nil

}

func FindBaseOid(client *Client, repo *Repository, ref string) (string, string, error) {
	query := `
	query BranchIssueReferenceFindBaseOid($repositoryName: String!, $repositoryOwner: String!, $ref: String!) {
		repository(name: $repositoryName, owner: $repositoryOwner) {
			defaultBranchRef {
				target {
					oid
				}
			}
			ref(qualifiedName: $ref) {
				target {
					oid
				}
			}
		}
	}`

	variables := map[string]interface{}{
		"repositoryName":  repo.Name,
		"repositoryOwner": repo.RepoOwner(),
		"ref":             ref,
	}

	result := struct {
		Repository struct {
			DefaultBranchRef struct {
				Target struct {
					Oid string
				}
			}
			Ref struct {
				Target struct {
					Oid string
				}
			}
		}
	}{}

	err := client.GraphQL(repo.RepoHost(), query, variables, &result)
	if err != nil {
		return "", "", err
	}
	return result.Repository.Ref.Target.Oid, result.Repository.DefaultBranchRef.Target.Oid, nil
}
