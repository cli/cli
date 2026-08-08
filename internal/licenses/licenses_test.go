package licenses

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/MakeNowJust/heredoc"
	"github.com/stretchr/testify/require"
)

func TestContent(t *testing.T) {
	// This test is to ensure that we don't accidentally commit actual license
	// files in the repo. The embedded content is only included in release builds,
	// so in a normal test build we should get a default message.
	require.Equal(t, "License information is only available in official release builds.\n", Content())
}

func TestContent_tableTests(t *testing.T) {
	tests := []struct {
		name     string
		fsys     fstest.MapFS
		expected string
	}{
		{
			name: "report only",
			fsys: fstest.MapFS{
				"embed/os-arch/PLACEHOLDER": &fstest.MapFile{}, // Checked-in placeholder, so it's always there.
				"embed/os-arch/report.txt":  &fstest.MapFile{Data: []byte("dep1 (v1.0.0) - MIT - https://example.com\n")},
			},
			expected: heredoc.Doc(`
				dep1 (v1.0.0) - MIT - https://example.com

			`),
		},
		{
			name: "empty third-party dir",
			fsys: fstest.MapFS{
				"embed/os-arch/PLACEHOLDER": &fstest.MapFile{}, // Checked-in placeholder, so it's always there.
				"embed/os-arch/report.txt":  &fstest.MapFile{Data: []byte("dep1 (v1.0.0) - MIT - https://example.com\n")},
				"embed/os-arch/third-party": &fstest.MapFile{Data: []byte{}, Mode: fs.ModeDir},
			},
			expected: heredoc.Doc(`
				dep1 (v1.0.0) - MIT - https://example.com

			`),
		},
		{
			name: "unknown file at root ignored",
			fsys: fstest.MapFS{
				"embed/os-arch/PLACEHOLDER": &fstest.MapFile{}, // Checked-in placeholder, so it's always there.
				"embed/os-arch/report.txt":  &fstest.MapFile{Data: []byte("dep1 (v1.0.0) - MIT - https://example.com\n")},
				"embed/os-arch/unknown": &fstest.MapFile{
					Data: []byte("MIT License\n\nCopyright (c) 2024"),
				},
			},
			expected: heredoc.Doc(`
				dep1 (v1.0.0) - MIT - https://example.com

			`),
		},
		{
			name: "unknown directory at root ignored",
			fsys: fstest.MapFS{
				"embed/os-arch/PLACEHOLDER": &fstest.MapFile{}, // Checked-in placeholder, so it's always there.
				"embed/os-arch/report.txt":  &fstest.MapFile{Data: []byte("dep1 (v1.0.0) - MIT - https://example.com\n")},
				"embed/os-arch/unknown/example.com/mod/LICENSE": &fstest.MapFile{
					Data: []byte("MIT License\n\nCopyright (c) 2024"),
				},
			},
			expected: heredoc.Doc(`
				dep1 (v1.0.0) - MIT - https://example.com

			`),
		},
		{
			name: "single module",
			fsys: fstest.MapFS{
				"embed/os-arch/PLACEHOLDER": &fstest.MapFile{}, // Checked-in placeholder, so it's always there.
				"embed/os-arch/report.txt":  &fstest.MapFile{Data: []byte("example.com/mod (v1.0.0) - MIT - https://example.com\n")},
				"embed/os-arch/third-party/example.com/mod/LICENSE": &fstest.MapFile{
					Data: []byte("MIT License\n\nCopyright (c) 2024"),
				},
			},
			expected: heredoc.Doc(`
				example.com/mod (v1.0.0) - MIT - https://example.com

				================================================================================
				example.com/mod
				================================================================================
				
				MIT License
				
				Copyright (c) 2024

			`),
		},
		{
			name: "multiple modules sorted alphabetically",
			fsys: fstest.MapFS{
				"embed/os-arch/PLACEHOLDER": &fstest.MapFile{}, // Checked-in placeholder, so it's always there.
				"embed/os-arch/report.txt":  &fstest.MapFile{Data: []byte("example.com/mod (v1.0.0) - MIT - https://example.com\n")},
				"embed/os-arch/third-party/github.com/zzz/pkg/LICENSE": &fstest.MapFile{
					Data: []byte("ZZZ License"),
				},
				"embed/os-arch/third-party/github.com/aaa/pkg/LICENSE": &fstest.MapFile{
					Data: []byte("AAA License"),
				},
			},
			expected: heredoc.Doc(`
				example.com/mod (v1.0.0) - MIT - https://example.com

				================================================================================
				github.com/aaa/pkg
				================================================================================
				
				AAA License

				================================================================================
				github.com/zzz/pkg
				================================================================================
				
				ZZZ License

			`),
		},
		{
			name: "license and notice files",
			fsys: fstest.MapFS{
				"embed/os-arch/PLACEHOLDER": &fstest.MapFile{}, // Checked-in placeholder, so it's always there.
				"embed/os-arch/report.txt":  &fstest.MapFile{Data: []byte("example.com/mod (v1.0.0) - MIT - https://example.com\n")},
				"embed/os-arch/third-party/example.com/mod/LICENSE": &fstest.MapFile{
					Data: []byte("Apache License 2.0"),
				},
				"embed/os-arch/third-party/example.com/mod/NOTICE": &fstest.MapFile{
					Data: []byte("Copyright 2024 Example Corp"),
				},
			},
			expected: heredoc.Doc(`
				example.com/mod (v1.0.0) - MIT - https://example.com

				================================================================================
				example.com/mod
				================================================================================
				
				Apache License 2.0
				
				Copyright 2024 Example Corp

			`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := content(tt.fsys, "embed/os-arch")
			require.Equal(t, tt.expected, got)
		})
	}
}
