package command

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/context"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/test"
)

func TestRepoFork_already_forked(t *testing.T) {
	initContext = func() context.Context {
		ctx := context.NewBlank()
		ctx.SetBaseRepo("OWNER/REPO")
		ctx.SetBranch("master")
		ctx.SetRemotes(map[string]string{
			"origin": "OWNER/REPO",
		})
		return ctx
	}
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	defer http.StubWithFixture(200, "forkResult.json")()

	output, err := RunCommand(repoForkCmd, "repo fork --remote=false")
	if err != nil {
		t.Errorf("got unexpected error: %v", err)
	}
	r := regexp.MustCompile(`someone/REPO already exists`)
	if !r.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
		return
	}
}

func TestRepoFork_reuseRemote(t *testing.T) {
	initContext = func() context.Context {
		ctx := context.NewBlank()
		ctx.SetBaseRepo("OWNER/REPO")
		ctx.SetBranch("master")
		ctx.SetRemotes(map[string]string{
			"upstream": "OWNER/REPO",
			"origin":   "someone/REPO",
		})
		return ctx
	}
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	defer http.StubWithFixture(200, "forkResult.json")()

	output, err := RunCommand(repoForkCmd, "repo fork")
	if err != nil {
		t.Errorf("got unexpected error: %v", err)
	}
	if !strings.Contains(output.String(), "Using existing remote origin") {
		t.Errorf("output did not match: %q", output)
		return
	}
}

func stubSince(d time.Duration) func() {
	originalSince := Since
	Since = func(t time.Time) time.Duration {
		return d
	}
	return func() {
		Since = originalSince
	}
}

func TestRepoFork_in_parent(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubSince(2 * time.Second)()
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	defer http.StubWithFixture(200, "forkResult.json")()

	output, err := RunCommand(repoForkCmd, "repo fork --remote=false")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	eq(t, output.Stderr(), "")

	r := regexp.MustCompile(`Created fork someone/REPO`)
	if !r.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
		return
	}
}

func TestRepoFork_outside(t *testing.T) {
	tests := []struct {
		name string
		args string
	}{
		{
			name: "url arg",
			args: "repo fork --clone=false http://github.com/OWNER/REPO.git",
		},
		{
			name: "full name arg",
			args: "repo fork --clone=false OWNER/REPO",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer stubSince(2 * time.Second)()
			http := initFakeHTTP()
			defer http.StubWithFixture(200, "forkResult.json")()

			output, err := RunCommand(repoForkCmd, tt.args)
			if err != nil {
				t.Errorf("error running command `repo fork`: %v", err)
			}

			eq(t, output.Stderr(), "")

			r := regexp.MustCompile(`Created fork someone/REPO`)
			if !r.MatchString(output.String()) {
				t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
				return
			}
		})
	}
}

func TestRepoFork_in_parent_yes(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubSince(2 * time.Second)()
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	defer http.StubWithFixture(200, "forkResult.json")()

	var seenCmds []*exec.Cmd
	defer run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmds = append(seenCmds, cmd)
		return &test.OutputStub{}
	})()

	output, err := RunCommand(repoForkCmd, "repo fork --remote")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	expectedCmds := []string{
		"git remote rename origin upstream",
		"git remote add -f origin https://github.com/someone/REPO.git",
	}

	for x, cmd := range seenCmds {
		eq(t, strings.Join(cmd.Args, " "), expectedCmds[x])
	}

	eq(t, output.Stderr(), "")

	test.ExpectLines(t, output.String(),
		"Created fork someone/REPO",
		"Added remote origin")
}

func TestRepoFork_outside_yes(t *testing.T) {
	defer stubSince(2 * time.Second)()
	http := initFakeHTTP()
	defer http.StubWithFixture(200, "forkResult.json")()

	cs, restore := test.InitCmdStubber()
	defer restore()

	cs.Stub("") // git clone
	cs.Stub("") // git remote add

	output, err := RunCommand(repoForkCmd, "repo fork --clone OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	eq(t, output.Stderr(), "")

	eq(t, strings.Join(cs.Calls[0].Args, " "), "git clone https://github.com/someone/REPO.git")
	eq(t, strings.Join(cs.Calls[1].Args, " "), "git -C REPO remote add -f upstream https://github.com/OWNER/REPO.git")

	test.ExpectLines(t, output.String(),
		"Created fork someone/REPO",
		"Cloned fork")
}

func TestRepoFork_outside_survey_yes(t *testing.T) {
	defer stubSince(2 * time.Second)()
	http := initFakeHTTP()
	defer http.StubWithFixture(200, "forkResult.json")()

	cs, restore := test.InitCmdStubber()
	defer restore()

	cs.Stub("") // git clone
	cs.Stub("") // git remote add

	oldConfirm := Confirm
	Confirm = func(_ string, result *bool) error {
		*result = true
		return nil
	}
	defer func() { Confirm = oldConfirm }()

	output, err := RunCommand(repoForkCmd, "repo fork OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	eq(t, output.Stderr(), "")

	eq(t, strings.Join(cs.Calls[0].Args, " "), "git clone https://github.com/someone/REPO.git")
	eq(t, strings.Join(cs.Calls[1].Args, " "), "git -C REPO remote add -f upstream https://github.com/OWNER/REPO.git")

	test.ExpectLines(t, output.String(),
		"Created fork someone/REPO",
		"Cloned fork")
}

func TestRepoFork_outside_survey_no(t *testing.T) {
	defer stubSince(2 * time.Second)()
	http := initFakeHTTP()
	defer http.StubWithFixture(200, "forkResult.json")()

	cmdRun := false
	defer run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		cmdRun = true
		return &test.OutputStub{}
	})()

	oldConfirm := Confirm
	Confirm = func(_ string, result *bool) error {
		*result = false
		return nil
	}
	defer func() { Confirm = oldConfirm }()

	output, err := RunCommand(repoForkCmd, "repo fork OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	eq(t, output.Stderr(), "")

	eq(t, cmdRun, false)

	r := regexp.MustCompile(`Created fork someone/REPO`)
	if !r.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
		return
	}
}

func TestRepoFork_in_parent_survey_yes(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubSince(2 * time.Second)()
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	defer http.StubWithFixture(200, "forkResult.json")()

	var seenCmds []*exec.Cmd
	defer run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmds = append(seenCmds, cmd)
		return &test.OutputStub{}
	})()

	oldConfirm := Confirm
	Confirm = func(_ string, result *bool) error {
		*result = true
		return nil
	}
	defer func() { Confirm = oldConfirm }()

	output, err := RunCommand(repoForkCmd, "repo fork")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	expectedCmds := []string{
		"git remote rename origin upstream",
		"git remote add -f origin https://github.com/someone/REPO.git",
	}

	for x, cmd := range seenCmds {
		eq(t, strings.Join(cmd.Args, " "), expectedCmds[x])
	}

	eq(t, output.Stderr(), "")

	test.ExpectLines(t, output.String(),
		"Created fork someone/REPO",
		"Renamed origin remote to upstream",
		"Added remote origin")
}

func TestRepoFork_in_parent_survey_no(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubSince(2 * time.Second)()
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	defer http.StubWithFixture(200, "forkResult.json")()

	cmdRun := false
	defer run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		cmdRun = true
		return &test.OutputStub{}
	})()

	oldConfirm := Confirm
	Confirm = func(_ string, result *bool) error {
		*result = false
		return nil
	}
	defer func() { Confirm = oldConfirm }()

	output, err := RunCommand(repoForkCmd, "repo fork")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	eq(t, output.Stderr(), "")

	eq(t, cmdRun, false)

	r := regexp.MustCompile(`Created fork someone/REPO`)
	if !r.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
		return
	}
}

func TestParseExtraArgs(t *testing.T) {
	type Wanted struct {
		args []string
		dir  string
	}
	tests := []struct {
		name string
		args []string
		want Wanted
	}{
		{
			name: "args and target",
			args: []string{"target_directory", "-o", "upstream", "--depth", "1"},
			want: Wanted{
				args: []string{"-o", "upstream", "--depth", "1"},
				dir:  "target_directory",
			},
		},
		{
			name: "only args",
			args: []string{"-o", "upstream", "--depth", "1"},
			want: Wanted{
				args: []string{"-o", "upstream", "--depth", "1"},
				dir:  "",
			},
		},
		{
			name: "only target",
			args: []string{"target_directory"},
			want: Wanted{
				args: []string{},
				dir:  "target_directory",
			},
		},
		{
			name: "no args",
			args: []string{},
			want: Wanted{
				args: []string{},
				dir:  "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, dir := parseCloneArgs(tt.args)
			got := Wanted{
				args: args,
				dir:  dir,
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}

}

func TestRepoClone(t *testing.T) {
	tests := []struct {
		name string
		args string
		want string
	}{
		{
			name: "shorthand",
			args: "repo clone OWNER/REPO",
			want: "git clone https://github.com/OWNER/REPO.git",
		},
		{
			name: "shorthand with directory",
			args: "repo clone OWNER/REPO target_directory",
			want: "git clone https://github.com/OWNER/REPO.git target_directory",
		},
		{
			name: "clone arguments",
			args: "repo clone OWNER/REPO -- -o upstream --depth 1",
			want: "git clone -o upstream --depth 1 https://github.com/OWNER/REPO.git",
		},
		{
			name: "clone arguments with directory",
			args: "repo clone OWNER/REPO target_directory -- -o upstream --depth 1",
			want: "git clone -o upstream --depth 1 https://github.com/OWNER/REPO.git target_directory",
		},
		{
			name: "HTTPS URL",
			args: "repo clone https://github.com/OWNER/REPO",
			want: "git clone https://github.com/OWNER/REPO",
		},
		{
			name: "SSH URL",
			args: "repo clone git@github.com:OWNER/REPO.git",
			want: "git clone git@github.com:OWNER/REPO.git",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			http := initFakeHTTP()
			http.StubResponse(200, bytes.NewBufferString(`
			{ "data": { "repository": {
				"parent": null
			} } }
			`))

			cs, restore := test.InitCmdStubber()
			defer restore()

			cs.Stub("") // git clone

			output, err := RunCommand(repoCloneCmd, tt.args)
			if err != nil {
				t.Fatalf("error running command `repo clone`: %v", err)
			}

			eq(t, output.String(), "")
			eq(t, output.Stderr(), "")
			eq(t, cs.Count, 1)
			eq(t, strings.Join(cs.Calls[0].Args, " "), tt.want)
		})
	}
}

func TestRepoClone_hasParent(t *testing.T) {
	http := initFakeHTTP()
	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": {
		"parent": {
			"owner": {"login": "hubot"},
			"name": "ORIG"
		}
	} } }
	`))

	cs, restore := test.InitCmdStubber()
	defer restore()

	cs.Stub("") // git clone
	cs.Stub("") // git remote add

	_, err := RunCommand(repoCloneCmd, "repo clone OWNER/REPO")
	if err != nil {
		t.Fatalf("error running command `repo clone`: %v", err)
	}

	eq(t, cs.Count, 2)
	eq(t, strings.Join(cs.Calls[1].Args, " "), "git -C REPO remote add -f upstream https://github.com/hubot/ORIG.git")
}

func TestRepoCreate(t *testing.T) {
	ctx := context.NewBlank()
	ctx.SetBranch("master")
	initContext = func() context.Context {
		return ctx
	}

	http := initFakeHTTP()
	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "createRepository": {
		"repository": {
			"id": "REPOID",
			"url": "https://github.com/OWNER/REPO",
			"name": "REPO",
			"owner": {
				"login": "OWNER"
			}
		}
	} } }
	`))

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(repoCreateCmd, "repo create REPO")
	if err != nil {
		t.Errorf("error running command `repo create`: %v", err)
	}

	eq(t, output.String(), "https://github.com/OWNER/REPO\n")
	eq(t, output.Stderr(), "")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	eq(t, strings.Join(seenCmd.Args, " "), "git remote add -f origin https://github.com/OWNER/REPO.git")

	var reqBody struct {
		Query     string
		Variables struct {
			Input map[string]interface{}
		}
	}

	if len(http.Requests) != 1 {
		t.Fatalf("expected 1 HTTP request, got %d", len(http.Requests))
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[0].Body)
	_ = json.Unmarshal(bodyBytes, &reqBody)
	if repoName := reqBody.Variables.Input["name"].(string); repoName != "REPO" {
		t.Errorf("expected %q, got %q", "REPO", repoName)
	}
	if repoVisibility := reqBody.Variables.Input["visibility"].(string); repoVisibility != "PRIVATE" {
		t.Errorf("expected %q, got %q", "PRIVATE", repoVisibility)
	}
	if _, ownerSet := reqBody.Variables.Input["ownerId"]; ownerSet {
		t.Error("expected ownerId not to be set")
	}
}

func TestRepoCreate_org(t *testing.T) {
	ctx := context.NewBlank()
	ctx.SetBranch("master")
	initContext = func() context.Context {
		return ctx
	}

	http := initFakeHTTP()
	http.StubResponse(200, bytes.NewBufferString(`
	{ "node_id": "ORGID"
	}
	`))
	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "createRepository": {
		"repository": {
			"id": "REPOID",
			"url": "https://github.com/ORG/REPO",
			"name": "REPO",
			"owner": {
				"login": "ORG"
			}
		}
	} } }
	`))

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(repoCreateCmd, "repo create ORG/REPO")
	if err != nil {
		t.Errorf("error running command `repo create`: %v", err)
	}

	eq(t, output.String(), "https://github.com/ORG/REPO\n")
	eq(t, output.Stderr(), "")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	eq(t, strings.Join(seenCmd.Args, " "), "git remote add -f origin https://github.com/ORG/REPO.git")

	var reqBody struct {
		Query     string
		Variables struct {
			Input map[string]interface{}
		}
	}

	if len(http.Requests) != 2 {
		t.Fatalf("expected 2 HTTP requests, got %d", len(http.Requests))
	}

	eq(t, http.Requests[0].URL.Path, "/users/ORG")

	bodyBytes, _ := ioutil.ReadAll(http.Requests[1].Body)
	_ = json.Unmarshal(bodyBytes, &reqBody)
	if orgID := reqBody.Variables.Input["ownerId"].(string); orgID != "ORGID" {
		t.Errorf("expected %q, got %q", "ORGID", orgID)
	}
	if _, teamSet := reqBody.Variables.Input["teamId"]; teamSet {
		t.Error("expected teamId not to be set")
	}
}

func TestRepoCreate_orgWithTeam(t *testing.T) {
	ctx := context.NewBlank()
	ctx.SetBranch("master")
	initContext = func() context.Context {
		return ctx
	}

	http := initFakeHTTP()
	http.StubResponse(200, bytes.NewBufferString(`
	{ "node_id": "TEAMID",
	  "organization": { "node_id": "ORGID" }
	}
	`))
	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "createRepository": {
		"repository": {
			"id": "REPOID",
			"url": "https://github.com/ORG/REPO",
			"name": "REPO",
			"owner": {
				"login": "ORG"
			}
		}
	} } }
	`))

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(repoCreateCmd, "repo create ORG/REPO --team monkeys")
	if err != nil {
		t.Errorf("error running command `repo create`: %v", err)
	}

	eq(t, output.String(), "https://github.com/ORG/REPO\n")
	eq(t, output.Stderr(), "")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	eq(t, strings.Join(seenCmd.Args, " "), "git remote add -f origin https://github.com/ORG/REPO.git")

	var reqBody struct {
		Query     string
		Variables struct {
			Input map[string]interface{}
		}
	}

	if len(http.Requests) != 2 {
		t.Fatalf("expected 2 HTTP requests, got %d", len(http.Requests))
	}

	eq(t, http.Requests[0].URL.Path, "/orgs/ORG/teams/monkeys")

	bodyBytes, _ := ioutil.ReadAll(http.Requests[1].Body)
	_ = json.Unmarshal(bodyBytes, &reqBody)
	if orgID := reqBody.Variables.Input["ownerId"].(string); orgID != "ORGID" {
		t.Errorf("expected %q, got %q", "ORGID", orgID)
	}
	if teamID := reqBody.Variables.Input["teamId"].(string); teamID != "TEAMID" {
		t.Errorf("expected %q, got %q", "TEAMID", teamID)
	}
}

func TestRepoView_web(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
	{ }
	`))

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(repoViewCmd, "repo view -w")
	if err != nil {
		t.Errorf("error running command `repo view`: %v", err)
	}

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening github.com/OWNER/REPO in your browser.\n")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/OWNER/REPO")
}

func TestRepoView_web_ownerRepo(t *testing.T) {
	ctx := context.NewBlank()
	ctx.SetBranch("master")
	initContext = func() context.Context {
		return ctx
	}
	http := initFakeHTTP()
	http.StubResponse(200, bytes.NewBufferString(`
	{ }
	`))

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(repoViewCmd, "repo view -w cli/cli")
	if err != nil {
		t.Errorf("error running command `repo view`: %v", err)
	}

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening github.com/cli/cli in your browser.\n")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/cli/cli")
}

func TestRepoView_web_fullURL(t *testing.T) {
	ctx := context.NewBlank()
	ctx.SetBranch("master")
	initContext = func() context.Context {
		return ctx
	}
	http := initFakeHTTP()
	http.StubResponse(200, bytes.NewBufferString(`
	{ }
	`))
	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(repoViewCmd, "repo view -w https://github.com/cli/cli")
	if err != nil {
		t.Errorf("error running command `repo view`: %v", err)
	}

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening github.com/cli/cli in your browser.\n")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/cli/cli")
}

func TestRepoView(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": {
				"repository": {
		      "description": "social distancing"
		}}}
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "name": "readme.md",
		  "content": "IyB0cnVseSBjb29sIHJlYWRtZSBjaGVjayBpdCBvdXQ="}
	`))

	output, err := RunCommand(repoViewCmd, "repo view")
	if err != nil {
		t.Errorf("error running command `repo view`: %v", err)
	}

	test.ExpectLines(t, output.String(),
		"OWNER/REPO",
		"social distancing",
		"truly cool readme",
		"View this repository on GitHub: https://github.com/OWNER/REPO")

}

func TestRepoView_nonmarkdown_readme(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": {
				"repository": {
		      "description": "social distancing"
		}}}
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "name": "readme.org",
		  "content": "IyB0cnVseSBjb29sIHJlYWRtZSBjaGVjayBpdCBvdXQ="}
	`))

	output, err := RunCommand(repoViewCmd, "repo view")
	if err != nil {
		t.Errorf("error running command `repo view`: %v", err)
	}

	test.ExpectLines(t, output.String(),
		"OWNER/REPO",
		"social distancing",
		"# truly cool readme",
		"View this repository on GitHub: https://github.com/OWNER/REPO")
}

func TestRepoView_blanks(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString("{}"))
	http.StubResponse(200, bytes.NewBufferString("{}"))

	output, err := RunCommand(repoViewCmd, "repo view")
	if err != nil {
		t.Errorf("error running command `repo view`: %v", err)
	}

	test.ExpectLines(t, output.String(),
		"OWNER/REPO",
		"No description provided",
		"No README provided",
		"View this repository on GitHub: https://github.com/OWNER/REPO")
}
