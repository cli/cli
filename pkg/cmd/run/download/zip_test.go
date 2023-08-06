package download

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_extractZip(t *testing.T) {
	tmpDir := t.TempDir()
	extractPath := filepath.Join(tmpDir, "artifact")

	zipFile, err := zip.OpenReader("./fixtures/myproject.zip")
	require.NoError(t, err)
	defer zipFile.Close()

	err = extractZip(&zipFile.Reader, extractPath)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(extractPath, "src", "main.go"))
	require.NoError(t, err)
}

func Test_filepathDescendsFrom(t *testing.T) {
	type args struct {
		p   string
		dir string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "root child",
			args: args{
				p:   filepath.FromSlash("/hoi.txt"),
				dir: filepath.FromSlash("/"),
			},
			want: true,
		},
		{
			name: "abs descendant",
			args: args{
				p:   filepath.FromSlash("/var/logs/hoi.txt"),
				dir: filepath.FromSlash("/"),
			},
			want: true,
		},
		{
			name: "abs trailing slash",
			args: args{
				p:   filepath.FromSlash("/var/logs/hoi.txt"),
				dir: filepath.FromSlash("/var/logs/"),
			},
			want: true,
		},
		{
			name: "abs mismatch",
			args: args{
				p:   filepath.FromSlash("/var/logs/hoi.txt"),
				dir: filepath.FromSlash("/var/pids"),
			},
			want: false,
		},
		{
			name: "abs partial prefix",
			args: args{
				p:   filepath.FromSlash("/var/logs/hoi.txt"),
				dir: filepath.FromSlash("/var/log"),
			},
			want: false,
		},
		{
			name: "rel child",
			args: args{
				p:   filepath.FromSlash("hoi.txt"),
				dir: filepath.FromSlash("."),
			},
			want: true,
		},
		{
			name: "rel descendant",
			args: args{
				p:   filepath.FromSlash("./log/hoi.txt"),
				dir: filepath.FromSlash("."),
			},
			want: true,
		},
		{
			name: "mixed rel styles",
			args: args{
				p:   filepath.FromSlash("./log/hoi.txt"),
				dir: filepath.FromSlash("log"),
			},
			want: true,
		},
		{
			name: "rel clean",
			args: args{
				p:   filepath.FromSlash("cats/../dogs/pug.txt"),
				dir: filepath.FromSlash("dogs"),
			},
			want: true,
		},
		{
			name: "rel mismatch",
			args: args{
				p:   filepath.FromSlash("dogs/pug.txt"),
				dir: filepath.FromSlash("dog"),
			},
			want: false,
		},
		{
			name: "rel breakout",
			args: args{
				p:   filepath.FromSlash("../escape.txt"),
				dir: filepath.FromSlash("."),
			},
			want: false,
		},
		{
			name: "rel sneaky breakout",
			args: args{
				p:   filepath.FromSlash("dogs/../../escape.txt"),
				dir: filepath.FromSlash("dogs"),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filepathDescendsFrom(tt.args.p, tt.args.dir); got != tt.want {
				t.Errorf("filepathDescendsFrom() = %v, want %v", got, tt.want)
			}
		})
	}
}
