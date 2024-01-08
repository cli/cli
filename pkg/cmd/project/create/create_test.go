package create

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestNewCmdCreate(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		wants         createOpts
		wantsErr      bool
		wantsErrMsg   string
		wantsExporter bool
	}{
		{
			name: "title",
			cli:  "--title t",
			wants: createOpts{
				title: "t",
			},
		},
		{
			name: "owner",
			cli:  "--title t --owner monalisa",
			wants: createOpts{
				owner: "monalisa",
				title: "t",
			},
		},
		{
			name: "json",
			cli:  "--title t --format json",
			wants: createOpts{
				title: "t",
			},
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

			var gotOpts createOpts
			cmd := NewCmdCreate(f, func(config createConfig) error {
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

			assert.Equal(t, tt.wants.title, gotOpts.title)
			assert.Equal(t, tt.wants.owner, gotOpts.owner)
			assert.Equal(t, tt.wantsExporter, gotOpts.exporter != nil)
		})
	}
}

func TestRunCreate_User(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgOwner.*",
			"variables": map[string]string{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"id":    "an ID",
					"login": "monalisa",
				},
			},
			"errors": []interface{}{
				map[string]interface{}{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	// create project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CreateProjectV2.*"variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"ownerId":"an ID","title":"a title"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"createProjectV2": map[string]interface{}{
					"projectV2": map[string]interface{}{
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
	config := createConfig{
		opts: createOpts{
			title: "a title",
			owner: "monalisa",
		},
		client: client,
		io:     ios,
	}

	err := runCreate(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"http://a-url.com\n",
		stdout.String())
}

func TestRunCreate_Org(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	// get org ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgOwner.*",
			"variables": map[string]string{
				"login": "github",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"organization": map[string]interface{}{
					"id":    "an ID",
					"login": "github",
				},
			},
			"errors": []interface{}{
				map[string]interface{}{
					"type": "NOT_FOUND",
					"path": []string{"user"},
				},
			},
		})

	// create project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CreateProjectV2.*"variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"ownerId":"an ID","title":"a title"}}}`).Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"createProjectV2": map[string]interface{}{
					"projectV2": map[string]interface{}{
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
	config := createConfig{
		opts: createOpts{
			title: "a title",
			owner: "github",
		},
		client: client,
		io:     ios,
	}

	err := runCreate(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"http://a-url.com\n",
		stdout.String())
}

func TestRunCreate_Me(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)
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
					"id":    "an ID",
					"login": "me",
				},
			},
		})

	// create project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CreateProjectV2.*"variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"ownerId":"an ID","title":"a title"}}}`).Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"createProjectV2": map[string]interface{}{
					"projectV2": map[string]interface{}{
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
	ios.SetStdoutTTY(true)
	config := createConfig{
		opts: createOpts{
			title: "a title",
			owner: "@me",
		},
		client: client,
		io:     ios,
	}

	err := runCreate(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"http://a-url.com\n",
		stdout.String())
}

func TestRunCreate_JSON(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgOwner.*",
			"variables": map[string]string{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"id":    "an ID",
					"login": "monalisa",
				},
			},
			"errors": []interface{}{
				map[string]interface{}{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	// create project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CreateProjectV2.*"variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"ownerId":"an ID","title":"a title"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"createProjectV2": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"number": 1,
						"title":  "a title",
						"url":    "http://a-url.com",
						"owner": map[string]string{
							"login": "monalisa",
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := createConfig{
		opts: createOpts{
			title:    "a title",
			owner:    "monalisa",
			exporter: cmdutil.NewJSONExporter(),
		},
		client: client,
		io:     ios,
	}

	err := runCreate(config)
	assert.NoError(t, err)
	assert.JSONEq(
		t,
		`{"number":1,"url":"http://a-url.com","shortDescription":"","public":false,"closed":false,"title":"a title","id":"","readme":"","items":{"totalCount":0},"fields":{"totalCount":0},"owner":{"type":"","login":"monalisa"}}`,
		stdout.String())
}
