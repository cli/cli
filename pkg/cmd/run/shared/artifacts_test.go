package shared

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadWorkflowArtifactsPageinates(t *testing.T) {
	testRepoOwner := "OWNER"
	testRepoName := "REPO"
	testRunId := "1234567890"

	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	firstReq := httpmock.QueryMatcher(
		"GET",
		fmt.Sprintf("repos/%s/%s/actions/runs/%s/artifacts", testRepoOwner, testRepoName, testRunId),
		url.Values{"per_page": []string{"100"}},
	)

	firstArtifact := Artifact{
		Name:        "artifact-0",
		Size:        2,
		DownloadURL: fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/artifacts/987654320/zip", testRepoOwner, testRepoName),
		Expired:     true,
	}
	firstRes := httpmock.JSONResponse(artifactsPayload{Artifacts: []Artifact{firstArtifact}})
	testLinkUri := fmt.Sprintf("repositories/123456789/actions/runs/%s/artifacts", testRunId)
	testLinkUrl := fmt.Sprintf("https://api.github.com/%s", testLinkUri)
	firstRes = httpmock.WithHeader(
		firstRes,
		"Link",
		fmt.Sprintf(`<%s?per_page=100&page=2>; rel="next", <%s?per_page=100&page=2>; rel="last"`, testLinkUrl, testLinkUrl),
	)

	secondReq := httpmock.QueryMatcher(
		"GET",
		testLinkUri,
		url.Values{"per_page": []string{"100"}, "page": []string{"2"}},
	)

	secondArtifact := Artifact{
		Name:        "artifact-1",
		Size:        2,
		DownloadURL: fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/artifacts/987654321/zip", testRepoOwner, testRepoName),
		Expired:     false,
	}
	secondRes := httpmock.JSONResponse(artifactsPayload{Artifacts: []Artifact{secondArtifact}})

	reg.Register(firstReq, firstRes)
	reg.Register(secondReq, secondRes)

	client, err := httpmock.RESTClientFunc(reg)("github.com")
	require.NoError(t, err)
	repo := ghrepo.New(testRepoOwner, testRepoName)

	result, err := ListArtifacts(context.Background(), client, repo, testRunId)
	assert.NoError(t, err)
	assert.Equal(t, []Artifact{firstArtifact, secondArtifact}, result)
}
