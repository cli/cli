package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	prShared "github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdCreate(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "my-body.md")
	err := ioutil.WriteFile(tmpFile, []byte("a body from file"), 0600)
	require.NoError(t, err)

	tests := []struct {
		name      string
		tty       bool
		stdin     string
		cli       string
		wantsErr  bool
		wantsOpts CreateOptions
	}{
		{
			name:     "empty non-tty",
			tty:      false,
			cli:      "",
			wantsErr: true,
		},
		{
			name:     "only title non-tty",
			tty:      false,
			cli:      "-t mytitle",
			wantsErr: true,
		},
		{
			name:     "empty tty",
			tty:      true,
			cli:      "",
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:       "",
				Body:        "",
				RecoverFile: "",
				WebMode:     false,
				Interactive: true,
			},
		},
		{
			name:     "body from stdin",
			tty:      false,
			stdin:    "this is on standard input",
			cli:      "-t mytitle -F -",
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:       "mytitle",
				Body:        "this is on standard input",
				RecoverFile: "",
				WebMode:     false,
				Interactive: false,
			},
		},
		{
			name:     "body from file",
			tty:      false,
			cli:      fmt.Sprintf("-t mytitle -F '%s'", tmpFile),
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:       "mytitle",
				Body:        "a body from file",
				RecoverFile: "",
				WebMode:     false,
				Interactive: false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, stdin, stdout, stderr := iostreams.Test()
			if tt.stdin != "" {
				_, _ = stdin.WriteString(tt.stdin)
			} else if tt.tty {
				io.SetStdinTTY(true)
				io.SetStdoutTTY(true)
			}

			f := &cmdutil.Factory{
				IOStreams: io,
			}

			var opts *CreateOptions
			cmd := NewCmdCreate(f, func(o *CreateOptions) error {
				opts = o
				return nil
			})

			args, err := shlex.Split(tt.cli)
			require.NoError(t, err)
			cmd.SetArgs(args)
			cmd.SetOut(ioutil.Discard)
			cmd.SetErr(ioutil.Discard)
			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, "", stdout.String())
			assert.Equal(t, "", stderr.String())

			assert.Equal(t, tt.wantsOpts.Body, opts.Body)
			assert.Equal(t, tt.wantsOpts.Title, opts.Title)
			assert.Equal(t, tt.wantsOpts.RecoverFile, opts.RecoverFile)
			assert.Equal(t, tt.wantsOpts.WebMode, opts.WebMode)
			assert.Equal(t, tt.wantsOpts.Interactive, opts.Interactive)
		})
	}
}

func Test_createRun(t *testing.T) {
	tests := []struct {
		name        string
		opts        CreateOptions
		httpStubs   func(*httpmock.Registry)
		wantsStdout string
		wantsStderr string
		wantsBrowse string
		wantsErr    string
	}{
		{
			name: "no args",
			opts: CreateOptions{
				WebMode: true,
			},
			wantsBrowse: "https://github.com/OWNER/REPO/issues/new",
			wantsStderr: "Opening github.com/OWNER/REPO/issues/new in your browser.\n",
		},
		{
			name: "title and body",
			opts: CreateOptions{
				WebMode: true,
				Title:   "myissue",
				Body:    "hello cli",
			},
			wantsBrowse: "https://github.com/OWNER/REPO/issues/new?body=hello+cli&title=myissue",
			wantsStderr: "Opening github.com/OWNER/REPO/issues/new in your browser.\n",
		},
		{
			name: "assignee",
			opts: CreateOptions{
				WebMode:   true,
				Assignees: []string{"monalisa"},
			},
			wantsBrowse: "https://github.com/OWNER/REPO/issues/new?assignees=monalisa&body=",
			wantsStderr: "Opening github.com/OWNER/REPO/issues/new in your browser.\n",
		},
		{
			name: "@me",
			opts: CreateOptions{
				WebMode:   true,
				Assignees: []string{"@me"},
			},
			httpStubs: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`
					{ "data": {
						"viewer": { "login": "MonaLisa" }
					} }`))
			},
			wantsBrowse: "https://github.com/OWNER/REPO/issues/new?assignees=MonaLisa&body=",
			wantsStderr: "Opening github.com/OWNER/REPO/issues/new in your browser.\n",
		},
		{
			name: "project",
			opts: CreateOptions{
				WebMode:  true,
				Projects: []string{"cleanup"},
			},
			httpStubs: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query RepositoryProjectList\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "projects": {
						"nodes": [
							{ "name": "Cleanup", "id": "CLEANUPID", "resourcePath": "/OWNER/REPO/projects/1" }
						],
						"pageInfo": { "hasNextPage": false }
					} } } }`))
				r.Register(
					httpmock.GraphQL(`query OrganizationProjectList\b`),
					httpmock.StringResponse(`
					{ "data": { "organization": { "projects": {
						"nodes": [
							{ "name": "Triage", "id": "TRIAGEID", "resourcePath": "/orgs/ORG/projects/1"  }
						],
						"pageInfo": { "hasNextPage": false }
					} } } }`))
			},
			wantsBrowse: "https://github.com/OWNER/REPO/issues/new?body=&projects=OWNER%2FREPO%2F1",
			wantsStderr: "Opening github.com/OWNER/REPO/issues/new in your browser.\n",
		},
		{
			name: "has templates",
			opts: CreateOptions{
				WebMode: true,
			},
			httpStubs: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query IssueTemplates\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "issueTemplates": [
						{ "name": "Bug report",
							"body": "Does not work :((" },
						{ "name": "Submit a request",
							"body": "I have a suggestion for an enhancement" }
					] } } }`),
				)
			},
			wantsBrowse: "https://github.com/OWNER/REPO/issues/new/choose",
			wantsStderr: "Opening github.com/OWNER/REPO/issues/new/choose in your browser.\n",
		},
		{
			name: "too long body",
			opts: CreateOptions{
				WebMode: true,
				Body:    strings.Repeat("A", 9216),
			},
			wantsErr: "cannot open in browser: maximum URL length exceeded",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpReg := &httpmock.Registry{}
			defer httpReg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(httpReg)
			}

			io, _, stdout, stderr := iostreams.Test()
			io.SetStdoutTTY(true)
			opts := &tt.opts
			opts.IO = io
			opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: httpReg}, nil
			}
			opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			}
			browser := &cmdutil.TestBrowser{}
			opts.Browser = browser

			err := createRun(opts)
			if tt.wantsErr == "" {
				require.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantsErr)
				return
			}

			assert.Equal(t, tt.wantsStdout, stdout.String())
			assert.Equal(t, tt.wantsStderr, stderr.String())
			browser.Verify(t, tt.wantsBrowse)
		})
	}
}

/*** LEGACY TESTS ***/

func runCommand(rt http.RoundTripper, isTTY bool, cli string) (*test.CmdOut, error) {
	return runCommandWithRootDirOverridden(rt, isTTY, cli, "")
}

func runCommandWithRootDirOverridden(rt http.RoundTripper, isTTY bool, cli string, rootDir string) (*test.CmdOut, error) {
	io, _, stdout, stderr := iostreams.Test()
	io.SetStdoutTTY(isTTY)
	io.SetStdinTTY(isTTY)
	io.SetStderrTTY(isTTY)

	browser := &cmdutil.TestBrowser{}
	factory := &cmdutil.Factory{
		IOStreams: io,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		Config: func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
		Browser: browser,
	}

	cmd := NewCmdCreate(factory, func(opts *CreateOptions) error {
		opts.RootDirOverride = rootDir
		return createRun(opts)
	})

	argv, err := shlex.Split(cli)
	if err != nil {
		return nil, err
	}
	cmd.SetArgs(argv)

	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(ioutil.Discard)
	cmd.SetErr(ioutil.Discard)

	_, err = cmd.ExecuteC()
	return &test.CmdOut{
		OutBuf:     stdout,
		ErrBuf:     stderr,
		BrowsedURL: browser.BrowsedURL(),
	}, err
}

func TestIssueCreate(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"id": "REPOID",
				"hasIssuesEnabled": true
			} } }`),
	)
	http.Register(
		httpmock.GraphQL(`mutation IssueCreate\b`),
		httpmock.GraphQLMutation(`
				{ "data": { "createIssue": { "issue": {
					"URL": "https://github.com/OWNER/REPO/issues/12"
				} } } }`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["repositoryId"], "REPOID")
				assert.Equal(t, inputs["title"], "hello")
				assert.Equal(t, inputs["body"], "cash rules everything around me")
			}),
	)

	output, err := runCommand(http, true, `-t hello -b "cash rules everything around me"`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "https://github.com/OWNER/REPO/issues/12\n", output.String())
}

func TestIssueCreate_recover(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"id": "REPOID",
				"hasIssuesEnabled": true
			} } }`))
	http.Register(
		httpmock.GraphQL(`query RepositoryResolveMetadataIDs\b`),
		httpmock.StringResponse(`
		{ "data": {
			"u000": { "login": "MonaLisa", "id": "MONAID" },
			"repository": {
				"l000": { "name": "bug", "id": "BUGID" },
				"l001": { "name": "TODO", "id": "TODOID" }
			}
		} }
		`))
	http.Register(
		httpmock.GraphQL(`mutation IssueCreate\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "createIssue": { "issue": {
			"URL": "https://github.com/OWNER/REPO/issues/12"
		} } } }
	`, func(inputs map[string]interface{}) {
			assert.Equal(t, "recovered title", inputs["title"])
			assert.Equal(t, "recovered body", inputs["body"])
			assert.Equal(t, []interface{}{"BUGID", "TODOID"}, inputs["labelIds"])
		}))

	as, teardown := prompt.InitAskStubber()
	defer teardown()

	as.Stub([]*prompt.QuestionStub{
		{
			Name:    "Title",
			Default: true,
		},
	})
	as.Stub([]*prompt.QuestionStub{
		{
			Name:    "Body",
			Default: true,
		},
	})
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "confirmation",
			Value: 0,
		},
	})

	tmpfile, err := ioutil.TempFile(t.TempDir(), "testrecover*")
	assert.NoError(t, err)
	defer tmpfile.Close()

	state := prShared.IssueMetadataState{
		Title:  "recovered title",
		Body:   "recovered body",
		Labels: []string{"bug", "TODO"},
	}

	data, err := json.Marshal(state)
	assert.NoError(t, err)

	_, err = tmpfile.Write(data)
	assert.NoError(t, err)

	args := fmt.Sprintf("--recover '%s'", tmpfile.Name())

	output, err := runCommandWithRootDirOverridden(http, true, args, "")
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "https://github.com/OWNER/REPO/issues/12\n", output.String())
}

func TestIssueCreate_nonLegacyTemplate(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"id": "REPOID",
				"hasIssuesEnabled": true
			} } }`),
	)
	http.Register(
		httpmock.GraphQL(`query IssueTemplates\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": { "issueTemplates": [
				{ "name": "Bug report",
				  "body": "Does not work :((" },
				{ "name": "Submit a request",
				  "body": "I have a suggestion for an enhancement" }
			] } } }`),
	)
	http.Register(
		httpmock.GraphQL(`mutation IssueCreate\b`),
		httpmock.GraphQLMutation(`
			{ "data": { "createIssue": { "issue": {
				"URL": "https://github.com/OWNER/REPO/issues/12"
			} } } }`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["repositoryId"], "REPOID")
				assert.Equal(t, inputs["title"], "hello")
				assert.Equal(t, inputs["body"], "I have a suggestion for an enhancement")
			}),
	)

	as, teardown := prompt.InitAskStubber()
	defer teardown()

	// template
	as.StubOne(1)
	// body
	as.Stub([]*prompt.QuestionStub{
		{
			Name:    "Body",
			Default: true,
		},
	}) // body
	// confirm
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "confirmation",
			Value: 0,
		},
	})

	output, err := runCommandWithRootDirOverridden(http, true, `-t hello`, "./fixtures/repoWithNonLegacyIssueTemplates")
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "https://github.com/OWNER/REPO/issues/12\n", output.String())
	assert.Equal(t, "", output.BrowsedURL)
}

func TestIssueCreate_continueInBrowser(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"id": "REPOID",
				"hasIssuesEnabled": true
			} } }`),
	)

	as, teardown := prompt.InitAskStubber()
	defer teardown()

	// title
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "Title",
			Value: "hello",
		},
	})
	// confirm
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "confirmation",
			Value: 1,
		},
	})

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	output, err := runCommand(http, true, `-b body`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, heredoc.Doc(`

		Creating issue in OWNER/REPO

		Opening github.com/OWNER/REPO/issues/new in your browser.
	`), output.Stderr())
	assert.Equal(t, "https://github.com/OWNER/REPO/issues/new?body=body&title=hello", output.BrowsedURL)
}

func TestIssueCreate_metadata(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.StubRepoInfoResponse("OWNER", "REPO", "main")
	http.Register(
		httpmock.GraphQL(`query RepositoryResolveMetadataIDs\b`),
		httpmock.StringResponse(`
		{ "data": {
			"u000": { "login": "MonaLisa", "id": "MONAID" },
			"repository": {
				"l000": { "name": "bug", "id": "BUGID" },
				"l001": { "name": "TODO", "id": "TODOID" }
			}
		} }
		`))
	http.Register(
		httpmock.GraphQL(`query RepositoryMilestoneList\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "milestones": {
			"nodes": [
				{ "title": "GA", "id": "GAID" },
				{ "title": "Big One.oh", "id": "BIGONEID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query RepositoryProjectList\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "projects": {
			"nodes": [
				{ "name": "Cleanup", "id": "CLEANUPID" },
				{ "name": "Roadmap", "id": "ROADMAPID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query OrganizationProjectList\b`),
		httpmock.StringResponse(`
		{	"data": { "organization": null },
			"errors": [{
				"type": "NOT_FOUND",
				"path": [ "organization" ],
				"message": "Could not resolve to an Organization with the login of 'OWNER'."
			}]
		}
		`))
	http.Register(
		httpmock.GraphQL(`mutation IssueCreate\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "createIssue": { "issue": {
			"URL": "https://github.com/OWNER/REPO/issues/12"
		} } } }
	`, func(inputs map[string]interface{}) {
			assert.Equal(t, "TITLE", inputs["title"])
			assert.Equal(t, "BODY", inputs["body"])
			assert.Equal(t, []interface{}{"MONAID"}, inputs["assigneeIds"])
			assert.Equal(t, []interface{}{"BUGID", "TODOID"}, inputs["labelIds"])
			assert.Equal(t, []interface{}{"ROADMAPID"}, inputs["projectIds"])
			assert.Equal(t, "BIGONEID", inputs["milestoneId"])
			if v, ok := inputs["userIds"]; ok {
				t.Errorf("did not expect userIds: %v", v)
			}
			if v, ok := inputs["teamIds"]; ok {
				t.Errorf("did not expect teamIds: %v", v)
			}
		}))

	output, err := runCommand(http, true, `-t TITLE -b BODY -a monalisa -l bug -l todo -p roadmap -m 'big one.oh'`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "https://github.com/OWNER/REPO/issues/12\n", output.String())
}

func TestIssueCreate_disabledIssues(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"id": "REPOID",
				"hasIssuesEnabled": false
			} } }`),
	)

	_, err := runCommand(http, true, `-t heres -b johnny`)
	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
		t.Errorf("error running command `issue create`: %v", err)
	}
}

func TestIssueCreate_AtMeAssignee(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`
		{ "data": {
			"viewer": { "login": "MonaLisa" }
		} }
		`),
	)
	http.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": {
			"id": "REPOID",
			"hasIssuesEnabled": true
		} } }
	`))
	http.Register(
		httpmock.GraphQL(`query RepositoryResolveMetadataIDs\b`),
		httpmock.StringResponse(`
		{ "data": {
			"u000": { "login": "MonaLisa", "id": "MONAID" },
			"u001": { "login": "SomeOneElse", "id": "SOMEID" },
			"repository": {
				"l000": { "name": "bug", "id": "BUGID" },
				"l001": { "name": "TODO", "id": "TODOID" }
			}
		} }
		`),
	)
	http.Register(
		httpmock.GraphQL(`mutation IssueCreate\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "createIssue": { "issue": {
			"URL": "https://github.com/OWNER/REPO/issues/12"
		} } } }
	`, func(inputs map[string]interface{}) {
			assert.Equal(t, "hello", inputs["title"])
			assert.Equal(t, "cash rules everything around me", inputs["body"])
			assert.Equal(t, []interface{}{"MONAID", "SOMEID"}, inputs["assigneeIds"])
		}))

	output, err := runCommand(http, true, `-a @me -a someoneelse -t hello -b "cash rules everything around me"`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "https://github.com/OWNER/REPO/issues/12\n", output.String())
}
