package shared

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
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
