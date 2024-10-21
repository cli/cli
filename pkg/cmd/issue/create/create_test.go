package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/run"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdCreate(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "my-body.md")
	err := os.WriteFile(tmpFile, []byte("a body from file"), 0600)
	require.NoError(t, err)

	tests := []struct {
		name      string
		tty       bool
		stdin     string
		cli       string
		config    string
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
		{
			name:     "template from name tty",
			tty:      true,
			cli:      `-t mytitle --template "bug report"`,
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:       "mytitle",
				Body:        "",
				RecoverFile: "",
				WebMode:     false,
				Template:    "bug report",
				Interactive: true,
			},
		},
		{
			name:     "template from name non-tty",
			tty:      false,
			cli:      `-t mytitle --template "bug report"`,
			wantsErr: true,
		},
		{
			name:     "template and body",
			tty:      false,
			cli:      `-t mytitle --template "bug report" --body "issue body"`,
			wantsErr: true,
		},
		{
			name:     "template and body file",
			tty:      false,
			cli:      `-t mytitle --template "bug report" --body-file "body_file.md"`,
			wantsErr: true,
		},
		{
			name:     "editor by cli",
			tty:      true,
			cli:      "--editor",
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:       "",
				Body:        "",
				RecoverFile: "",
				WebMode:     false,
				EditorMode:  true,
				Interactive: false,
			},
		},
		{
			name:     "editor by config",
			tty:      true,
			cli:      "",
			config:   "prefer_editor_prompt: enabled",
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:       "",
				Body:        "",
				RecoverFile: "",
				WebMode:     false,
				EditorMode:  true,
				Interactive: false,
			},
		},
		{
			name:     "editor and template",
			tty:      true,
			cli:      `--editor --template "bug report"`,
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:       "",
				Body:        "",
				RecoverFile: "",
				WebMode:     false,
				EditorMode:  true,
				Template:    "bug report",
				Interactive: false,
			},
		},
		{
			name:     "editor and web",
			tty:      true,
			cli:      "--editor --web",
			wantsErr: true,
		},
		{
			name:     "can use web even though editor is enabled by config",
			tty:      true,
			cli:      `--web --title mytitle --body "issue body"`,
			config:   "prefer_editor_prompt: enabled",
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:       "mytitle",
				Body:        "issue body",
				RecoverFile: "",
				WebMode:     true,
				EditorMode:  false,
				Interactive: false,
			},
		},
		{
			name:     "editor with non-tty",
			tty:      false,
			cli:      "--editor",
			wantsErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, stdin, stdout, stderr := iostreams.Test()
			if tt.stdin != "" {
				_, _ = stdin.WriteString(tt.stdin)
			} else if tt.tty {
				ios.SetStdinTTY(true)
				ios.SetStdoutTTY(true)
			}

			f := &cmdutil.Factory{
				IOStreams: ios,
				Config: func() (gh.Config, error) {
					if tt.config != "" {
						return config.NewFromString(tt.config), nil
					}
					return config.NewBlankConfig(), nil
				},
			}

			var opts *CreateOptions
			cmd := NewCmdCreate(f, func(o *CreateOptions) error {
				opts = o
				return nil
			})

			args, err := shlex.Split(tt.cli)
			require.NoError(t, err)
			cmd.SetArgs(args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
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
			assert.Equal(t, tt.wantsOpts.Template, opts.Template)
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
			wantsStderr: "Opening https://github.com/OWNER/REPO/issues/new in your browser.\n",
		},
		{
			name: "title and body",
			opts: CreateOptions{
				WebMode: true,
				Title:   "myissue",
				Body:    "hello cli",
			},
			wantsBrowse: "https://github.com/OWNER/REPO/issues/new?body=hello+cli&title=myissue",
			wantsStderr: "Opening https://github.com/OWNER/REPO/issues/new in your browser.\n",
		},
		{
			name: "assignee",
			opts: CreateOptions{
				WebMode:   true,
				Assignees: []string{"monalisa"},
			},
			wantsBrowse: "https://github.com/OWNER/REPO/issues/new?assignees=monalisa&body=",
			wantsStderr: "Opening https://github.com/OWNER/REPO/issues/new in your browser.\n",
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
			wantsStderr: "Opening https://github.com/OWNER/REPO/issues/new in your browser.\n",
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
				r.Register(
					httpmock.GraphQL(`query RepositoryProjectV2List\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "projectsV2": {
						"nodes": [
							{ "title": "CleanupV2", "id": "CLEANUPV2ID", "resourcePath": "/OWNER/REPO/projects/2" }
						],
						"pageInfo": { "hasNextPage": false }
					} } } }`))
				r.Register(
					httpmock.GraphQL(`query OrganizationProjectV2List\b`),
					httpmock.StringResponse(`
					{ "data": { "organization": { "projectsV2": {
						"nodes": [
							{ "title": "Triage", "id": "TRIAGEID", "resourcePath": "/orgs/ORG/projects/2"  }
						],
						"pageInfo": { "hasNextPage": false }
					} } } }`))
				r.Register(
					httpmock.GraphQL(`query UserProjectV2List\b`),
					httpmock.StringResponse(`
					{ "data": { "viewer": { "projectsV2": {
						"nodes": [
							{ "title": "Monalisa", "id": "MONALISAID", "resourcePath": "/users/MONALISA/projects/1"  }
						],
						"pageInfo": { "hasNextPage": false }
					} } } }`))
			},
			wantsBrowse: "https://github.com/OWNER/REPO/issues/new?body=&projects=OWNER%2FREPO%2F1",
			wantsStderr: "Opening https://github.com/OWNER/REPO/issues/new in your browser.\n",
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
			wantsStderr: "Opening https://github.com/OWNER/REPO/issues/new/choose in your browser.\n",
		},
		{
			name: "too long body",
			opts: CreateOptions{
				WebMode: true,
				Body:    strings.Repeat("A", 9216),
			},
			wantsErr: "cannot open in browser: maximum URL length exceeded",
		},
		{
			name: "editor",
			httpStubs: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`
			{ "data": { "repository": {
				"id": "REPOID",
				"hasIssuesEnabled": true
			} } }`))
				r.Register(
					httpmock.GraphQL(`mutation IssueCreate\b`),
					httpmock.GraphQLMutation(`
		{ "data": { "createIssue": { "issue": {
			"URL": "https://github.com/OWNER/REPO/issues/12"
		} } } }
	`, func(inputs map[string]interface{}) {
						assert.Equal(t, "title", inputs["title"])
						assert.Equal(t, "body", inputs["body"])
					}))
			},
			opts: CreateOptions{
				EditorMode:       true,
				TitledEditSurvey: func(string, string) (string, string, error) { return "title", "body", nil },
			},
			wantsStdout: "https://github.com/OWNER/REPO/issues/12\n",
			wantsStderr: "\nCreating issue in OWNER/REPO\n\n",
		},
		{
			name: "editor and template",
			httpStubs: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`
			{ "data": { "repository": {
				"id": "REPOID",
				"hasIssuesEnabled": true
			} } }`))
				r.Register(
					httpmock.GraphQL(`query IssueTemplates\b`),
					httpmock.StringResponse(`
			{ "data": { "repository": { "issueTemplates": [
				{ "name": "Bug report",
				  "title": "bug: ",
				  "body": "Does not work :((" }
			] } } }`),
				)
				r.Register(
					httpmock.GraphQL(`mutation IssueCreate\b`),
					httpmock.GraphQLMutation(`
		{ "data": { "createIssue": { "issue": {
			"URL": "https://github.com/OWNER/REPO/issues/12"
		} } } }
	`, func(inputs map[string]interface{}) {
						assert.Equal(t, "bug: ", inputs["title"])
						assert.Equal(t, "Does not work :((", inputs["body"])
					}))
			},
			opts: CreateOptions{
				EditorMode:       true,
				Template:         "Bug report",
				TitledEditSurvey: func(title string, body string) (string, string, error) { return title, body, nil },
			},
			wantsStdout: "https://github.com/OWNER/REPO/issues/12\n",
			wantsStderr: "\nCreating issue in OWNER/REPO\n\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpReg := &httpmock.Registry{}
			defer httpReg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(httpReg)
			}

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(true)
			opts := &tt.opts
			opts.IO = ios
			opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: httpReg}, nil
			}
			opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			}
			browser := &browser.Stub{}
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

func runCommand(rt http.RoundTripper, isTTY bool, cli string, pm *prompter.PrompterMock) (*test.CmdOut, error) {
	return runCommandWithRootDirOverridden(rt, isTTY, cli, "", pm)
}

func runCommandWithRootDirOverridden(rt http.RoundTripper, isTTY bool, cli string, rootDir string, pm *prompter.PrompterMock) (*test.CmdOut, error) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(isTTY)
	ios.SetStdinTTY(isTTY)
	ios.SetStderrTTY(isTTY)

	browser := &browser.Stub{}
	factory := &cmdutil.Factory{
		IOStreams: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		Config: func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
		Browser:  browser,
		Prompter: pm,
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
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

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

	output, err := runCommand(http, true, `-t hello -b "cash rules everything around me"`, nil)
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

	pm := &prompter.PrompterMock{}
	pm.InputFunc = func(p, d string) (string, error) {
		if p == "Title (required)" {
			return d, nil
		} else {
			return "", prompter.NoSuchPromptErr(p)
		}
	}
	pm.MarkdownEditorFunc = func(p, d string, ba bool) (string, error) {
		if p == "Body" {
			return d, nil
		} else {
			return "", prompter.NoSuchPromptErr(p)
		}
	}
	pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
		if p == "What's next?" {
			return prompter.IndexFor(opts, "Submit")
		} else {
			return -1, prompter.NoSuchPromptErr(p)
		}
	}

	tmpfile, err := os.CreateTemp(t.TempDir(), "testrecover*")
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

	output, err := runCommandWithRootDirOverridden(http, true, args, "", pm)
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

	pm := &prompter.PrompterMock{}
	pm.MarkdownEditorFunc = func(p, d string, ba bool) (string, error) {
		if p == "Body" {
			return d, nil
		} else {
			return "", prompter.NoSuchPromptErr(p)
		}
	}
	pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
		switch p {
		case "What's next?":
			return prompter.IndexFor(opts, "Submit")
		case "Choose a template":
			return prompter.IndexFor(opts, "Submit a request")
		default:
			return -1, prompter.NoSuchPromptErr(p)
		}
	}

	output, err := runCommandWithRootDirOverridden(http, true, `-t hello`, "./fixtures/repoWithNonLegacyIssueTemplates", pm)
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

	pm := &prompter.PrompterMock{}
	pm.InputFunc = func(p, d string) (string, error) {
		if p == "Title (required)" {
			return "hello", nil
		} else {
			return "", prompter.NoSuchPromptErr(p)
		}
	}
	pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
		if p == "What's next?" {
			return prompter.IndexFor(opts, "Continue in browser")
		} else {
			return -1, prompter.NoSuchPromptErr(p)
		}
	}

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	output, err := runCommand(http, true, `-b body`, pm)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, heredoc.Doc(`

		Creating issue in OWNER/REPO

		Opening https://github.com/OWNER/REPO/issues/new in your browser.
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
		httpmock.GraphQL(`query RepositoryProjectV2List\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "projectsV2": {
			"nodes": [],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query OrganizationProjectV2List\b`),
		httpmock.StringResponse(`
		{	"data": { "organization": { "projectsV2": {
			"nodes": [],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query UserProjectV2List\b`),
		httpmock.StringResponse(`
		{	"data": { "viewer": { "projectsV2": {
			"nodes": [],
			"pageInfo": { "hasNextPage": false }
		} } } }
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
			assert.NotContains(t, inputs, "userIds")
			assert.NotContains(t, inputs, "teamIds")
			assert.NotContains(t, inputs, "projectV2Ids")
		}))

	output, err := runCommand(http, true, `-t TITLE -b BODY -a monalisa -l bug -l todo -p roadmap -m 'big one.oh'`, nil)
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

	_, err := runCommand(http, true, `-t heres -b johnny`, nil)
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

	output, err := runCommand(http, true, `-a @me -a someoneelse -t hello -b "cash rules everything around me"`, nil)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "https://github.com/OWNER/REPO/issues/12\n", output.String())
}

func TestIssueCreate_projectsV2(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.StubRepoInfoResponse("OWNER", "REPO", "main")
	http.Register(
		httpmock.GraphQL(`query RepositoryProjectList\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "projects": {
			"nodes": [],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query OrganizationProjectList\b`),
		httpmock.StringResponse(`
		{	"data": { "organization": { "projects": {
			"nodes": [],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query RepositoryProjectV2List\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "projectsV2": {
			"nodes": [
				{ "title": "CleanupV2", "id": "CLEANUPV2ID" },
				{ "title": "RoadmapV2", "id": "ROADMAPV2ID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query OrganizationProjectV2List\b`),
		httpmock.StringResponse(`
		{ "data": { "organization": { "projectsV2": {
			"nodes": [
				{ "title": "TriageV2", "id": "TRIAGEV2ID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query UserProjectV2List\b`),
		httpmock.StringResponse(`
		{ "data": { "viewer": { "projectsV2": {
			"nodes": [
				{ "title": "MonalisaV2", "id": "MONALISAV2ID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`mutation IssueCreate\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "createIssue": { "issue": {
			"id": "Issue#1",
			"URL": "https://github.com/OWNER/REPO/issues/12"
		} } } }
	`, func(inputs map[string]interface{}) {
			assert.Equal(t, "TITLE", inputs["title"])
			assert.Equal(t, "BODY", inputs["body"])
			assert.Nil(t, inputs["projectIds"])
			assert.NotContains(t, inputs, "projectV2Ids")
		}))
	http.Register(
		httpmock.GraphQL(`mutation UpdateProjectV2Items\b`),
		httpmock.GraphQLQuery(`
			{ "data": { "add_000": { "item": {
				"id": "1"
			} } } }
	`, func(mutations string, inputs map[string]interface{}) {
			variables, err := json.Marshal(inputs)
			assert.NoError(t, err)
			expectedMutations := "mutation UpdateProjectV2Items($input_000: AddProjectV2ItemByIdInput!) {add_000: addProjectV2ItemById(input: $input_000) { item { id } }}"
			expectedVariables := `{"input_000":{"contentId":"Issue#1","projectId":"ROADMAPV2ID"}}`
			assert.Equal(t, expectedMutations, mutations)
			assert.Equal(t, expectedVariables, string(variables))
		}))

	output, err := runCommand(http, true, `-t TITLE -b BODY -p roadmapv2`, nil)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "https://github.com/OWNER/REPO/issues/12\n", output.String())
}
