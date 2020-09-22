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
