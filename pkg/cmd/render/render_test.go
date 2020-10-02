package render

import (
	"testing"

	"github.com/cli/cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

const (
	fixtureFile = "./fixture.md"
)

func Test_renderRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       *RenderOptions
		repoName   string
		stdoutTTY  bool
		wantOut    string
		wantStderr string
		wantErr    bool
	}{
		{
			name:       "no file path",
			wantStderr: "Markdown file was expected, but got '.' instead\n",
		},
		{
			name: "file not exist",
			opts: &RenderOptions{
				FilePath: "filepath.md",
			},
			wantStderr: "failed to read file 'filepath.md'",
		},
		{
			name: "no markdown file",
			opts: &RenderOptions{
				FilePath: "fixture.file",
			},
			wantStderr: "Markdown file was expected, but got '.file' instead\n",
		},
		{
			name: "tty",
			opts: &RenderOptions{
				FilePath: fixtureFile,
			},
			stdoutTTY: true,
			wantOut:   "\n  # Fixture file                                                              \n\n",
		},
	}
	for _, tt := range tests {
		if tt.opts == nil {
			tt.opts = &RenderOptions{}
		}

		io, _, stdout, stderr := iostreams.Test()
		tt.opts.IO = io

		t.Run(tt.name, func(t *testing.T) {
			io.SetStdoutTTY(tt.stdoutTTY)

			if err := renderRun(tt.opts); (err != nil) != tt.wantErr {
				t.Errorf("renderRun() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.Equal(t, tt.wantStderr, stderr.String())
			assert.Equal(t, tt.wantOut, stdout.String())
		})
	}
}
