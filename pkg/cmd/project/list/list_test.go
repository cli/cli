package list

import (
	"bytes"
	"os"
	"testing"

	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestNewCmdlist(t *testing.T) {
	tests := []struct {
		name        string
		cli         string
		wants       listOpts
		wantsErr    bool
		wantsErrMsg string
	}{
		{
			name: "login",
			cli:  "--login monalisa",
			wants: listOpts{
				login: "monalisa",
				limit: 30,
			},
		},
		{
			name: "closed",
			cli:  "--closed",
			wants: listOpts{
				closed: true,
				limit:  30,
			},
		},
		{
			name: "web",
			cli:  "--web",
			wants: listOpts{
				web:   true,
				limit: 30,
			},
		},
		{
			name: "json",
			cli:  "--format json",
			wants: listOpts{
				format: "json",
				limit:  30,
			},
		},
	}

	os.Setenv("GH_TOKEN", "auth-token")
	defer os.Unsetenv("GH_TOKEN")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts listOpts
			cmd := NewCmdList(f, func(config listConfig) error {
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

			assert.Equal(t, tt.wants.login, gotOpts.login)
			assert.Equal(t, tt.wants.closed, gotOpts.closed)
			assert.Equal(t, tt.wants.web, gotOpts.web)
			assert.Equal(t, tt.wants.limit, gotOpts.limit)
			assert.Equal(t, tt.wants.format, gotOpts.format)
		})
	}
}

func TestRunList(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgLogin.*",
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
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"login": "monalisa",
					"projectsV2": map[string]interface{}{
						"nodes": []interface{}{
							map[string]interface{}{
								"title":            "Project 1",
								"shortDescription": "Short description 1",
								"url":              "url1",
								"closed":           false,
								"ID":               "1",
							},
							map[string]interface{}{
								"title":            "Project 2",
								"shortDescription": "",
								"url":              "url2",
								"closed":           true,
								"ID":               "2",
							},
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := listConfig{
		tp: tableprinter.New(ios),
		opts: listOpts{
			login: "monalisa",
		},
		client: client,
		io:     ios,
	}

	err := runList(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Project 1\tShort description 1\turl1\topen\t1\n",
		stdout.String())
}

func TestRunList_Me(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// get viewer ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query ViewerLogin.*",
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
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]interface{}{
					"login": "monalisa",
					"projectsV2": map[string]interface{}{
						"nodes": []interface{}{
							map[string]interface{}{
								"title":            "Project 1",
								"shortDescription": "Short description 1",
								"url":              "url1",
								"closed":           false,
								"ID":               "1",
							},
							map[string]interface{}{
								"title":            "Project 2",
								"shortDescription": "",
								"url":              "url2",
								"closed":           true,
								"ID":               "2",
							},
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := listConfig{
		tp: tableprinter.New(ios),
		opts: listOpts{
			login: "@me",
		},
		client: client,
		io:     ios,
	}

	err := runList(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Project 1\tShort description 1\turl1\topen\t1\n",
		stdout.String())
}

func TestRunListViewer(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// get viewer ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query ViewerLogin.*",
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
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]interface{}{
					"login": "monalisa",
					"projectsV2": map[string]interface{}{
						"nodes": []interface{}{
							map[string]interface{}{
								"title":            "Project 1",
								"shortDescription": "Short description 1",
								"url":              "url1",
								"closed":           false,
								"ID":               "1",
							},
							map[string]interface{}{
								"title":            "Project 2",
								"shortDescription": "",
								"url":              "url2",
								"closed":           true,
								"ID":               "2",
							},
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := listConfig{
		tp:     tableprinter.New(ios),
		opts:   listOpts{},
		client: client,
		io:     ios,
	}

	err := runList(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Project 1\tShort description 1\turl1\topen\t1\n",
		stdout.String())
}

func TestRunListOrg(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// get org ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgLogin.*",
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
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"organization": map[string]interface{}{
					"login": "monalisa",
					"projectsV2": map[string]interface{}{
						"nodes": []interface{}{
							map[string]interface{}{
								"title":            "Project 1",
								"shortDescription": "Short description 1",
								"url":              "url1",
								"closed":           false,
								"ID":               "1",
							},
							map[string]interface{}{
								"title":            "Project 2",
								"shortDescription": "",
								"url":              "url2",
								"closed":           true,
								"ID":               "2",
							},
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := listConfig{
		tp: tableprinter.New(ios),
		opts: listOpts{
			login: "github",
		},
		client: client,
		io:     ios,
	}

	err := runList(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Project 1\tShort description 1\turl1\topen\t1\n",
		stdout.String())
}

func TestRunListEmpty(t *testing.T) {
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
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]interface{}{
					"login": "monalisa",
					"projectsV2": map[string]interface{}{
						"nodes": []interface{}{},
					},
				},
			},
		})
	client := queries.NewTestClient()

	ios, _, _, _ := iostreams.Test()
	config := listConfig{
		tp:     tableprinter.New(ios),
		opts:   listOpts{},
		client: client,
		io:     ios,
	}

	err := runList(config)
	assert.EqualError(
		t,
		err,
		"No projects found for @me")
}

func TestRunListWithClosed(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgLogin.*",
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
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"login": "monalisa",
					"projectsV2": map[string]interface{}{
						"nodes": []interface{}{
							map[string]interface{}{
								"title":            "Project 1",
								"shortDescription": "Short description 1",
								"url":              "url1",
								"closed":           false,
								"ID":               "1",
							},
							map[string]interface{}{
								"title":            "Project 2",
								"shortDescription": "",
								"url":              "url2",
								"closed":           true,
								"ID":               "2",
							},
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := listConfig{
		tp: tableprinter.New(ios),
		opts: listOpts{
			login:  "monalisa",
			closed: true,
		},
		client: client,
		io:     ios,
	}

	err := runList(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Project 1\tShort description 1\turl1\topen\t1\nProject 2\t\turl2\tclosed\t2\n",
		stdout.String())
}

func TestRunListWeb_User(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgLogin.*",
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

	client := queries.NewTestClient()
	buf := bytes.Buffer{}
	config := listConfig{
		opts: listOpts{
			login: "monalisa",
			web:   true,
		},
		URLOpener: func(url string) error {
			buf.WriteString(url)
			return nil
		},
		client: client,
	}

	err := runList(config)
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/users/monalisa/projects", buf.String())
}

func TestRunListWeb_Org(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	// get org ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgLogin.*",
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

	client := queries.NewTestClient()
	buf := bytes.Buffer{}
	config := listConfig{
		opts: listOpts{
			login: "github",
			web:   true,
		},
		URLOpener: func(url string) error {
			buf.WriteString(url)
			return nil
		},
		client: client,
	}

	err := runList(config)
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/orgs/github/projects", buf.String())
}

func TestRunListWeb_Me(t *testing.T) {
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

	client := queries.NewTestClient()
	buf := bytes.Buffer{}
	config := listConfig{
		opts: listOpts{
			login: "@me",
			web:   true,
		},
		URLOpener: func(url string) error {
			buf.WriteString(url)
			return nil
		},
		client: client,
	}

	err := runList(config)
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/users/theviewer/projects", buf.String())
}

func TestRunListWeb_Empty(t *testing.T) {
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

	client := queries.NewTestClient()
	buf := bytes.Buffer{}
	config := listConfig{
		opts: listOpts{
			web: true,
		},
		URLOpener: func(url string) error {
			buf.WriteString(url)
			return nil
		},
		client: client,
	}

	err := runList(config)
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/users/theviewer/projects", buf.String())
}

func TestRunListWeb_Closed(t *testing.T) {
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

	client := queries.NewTestClient()
	buf := bytes.Buffer{}
	config := listConfig{
		opts: listOpts{
			web:    true,
			closed: true,
		},
		URLOpener: func(url string) error {
			buf.WriteString(url)
			return nil
		},
		client: client,
	}

	err := runList(config)
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/users/theviewer/projects?query=is%3Aclosed", buf.String())
}
