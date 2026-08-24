package close

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/h2non/gock.v1"
)

func TestNewCmdClose(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		wants         closeOpts
		wantsErr      bool
		wantsErrMsg   string
		wantsExporter bool
	}{
		{
			name:        "not-a-number",
			cli:         "x",
			wantsErr:    true,
			wantsErrMsg: "invalid number: x",
		},
		{
			name: "number",
			cli:  "123",
			wants: closeOpts{
				number: 123,
			},
		},
		{
			name: "owner",
			cli:  "--owner monalisa",
			wants: closeOpts{
				owner: "monalisa",
			},
		},
		{
			name: "reopen",
			cli:  "--undo",
			wants: closeOpts{
				reopen: true,
			},
		},
		{
			name:          "json",
			cli:           "--format json",
			wantsExporter: true,
		},
	}

	t.Setenv("GH_TOKEN", "auth-token")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts closeOpts
			cmd := NewCmdClose(f, func(config closeConfig) error {
				gotOpts = config.opts
				return nil
			})

			cmd.SetArgs(argv)
			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				assert.Equal(t, tt.wantsErrMsg, err.Error())
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.number, gotOpts.number)
			assert.Equal(t, tt.wants.owner, gotOpts.owner)
			assert.Equal(t, tt.wantsExporter, gotOpts.exporter != nil)
		})
	}
}

func TestRunClose_User(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]any{
			"query": "query UserOrgOwner.*",
			"variables": map[string]any{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"id": "an ID",
				},
			},
			"errors": []any{
				map[string]any{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	// get user project ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]any{
			"query": "query UserProject.*",
			"variables": map[string]any{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  0,
				"afterItems":  nil,
				"firstFields": 0,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]string{
						"id": "an ID",
					},
				},
			},
		})

	// close project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CloseProjectV2.*"variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"projectId":"an ID","closed":true}}}`).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"updateProjectV2": map[string]any{
					"projectV2": map[string]any{
						"title": "a title",
						"url":   "http://a-url.com",
						"owner": map[string]string{
							"login": "monalisa",
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)

	config := closeConfig{
		io: ios,
		opts: closeOpts{
			number: 1,
			owner:  "monalisa",
		},
		client: client,
	}

	err := runClose(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"http://a-url.com\n",
		stdout.String())
}

func TestRunClose_Org(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

	// get org ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]any{
			"query": "query UserOrgOwner.*",
			"variables": map[string]any{
				"login": "github",
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"organization": map[string]any{
					"id": "an ID",
				},
			},
			"errors": []any{
				map[string]any{
					"type": "NOT_FOUND",
					"path": []string{"user"},
				},
			},
		})

	// get org project ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]any{
			"query": "query OrgProject.*",
			"variables": map[string]any{
				"login":       "github",
				"number":      1,
				"firstItems":  0,
				"afterItems":  nil,
				"firstFields": 0,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"organization": map[string]any{
					"projectV2": map[string]string{
						"id": "an ID",
					},
				},
			},
		})

	// close project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CloseProjectV2.*"variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"projectId":"an ID","closed":true}}}`).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"updateProjectV2": map[string]any{
					"projectV2": map[string]any{
						"title": "a title",
						"url":   "http://a-url.com",
						"owner": map[string]string{
							"login": "monalisa",
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := closeConfig{
		io: ios,
		opts: closeOpts{
			number: 1,
			owner:  "github",
		},
		client: client,
	}

	err := runClose(config)
	assert.NoError(t, err)
	assert.Equal(t, "", stdout.String())
}

func TestRunClose_Me(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

	// get viewer ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]any{
			"query": "query ViewerOwner.*",
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"viewer": map[string]any{
					"id": "an ID",
				},
			},
		})

	// get viewer project ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]any{
			"query": "query ViewerProject.*",
			"variables": map[string]any{
				"number":      1,
				"firstItems":  0,
				"afterItems":  nil,
				"firstFields": 0,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"viewer": map[string]any{
					"projectV2": map[string]string{
						"id": "an ID",
					},
				},
			},
		})

	// close project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CloseProjectV2.*"variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"projectId":"an ID","closed":true}}}`).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"updateProjectV2": map[string]any{
					"projectV2": map[string]any{
						"title": "a title",
						"url":   "http://a-url.com",
						"owner": map[string]string{
							"login": "me",
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := closeConfig{
		io: ios,
		opts: closeOpts{
			number: 1,
			owner:  "@me",
		},
		client: client,
	}

	err := runClose(config)
	assert.NoError(t, err)
	assert.Equal(t, "", stdout.String())
}

func TestRunClose_Reopen(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]any{
			"query": "query UserOrgOwner.*",
			"variables": map[string]any{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"id": "an ID",
				},
			},
			"errors": []any{
				map[string]any{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	// get user project ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]any{
			"query": "query UserProject.*",
			"variables": map[string]any{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  0,
				"afterItems":  nil,
				"firstFields": 0,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]string{
						"id": "an ID",
					},
				},
			},
		})

	// close project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CloseProjectV2.*"variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"projectId":"an ID","closed":false}}}`).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"updateProjectV2": map[string]any{
					"projectV2": map[string]any{
						"title": "a title",
						"url":   "http://a-url.com",
						"owner": map[string]string{
							"login": "monalisa",
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)

	config := closeConfig{
		io: ios,
		opts: closeOpts{
			number: 1,
			owner:  "monalisa",
			reopen: true,
		},
		client: client,
	}

	err := runClose(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"http://a-url.com\n",
		stdout.String())
}

func TestRunClose_JSON(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]any{
			"query": "query UserOrgOwner.*",
			"variables": map[string]any{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"id": "an ID",
				},
			},
			"errors": []any{
				map[string]any{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	// get user project ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]any{
			"query": "query UserProject.*",
			"variables": map[string]any{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  0,
				"afterItems":  nil,
				"firstFields": 0,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]string{
						"id": "an ID",
					},
				},
			},
		})

	// close project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CloseProjectV2.*"variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"projectId":"an ID","closed":true}}}`).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"updateProjectV2": map[string]any{
					"projectV2": map[string]any{
						"number": 1,
						"title":  "a title",
						"url":    "http://a-url.com",
						"owner": map[string]any{
							"__typename": "User",
							"login":      "monalisa",
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := closeConfig{
		io: ios,
		opts: closeOpts{
			number:   1,
			owner:    "monalisa",
			exporter: cmdutil.NewJSONExporter(),
		},
		client: client,
	}

	err := runClose(config)
	assert.NoError(t, err)
	assert.JSONEq(
		t,
		`{"number":1,"url":"http://a-url.com","shortDescription":"","public":false,"closed":false,"title":"a title","id":"","readme":"","items":{"totalCount":0},"fields":{"totalCount":0},"owner":{"type":"User","login":"monalisa"}}`,
		stdout.String())
}

func TestRunClose_InteractiveProjectFilter(t *testing.T) {
	tests := []struct {
		name       string
		reopen     bool
		wantOption string
		wantID     string
		wantClosed bool
		wantURL    string
	}{
		{
			name:       "close offers open projects",
			wantOption: "Open Project (#1)",
			wantID:     "open-project-ID",
			wantClosed: true,
			wantURL:    "http://open-url.com",
		},
		{
			name:       "undo offers closed projects",
			reopen:     true,
			wantOption: "Closed Project (#2)",
			wantID:     "closed-project-ID",
			wantURL:    "http://closed-url.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer gock.Off()
			registerInteractiveCloseResponses(tt.wantID, tt.wantClosed, tt.wantURL)

			pm := &prompter.PrompterMock{}
			pm.SelectFunc = func(prompt, _ string, options []string) (int, error) {
				switch prompt {
				case "Which owner would you like to use?":
					return prompter.IndexFor(options, "monalisa")
				case "Which project would you like to use?":
					assert.Equal(t, []string{tt.wantOption}, options)
					return 0, nil
				default:
					return -1, prompter.NoSuchPromptErr(prompt)
				}
			}

			client := queries.NewTestClient(queries.WithPrompter(pm))
			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)

			err := runClose(closeConfig{
				io:     ios,
				opts:   closeOpts{reopen: tt.reopen},
				client: client,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantURL+"\n", stdout.String())
		})
	}
}

func registerInteractiveCloseResponses(projectID string, closed bool, projectURL string) {
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query":     "query ViewerLoginAndOrgs.*",
			"variables": map[string]interface{}{"after": nil},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]interface{}{
					"id": "viewer-ID", "login": "monalisa",
					"organizations": map[string]interface{}{"nodes": []interface{}{}},
				},
			},
		})

	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query ViewerProjects.*",
			"variables": map[string]interface{}{
				"after": nil, "afterFields": nil, "afterItems": nil,
				"first": 30, "firstFields": 0, "firstItems": 0,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]interface{}{
					"login": "monalisa",
					"projectsV2": map[string]interface{}{
						"totalCount": 2,
						"nodes": []map[string]interface{}{
							{"id": "open-project-ID", "number": 1, "title": "Open Project", "closed": false},
							{"id": "closed-project-ID", "number": 2, "title": "Closed Project", "closed": true},
						},
					},
				},
			},
		})

	gock.New("https://api.github.com").
		Post("/graphql").
		AddMatcher(matchProjectStateMutation(projectID, closed)).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"updateProjectV2": map[string]interface{}{
					"projectV2": map[string]interface{}{"url": projectURL},
				},
			},
		})
}

func TestMatchProjectStateMutation_MissingClosed(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "/graphql", strings.NewReader(`{"query":"mutation CloseProjectV2","variables":{"input":{"projectId":"project-ID"}}}`))
	require.NoError(t, err)

	matched, err := matchProjectStateMutation("project-ID", false)(req, nil)

	assert.False(t, matched)
	assert.EqualError(t, err, "variables.input.closed is missing")
}

func matchProjectStateMutation(projectID string, closed bool) gock.MatchFunc {
	return func(req *http.Request, _ *gock.Request) (bool, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return false, fmt.Errorf("read project state mutation request: %w", err)
		}
		var payload struct {
			Query     string `json:"query"`
			Variables struct {
				Input struct {
					ProjectID string `json:"projectId"`
					Closed    *bool  `json:"closed"`
				} `json:"input"`
			} `json:"variables"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return false, fmt.Errorf("decode project state mutation request: %w", err)
		}
		if payload.Variables.Input.Closed == nil {
			return false, errors.New("variables.input.closed is missing")
		}
		return strings.Contains(payload.Query, "mutation CloseProjectV2") &&
			payload.Variables.Input.ProjectID == projectID &&
			*payload.Variables.Input.Closed == closed, nil
	}
}
