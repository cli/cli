package ghrepo

import (
	"errors"
	"fmt"
	"net/url"
	"testing"
)

func Test_repoFromURL(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		result string
		err    error
	}{
		{
			name:   "github.com URL",
			input:  "https://github.com/monalisa/octo-cat.git",
			result: "monalisa/octo-cat",
			err:    nil,
		},
		{
			name:   "www.github.com URL",
			input:  "http://www.GITHUB.com/monalisa/octo-cat.git",
			result: "monalisa/octo-cat",
			err:    nil,
		},
		{
			name:   "unsupported hostname",
			input:  "https://example.com/one/two",
			result: "",
			err:    errors.New("unsupported hostname: example.com"),
		},
		{
			name:   "filesystem path",
			input:  "/path/to/file",
			result: "",
			err:    errors.New("unsupported hostname: "),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.input)
			if err != nil {
				t.Fatalf("got error %q", err)
			}

			repo, err := FromURL(u)
			if err != nil {
				if tt.err == nil {
					t.Fatalf("got error %q", err)
				} else if tt.err.Error() == err.Error() {
					return
				}
				t.Fatalf("got error %q", err)
			}

			got := fmt.Sprintf("%s/%s", repo.RepoOwner(), repo.RepoName())
			if tt.result != got {
				t.Errorf("expected %q, got %q", tt.result, got)
			}
		})
	}
}
