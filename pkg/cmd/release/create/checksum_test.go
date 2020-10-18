package create

import (
	"io"
	"testing"

	"github.com/cli/cli/pkg/cmd/release/shared"
	"github.com/stretchr/testify/assert"

	"github.com/spf13/afero"
)

func getMockFile(data string, filename string) afero.File {
	fs := new(afero.MemMapFs)
	f, _ := afero.TempFile(fs, "", filename)
	f.WriteString(data)
	f.Close()
	file, _ := fs.Open(f.Name())

	return file
}

func generateMockAssets() []*shared.AssetForUpload {
	Assets := []*shared.AssetForUpload{
		{
			Name:  "datafile1",
			Label: "",
			Open: func() (io.ReadCloser, error) {
				return getMockFile("datafile1", "datafile1"), nil
			},
		},
		{
			Name:  "datafile2",
			Label: "Linux build",
			Open: func() (io.ReadCloser, error) {
				return getMockFile("nothing", "datafile2"), nil
			},
		},
	}
	return Assets
}

func Test_ChecksumCreate(t *testing.T) {

	tests := []struct {
		name    string
		args    afero.File
		want    string
		wantErr string
	}{
		{
			name: "Generate Checksum from file",
			args: getMockFile("data", "randomfile"),
			want: "3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checksum, err := generateChecksum(tt.args)
			assert.Nil(t, err)
			assert.Equal(t, tt.want, checksum)
		})
	}
}

func Test_ChecksumFromAsset(t *testing.T) {

	tests := []struct {
		name    string
		args    []*shared.AssetForUpload
		want    map[string]string
		wantErr string
	}{
		{
			name: "Generate Checksum for assets",
			args: generateMockAssets(),
			want: map[string]string{
				"datafile1": "202018daae6cd9635e5b3ba3e5d9014c4998981700832d7a6c7644c675f2a4b5",
				"datafile2": "1785cfc3bc6ac7738e8b38cdccd1af12563c2b9070e07af336a1bf8c0f772b6a",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checksumMap, err := generateChecksumFromAssets(tt.args)
			assert.Nil(t, err)
			assert.Equal(t, tt.want, checksumMap)
		})
	}
}
