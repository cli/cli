package shared

import "testing"

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
