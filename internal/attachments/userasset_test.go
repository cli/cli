package attachments

import (
	"io/fs"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFile creates a file of the given size relative to the test's working
// directory. Truncate makes a sparse file, so a size over the image limit costs
// nothing to create.
func writeFile(t *testing.T, name string, size int64) {
	t.Helper()

	f, err := os.Create(name)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, f.Truncate(size))
}

func TestNewAsset(t *testing.T) {
	tests := []struct {
		name            string
		file            string
		size            int64
		setup           func(t *testing.T)
		path            string
		alt             string
		wantPath        string
		wantAlt         string
		wantContentType string
		wantErr         string
	}{
		{
			name:            "png",
			file:            "shot.png",
			path:            "./shot.png",
			wantAlt:         "shot",
			wantContentType: "image/png",
		},
		{
			name:            "jpg",
			file:            "shot.jpg",
			path:            "./shot.jpg",
			wantAlt:         "shot",
			wantContentType: "image/jpeg",
		},
		{
			name:            "jpeg",
			file:            "shot.jpeg",
			path:            "./shot.jpeg",
			wantAlt:         "shot",
			wantContentType: "image/jpeg",
		},
		{
			name:            "gif",
			file:            "shot.gif",
			path:            "./shot.gif",
			wantAlt:         "shot",
			wantContentType: "image/gif",
		},
		{
			name:            "webp",
			file:            "shot.webp",
			path:            "./shot.webp",
			wantAlt:         "shot",
			wantContentType: "image/webp",
		},
		{
			name:            "svg",
			file:            "shot.svg",
			path:            "./shot.svg",
			wantAlt:         "shot",
			wantContentType: "image/svg+xml",
		},
		{
			// A video has no alt attribute to fill, so the author cannot supply
			// one. The filename stands in, extension included, because it
			// becomes the link text when the reference degrades to a link.
			name:            "mp4",
			file:            "repro.mp4",
			path:            "./repro.mp4",
			wantAlt:         "repro.mp4",
			wantContentType: "video/mp4",
		},
		{
			name:            "mov",
			file:            "repro.mov",
			path:            "./repro.mov",
			wantAlt:         "repro.mov",
			wantContentType: "video/quicktime",
		},
		{
			name:            "webm",
			file:            "repro.webm",
			path:            "./repro.webm",
			wantAlt:         "repro.webm",
			wantContentType: "video/webm",
		},
		{
			name:            "uppercase extension",
			file:            "SHOT.PNG",
			path:            "./SHOT.PNG",
			wantAlt:         "SHOT",
			wantContentType: "image/png",
		},
		{
			name:            "alt text supplied",
			file:            "shot.png",
			path:            "./shot.png",
			alt:             "The login error state",
			wantAlt:         "The login error state",
			wantContentType: "image/png",
		},
		{
			name:            "alt text defaults to the filename with dots as spaces",
			file:            "Screenshot 2026-08-10 at 5.38.10 PM.png",
			path:            "./Screenshot 2026-08-10 at 5.38.10 PM.png",
			wantAlt:         "Screenshot 2026-08-10 at 5 38 10 PM",
			wantContentType: "image/png",
		},
		{
			name:            "image at exactly the size limit",
			file:            "big.png",
			size:            maxImageBytes,
			path:            "./big.png",
			wantAlt:         "big",
			wantContentType: "image/png",
		},
		{
			name:            "video over the image size limit",
			file:            "clip.mp4",
			size:            20 * 1024 * 1024,
			path:            "./clip.mp4",
			wantAlt:         "clip.mp4",
			wantContentType: "video/mp4",
		},
		{
			name:            "video at exactly the video size limit",
			file:            "clip.mp4",
			size:            maxVideoBytes,
			path:            "./clip.mp4",
			wantAlt:         "clip.mp4",
			wantContentType: "video/mp4",
		},
		{
			name:    "video over the video size limit",
			file:    "clip.mp4",
			size:    105 * 1024 * 1024,
			path:    "./clip.mp4",
			wantErr: "./clip.mp4: videos must be at most 100.0 MB",
		},
		{
			name:    "image one byte over the limit",
			file:    "big.png",
			size:    maxImageBytes + 1,
			path:    "./big.png",
			wantErr: "./big.png: images must be at most 10.0 MB",
		},
		{
			name:    "image well over the size limit",
			file:    "huge.png",
			size:    14889779,
			path:    "./huge.png",
			wantErr: "./huge.png: images must be at most 10.0 MB",
		},
		{
			name:    "unsupported extension",
			file:    "notes.txt",
			path:    "./notes.txt",
			wantErr: "./notes.txt is not a supported file type (supported: png, jpg, jpeg, gif, webp, svg, mp4, mov, webm)",
		},
		{
			name:    "no extension",
			file:    "notes",
			path:    "./notes",
			wantErr: "./notes is not a supported file type (supported: png, jpg, jpeg, gif, webp, svg, mp4, mov, webm)",
		},
		{
			name:    "empty file",
			file:    "empty.png",
			size:    0,
			path:    "./empty.png",
			wantErr: "./empty.png is empty",
		},
		{
			name: "directory",
			setup: func(t *testing.T) {
				require.NoError(t, os.Mkdir("shots.png", 0o755))
			},
			path:    "./shots.png",
			wantErr: "./shots.png is a directory",
		},
		{
			name: "not a regular file",
			setup: func(t *testing.T) {
				if runtime.GOOS == "windows" {
					t.Skip("no stable path to a non-regular file on Windows")
				}
			},
			path:    "/dev/null",
			wantErr: "/dev/null is not a regular file",
		},
		{
			name:    "alt text on a video",
			file:    "repro.mp4",
			path:    "./repro.mp4",
			alt:     "Screen recording of the crash",
			wantErr: "cannot set alt text on video",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if tt.file != "" {
				size := tt.size
				if size == 0 && tt.name != "empty file" {
					size = 1
				}
				writeFile(t, tt.file, size)
			}
			if tt.setup != nil {
				tt.setup(t)
			}

			a, err := newAsset(tt.path, tt.alt)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)

			wantPath := tt.wantPath
			if wantPath == "" {
				wantPath = tt.path
			}
			assert.Equal(t, wantPath, a.Path())
			assert.Equal(t, tt.wantAlt, a.getAsset().alt)
			assert.Equal(t, tt.wantContentType, a.getAsset().contentType)

			// The identity the duplicate check compares, which has to be the
			// file this path led to.
			wantInfo, err := os.Stat(tt.file)
			require.NoError(t, err)
			assert.True(t, os.SameFile(wantInfo, a.getAsset().info))
		})
	}
}

// A missing file reports the path the user typed rather than the syscall gh
// happened to make. The reason text comes from the operating system, so this
// checks the shape and the cause instead of the whole string.
func TestNewAssetMissingFile(t *testing.T) {
	t.Chdir(t.TempDir())

	_, err := newAsset("./nope.png", "")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "./nope.png: "), "got %q", err.Error())
	assert.NotContains(t, err.Error(), "stat ")
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestAssetMarkdown(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		alt         string
		want        string
	}{
		{
			name:        "image",
			contentType: "image/png",
			alt:         "The login error state",
			want:        "![The login error state](https://example.com/assets/1)",
		},
		{
			name:        "image with empty alt text",
			contentType: "image/png",
			want:        "![](https://example.com/assets/1)",
		},
		{
			name:        "video renders as a bare URL so it plays",
			contentType: "video/mp4",
			want:        "https://example.com/assets/1",
		},
		{
			// A bare URL is the only form that plays.
			name:        "a video alt does not reach the embed",
			contentType: "video/mp4",
			alt:         "repro.mp4",
			want:        "https://example.com/assets/1",
		},
		{
			name:        "alt text cannot close the image early",
			contentType: "image/png",
			alt:         "![x](https://evil.example.com/x.png)",
			want:        `![!\[x\](https://evil.example.com/x.png)](https://example.com/assets/1)`,
		},
		{
			name:        "backslashes are escaped",
			contentType: "image/png",
			alt:         `a\b`,
			want:        `![a\\b](https://example.com/assets/1)`,
		},
		{
			name:        "newlines cannot break out of the image",
			contentType: "image/png",
			alt:         "first\nsecond\r\nthird",
			want:        "![first second  third](https://example.com/assets/1)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var a UserAsset = &imageAsset{asset{contentType: tt.contentType, alt: tt.alt}}
			if strings.HasPrefix(tt.contentType, "video/") {
				a = &videoAsset{asset{contentType: tt.contentType, alt: tt.alt}}
			}

			assert.Equal(t, tt.want, a.markdown("https://example.com/assets/1"))
		})
	}
}

func TestAssetRendersAsPlayer(t *testing.T) {
	assert.True(t, (&videoAsset{}).rendersAsPlayer())
	assert.False(t, (&imageAsset{}).rendersAsPlayer())
}
