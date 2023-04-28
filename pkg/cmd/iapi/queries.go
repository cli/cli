package iapi

import (
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
)

type queryTypes struct {
	Schema struct {
		QueryType struct {
			Fields []struct {
				Name string
				Type struct {
					Name   string
					OfType struct {
						Name string
					}
				}
				Args []struct {
					Name string
				}
			}
		}
	} `json:"__schema"`
}

func queryTypesQuery() string {
	return heredoc.Doc(`
		query {
			__schema {
				queryType {
					fields {
						name
						type {
							name
							ofType {
								name
							}
						}
						args {
							name
						}
					}
				}
			}
		}
	`)
}

type types struct {
	Type struct {
		Name   string
		Fields []struct {
			Name string
			Type struct {
				Name   string
				OfType struct {
					Name string
				}
			}
			Args []struct {
				Name string
			}
		}
	} `json:"__type"`
}

func typeQuery(s string) string {
	return heredoc.Docf(`
		query {
			__type(name: "%s") {
				name
				fields {
					name
					type {
						name
						ofType {
							name
						}
					}
					args {
						name
					}
				}
			}
		}
	`, strings.Title(s))
}

type fieldResponse struct {
	Kind string
	Args bool
}

func retrieveFieldOpts(client *api.Client, kind string) (map[string]fieldResponse, error) {
	var response types
	err := client.GraphQL("github.com", typeQuery(kind), nil, &response)
	if err != nil {
		return nil, err
	}
	m := map[string]fieldResponse{}
	for _, v := range response.Type.Fields {
		args := len(v.Args) > 0
		if v.Type.Name != "" {
			m[v.Name] = fieldResponse{Kind: v.Type.Name, Args: args}
			continue
		}
		if v.Type.OfType.Name != "" {
			m[v.Name] = fieldResponse{Kind: v.Type.OfType.Name, Args: args}
			continue
		}
		m[v.Name] = fieldResponse{Kind: v.Name, Args: args}
	}
	return m, nil
}

func retrieveArgumentOpts(client *api.Client, kind string, field string) ([]string, error) {
	var response types
	err := client.GraphQL("github.com", typeQuery(kind), nil, &response)
	if err != nil {
		return nil, err
	}
	var o []string
	for _, v := range response.Type.Fields {
		if v.Name == field {
			for _, a := range v.Args {
				o = append(o, a.Name)
			}
		}
	}
	return o, nil
}

func retrieveFieldsAvailable(client *api.Client, kind string) (bool, error) {
	var response types
	err := client.GraphQL("github.com", typeQuery(kind), nil, &response)
	if err != nil {
		return false, err
	}
	fa := len(response.Type.Fields) > 0
	return fa, nil
}
