package shared

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_typeForFilename(t *testing.T) {
	tests := []struct {
		name string
		file string
		want string
	}{
		{
			name: "tar",
			file: "ball.tar",
			want: "application/x-tar",
		},
		{
			name: "tgz",
			file: "ball.tgz",
			want: "application/x-gtar",
		},
		{
			name: "tar.gz",
			file: "ball.tar.gz",
			want: "application/x-gtar",
		},
		{
			name: "bz2",
			file: "ball.tar.bz2",
			want: "application/x-bzip2",
		},
		{
			name: "zip",
			file: "archive.zip",
			want: "application/zip",
		},
		{
			name: "js",
			file: "app.js",
			want: "application/javascript",
		},
		{
			name: "dmg",
			file: "apple.dmg",
			want: "application/x-apple-diskimage",
		},
		{
			name: "rpm",
			file: "package.rpm",
			want: "application/x-rpm",
		},
		{
			name: "deb",
			file: "package.deb",
			want: "application/x-debian-package",
		},
		{
			name: "no extension",
			file: "myfile",
			want: "application/octet-stream",
		},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			if got := typeForFilename(tt.file); got != tt.want {
				t.Errorf("typeForFilename() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_duplicateAssetNames(t *testing.T) {
	asset := func(name string) *AssetForUpload {
		return &AssetForUpload{Name: name}
	}

	tests := []struct {
		name   string
		assets []*AssetForUpload
		want   []string
	}{
		{
			name:   "no assets",
			assets: nil,
			want:   nil,
		},
		{
			name:   "single asset",
			assets: []*AssetForUpload{asset("foo.zip")},
			want:   nil,
		},
		{
			name:   "all unique",
			assets: []*AssetForUpload{asset("foo.zip"), asset("bar.zip"), asset("baz.zip")},
			want:   nil,
		},
		{
			name:   "one duplicate pair",
			assets: []*AssetForUpload{asset("foo.zip"), asset("bar.zip"), asset("foo.zip")},
			want:   []string{"foo.zip"},
		},
		{
			name:   "multiple duplicates reported sorted",
			assets: []*AssetForUpload{asset("beta.zip"), asset("alpha.zip"), asset("beta.zip"), asset("alpha.zip")},
			want:   []string{"alpha.zip", "beta.zip"},
		},
		{
			name:   "same name three times reported once",
			assets: []*AssetForUpload{asset("dup"), asset("dup"), asset("dup")},
			want:   []string{"dup"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, duplicateAssetNames(tt.assets))
		})
	}
}

func TestAssetsFromArgs_duplicateFilenames(t *testing.T) {
	// Two source files in different directories share the same base name, so
	// they resolve to the same release asset name and must be rejected.
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	dup1 := filepath.Join(dir1, "dup.txt")
	dup2 := filepath.Join(dir2, "dup.txt")
	require.NoError(t, os.WriteFile(dup1, []byte("one"), 0600))
	require.NoError(t, os.WriteFile(dup2, []byte("two"), 0600))

	_, err := AssetsFromArgs([]string{dup1, dup2})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dup.txt")
	assert.Contains(t, err.Error(), "same filename")
}

func TestAssetsFromArgs_uniqueFilenames(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	require.NoError(t, os.WriteFile(a, []byte("a"), 0600))
	require.NoError(t, os.WriteFile(b, []byte("b"), 0600))

	assets, err := AssetsFromArgs([]string{a, b})
	require.NoError(t, err)
	require.Len(t, assets, 2)
	assert.Equal(t, "a.txt", assets[0].Name)
	assert.Equal(t, "b.txt", assets[1].Name)
}

func TestAssetsFromArgs_sameFileTwice(t *testing.T) {
	// Passing the same file twice resolves to the same asset name and must be
	// rejected, since deduplication is by asset name rather than source path.
	dir := t.TempDir()
	f := filepath.Join(dir, "same.txt")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0600))

	_, err := AssetsFromArgs([]string{f, f})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "same.txt")
}

func TestAssetsFromArgs_duplicateNameDifferentLabels(t *testing.T) {
	// A differing display label does not change the asset name, so files with
	// the same base name still collide even when labeled distinctly.
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	f1 := filepath.Join(dir1, "asset.txt")
	f2 := filepath.Join(dir2, "asset.txt")
	require.NoError(t, os.WriteFile(f1, []byte("1"), 0600))
	require.NoError(t, os.WriteFile(f2, []byte("2"), 0600))

	_, err := AssetsFromArgs([]string{f1 + "#first", f2 + "#second"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "asset.txt")
}

func Test_uploadWithDelete_retry(t *testing.T) {
	retryInterval = 0
	ctx := context.Background()

	tries := 0
	client := funcClient(func(req *http.Request) (*http.Response, error) {
		tries++
		if tries == 1 {
			return nil, errors.New("made up exception")
		} else if tries == 2 {
			return &http.Response{
				Request:    req,
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
			}, nil
		}
		return &http.Response{
			Request:    req,
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
		}, nil
	})
	err := uploadWithDelete(ctx, client, "http://example.com/upload", AssetForUpload{
		Name:  "asset",
		Label: "",
		Size:  8,
		Open: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewBufferString(`somebody`)), nil
		},
		MIMEType: "application/octet-stream",
	})
	if err != nil {
		t.Errorf("uploadWithDelete() error: %v", err)
	}
	if tries != 3 {
		t.Errorf("tries = %d, expected %d", tries, 3)
	}
}

type funcClient func(*http.Request) (*http.Response, error)

func (f funcClient) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}
