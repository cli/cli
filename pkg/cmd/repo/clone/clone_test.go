package clone

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func runCloneCommand(httpClient *http.Client, cli string) (*test.CmdOut, error) {
	io, stdin, stdout, stderr := iostreams.Test()
	fac := &cmdutil.Factory{
		IOStreams: io,
		HttpClient: func() (*http.Client, error) {
			return httpClient, nil
		},
		Config: func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		},
	}

	cmd := NewCmdClone(fac, nil)

	argv, err := shlex.Split(cli)
	cmd.SetArgs(argv)

	cmd.SetIn(stdin)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	if err != nil {
		panic(err)
	}

	_, err = cmd.ExecuteC()

	if err != nil {
		return nil, err
	}

	return &test.CmdOut{OutBuf: stdout, ErrBuf: stderr}, nil
}

func Test_RepoClone(t *testing.T) {
	tests := []struct {
		name string
		args string
		want string
	}{
		{
			name: "shorthand",
			args: "OWNER/REPO",
			want: "git clone https://github.com/OWNER/REPO.git",
		},
		{
			name: "shorthand with directory",
			args: "OWNER/REPO target_directory",
			want: "git clone https://github.com/OWNER/REPO.git target_directory",
		},
		{
			name: "clone arguments",
			args: "OWNER/REPO -- -o upstream --depth 1",
			want: "git clone -o upstream --depth 1 https://github.com/OWNER/REPO.git",
		},
		{
			name: "clone arguments with directory",
			args: "OWNER/REPO target_directory -- -o upstream --depth 1",
			want: "git clone -o upstream --depth 1 https://github.com/OWNER/REPO.git target_directory",
		},
		{
			name: "HTTPS URL",
			args: "https://github.com/OWNER/REPO",
			want: "git clone https://github.com/OWNER/REPO.git",
		},
		{
			name: "SSH URL",
			args: "git@github.com:OWNER/REPO.git",
			want: "git clone git@github.com:OWNER/REPO.git",
		},
		{
			name: "Non-canonical capitalization",
			args: "Owner/Repo",
			want: "git clone https://github.com/OWNER/REPO.git",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			reg.Register(
				httpmock.GraphQL(`query RepositoryInfo\b`),
				httpmock.StringResponse(`
				{ "data": { "repository": {
					"name": "REPO",
					"owner": {
						"login": "OWNER"
					}
				} } }
				`))

			httpClient := &http.Client{Transport: reg}

			cs, restore := test.InitCmdStubber()
			defer restore()

			cs.Stub("") // git clone

			output, err := runCloneCommand(httpClient, tt.args)
			if err != nil {
				t.Fatalf("error running command `repo clone`: %v", err)
			}

			assert.Equal(t, "", output.String())
			assert.Equal(t, "", output.Stderr())
			assert.Equal(t, 1, cs.Count)
			assert.Equal(t, tt.want, strings.Join(cs.Calls[0].Args, " "))
			reg.Verify(t)
		})
	}
}

func Test_RepoClone_hasParent(t *testing.T) {
	reg := &httpmock.Registry{}
	reg.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
				{ "data": { "repository": {
					"name": "REPO",
					"owner": {
						"login": "OWNER"
					},
					"parent": {
						"name": "ORIG",
						"owner": {
							"login": "hubot"
						}
					}
				} } }
				`))

	httpClient := &http.Client{Transport: reg}

	cs, restore := test.InitCmdStubber()
	defer restore()

	cs.Stub("") // git clone
	cs.Stub("") // git remote add

	_, err := runCloneCommand(httpClient, "OWNER/REPO")
	if err != nil {
		t.Fatalf("error running command `repo clone`: %v", err)
	}

	assert.Equal(t, 2, cs.Count)
	assert.Equal(t, "git -C REPO remote add -f upstream https://github.com/hubot/ORIG.git", strings.Join(cs.Calls[1].Args, " "))
}

func Test_RepoClone_withoutUsername(t *testing.T) {
	reg := &httpmock.Registry{}
	reg.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`
		{ "data": { "viewer": {
			"login": "OWNER"
		}}}`))
	reg.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
				{ "data": { "repository": {
					"name": "REPO",
					"owner": {
						"login": "OWNER"
					}
				} } }
				`))
	reg.Register(
		httpmock.GraphQL(`query RepositoryFindParent\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": {
			"parent": null
		} } }`))

	httpClient := &http.Client{Transport: reg}

	cs, restore := test.InitCmdStubber()
	defer restore()

	cs.Stub("") // git clone

	output, err := runCloneCommand(httpClient, "REPO")
	if err != nil {
		t.Fatalf("error running command `repo clone`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
	assert.Equal(t, 1, cs.Count)
	assert.Equal(t, "git clone https://github.com/OWNER/REPO.git", strings.Join(cs.Calls[0].Args, " "))
}

func Test_RepoClone_flagError(t *testing.T) {
	_, err := runCloneCommand(nil, "--depth 1 OWNER/REPO")
	if err == nil || err.Error() != "unknown flag: --depth\nSeparate git clone flags with '--'." {
		t.Errorf("unexpected error %v", err)
	}
}
