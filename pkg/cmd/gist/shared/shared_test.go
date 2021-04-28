package shared

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_GetGistIDFromURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "url",
			url:  "https://gist.github.com/1234",
			want: "1234",
		},
		{
			name: "url with username",
			url:  "https://gist.github.com/octocat/1234",
			want: "1234",
		},
		{
			name: "url, specific file",
			url:  "https://gist.github.com/1234#file-test-md",
			want: "1234",
		},
		{
			name:    "invalid url",
			url:     "https://gist.github.com",
			wantErr: true,
			want:    "Invalid gist URL https://gist.github.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := GistIDFromURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
				assert.EqualError(t, err, tt.want)
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.want, id)
		})
	}
}

func TestIsBinaryContents(t *testing.T) {
	tests := []struct {
		fileContent []byte
		want        bool
	}{
		{
			want:        false,
			fileContent: []byte("package main"),
		},
		{
			want:        false,
			fileContent: []byte(""),
		},
		{
			want:        false,
			fileContent: []byte(nil),
		},
		{
			want: true,
			fileContent: []byte{239, 191, 189, 239, 191, 189, 239, 191, 189, 239,
				191, 189, 239, 191, 189, 16, 74, 70, 73, 70, 239, 191, 189, 1, 1, 1,
				1, 44, 1, 44, 239, 191, 189, 239, 191, 189, 239, 191, 189, 239, 191,
				189, 239, 191, 189, 67, 239, 191, 189, 8, 6, 6, 7, 6, 5, 8, 7, 7, 7,
				9, 9, 8, 10, 12, 20, 10, 12, 11, 11, 12, 25, 18, 19, 15, 20, 29, 26,
				31, 30, 29, 26, 28, 28, 32, 36, 46, 39, 32, 34, 44, 35, 28, 28, 40,
				55, 41, 44, 48, 49, 52, 52, 52, 31, 39, 57, 61, 56, 50, 60, 46, 51,
				52, 50, 239, 191, 189, 239, 191, 189, 239, 191, 189, 67, 1, 9, 9, 9, 12},
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, IsBinaryContents(tt.fileContent))
	}
}
