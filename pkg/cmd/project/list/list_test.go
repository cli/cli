package list

import (
	"bytes"
	"testing"

	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/go-gh/v2/pkg/api"
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
			name:        "user-and-org",
			cli:         "--user monalisa --org github",
			wantsErr:    true,
			wantsErrMsg: "if any flags in the group [user org] are set none of the others can be; [org user] were all set",
		},
		{
			name: "user",
			cli:  "--user monalisa",
			wants: listOpts{
				userOwner: "monalisa",
			},
		},
		{
			name: "org",
			cli:  "--org github",
			wants: listOpts{
				orgOwner: "github",
			},
		},
		{
			name: "closed",
			cli:  "--closed",
			wants: listOpts{
				closed: true,
			},
		},
		{
			name: "web",
			cli:  "--web",
			wants: listOpts{
				web: true,
			},
		},
		{
			name: "limit",
			cli:  "--limit all",
			wants: listOpts{
				limit: "all",
			},
		},
		{
			name: "json",
			cli:  "--format json",
			wants: listOpts{
				format: "json",
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

			assert.Equal(t, tt.wants.userOwner, gotOpts.userOwner)
			assert.Equal(t, tt.wants.orgOwner, gotOpts.orgOwner)
			assert.Equal(t, tt.wants.closed, gotOpts.closed)
			assert.Equal(t, tt.wants.web, gotOpts.web)
			assert.Equal(t, tt.wants.limit, gotOpts.limit)
			assert.Equal(t, tt.wants.format, gotOpts.format)
		})
	}
}

func TestBuildURLViewer(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Post("/graphql").
		Reply(200).
		JSON(`
			{"data":
				{"viewer":
					{
						"login":"theviewer"
					}
				}
			}
		`)

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	url, err := buildURL(listConfig{
		opts:   listOpts{},
		client: client,
	})
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/users/theviewer/projects", url)
}

func TestBuildURLUser(t *testing.T) {
	url, err := buildURL(listConfig{
		opts: listOpts{
			userOwner: "monalisa",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/users/monalisa/projects", url)
}

func TestBuildURLOrg(t *testing.T) {
	url, err := buildURL(listConfig{
		opts: listOpts{
			orgOwner: "github",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/orgs/github/projects", url)
}

func TestBuildURLWithClosed(t *testing.T) {
	url, err := buildURL(listConfig{
		opts: listOpts{
			orgOwner: "github",
			closed:   true,
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/orgs/github/projects?query=is%3Aclosed", url)
}

func TestRunList(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Post("/graphql").
		Reply(200).
		JSON(`
			{"data":
				{"user":
					{
						"login":"monalisa",
						"projectsV2": {
							"nodes": [
								{"title": "Project 1", "shortDescription": "Short description 1", "url": "url1", "closed": false, "ID": "1"},
								{"title": "Project 2", "shortDescription": "", "url": "url2", "closed": true, "ID": "2"}
							]
						}
					}
				}
			}
		`)

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := listConfig{
		tp: tableprinter.New(ios),
		opts: listOpts{
			userOwner: "monalisa",
		},
		client: client,
	}

	err = runList(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Title\tDescription\tURL\tID\nProject 1\tShort description 1\turl1\t1\n",
		stdout.String())
}

func TestRunList_Me(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Post("/graphql").
		Reply(200).
		JSON(`
			{"data":
				{"viewer":
					{
						"login":"monalisa",
						"projectsV2": {
							"nodes": [
								{"title": "Project 1", "shortDescription": "Short description 1", "url": "url1", "closed": false, "ID": "1"},
								{"title": "Project 2", "shortDescription": "", "url": "url2", "closed": true, "ID": "2"}
							]
						}
					}
				}
			}
		`)

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := listConfig{
		tp: tableprinter.New(ios),
		opts: listOpts{
			userOwner: "@me",
		},
		client: client,
	}

	err = runList(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Title\tDescription\tURL\tID\nProject 1\tShort description 1\turl1\t1\n",
		stdout.String())
}

func TestRunListViewer(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Post("/graphql").
		Reply(200).
		JSON(`
			{"data":
				{"viewer":
					{
						"login":"monalisa",
						"projectsV2": {
							"nodes": [
								{"title": "Project 1", "shortDescription": "Short description 1", "url": "url1", "closed": false, "ID": "1"},
								{"title": "Project 2", "shortDescription": "", "url": "url2", "closed": true, "ID": "2"}
							]
						}
					}
				}
			}
		`)

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := listConfig{
		tp:     tableprinter.New(ios),
		opts:   listOpts{},
		client: client,
	}

	err = runList(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Title\tDescription\tURL\tID\nProject 1\tShort description 1\turl1\t1\n",
		stdout.String())
}

func TestRunListOrg(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Post("/graphql").
		Reply(200).
		JSON(`
			{"data":
				{"organization":
					{
						"login":"monalisa",
						"projectsV2": {
							"nodes": [
								{"title": "Project 1", "shortDescription": "Short description 1", "url": "url1", "closed": false, "ID": "1"},
								{"title": "Project 2", "shortDescription": "", "url": "url2", "closed": true, "ID": "2"}
							]
						}
					}
				}
			}
		`)

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := listConfig{
		tp: tableprinter.New(ios),
		opts: listOpts{
			orgOwner: "github",
		},
		client: client,
	}

	err = runList(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Title\tDescription\tURL\tID\nProject 1\tShort description 1\turl1\t1\n",
		stdout.String())
}

func TestRunListEmpty(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Post("/graphql").
		Reply(200).
		JSON(`
			{"data":
				{"viewer":
					{
						"login":"monalisa",
						"projectsV2": {
							"nodes": []
						}
					}
				}
			}
		`)

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := listConfig{
		tp:     tableprinter.New(ios),
		opts:   listOpts{},
		client: client,
	}

	err = runList(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"No projects found for me\n",
		stdout.String())
}

func TestRunListWithClosed(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Post("/graphql").
		Reply(200).
		JSON(`
			{"data":
				{"user":
					{
						"login":"monalisa",
						"projectsV2": {
							"nodes": [
								{"title": "Project 1", "shortDescription": "Short description 1", "url": "url1", "closed": false, "ID": "1"},
								{"title": "Project 2", "shortDescription": "", "url": "url2", "closed": true, "ID": "2"}
							]
						}
					}
				}
			}
		`)

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := listConfig{
		tp: tableprinter.New(ios),
		opts: listOpts{
			userOwner: "monalisa",
			closed:    true,
		},
		client: client,
	}

	err = runList(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Title\tDescription\tURL\tState\tID\nProject 1\tShort description 1\turl1\topen\t1\nProject 2\t - \turl2\tclosed\t2\n",
		stdout.String())
}

func TestRunListWeb(t *testing.T) {
	buf := bytes.Buffer{}
	config := listConfig{
		opts: listOpts{
			userOwner: "monalisa",
			web:       true,
		},
		URLOpener: func(url string) error {
			buf.WriteString(url)
			return nil
		},
	}

	err := runList(config)
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/users/monalisa/projects", buf.String())
}
