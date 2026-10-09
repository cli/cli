package shared

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadContent(t *testing.T) {
	// The first bytes of a PNG image.
	const pngBytes = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00@\x00\x00\x00(\x08\x02\x00\x00\x00"

	tests := []struct {
		name      string
		files     map[string]string
		symlinks  map[string]string
		path      string
		stdin     string
		want      string
		wantErr   string
		wantErrIs error
	}{
		{
			name:  "a text file",
			files: map[string]string{"signin-plan.md": "# Sign-in plan\n\n1. Register the callback URL.\n"},
			path:  "signin-plan.md",
			want:  "# Sign-in plan\n\n1. Register the callback URL.\n",
		},
		{
			name:  "a file in another directory",
			files: map[string]string{"docs/signin-plan.md": "# Sign-in plan\n"},
			path:  "docs/signin-plan.md",
			want:  "# Sign-in plan\n",
		},
		{
			name:  "text in any language",
			files: map[string]string{"memo.md": "# メモ\n\nラテアートの練習。\n"},
			path:  "memo.md",
			want:  "# メモ\n\nラテアートの練習。\n",
		},
		{
			name:     "a symlink is followed",
			files:    map[string]string{"outside.md": "# Sign-in plan\n"},
			symlinks: map[string]string{"signin-plan.md": "outside.md"},
			path:     "signin-plan.md",
			want:     "# Sign-in plan\n",
		},
		{
			name:  "standard input",
			path:  "-",
			stdin: "# OAuth research\n",
			want:  "# OAuth research\n",
		},
		{
			name:  "standard input is only read for -",
			files: map[string]string{"signin-plan.md": "# Sign-in plan\n"},
			path:  "signin-plan.md",
			stdin: "# OAuth research\n",
			want:  "# Sign-in plan\n",
		},
		{
			name:      "a missing file, without its path",
			path:      "missing.md",
			wantErrIs: fs.ErrNotExist,
		},
		{
			name:    "an empty file",
			files:   map[string]string{"empty.md": ""},
			path:    "empty.md",
			wantErr: "file is empty",
		},
		{
			name:    "a file with only whitespace",
			files:   map[string]string{"empty.md": "\n  \n\t\r\n"},
			path:    "empty.md",
			wantErr: "file is empty",
		},
		{
			name:    "empty standard input",
			path:    "-",
			wantErr: "input is empty",
		},
		{
			name:    "standard input with only whitespace",
			path:    "-",
			stdin:   "\n \n",
			wantErr: "input is empty",
		},
		{
			name:    "a binary file",
			files:   map[string]string{"signin-flow.png": pngBytes},
			path:    "signin-flow.png",
			wantErr: "binary file not supported",
		},
		{
			name:    "binary standard input",
			path:    "-",
			stdin:   pngBytes,
			wantErr: "binary input not supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			for path, content := range tt.files {
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
				require.NoError(t, os.WriteFile(path, []byte(content), 0644))
			}
			for path, target := range tt.symlinks {
				if err := os.Symlink(target, path); err != nil {
					t.Skipf("cannot create a symlink here: %v", err)
				}
			}
			stdin := io.NopCloser(strings.NewReader(tt.stdin))

			got, err := ReadContent(tt.path, stdin)

			switch {
			case tt.wantErrIs != nil:
				require.ErrorIs(t, err, tt.wantErrIs)
				assert.NotContains(t, err.Error(), tt.path)
				return
			case tt.wantErr != "":
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsShortcutFile(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "a .url file",
			path: "oauth-runbook.url",
			want: true,
		},
		{
			name: "any case, as on Windows",
			path: "docs/OAuth-Runbook.URL",
			want: true,
		},
		{
			name: "a Markdown file",
			path: "signin-plan.md",
		},
		{
			name: "url elsewhere in the name",
			path: "url-notes.md",
		},
		{
			name: "no extension",
			path: "url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsShortcutFile(tt.path))
		})
	}
}

func TestShortcutURL(t *testing.T) {
	const runbook = "https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook"

	tests := []struct {
		name      string
		content   string
		want      string
		wantErr   string
		wantErrIs error
	}{
		{
			name:    "a shortcut as gh writes it",
			content: "[InternetShortcut]\r\nURL=" + runbook + "\r\n",
			want:    runbook,
		},
		{
			name:    "Unix line endings",
			content: "[InternetShortcut]\nURL=" + runbook + "\n",
			want:    runbook,
		},
		{
			name: "a shortcut saved by a browser on Windows",
			content: "\ufeff[{000214A0-0000-0000-C000-000000000046}]\r\nProp3=19,11\r\n[InternetShortcut]\r\nIDList=\r\n" +
				"URL=https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook#staging\r\nIconIndex=0\r\n",
			want: "https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook#staging",
		},
		{
			name:    "any case, with spaces around the key and value",
			content: "[internetshortcut]\n  url = " + runbook + "  \n",
			want:    runbook,
		},
		{
			name:    "comments and blank lines are ignored",
			content: "; saved by gh\n\n[InternetShortcut]\n; the runbook\nURL=" + runbook + "\n",
			want:    runbook,
		},
		{
			name:    "the first URL wins",
			content: "[InternetShortcut]\nURL=" + runbook + "\nURL=https://github.com/monalisa/monas-cafe\n",
			want:    runbook,
		},
		{
			name:    "a URL in another section is ignored",
			content: "[Other]\nURL=https://monas-cafe.example/other\n[InternetShortcut]\nURL=" + runbook + "\n",
			want:    runbook,
		},
		{
			name:    "a later section doesn't change the URL",
			content: "[InternetShortcut]\nURL=" + runbook + "\n[InternetShortcut.W]\nURL=+AGgAdA-\n",
			want:    runbook,
		},
		{
			name:      "a bare URL",
			content:   runbook + "\n",
			wantErrIs: ErrNoShortcutSection,
		},
		{
			name:      "only a URL in another section",
			content:   "[Other]\nURL=" + runbook + "\n",
			wantErrIs: ErrNoShortcutSection,
		},
		{
			name:    "no URL in the section",
			content: "[InternetShortcut]\nIconIndex=0\n",
			wantErr: "no URL in the [InternetShortcut] section",
		},
		{
			name:    "a URL that isn't http(s)",
			content: "[InternetShortcut]\nURL=file:///etc/hosts\n",
			wantErr: "the shortcut doesn't link to an http(s) URL",
		},
		{
			name:    "a URL that isn't ASCII",
			content: "[InternetShortcut]\nURL=https://ラテ.example/menu\n",
			wantErr: "the shortcut doesn't link to an http(s) URL",
		},
		{
			name:    "an empty URL",
			content: "[InternetShortcut]\nURL=\n",
			wantErr: "the shortcut doesn't link to an http(s) URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ShortcutURL(tt.content)

			switch {
			case tt.wantErrIs != nil:
				require.ErrorIs(t, err, tt.wantErrIs)
				return
			case tt.wantErr != "":
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
