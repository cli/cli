package download

import (
	"bytes"
	"crypto/md5" //nolint:gosec
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safepaths"
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
	destDir, err := safepaths.ParseAbsolute(filepath.Join(tmpDir, "artifact"))
	require.NoError(t, err)

	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/actions/artifacts/12345/zip"),
		httpmock.FileResponse("./fixtures/myproject.zip"))

	api := &apiPlatform{
		client: &http.Client{Transport: reg},
	}
	require.NoError(t, api.Download("https://api.github.com/repos/OWNER/REPO/actions/artifacts/12345/zip", destDir))

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
}

func Test_DownloadByID(t *testing.T) {
	tmpDir := t.TempDir()
	destDir, err := safepaths.ParseAbsolute(filepath.Join(tmpDir, "artifact"))
	require.NoError(t, err)

	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/actions/artifacts/6384611797/zip"),
		httpmock.FileResponse("./fixtures/myproject.zip"))

	p := &apiPlatform{
		client:       &http.Client{Transport: reg},
		repo:         ghrepo.New("OWNER", "REPO"),
		singleStream: true, // fixture is small; avoid HEAD round-trip in test
	}
	require.NoError(t, p.DownloadByID(6384611797, destDir))
	assert.Equal(t, expectedArtifactPaths("artifact"), collectPaths(t, destDir.String()))
}

func expectedArtifactPaths(destDir string) []string {
	return []string{
		destDir + "/",
		filepath.Join(destDir, "bin") + "/",
		filepath.Join(destDir, "bin", "myexe"),
		filepath.Join(destDir, "readme.md"),
		filepath.Join(destDir, "src") + "/",
		filepath.Join(destDir, "src", "main.go"),
		filepath.Join(destDir, "src", "util.go"),
	}
}

func collectPaths(t *testing.T, root string) []string {
	t.Helper()
	base := filepath.Dir(root)
	var paths []string
	require.NoError(t, filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if p == base {
			return nil
		}
		rel := strings.TrimPrefix(p, base+string(filepath.Separator))
		if info.IsDir() {
			rel += "/"
		} else if info.Mode()&0111 != 0 {
			rel += "(X)"
		}
		paths = append(paths, rel)
		return nil
	}))
	sort.Strings(paths)
	return paths
}

func fixtureData(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("./fixtures/myproject.zip")
	require.NoError(t, err)
	return data
}

func fixtureContentMD5(t *testing.T) string {
	t.Helper()
	data := fixtureData(t)
	h := md5.New() //nolint:gosec
	_, _ = h.Write(data)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func newRangeServer(fileData []byte, contentMD5 string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if contentMD5 != "" {
			w.Header().Set("Content-MD5", contentMD5)
		}
		http.ServeContent(w, r, "artifact.zip", time.Time{}, bytes.NewReader(fileData))
	}))
}

func newNoRangeServer(fileData []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(fileData)))
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fileData)
	}))
}

func withChunkParams(t *testing.T, threshold, size int64, concurrency int) {
	t.Helper()
	origThreshold, origSize, origConcurrency := chunkThreshold, chunkSize, chunkConcurrency
	chunkThreshold = threshold
	chunkSize = size
	chunkConcurrency = concurrency
	t.Cleanup(func() {
		chunkThreshold = origThreshold
		chunkSize = origSize
		chunkConcurrency = origConcurrency
	})
}

func Test_validateContentRange(t *testing.T) {
	tests := []struct {
		name         string
		contentRange string
		start        int64
		end          int64
		wantErr      bool
	}{
		{name: "matching range", contentRange: "bytes 0-999/5000", start: 0, end: 999, wantErr: false},
		{name: "empty header is allowed", contentRange: "", start: 0, end: 999, wantErr: false},
		{name: "mismatched start", contentRange: "bytes 100-999/5000", start: 0, end: 999, wantErr: true},
		{name: "mismatched end", contentRange: "bytes 0-500/5000", start: 0, end: 999, wantErr: true},
		{name: "malformed header", contentRange: "invalid", start: 0, end: 999, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateContentRange(tt.contentRange, tt.start, tt.end)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_downloadChunk(t *testing.T) {
	const content = "0123456789ABCDEFGHIJ" // 20 bytes
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "data.bin", time.Time{}, strings.NewReader(content))
	}))
	defer srv.Close()

	tmpfile, err := os.CreateTemp(t.TempDir(), "chunk-*.bin")
	require.NoError(t, err)
	require.NoError(t, tmpfile.Truncate(20))

	require.NoError(t, downloadChunk(t.Context(), &http.Client{}, srv.URL, tmpfile, 5, 14))

	require.NoError(t, tmpfile.Close())
	got, err := os.ReadFile(tmpfile.Name())
	require.NoError(t, err)
	assert.Equal(t, "56789ABCDE", string(got[5:15]))
}

func Test_downloadArtifactChunked_noRangeSupport(t *testing.T) {
	srv := newNoRangeServer(fixtureData(t))
	defer srv.Close()

	tmpDir := t.TempDir()
	destDir, err := safepaths.ParseAbsolute(filepath.Join(tmpDir, "out"))
	require.NoError(t, err)

	err = downloadArtifactChunked(&http.Client{}, srv.URL, destDir)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errNoRangeSupport))
}

func Test_downloadArtifactChunked_smallFile(t *testing.T) {
	withChunkParams(t, 1024*1024*1024, chunkSize, chunkConcurrency)

	srv := newRangeServer(fixtureData(t), "")
	defer srv.Close()

	tmpDir := t.TempDir()
	destDir, err := safepaths.ParseAbsolute(filepath.Join(tmpDir, "out"))
	require.NoError(t, err)

	err = downloadArtifactChunked(&http.Client{}, srv.URL, destDir)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errNoRangeSupport))
}

func Test_downloadArtifactChunked_happyPath(t *testing.T) {
	data := fixtureData(t)
	// Lower thresholds so the 1710-byte fixture triggers chunking.
	// chunk size 600 → 3 chunks: [0-599], [600-1199], [1200-1709].
	withChunkParams(t, 100, 600, 4)

	var rangeRequests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			rangeRequests++
		}
		http.ServeContent(w, r, "artifact.zip", time.Time{}, bytes.NewReader(data))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	destDir, err := safepaths.ParseAbsolute(filepath.Join(tmpDir, "artifact"))
	require.NoError(t, err)

	require.NoError(t, downloadArtifactChunked(&http.Client{}, srv.URL, destDir))
	assert.Equal(t, 3, rangeRequests, "expected 3 range requests for a 1710-byte file with 600-byte chunks")

	assert.Equal(t, expectedArtifactPaths("artifact"), collectPaths(t, destDir.String()))
}

func Test_downloadArtifactChunked_md5Valid(t *testing.T) {
	data := fixtureData(t)
	withChunkParams(t, 100, 600, 4)

	srv := newRangeServer(data, fixtureContentMD5(t))
	defer srv.Close()

	tmpDir := t.TempDir()
	destDir, err := safepaths.ParseAbsolute(filepath.Join(tmpDir, "artifact"))
	require.NoError(t, err)

	require.NoError(t, downloadArtifactChunked(&http.Client{}, srv.URL, destDir))
}

func Test_downloadArtifactChunked_md5Mismatch(t *testing.T) {
	data := fixtureData(t)
	withChunkParams(t, 100, 600, 4)

	srv := newRangeServer(data, "AAAAAAAAAAAAAAAAAAAAAA==") // wrong MD5
	defer srv.Close()

	tmpDir := t.TempDir()
	destDir, err := safepaths.ParseAbsolute(filepath.Join(tmpDir, "artifact"))
	require.NoError(t, err)

	err = downloadArtifactChunked(&http.Client{}, srv.URL, destDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "integrity check failed")
}

func Test_downloadArtifactSingleStream(t *testing.T) {
	data := fixtureData(t)
	srv := newNoRangeServer(data)
	defer srv.Close()

	tmpDir := t.TempDir()
	destDir, err := safepaths.ParseAbsolute(filepath.Join(tmpDir, "artifact"))
	require.NoError(t, err)

	require.NoError(t, downloadArtifactSingleStream(&http.Client{}, srv.URL, destDir))
	assert.Equal(t, expectedArtifactPaths("artifact"), collectPaths(t, destDir.String()))
}

func Test_downloadArtifact_singleStreamForced(t *testing.T) {
	data := fixtureData(t)

	var headSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			headSeen = true
		}
		http.ServeContent(w, r, "artifact.zip", time.Time{}, bytes.NewReader(data))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	destDir, err := safepaths.ParseAbsolute(filepath.Join(tmpDir, "artifact"))
	require.NoError(t, err)

	require.NoError(t, downloadArtifact(&http.Client{}, srv.URL, destDir, true))
	assert.False(t, headSeen, "downloadArtifact with singleStream=true must not issue a HEAD request")
	assert.Equal(t, expectedArtifactPaths("artifact"), collectPaths(t, destDir.String()))
}

func Test_downloadArtifact_chunkedFallsBack(t *testing.T) {
	data := fixtureData(t)
	withChunkParams(t, 100, 600, 4)

	srv := newNoRangeServer(data)
	defer srv.Close()

	tmpDir := t.TempDir()
	destDir, err := safepaths.ParseAbsolute(filepath.Join(tmpDir, "artifact"))
	require.NoError(t, err)

	require.NoError(t, downloadArtifact(&http.Client{}, srv.URL, destDir, false))
	assert.Equal(t, expectedArtifactPaths("artifact"), collectPaths(t, destDir.String()))
}
