package download

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseChecksumFile(t *testing.T) {
	tests := []struct {
		name string
		data string
		want map[string]checksumEntry
	}{
		{
			name: "sha256sum format",
			data: "03ac674216f3e15c761ee1a5e255f067953623c8b388b4459e13f978d7c846f4  windows-32bit.zip\n",
			want: map[string]checksumEntry{
				"windows-32bit.zip": {hash: "03ac674216f3e15c761ee1a5e255f067953623c8b388b4459e13f978d7c846f4", alg: "sha256"},
			},
		},
		{
			name: "sha1sum format",
			data: "da39a3ee5e6b4b0d3255bfef95601890afd80709  empty.txt\n",
			want: map[string]checksumEntry{
				"empty.txt": {hash: "da39a3ee5e6b4b0d3255bfef95601890afd80709", alg: "sha1"},
			},
		},
		{
			name: "binary mode marker is stripped from the filename",
			data: "03ac674216f3e15c761ee1a5e255f067953623c8b388b4459e13f978d7c846f4 *windows-32bit.zip\n",
			want: map[string]checksumEntry{
				"windows-32bit.zip": {hash: "03ac674216f3e15c761ee1a5e255f067953623c8b388b4459e13f978d7c846f4", alg: "sha256"},
			},
		},
		{
			name: "blank lines and comments are ignored",
			data: "\n# checksums\n\n03ac674216f3e15c761ee1a5e255f067953623c8b388b4459e13f978d7c846f4  windows-32bit.zip\n",
			want: map[string]checksumEntry{
				"windows-32bit.zip": {hash: "03ac674216f3e15c761ee1a5e255f067953623c8b388b4459e13f978d7c846f4", alg: "sha256"},
			},
		},
		{
			name: "unrecognized digest length is ignored",
			data: "deadbeef  windows-32bit.zip\n",
			want: map[string]checksumEntry{},
		},
		{
			name: "empty file",
			data: "",
			want: map[string]checksumEntry{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseChecksumFile(tt.data))
		})
	}
}

func TestAlgForDigestLength(t *testing.T) {
	tests := []struct {
		length int
		want   string
	}{
		{40, "sha1"},
		{64, "sha256"},
		{128, "sha512"},
		{32, ""},
		{0, ""},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, algForDigestLength(tt.length))
	}
}
