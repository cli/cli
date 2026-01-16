package reviewthread

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func runUnresolveCommand(rt http.RoundTripper, isTTY bool, cli string) (*test.CmdOut, error) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(isTTY)
	ios.SetStdinTTY(isTTY)
	ios.SetStderrTTY(isTTY)

	factory := &cmdutil.Factory{
		IOStreams: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
	}

	cmd := NewCmdUnresolve(factory, nil)

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
		OutBuf: stdout,
		ErrBuf: stderr,
	}, err
}

func TestUnresolveNoArgs(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	_, err := runUnresolveCommand(http, true, "")

	assert.EqualError(t, err, "thread ID required")
}

func TestUnresolve(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`mutation UnresolveReviewThread\b`),
		httpmock.StringResponse(`{"data": {"unresolveReviewThread": {"thread": {"id": "THREAD_ID", "isResolved": false}}}}`),
	)

	output, err := runUnresolveCommand(http, true, "THREAD_ID")
	assert.NoError(t, err)
	assert.Equal(t, "✓ Unresolved review thread\n", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestUnresolve_apiError(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`mutation UnresolveReviewThread\b`),
		httpmock.StringResponse(`{"errors": [{"message": "Could not unresolve thread"}]}`),
	)

	_, err := runUnresolveCommand(http, true, "INVALID_THREAD_ID")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unresolve thread")
}
