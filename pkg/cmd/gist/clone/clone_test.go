package clone

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func runCloneCommand(httpClient *http.Client, cli string) (*test.CmdOut, error) {
	ios, stdin, stdout, stderr := iostreams.Test()
	fac := &cmdutil.Factory{
		IOStreams: ios,
		HttpClient: func() (*http.Client, error) {
			return httpClient, nil
		},
		Config: func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		},
		GitClient: &git.Client{
			GhPath:  "some/path/gh",
			GitPath: "some/path/git",
		},
	}

	cmd := NewCmdClone(fac, nil)

	argv, err := shlex.Split(cli)
	cmd.SetArgs(argv)

	cmd.SetIn(stdin)
	cmd.SetOut(stderr)
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

func Test_GistClone(t *testing.T) {
	tests := []struct {
		name string
		args string
		want string
	}{
		{
			name: "shorthand",
			args: "GIST",
			want: "git clone https://gist.github.com/GIST.git",
		},
		{
			name: "shorthand with directory",
			args: "GIST target_directory",
			want: "git clone https://gist.github.com/GIST.git target_directory",
		},
		{
			name: "clone arguments",
			args: "GIST -- -o upstream --depth 1",
			want: "git clone -o upstream --depth 1 https://gist.github.com/GIST.git",
		},
		{
			name: "clone arguments with directory",
			args: "GIST target_directory -- -o upstream --depth 1",
			want: "git clone -o upstream --depth 1 https://gist.github.com/GIST.git target_directory",
		},
		{
			name: "HTTPS URL",
			args: "https://gist.github.com/OWNER/GIST",
			want: "git clone https://gist.github.com/OWNER/GIST",
		},
		{
			name: "SSH URL",
			args: "git@gist.github.com:GIST.git",
			want: "git clone git@gist.github.com:GIST.git",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			httpClient := &http.Client{Transport: reg}

			cs, restore := run.Stub()
			defer restore(t)
			cs.Register(tt.want, 0, "")

			output, err := runCloneCommand(httpClient, tt.args)
			if err != nil {
				t.Fatalf("error running command `gist clone`: %v", err)
			}

			assert.Equal(t, "", output.String())
			assert.Equal(t, "", output.Stderr())
		})
	}
}

func Test_GistClone_flagError(t *testing.T) {
	_, err := runCloneCommand(nil, "--depth 1 GIST")
	if err == nil || err.Error() != "unknown flag: --depth\nSeparate git clone flags with '--'." {
		t.Errorf("unexpected error %v", err)
	}
}

func Test_formatRemoteURL(t *testing.T) {
	type args struct {
		hostname string
		gistID   string
		protocol string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "github.com HTTPS",
			args: args{
				hostname: "github.com",
				protocol: "https",
				gistID:   "ID",
			},
			want: "https://gist.github.com/ID.git",
		},
		{
			name: "github.com SSH",
			args: args{
				hostname: "github.com",
				protocol: "ssh",
				gistID:   "ID",
			},
			want: "git@gist.github.com:ID.git",
		},
		{
			name: "Enterprise HTTPS",
			args: args{
				hostname: "acme.org",
				protocol: "https",
				gistID:   "ID",
			},
			want: "https://acme.org/gist/ID.git",
		},
		{
			name: "Enterprise SSH",
			args: args{
				hostname: "acme.org",
				protocol: "ssh",
				gistID:   "ID",
			},
			want: "git@acme.org:gist/ID.git",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatRemoteURL(tt.args.hostname, tt.args.gistID, tt.args.protocol); got != tt.want {
				t.Errorf("formatRemoteURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
