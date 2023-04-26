package create

import (
	"os"
	"testing"

	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestNewCmdCreate(t *testing.T) {
	tests := []struct {
		name        string
		cli         string
		wants       createOpts
		wantsErr    bool
		wantsErrMsg string
	}{
		{
			name:        "user-and-org",
			cli:         "--title t --user monalisa --org github",
			wantsErr:    true,
			wantsErrMsg: "if any flags in the group [user org] are set none of the others can be; [org user] were all set",
		},
		{
			name: "title",
			cli:  "--title t",
			wants: createOpts{
				title: "t",
			},
		},
		{
			name: "user",
			cli:  "--title t --user monalisa",
			wants: createOpts{
				userOwner: "monalisa",
				title:     "t",
			},
		},
		{
			name: "org",
			cli:  "--title t --org github",
			wants: createOpts{
				orgOwner: "github",
				title:    "t",
			},
		},
		{
			name: "json",
			cli:  "--title t --format json",
			wants: createOpts{
				format: "json",
				title:  "t",
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
			assert.Equal(t, tt.wants.userOwner, gotOpts.userOwner)
			assert.Equal(t, tt.wants.orgOwner, gotOpts.orgOwner)
			assert.Equal(t, tt.wants.format, gotOpts.format)
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
			"query": "query UserLogin.*",
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

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := createConfig{
		tp: tableprinter.New(ios),
		opts: createOpts{
			title:     "a title",
			userOwner: "monalisa",
		},
		client: client,
	}

	err = runCreate(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created project 'a title'\nhttp://a-url.com\n",
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
			"query": "query OrgLogin.*",
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

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := createConfig{
		tp: tableprinter.New(ios),
		opts: createOpts{
			title:    "a title",
			orgOwner: "github",
		},
		client: client,
	}

	err = runCreate(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created project 'a title'\nhttp://a-url.com\n",
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
			"query": "query ViewerLogin.*",
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

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := createConfig{
		tp: tableprinter.New(ios),
		opts: createOpts{
			title:     "a title",
			userOwner: "@me",
		},
		client: client,
	}

	err = runCreate(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created project 'a title'\nhttp://a-url.com\n",
		stdout.String())
}
