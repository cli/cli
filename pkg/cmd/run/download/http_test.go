package download

import (
	"archive/zip"
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safepaths"
	"github.com/cli/cli/v2/internal/safeurl"
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
	tests := []struct {
		name    string
		setup   func(t *testing.T, destDir, outside string)
		wantErr string
		verify  func(t *testing.T, tmpDir, outside string)
	}{
		{
			name: "extracts artifact",
			verify: func(t *testing.T, tmpDir, _ string) {
				t.Helper()
				var paths []string
				parentPrefix := tmpDir + string(filepath.Separator)
				err := filepath.Walk(tmpDir, func(p string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					if p == tmpDir {
						return nil
					}
					entry := strings.TrimPrefix(p, parentPrefix)
					if info.IsDir() {
						entry += "/"
					} else if info.Mode()&0o111 != 0 {
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
					filepath.Join("artifact", "empty") + "/",
					filepath.Join("artifact", "readme.md"),
					filepath.Join("artifact", "src") + "/",
					filepath.Join("artifact", "src", "main.go"),
					filepath.Join("artifact", "src", "util.go"),
				}, paths)
			},
		},
		{
			name: "rejects artifact ancestor symlink",
			setup: func(t *testing.T, destDir, outside string) {
				t.Helper()
				require.NoError(t, os.Symlink(outside, filepath.Join(destDir, "src")))
			},
			wantErr: "symbolic link",
			verify: func(t *testing.T, _, outside string) {
				t.Helper()
				assert.NoFileExists(t, filepath.Join(outside, "main.go"))
				assert.NoFileExists(t, filepath.Join(outside, "util.go"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			destDir := filepath.Join(tmpDir, "artifact")
			require.NoError(t, os.MkdirAll(destDir, 0o755))
			outside := t.TempDir()
			if tt.setup != nil {
				tt.setup(t, destDir, outside)
			}

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(
				httpmock.REST("GET", "repos/OWNER/REPO/actions/artifacts/12345/zip"),
				httpmock.BinaryResponse(testArtifactZip(t)))

			api := &apiPlatform{
				client: &http.Client{Transport: reg},
				repo:   ghrepo.New("OWNER", "REPO"),
			}
			err := api.Download(
				safeurl.NewImmutableSafeURL("https://api.github.com/repos/OWNER/REPO/actions/artifacts/12345/zip"),
				func() (*safepaths.Root, error) {
					return safepaths.OpenRoot(destDir)
				},
			)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tt.verify != nil {
				tt.verify(t, tmpDir, outside)
			}
		})
	}
}

func testArtifactZip(t *testing.T) []byte {
	t.Helper()

	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	files := []struct {
		name string
		mode os.FileMode
	}{
		{name: "bin/myexe", mode: 0o644},
		{name: "empty/", mode: os.ModeDir | 0o755},
		{name: "../outside.txt", mode: 0o644},
		{name: "readme.md", mode: 0o644},
		{name: "src/main.go", mode: 0o644},
		{name: "src/util.go", mode: 0o644},
	}
	for _, file := range files {
		header := &zip.FileHeader{Name: file.name, Method: zip.Store}
		header.SetMode(file.mode)
		entry, err := writer.CreateHeader(header)
		require.NoError(t, err)
		if !file.mode.IsDir() {
			_, err = entry.Write([]byte("content"))
			require.NoError(t, err)
		}
	}
	require.NoError(t, writer.Close())
	return archive.Bytes()
}
