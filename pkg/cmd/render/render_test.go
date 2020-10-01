package render

import (
	"testing"

	"github.com/MakeNowJust/heredoc"
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
			wantStderr: "failed to read file ",
		},
		{
			name: "file not exist",
			opts: &RenderOptions{
				FilePath: "filepath",
			},
			wantStderr: "failed to read file filepath",
		},
		{
			name: "notty",
			opts: &RenderOptions{
				FilePath: fixtureFile,
			},
			wantOut: heredoc.Doc(`
				name:	fixture.md
				full path:	./fixture.md
				--
				# Fixture file

				content here
				`),
		},
		{
			name: "tty",
			opts: &RenderOptions{
				FilePath: fixtureFile,
			},
			stdoutTTY: true,
			wantOut: heredoc.Doc(`
				Name: fixture.md
				Path: ./fixture.md


				  # Fixture file                                                              
                                                                                  
				  content here                                                                


			`),
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
