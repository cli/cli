package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

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
)

func eq(t *testing.T, got interface{}, expected interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected: %v, got: %v", expected, got)
	}
}

func runCommand(rt http.RoundTripper, isTTY bool, cli string) (*test.CmdOut, error) {
	return runCommandWithRootDirOverridden(rt, isTTY, cli, "")
}

func runCommandWithRootDirOverridden(rt http.RoundTripper, isTTY bool, cli string, rootDir string) (*test.CmdOut, error) {
	io, _, stdout, stderr := iostreams.Test()
	io.SetStdoutTTY(isTTY)
	io.SetStdinTTY(isTTY)
	io.SetStderrTTY(isTTY)

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
		OutBuf: stdout,
		ErrBuf: stderr,
	}, err
}

func TestIssueCreate_nontty_error(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	_, err := runCommand(http, false, `-t hello`)
	if err == nil {
		t.Fatal("expected error running command `issue create`")
	}

	assert.Equal(t, "must provide --title and --body when not running interactively", err.Error())
}

func TestIssueCreate(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": {
			"id": "REPOID",
			"hasIssuesEnabled": true
		} } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "createIssue": { "issue": {
			"URL": "https://github.com/OWNER/REPO/issues/12"
		} } } }
	`))

	output, err := runCommand(http, true, `-t hello -b "cash rules everything around me"`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[1].Body)
	reqBody := struct {
		Variables struct {
			Input struct {
				RepositoryID string
				Title        string
				Body         string
			}
		}
	}{}
	_ = json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.Input.RepositoryID, "REPOID")
	eq(t, reqBody.Variables.Input.Title, "hello")
	eq(t, reqBody.Variables.Input.Body, "cash rules everything around me")

	eq(t, output.String(), "https://github.com/OWNER/REPO/issues/12\n")
}

func TestIssueCreate_recover(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.StubResponse(200, bytes.NewBufferString(`
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
			eq(t, inputs["title"], "recovered title")
			eq(t, inputs["body"], "recovered body")
			eq(t, inputs["labelIds"], []interface{}{"BUGID", "TODOID"})
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

	tmpfile, err := ioutil.TempFile(os.TempDir(), "testrecover*")
	assert.NoError(t, err)

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

	eq(t, output.String(), "https://github.com/OWNER/REPO/issues/12\n")
}

func TestIssueCreate_nonLegacyTemplate(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": {
			"id": "REPOID",
			"hasIssuesEnabled": true
		} } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "createIssue": { "issue": {
			"URL": "https://github.com/OWNER/REPO/issues/12"
		} } } }
	`))

	as, teardown := prompt.InitAskStubber()
	defer teardown()

	// template
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "index",
			Value: 1,
		},
	})
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

	bodyBytes, _ := ioutil.ReadAll(http.Requests[1].Body)
	reqBody := struct {
		Variables struct {
			Input struct {
				RepositoryID string
				Title        string
				Body         string
			}
		}
	}{}
	_ = json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.Input.RepositoryID, "REPOID")
	eq(t, reqBody.Variables.Input.Title, "hello")
	eq(t, reqBody.Variables.Input.Body, "I have a suggestion for an enhancement")

	eq(t, output.String(), "https://github.com/OWNER/REPO/issues/12\n")
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
			eq(t, inputs["title"], "TITLE")
			eq(t, inputs["body"], "BODY")
			eq(t, inputs["assigneeIds"], []interface{}{"MONAID"})
			eq(t, inputs["labelIds"], []interface{}{"BUGID", "TODOID"})
			eq(t, inputs["projectIds"], []interface{}{"ROADMAPID"})
			eq(t, inputs["milestoneId"], "BIGONEID")
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

	eq(t, output.String(), "https://github.com/OWNER/REPO/issues/12\n")
}

func TestIssueCreate_disabledIssues(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": {
			"id": "REPOID",
			"hasIssuesEnabled": false
		} } }
	`))

	_, err := runCommand(http, true, `-t heres -b johnny`)
	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
		t.Errorf("error running command `issue create`: %v", err)
	}
}

func TestIssueCreate_web(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := runCommand(http, true, `--web`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/OWNER/REPO/issues/new")
	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening github.com/OWNER/REPO/issues/new in your browser.\n")
}

func TestIssueCreate_webTitleBody(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := runCommand(http, true, `-w -t mytitle -b mybody`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := strings.ReplaceAll(seenCmd.Args[len(seenCmd.Args)-1], "^", "")
	eq(t, url, "https://github.com/OWNER/REPO/issues/new?body=mybody&title=mytitle")
	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening github.com/OWNER/REPO/issues/new in your browser.\n")
}
