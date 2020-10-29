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
	"github.com/stretchr/testify/require"
)

func TestNewCmdClone(t *testing.T) {
	testCases := []struct {
		name     string
		args     string
		wantOpts CloneOptions
		wantErr  string
	}{
		{
			name:    "no arguments",
			args:    "",
			wantErr: "cannot clone: repository argument required",
		},
		{
			name: "repo argument",
			args: "OWNER/REPO",
			wantOpts: CloneOptions{
				Repository: "OWNER/REPO",
				GitArgs:    []string{},
			},
		},
		{
			name: "directory argument",
			args: "OWNER/REPO mydir",
			wantOpts: CloneOptions{
				Repository: "OWNER/REPO",
				GitArgs:    []string{"mydir"},
			},
		},
		{
			name: "git clone arguments",
			args: "OWNER/REPO -- --depth 1 --recurse-submodules",
			wantOpts: CloneOptions{
				Repository: "OWNER/REPO",
				GitArgs:    []string{"--depth", "1", "--recurse-submodules"},
			},
		},
		{
			name:    "unknown argument",
			args:    "OWNER/REPO --depth 1",
			wantErr: "unknown flag: --depth\nSeparate git clone flags with '--'.",
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			io, stdin, stdout, stderr := iostreams.Test()
			fac := &cmdutil.Factory{IOStreams: io}

			var opts *CloneOptions
			cmd := NewCmdClone(fac, func(co *CloneOptions) error {
				opts = co
				return nil
			})

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)
			cmd.SetArgs(argv)

			cmd.SetIn(stdin)
			cmd.SetOut(stdout)
			cmd.SetErr(stderr)

			_, err = cmd.ExecuteC()
			if err != nil {
				assert.Equal(t, tt.wantErr, err.Error())
				return
			} else if tt.wantErr != "" {
				t.Errorf("expected error %q, got nil", tt.wantErr)
			}

			assert.Equal(t, "", stdout.String())
			assert.Equal(t, "", stderr.String())

			assert.Equal(t, tt.wantOpts.Repository, opts.Repository)
			assert.Equal(t, tt.wantOpts.GitArgs, opts.GitArgs)
		})
	}
}

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
