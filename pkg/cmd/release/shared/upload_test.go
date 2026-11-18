package shared

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"runtime"
	"testing"

	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/internal/safeurl"
)

func Test_typeForFile(t *testing.T) {
	cs, teardown := run.Stub()
	defer teardown(t)
	wantBinaryMimeType := "application/octet-stream"
	switch runtime.GOOS {
	case "darwin", "linux":
		cs.Register("file -bEL --mime-type -- octocat-linux-arm64", 0, "application/x-pie-executable\n")
		wantBinaryMimeType = "application/x-pie-executable"
	}
	tests := []struct {
		filename string
		want     string
	}{
		{
			filename: "ball.tar",
			want:     "application/x-tar",
		},
		{
			filename: "ball.tgz",
			want:     "application/x-gtar",
		},
		{
			filename: "ball.tar.gz",
			want:     "application/x-gtar",
		},
		{
			filename: "ball.tar.bz2",
			want:     "application/x-bzip2",
		},
		{
			filename: "archive.zip",
			want:     "application/zip",
		},
		{
			filename: "app.js",
			want:     "application/javascript",
		},
		{
			filename: "apple.dmg",
			want:     "application/x-apple-diskimage",
		},
		{
			filename: "package.rpm",
			want:     "application/x-rpm",
		},
		{
			filename: "package.deb",
			want:     "application/x-debian-package",
		},
		{
			filename: "octocat-linux-arm64",
			want:     wantBinaryMimeType,
		},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			if got := typeForFile(tt.filename, tt.filename); got != tt.want {
				t.Errorf("typeForFile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_uploadWithDelete_retry(t *testing.T) {
	retryInterval = 0
	ctx := context.Background()

	tries := 0
	client := &http.Client{Transport: funcTripper(func(req *http.Request) (*http.Response, error) {
		tries++
		switch tries {
		case 1:
			return nil, errors.New("made up exception")
		case 2:
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
	})}
	err := uploadWithDelete(ctx, client, "github.com", safeurl.NewImmutableSafeURL("http://example.com/upload"), AssetForUpload{
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

// funcTripper drives uploadWithDelete from a plain function. It is a RoundTripper rather than
// a Do-er because the upload path takes a concrete *http.Client, which is what api.Client needs.
type funcTripper func(*http.Request) (*http.Response, error)

func (f funcTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
