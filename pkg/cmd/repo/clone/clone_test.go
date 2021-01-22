package clone

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/run"
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
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			} else {
				assert.NoError(t, err)
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
		{
			name: "clone wiki",
			args: "Owner/Repo.wiki",
			want: "git clone https://github.com/OWNER/REPO.wiki.git",
		},
		{
			name: "wiki URL",
			args: "https://github.com/owner/repo.wiki",
			want: "git clone https://github.com/OWNER/REPO.wiki.git",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(
				httpmock.GraphQL(`query RepositoryInfo\b`),
				httpmock.StringResponse(`
				{ "data": { "repository": {
					"name": "REPO",
					"owner": {
						"login": "OWNER"
					},
					"hasWikiEnabled": true
				} } }
				`))

			httpClient := &http.Client{Transport: reg}

			cs, restore := run.Stub()
			defer restore(t)
			cs.Register(`git clone`, 0, "", func(s []string) {
				assert.Equal(t, tt.want, strings.Join(s, " "))
			})

			output, err := runCloneCommand(httpClient, tt.args)
			if err != nil {
				t.Fatalf("error running command `repo clone`: %v", err)
			}

			assert.Equal(t, "", output.String())
			assert.Equal(t, "", output.Stderr())
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
						},
						"defaultBranchRef": {
							"name": "trunk"
						}
					}
				} } }
				`))

	httpClient := &http.Client{Transport: reg}

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git clone https://github.com/OWNER/REPO.git`, 0, "")
	cs.Register(`git -C REPO remote add -t trunk -f upstream https://github.com/hubot/ORIG.git`, 0, "")

	_, err := runCloneCommand(httpClient, "OWNER/REPO")
	if err != nil {
		t.Fatalf("error running command `repo clone`: %v", err)
	}
}

func Test_RepoClone_withoutUsername(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
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

	httpClient := &http.Client{Transport: reg}

	cs, restore := run.Stub()
	defer restore(t)
	cs.Register(`git clone https://github\.com/OWNER/REPO\.git`, 0, "")

	output, err := runCloneCommand(httpClient, "REPO")
	if err != nil {
		t.Fatalf("error running command `repo clone`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}
