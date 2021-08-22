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
			cs.Register(`git clone`, 0, "", func(s []string) {
				assert.Equal(t, tt.want, strings.Join(s, " "))
			})

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
