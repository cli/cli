package copy

import (
	"testing"

	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestNewCmdCopy(t *testing.T) {
	tests := []struct {
		name        string
		cli         string
		wants       copyOpts
		wantsErr    bool
		wantsErrMsg string
	}{
		{
			name:        "not-a-number",
			cli:         "x --title t",
			wantsErr:    true,
			wantsErrMsg: "invalid number: x",
		},
		{
			name: "title",
			cli:  "--title t",
			wants: copyOpts{
				title: "t",
			},
		},
		{
			name: "number",
			cli:  "123 --title t",
			wants: copyOpts{
				number: 123,
				title:  "t",
			},
		},
		{
			name: "source-user",
			cli:  "--source-user monalisa --title t",
			wants: copyOpts{
				sourceUserOwner: "monalisa",
				title:           "t",
			},
		},
		{
			name: "source-org",
			cli:  "--source-org github --title t",
			wants: copyOpts{
				sourceOrgOwner: "github",
				title:          "t",
			},
		},
		{
			name: "target-user",
			cli:  "--target-user monalisa --title t",
			wants: copyOpts{
				targetUserOwner: "monalisa",
				title:           "t",
			},
		},
		{
			name: "target-org",
			cli:  "--target-org github --title t",
			wants: copyOpts{
				targetOrgOwner: "github",
				title:          "t",
			},
		},
		{
			name: "drafts",
			cli:  "--drafts --title t",
			wants: copyOpts{
				includeDraftIssues: true,
				title:              "t",
			},
		},
		{
			name: "json",
			cli:  "--format json --title t",
			wants: copyOpts{
				format: "json",
				title:  "t",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts copyOpts
			cmd := NewCmdCopy(f, func(config copyConfig) error {
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
			assert.Equal(t, tt.wants.sourceUserOwner, gotOpts.sourceUserOwner)
			assert.Equal(t, tt.wants.sourceOrgOwner, gotOpts.sourceOrgOwner)
			assert.Equal(t, tt.wants.targetUserOwner, gotOpts.targetUserOwner)
			assert.Equal(t, tt.wants.targetOrgOwner, gotOpts.targetOrgOwner)
			assert.Equal(t, tt.wants.title, gotOpts.title)
			assert.Equal(t, tt.wants.includeDraftIssues, gotOpts.includeDraftIssues)
			assert.Equal(t, tt.wants.format, gotOpts.format)
		})
	}
}

func TestRunCopy_User(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// get user project ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserProject.*",
			"variables": map[string]interface{}{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  0,
				"afterItems":  nil,
				"firstFields": 0,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"projectV2": map[string]string{
						"id": "an ID",
					},
				},
			},
		})

	// get source user ID
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

	// get target user ID
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

	// Copy project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CopyProjectV2.*","variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"projectId":"an ID","ownerId":"an ID","title":"a title","includeDraftIssues":false}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"copyProjectV2": map[string]interface{}{
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
	config := copyConfig{
		tp: tableprinter.New(ios),
		opts: copyOpts{
			title:           "a title",
			sourceUserOwner: "monalisa",
			targetUserOwner: "monalisa",
			number:          1,
		},
		client: client,
	}

	err = runCopy(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created project copy 'a title'\nhttp://a-url.com\n",
		stdout.String())
}

func TestRunCopy_Org(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// get org project ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query OrgProject.*",
			"variables": map[string]interface{}{
				"login":       "github",
				"number":      1,
				"firstItems":  0,
				"afterItems":  nil,
				"firstFields": 0,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"organization": map[string]interface{}{
					"projectV2": map[string]string{
						"id": "an ID",
					},
				},
			},
		})
	// get source org ID
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

	// get target source org ID
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

	// Copy project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CopyProjectV2.*","variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"projectId":"an ID","ownerId":"an ID","title":"a title","includeDraftIssues":false}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"copyProjectV2": map[string]interface{}{
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
	config := copyConfig{
		tp: tableprinter.New(ios),
		opts: copyOpts{
			title:          "a title",
			sourceOrgOwner: "github",
			targetOrgOwner: "github",
			number:         1,
		},
		client: client,
	}

	err = runCopy(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created project copy 'a title'\nhttp://a-url.com\n",
		stdout.String())
}

func TestRunCopy_Me(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	// get viewer project ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query ViewerProject.*",
			"variables": map[string]interface{}{
				"number":      1,
				"firstItems":  0,
				"afterItems":  nil,
				"firstFields": 0,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]interface{}{
					"projectV2": map[string]string{
						"id": "an ID",
					},
				},
			},
		})

	// get source viewer ID
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

	// get target viewer ID
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

	// Copy project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CopyProjectV2.*","variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"projectId":"an ID","ownerId":"an ID","title":"a title","includeDraftIssues":false}}}`).Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"copyProjectV2": map[string]interface{}{
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
	config := copyConfig{
		tp: tableprinter.New(ios),
		opts: copyOpts{
			title:           "a title",
			sourceUserOwner: "@me",
			targetUserOwner: "@me",
			number:          1,
		},
		client: client,
	}

	err = runCopy(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created project copy 'a title'\nhttp://a-url.com\n",
		stdout.String())
}
