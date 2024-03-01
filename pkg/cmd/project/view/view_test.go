package view

import (
	"bytes"
	"testing"

	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestNewCmdview(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		wants         viewOpts
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
			wants: viewOpts{
				number: 123,
			},
		},
		{
			name: "owner",
			cli:  "--owner monalisa",
			wants: viewOpts{
				owner: "monalisa",
			},
		},
		{
			name: "web",
			cli:  "--web",
			wants: viewOpts{
				web: true,
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

			var gotOpts viewOpts
			cmd := NewCmdView(f, func(config viewConfig) error {
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
			assert.Equal(t, tt.wants.web, gotOpts.web)
		})
	}
}

func TestRunView_User(t *testing.T) {
	defer gock.Off()

	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgOwner.*",
			"variables": map[string]interface{}{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"id": "an ID",
				},
			},
			"errors": []interface{}{
				map[string]interface{}{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	gock.New("https://api.github.com").
		Post("/graphql").
		Reply(200).
		JSON(`
			{"data":
				{"user":
					{
						"login":"monalisa",
						"projectV2": {
							"number": 1,
							"items": {
								"totalCount": 10
							},
							"readme": null,
							"fields": {
								"nodes": [
									{
										"name": "Title"
									}
								]
							}
						}
					}
				}
			}
		`)

	client := queries.NewTestClient()

	ios, _, _, _ := iostreams.Test()
	config := viewConfig{
		opts: viewOpts{
			owner:  "monalisa",
			number: 1,
		},
		io:     ios,
		client: client,
	}

	err := runView(config)
	assert.NoError(t, err)

}

func TestRunView_Viewer(t *testing.T) {
	defer gock.Off()

	// get viewer ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query ViewerOwner.*",
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]interface{}{
					"id": "an ID",
				},
			},
		})

	gock.New("https://api.github.com").
		Post("/graphql").
		Reply(200).
		JSON(`
			{"data":
				{"viewer":
					{
						"login":"monalisa",
						"projectV2": {
							"number": 1,
							"items": {
								"totalCount": 10
							},
							"url":"https://github.com/orgs/github/projects/8",
							"readme": null,
							"fields": {
								"nodes": [
									{
										"name": "Title"
									}
								]
							}
						}
					}
				}
			}
		`)

	client := queries.NewTestClient()

	ios, _, _, _ := iostreams.Test()
	config := viewConfig{
		opts: viewOpts{
			owner:  "@me",
			number: 1,
		},
		io:     ios,
		client: client,
	}

	err := runView(config)
	assert.NoError(t, err)
}

func TestRunView_Org(t *testing.T) {
	defer gock.Off()

	// get org ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgOwner.*",
			"variables": map[string]interface{}{
				"login": "github",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"organization": map[string]interface{}{
					"id": "an ID",
				},
			},
			"errors": []interface{}{
				map[string]interface{}{
					"type": "NOT_FOUND",
					"path": []string{"user"},
				},
			},
		})

	gock.New("https://api.github.com").
		Post("/graphql").
		Reply(200).
		JSON(`
			{"data":
				{"organization":
					{
						"login":"monalisa",
						"projectV2": {
							"number": 1,
							"items": {
								"totalCount": 10
							},
							"url":"https://github.com/orgs/github/projects/8",
							"readme": null,
							"fields": {
								"nodes": [
									{
										"name": "Title"
									}
								]
							}
						}
					}
				}
			}
		`)

	client := queries.NewTestClient()

	ios, _, _, _ := iostreams.Test()
	config := viewConfig{
		opts: viewOpts{
			owner:  "github",
			number: 1,
		},
		io:     ios,
		client: client,
	}

	err := runView(config)
	assert.NoError(t, err)
}

func TestRunViewWeb_User(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgOwner.*",
			"variables": map[string]interface{}{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"id": "an ID",
				},
			},
			"errors": []interface{}{
				map[string]interface{}{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	gock.New("https://api.github.com").
		Post("/graphql").
		Reply(200).
		JSON(`
		{"data":
			{"user":
				{
					"login":"monalisa",
					"projectV2": {
						"number": 8,
						"items": {
							"totalCount": 10
						},
						"url":"https://github.com/users/monalisa/projects/8",
						"readme": null,
						"fields": {
							"nodes": [
								{
									"name": "Title"
								}
							]
						}
					}
				}
			}
		}
	`)

	client := queries.NewTestClient()
	buf := bytes.Buffer{}
	ios, _, _, _ := iostreams.Test()
	config := viewConfig{
		opts: viewOpts{
			owner:  "monalisa",
			web:    true,
			number: 8,
		},
		URLOpener: func(url string) error {
			buf.WriteString(url)
			return nil
		},
		client: client,
		io:     ios,
	}

	err := runView(config)
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/users/monalisa/projects/8", buf.String())
}

func TestRunViewWeb_Org(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	// get org ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgOwner.*",
			"variables": map[string]interface{}{
				"login": "github",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"organization": map[string]interface{}{
					"id": "an ID",
				},
			},
			"errors": []interface{}{
				map[string]interface{}{
					"type": "NOT_FOUND",
					"path": []string{"user"},
				},
			},
		})

	gock.New("https://api.github.com").
		Post("/graphql").
		Reply(200).
		JSON(`
		{"data":
			{"organization":
				{
					"login":"github",
					"projectV2": {
						"number": 8,
						"items": {
							"totalCount": 10
						},
						"url": "https://github.com/orgs/github/projects/8",
						"readme": null,
						"fields": {
							"nodes": [
								{
									"name": "Title"
								}
							]
						}
					}
				}
			}
		}
	`)

	client := queries.NewTestClient()
	buf := bytes.Buffer{}
	ios, _, _, _ := iostreams.Test()
	config := viewConfig{
		opts: viewOpts{
			owner:  "github",
			web:    true,
			number: 8,
		},
		URLOpener: func(url string) error {
			buf.WriteString(url)
			return nil
		},
		client: client,
		io:     ios,
	}

	err := runView(config)
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/orgs/github/projects/8", buf.String())
}

func TestRunViewWeb_Me(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	// get viewer ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query Viewer.*",
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]interface{}{
					"id":    "an ID",
					"login": "theviewer",
				},
			},
		})

	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query":     "query ViewerProject.*",
			"variables": map[string]interface{}{"afterFields": nil, "afterItems": nil, "firstFields": 100, "firstItems": 0, "number": 8},
		}).
		Reply(200).
		JSON(`
		{"data":
			{"viewer":
				{
					"login":"github",
					"projectV2": {
						"number": 8,
						"items": {
							"totalCount": 10
						},
						"readme": null,
						"url": "https://github.com/users/theviewer/projects/8",
						"fields": {
							"nodes": [
								{
									"name": "Title"
								}
							]
						}
					}
				}
			}
		}
	`)

	client := queries.NewTestClient()
	buf := bytes.Buffer{}
	ios, _, _, _ := iostreams.Test()
	config := viewConfig{
		opts: viewOpts{
			owner:  "@me",
			web:    true,
			number: 8,
		},
		URLOpener: func(url string) error {
			buf.WriteString(url)
			return nil
		},
		client: client,
		io:     ios,
	}

	err := runView(config)
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/users/theviewer/projects/8", buf.String())
}

func TestRunViewWeb_TTY(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(*viewOpts, *testing.T) func()
		promptStubs    func(*prompter.PrompterMock)
		gqlStubs       func(*testing.T)
		expectedBrowse string
		tty            bool
	}{
		{
			name: "Org project --web with tty",
			tty:  true,
			setup: func(vo *viewOpts, t *testing.T) func() {
				return func() {
					vo.web = true
				}
			},
			gqlStubs: func(t *testing.T) {
				gock.New("https://api.github.com").
					Post("/graphql").
					MatchType("json").
					JSON(map[string]interface{}{
						"query": "query ViewerLoginAndOrgs.*",
						"variables": map[string]interface{}{
							"after": nil,
						},
					}).
					Reply(200).
					JSON(map[string]interface{}{
						"data": map[string]interface{}{
							"viewer": map[string]interface{}{
								"id":    "monalisa-ID",
								"login": "monalisa",
								"organizations": map[string]interface{}{
									"nodes": []interface{}{
										map[string]interface{}{
											"login":                   "github",
											"viewerCanCreateProjects": true,
										},
									},
								},
							},
						},
					})

				gock.New("https://api.github.com").
					Post("/graphql").
					MatchType("json").
					JSON(map[string]interface{}{
						"query": "query OrgProjects.*",
						"variables": map[string]interface{}{
							"after":       nil,
							"afterFields": nil,
							"afterItems":  nil,
							"first":       30,
							"firstFields": 100,
							"firstItems":  0,
							"login":       "github",
						},
					}).
					Reply(200).
					JSON(map[string]interface{}{
						"data": map[string]interface{}{
							"organization": map[string]interface{}{
								"login": "github",
								"projectsV2": map[string]interface{}{
									"nodes": []interface{}{
										map[string]interface{}{
											"id":     "a-project-ID",
											"title":  "Get it done!",
											"url":    "https://github.com/orgs/github/projects/1",
											"number": 1,
										},
									},
								},
							},
						},
					})
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(prompt, _ string, opts []string) (int, error) {
					switch prompt {
					case "Which owner would you like to use?":
						return prompter.IndexFor(opts, "github")
					case "Which project would you like to use?":
						return prompter.IndexFor(opts, "Get it done! (#1)")
					default:
						return -1, prompter.NoSuchPromptErr(prompt)
					}
				}
			},
			expectedBrowse: "https://github.com/orgs/github/projects/1",
		},
		{
			name: "User project --web with tty",
			tty:  true,
			setup: func(vo *viewOpts, t *testing.T) func() {
				return func() {
					vo.web = true
				}
			},
			gqlStubs: func(t *testing.T) {
				gock.New("https://api.github.com").
					Post("/graphql").
					MatchType("json").
					JSON(map[string]interface{}{
						"query": "query ViewerLoginAndOrgs.*",
						"variables": map[string]interface{}{
							"after": nil,
						},
					}).
					Reply(200).
					JSON(map[string]interface{}{
						"data": map[string]interface{}{
							"viewer": map[string]interface{}{
								"id":    "monalisa-ID",
								"login": "monalisa",
								"organizations": map[string]interface{}{
									"nodes": []interface{}{
										map[string]interface{}{
											"login":                   "github",
											"viewerCanCreateProjects": true,
										},
									},
								},
							},
						},
					})

				gock.New("https://api.github.com").
					Post("/graphql").
					MatchType("json").
					JSON(map[string]interface{}{
						"query": "query ViewerProjects.*",
						"variables": map[string]interface{}{
							"after": nil, "afterFields": nil, "afterItems": nil, "first": 30, "firstFields": 100, "firstItems": 0,
						},
					}).
					Reply(200).
					JSON(map[string]interface{}{
						"data": map[string]interface{}{
							"viewer": map[string]interface{}{
								"login": "monalisa",
								"projectsV2": map[string]interface{}{
									"totalCount": 1,
									"nodes": []interface{}{
										map[string]interface{}{
											"id":     "a-project-ID",
											"number": 1,
											"title":  "@monalia's first project",
											"url":    "https://github.com/users/monalisa/projects/1",
										},
									},
								},
							},
						},
					})
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(prompt, _ string, opts []string) (int, error) {
					switch prompt {
					case "Which owner would you like to use?":
						return prompter.IndexFor(opts, "monalisa")
					case "Which project would you like to use?":
						return prompter.IndexFor(opts, "@monalia's first project (#1)")
					default:
						return -1, prompter.NoSuchPromptErr(prompt)
					}
				}
			},
			expectedBrowse: "https://github.com/users/monalisa/projects/1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer gock.Off()
			gock.Observe(gock.DumpRequest)

			if tt.gqlStubs != nil {
				tt.gqlStubs(t)
			}

			pm := &prompter.PrompterMock{}
			if tt.promptStubs != nil {
				tt.promptStubs(pm)
			}

			opts := viewOpts{}
			if tt.setup != nil {
				tt.setup(&opts, t)()
			}

			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			ios.SetStdinTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)

			buf := bytes.Buffer{}

			client := queries.NewTestClient(queries.WithPrompter(pm))

			config := viewConfig{
				opts: opts,
				URLOpener: func(url string) error {
					_, err := buf.WriteString(url)
					return err
				},
				client: client,
				io:     ios,
			}

			err := runView(config)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedBrowse, buf.String())
		})
	}
}
