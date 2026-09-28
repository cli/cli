package fontutil

import (
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/gofont/goregular"
)

func TestOpen(t *testing.T) {
	for _, tc := range []struct {
		name string
		font []byte
		err  string
	}{
		{name: "monospaced font", font: gomono.TTF},
		{name: "proportional font", font: goregular.TTF, err: "not monospaced"},
		{name: "invalid font", font: []byte("not a font"), err: "parse selected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "font.ttf")
			require.NoError(t, os.WriteFile(path, tc.font, 0o600))
			info, err := Probe(path)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, info.Family)
			require.Len(t, info.SHA256, 64)
		})
	}
}

func TestDraw(t *testing.T) {
	root := t.TempDir()
	for name, data := range map[string][]byte{
		"Mono-Regular.ttf": gomono.TTF, "Mono-Bold.ttf": gomonobold.TTF,
		"Mono-Italic.ttf": gomonoitalic.TTF, "Mono-BoldItalic.ttf": gomonobolditalic.TTF,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), data, 0o600))
	}
	for _, tc := range []struct {
		name  string
		text  string
		flags int
		err   string
	}{
		{name: "regular", text: "Mona"},
		{name: "bold", text: "Mona", flags: 1},
		{name: "italic", text: "Mona", flags: 2},
		{name: "bold italic", text: "Mona", flags: 3},
		{name: "missing glyph is not fabricated", text: "\U0010ffff", err: "no selected font"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set, err := Open(filepath.Join(root, "Mono-Regular.ttf"), 18, nil)
			require.NoError(t, err)
			defer func() { require.NoError(t, set.Close()) }()
			canvas := image.NewRGBA(image.Rect(0, 0, 8*set.CellWidth, set.CellHeight))
			err = set.DrawCells(canvas, 0, set.Ascent, tc.text, tc.flags, color.White)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.NotEqual(t, make([]byte, len(canvas.Pix)), canvas.Pix)
			advance, err := set.Measure(tc.text)
			require.NoError(t, err)
			require.Greater(t, advance, float64(0))
		})
	}
	set, err := Open(filepath.Join(root, "Mono-Regular.ttf"), 18, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, set.Close()) }()
	require.Len(t, set.Styles, 3)
	for index, name := range []string{"Mono-Bold.ttf", "Mono-Italic.ttf", "Mono-BoldItalic.ttf"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		sum := sha256.Sum256(data)
		require.Equal(t, filepath.Join(root, name), set.Styles[index].Path)
		require.NotEmpty(t, set.Styles[index].Family)
		require.Equal(t, hex.EncodeToString(sum[:]), set.Styles[index].SHA256)
	}
	require.Equal(t, 0, Cells('\u0301'))
	require.Equal(t, 2, Cells('\u754c'))
	require.Equal(t, 1, Cells('M'))
}
