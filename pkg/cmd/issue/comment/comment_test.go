package comment

import (
	"bytes"
	"testing"

	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdComment(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		output   CommentOptions
		wantsErr bool
	}{
		{
			name:     "no arguments",
			input:    "",
			output:   CommentOptions{},
			wantsErr: true,
		},
		{
			name:  "issue number",
			input: "1",
			output: CommentOptions{
				SelectorArg: "1",
				Interactive: true,
				InputType:   0,
				Body:        "",
			},
			wantsErr: false,
		},
		{
			name:  "issue url",
			input: "https://github.com/OWNER/REPO/issues/12",
			output: CommentOptions{
				SelectorArg: "https://github.com/OWNER/REPO/issues/12",
				Interactive: true,
				InputType:   0,
				Body:        "",
			},
			wantsErr: false,
		},
		{
			name:  "body flag",
			input: "1 --body test",
			output: CommentOptions{
				SelectorArg: "1",
				Interactive: false,
				InputType:   inline,
				Body:        "test",
			},
			wantsErr: false,
		},
		{
			name:  "editor flag",
			input: "1 --editor",
			output: CommentOptions{
				SelectorArg: "1",
				Interactive: false,
				InputType:   editor,
				Body:        "",
			},
			wantsErr: false,
		},
		{
			name:  "web flag",
			input: "1 --web",
			output: CommentOptions{
				SelectorArg: "1",
				Interactive: false,
				InputType:   web,
				Body:        "",
			},
			wantsErr: false,
		},
		{
			name:     "editor and web flags",
			input:    "1 --editor --web",
			output:   CommentOptions{},
			wantsErr: true,
		},
		{
			name:     "editor and body flags",
			input:    "1 --editor --body test",
			output:   CommentOptions{},
			wantsErr: true,
		},
		{
			name:     "web and body flags",
			input:    "1 --web --body test",
			output:   CommentOptions{},
			wantsErr: true,
		},
		{
			name:     "editor, web, and body flags",
			input:    "1 --editor --web --body test",
			output:   CommentOptions{},
			wantsErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			io.SetStdoutTTY(true)
			io.SetStdinTTY(true)
			io.SetStderrTTY(true)

			f := &cmdutil.Factory{
				IOStreams: io,
			}

			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)

			var gotOpts *CommentOptions
			cmd := NewCmdComment(f, func(opts *CommentOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.Flags().BoolP("help", "x", false, "")

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.output.SelectorArg, gotOpts.SelectorArg)
			assert.Equal(t, tt.output.Interactive, gotOpts.Interactive)
			assert.Equal(t, tt.output.InputType, gotOpts.InputType)
			assert.Equal(t, tt.output.Body, gotOpts.Body)
		})
	}
}

func Test_commentRun(t *testing.T) {
}
