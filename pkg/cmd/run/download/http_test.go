package download

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_List(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/123/artifacts"),
		httpmock.StringResponse(`{
			"total_count": 2,
			"artifacts": [
				{"name": "artifact-1"},
				{"name": "artifact-2"}
			]
		}`))

	api := &apiPlatform{
		client: &http.Client{Transport: reg},
		repo:   ghrepo.New("OWNER", "REPO"),
	}
	artifacts, err := api.List("123")
	require.NoError(t, err)

	require.Equal(t, 2, len(artifacts))
	assert.Equal(t, "artifact-1", artifacts[0].Name)
	assert.Equal(t, "artifact-2", artifacts[1].Name)
}

func Test_List_perRepository(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/actions/artifacts"),
		httpmock.StringResponse(`{}`))

	api := &apiPlatform{
		client: &http.Client{Transport: reg},
		repo:   ghrepo.New("OWNER", "REPO"),
	}
	_, err := api.List("")
	require.NoError(t, err)
}

func Test_Download(t *testing.T) {
	tmpDir := t.TempDir()
	destDir := filepath.Join(tmpDir, "artifact")

	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	matcher := httpmock.REST("GET", "repos/OWNER/REPO/actions/artifacts/12345/zip")
	responder := httpmock.FileResponse("./fixtures/myproject.zip")
	url := "https://api.github.com/repos/OWNER/REPO/actions/artifacts/12345/zip"
	reg.Register(matcher, responder)

	api := &apiPlatform{
		client: &http.Client{Transport: reg},
	}
	err := api.Download(url, destDir, false, false)
	require.NoError(t, err)

	var paths []string
	parentPrefix := tmpDir + string(filepath.Separator)
	err = filepath.Walk(tmpDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if p == tmpDir {
			return nil
		}
		entry := strings.TrimPrefix(p, parentPrefix)
		if info.IsDir() {
			entry += "/"
		} else if info.Mode()&0111 != 0 {
			entry += "(X)"
		}
		paths = append(paths, entry)
		return nil
	})
	require.NoError(t, err)

	sort.Strings(paths)
	assert.Equal(t, []string{
		"artifact/",
		filepath.Join("artifact", "bin") + "/",
		filepath.Join("artifact", "bin", "myexe"),
		filepath.Join("artifact", "readme.md"),
		filepath.Join("artifact", "src") + "/",
		filepath.Join("artifact", "src", "main.go"),
		filepath.Join("artifact", "src", "util.go"),
	}, paths)

	// test --clobber/--skip-existing option
	readme := parentPrefix + filepath.Join("artifact", "readme.md")
	stat, err := os.Stat(readme)
	require.NoError(t, err)
	mtimeFirst := stat.ModTime()

	// test existing error
	reg.Register(matcher, responder)
	err = api.Download(url, destDir, false, false)
	require.ErrorContains(t, err, "(use `--clobber` to override or `--skip-existing` to skip)")

	stat, err = os.Stat(readme)
	require.NoError(t, err)
	mtimeError := stat.ModTime()
	require.Equal(t, mtimeFirst, mtimeError)

	// test --clobber
	reg.Register(matcher, responder)
	err = api.Download(url, destDir, true, false)
	require.NoError(t, err)

	stat, err = os.Stat(readme)
	require.NoError(t, err)
	mtimeClobber := stat.ModTime()
	require.Less(t, mtimeFirst, mtimeClobber)

	// test --skip-existing
	reg.Register(matcher, responder)
	err = api.Download(url, destDir, false, true)
	require.NoError(t, err)

	stat, err = os.Stat(readme)
	require.NoError(t, err)
	mtimeSkip := stat.ModTime()
	require.Equal(t, mtimeClobber, mtimeSkip)
}
