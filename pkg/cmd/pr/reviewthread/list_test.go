package reviewthread

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func runListCommand(rt http.RoundTripper, isTTY bool, cli string) (*test.CmdOut, error) {
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

	cmd := NewCmdList(factory, nil)

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

func TestList(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	baseRepo := ghrepo.New("OWNER", "REPO")
	pr := &api.PullRequest{
		Number: 123,
	}
	shared.StubFinderForRunCommandStyleTests(t, "123", pr, baseRepo)

	http.Register(
		httpmock.GraphQL(`query ListReviewThreads\b`),
		httpmock.StringResponse(`{
			"data": {
				"repository": {
					"pullRequest": {
						"reviewThreads": {
							"nodes": [
								{
									"id": "THREAD_1",
									"isResolved": false,
									"comments": {
										"nodes": [
											{
												"path": "src/main.go",
												"line": 42,
												"body": "Please fix this"
											}
										]
									}
								},
								{
									"id": "THREAD_2",
									"isResolved": true,
									"comments": {
										"nodes": [
											{
												"path": "src/test.go",
												"line": 10,
												"body": "Already fixed"
											}
										]
									}
								}
							]
						}
					}
				}
			}
		}`),
	)

	output, err := runListCommand(http, true, "123")
	assert.NoError(t, err)
	assert.Contains(t, output.String(), "THREAD_1")
	assert.Contains(t, output.String(), "THREAD_2")
	assert.Contains(t, output.String(), "src/main.go:42")
	assert.Contains(t, output.String(), "src/test.go:10")
	assert.Contains(t, output.String(), "unresolved")
	assert.Contains(t, output.String(), "resolved")
}

func TestList_unresolvedOnly(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	baseRepo := ghrepo.New("OWNER", "REPO")
	pr := &api.PullRequest{
		Number: 123,
	}
	shared.StubFinderForRunCommandStyleTests(t, "123", pr, baseRepo)

	http.Register(
		httpmock.GraphQL(`query ListReviewThreads\b`),
		httpmock.StringResponse(`{
			"data": {
				"repository": {
					"pullRequest": {
						"reviewThreads": {
							"nodes": [
								{
									"id": "THREAD_1",
									"isResolved": false,
									"comments": {
										"nodes": [
											{
												"path": "src/main.go",
												"line": 42,
												"body": "Please fix this"
											}
										]
									}
								},
								{
									"id": "THREAD_2",
									"isResolved": true,
									"comments": {
										"nodes": [
											{
												"path": "src/test.go",
												"line": 10,
												"body": "Already fixed"
											}
										]
									}
								}
							]
						}
					}
				}
			}
		}`),
	)

	output, err := runListCommand(http, true, "123 --unresolved")
	assert.NoError(t, err)
	assert.Contains(t, output.String(), "THREAD_1")
	assert.NotContains(t, output.String(), "THREAD_2")
	assert.Contains(t, output.String(), "[unresolved]")
	assert.NotContains(t, output.String(), "[resolved]")
}

func TestList_noThreads(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	baseRepo := ghrepo.New("OWNER", "REPO")
	pr := &api.PullRequest{
		Number: 123,
	}
	shared.StubFinderForRunCommandStyleTests(t, "123", pr, baseRepo)

	http.Register(
		httpmock.GraphQL(`query ListReviewThreads\b`),
		httpmock.StringResponse(`{
			"data": {
				"repository": {
					"pullRequest": {
						"reviewThreads": {
							"nodes": []
						}
					}
				}
			}
		}`),
	)

	output, err := runListCommand(http, true, "123")
	assert.NoError(t, err)
	assert.Contains(t, output.String(), "No review threads found")
}

func TestList_noUnresolvedThreads(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	baseRepo := ghrepo.New("OWNER", "REPO")
	pr := &api.PullRequest{
		Number: 123,
	}
	shared.StubFinderForRunCommandStyleTests(t, "123", pr, baseRepo)

	http.Register(
		httpmock.GraphQL(`query ListReviewThreads\b`),
		httpmock.StringResponse(`{
			"data": {
				"repository": {
					"pullRequest": {
						"reviewThreads": {
							"nodes": [
								{
									"id": "THREAD_1",
									"isResolved": true,
									"comments": {
										"nodes": [
											{
												"path": "src/main.go",
												"line": 42,
												"body": "Already fixed"
											}
										]
									}
								}
							]
						}
					}
				}
			}
		}`),
	)

	output, err := runListCommand(http, true, "123 --unresolved")
	assert.NoError(t, err)
	assert.Contains(t, output.String(), "No unresolved review threads found")
}
