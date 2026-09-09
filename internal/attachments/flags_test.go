package attachments

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/google/shlex"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// attachCmd builds a command shaped like the ones that take --attach.
func attachCmd(t *testing.T, input string) (*cobra.Command, *Flag) {
	t.Helper()

	cmd := &cobra.Command{Use: "comment"}
	attachFlag := AddFlag(cmd)

	argv, err := shlex.Split(input)
	require.NoError(t, err)
	require.NoError(t, cmd.Flags().Parse(argv))

	return cmd, attachFlag
}

// Resolved through the public entry point, so a fixture is built the way a
// command builds one.
func assetsFromArgs(t *testing.T, args ...string) ([]UserAsset, error) {
	t.Helper()

	cmd := &cobra.Command{}
	attachFlag := AddFlag(cmd)

	argv := make([]string, 0, len(args)*2)
	for _, arg := range args {
		argv = append(argv, "--attach", arg)
	}
	require.NoError(t, cmd.Flags().Parse(argv))

	return attachFlag.UserAssets()
}

func TestAddFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "not passed",
			input: "",
			want:  []string{},
		},
		{
			name:  "one file",
			input: "--attach ./login.png",
			want:  []string{"./login.png"},
		},
		{
			name:  "repeated, in the order written",
			input: "--attach './login.png#FIRST' --attach ./error-state.png",
			want:  []string{"./login.png#FIRST", "./error-state.png"},
		},
		{
			name:  "a comma is part of the filename",
			input: "--attach './before,after.png'",
			want:  []string{"./before,after.png"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Given a command with the repeatable attachment flag
			cmd, attachFlag := attachCmd(t, tt.input)

			// When the parsed flag values are read
			values, err := cmd.Flags().GetStringArray("attach")

			// Then values retain their spelling and order
			require.NoError(t, err)
			assert.Equal(t, tt.want, values)
			assert.Equal(t, tt.input != "", attachFlag.Changed(), "Changed reports whether --attach was supplied")
			flag := cmd.Flags().Lookup("attach")
			assert.Empty(t, flag.Shorthand)
			assert.Equal(t, "Attach an image or video `file`, in '<file>#<image alt text>' format", flag.Usage)
		})
	}
}

func TestAttachmentTelemetry(t *testing.T) {
	t.Parallel()

	t.Run("flag not passed ignores operation updates", func(t *testing.T) {
		t.Parallel()

		// Given a command with no attachment flag
		_, attachFlag := attachCmd(t, "")
		recorder := &telemetry.InvocationRecorderSpy{}

		// When an operation update is attempted without an attachment event
		event := BeginTelemetry(recorder, "gh issue comment", attachFlag.Count())
		event.RecordOperations(UploadResult{AppendOperations: 1, ReplaceOperations: 1})

		// Then no event or sampling promotion occurs
		assert.Nil(t, event)
		assert.Empty(t, recorder.Events())
		assert.Zero(t, recorder.LastSampleRate)
	})

	tests := []struct {
		name      string
		input     string
		wantCount int64
	}{
		{
			name:      "one attachment",
			input:     "--attach ./first.png",
			wantCount: 1,
		},
		{
			name:      "several attachments before validation",
			input:     "--attach ./first.png --attach ./second.png --attach ./third.png",
			wantCount: 3,
		},
		{
			name:      "empty path counts before validation",
			input:     `--attach ""`,
			wantCount: 1,
		},
		{
			name:      "over limit counts before validation",
			input:     strings.Repeat("--attach ./missing.png ", maxAttachments+1),
			wantCount: maxAttachments + 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Given raw attachment inputs that have not been validated
			_, attachFlag := attachCmd(t, tt.input)
			recorder := &telemetry.InvocationRecorderSpy{}

			// When an attachment event is recorded without any upload operations
			BeginTelemetry(recorder, "gh issue comment", attachFlag.Count())

			// Then the raw count is retained at full sampling with zero operations
			assert.Equal(t, ghtelemetry.SAMPLE_ALL, recorder.LastSampleRate)
			assert.Equal(t, []ghtelemetry.Event{{
				Type: "attachment_invocation",
				Dimensions: ghtelemetry.Dimensions{
					"command": "gh issue comment",
				},
				Measures: ghtelemetry.Measures{
					"attach_count":      tt.wantCount,
					"append_ops_count":  0,
					"replace_ops_count": 0,
				},
			}}, recorder.Events())
		})
	}

	t.Run("completed markdown operations", func(t *testing.T) {
		t.Parallel()

		// Given a pending event for four attachments
		recorder := &telemetry.InvocationRecorderSpy{}
		event := BeginTelemetry(recorder, "gh issue comment", 4)

		// When four uploads produce one append and three replacements
		event.RecordOperations(UploadResult{Uploaded: 4, AppendOperations: 1, ReplaceOperations: 3})

		// Then telemetry retains the attachment count and records one append and three replacements
		events := recorder.Events()
		require.Len(t, events, 1)
		assert.Equal(t, ghtelemetry.Measures{
			"attach_count":      4,
			"append_ops_count":  1,
			"replace_ops_count": 3,
		}, events[0].Measures)
	})
}

func TestFlagUserAssets(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T)
		input     string
		wantPaths []string
		wantAlts  []string
		wantErr   string
		// wantErrIs covers an error whose text the operating system words
		// differently, so the assertion cannot be on the message.
		wantErrIs error
	}{
		{
			name:  "not passed",
			input: "",
		},
		{
			name:      "one file",
			input:     "--attach ./shot.png",
			wantPaths: []string{"./shot.png"},
		},
		{
			name:      "a filename containing a hash stays whole",
			input:     "--attach './shot#dark.png'",
			wantPaths: []string{"./shot#dark.png"},
			wantAlts:  []string{"shot#dark"},
		},
		{
			name:      "alt text can contain a hash",
			input:     "--attach './caption.png#first#second'",
			wantPaths: []string{"./caption.png"},
			wantAlts:  []string{"first#second"},
		},
		{
			name:      "the longest existing path wins",
			input:     "--attach './shot#dark.png#first.png#second'",
			wantPaths: []string{"./shot#dark.png#first.png"},
			wantAlts:  []string{"second"},
		},
		{
			name:      "a missing path falls back at the last hash",
			input:     "--attach './gone.png#caption'",
			wantErr:   "./gone.png: ",
			wantErrIs: fs.ErrNotExist,
		},
		{
			name:    "too many attachments are rejected before filesystem validation",
			input:   strings.Repeat("--attach ./missing.txt ", maxAttachments+1),
			wantErr: "`--attach` accepts at most 50 values per command",
		},
		{
			name:      "a file that does not exist",
			input:     "--attach ./gone.png",
			wantErr:   "./gone.png: ",
			wantErrIs: fs.ErrNotExist,
		},
		{
			// An explicitly empty value is invalid, not an absent flag.
			name:    "a lone empty value",
			input:   `--attach ""`,
			wantErr: "cannot attach an empty path; --attach needs a file path",
		},
		{
			name:    "an empty value beside a real one",
			input:   `--attach ./shot.png --attach ""`,
			wantErr: "cannot attach an empty path; --attach needs a file path",
		},
		{
			name:    "standard input",
			input:   "--attach -",
			wantErr: "cannot attach standard input; --attach needs a file path",
		},
		{
			name:      "a value holding a comma stays one path",
			input:     `--attach ./before,after.png`,
			wantPaths: []string{"./before,after.png"},
		},
		{
			name:    "the same file twice",
			input:   "--attach ./a.png --attach './a.png#Another caption'",
			wantErr: "./a.png and ./a.png are the same file; attached files must be unique",
		},
		{
			name:    "the same file under two different paths",
			input:   "--attach ./a.png --attach a.png",
			wantErr: "./a.png and a.png are the same file; attached files must be unique",
		},
		{
			name: "a symlink and the file it points at",
			setup: func(t *testing.T) {
				// Creating one needs a privilege Windows does not grant by
				// default, so a machine that cannot make a symlink cannot run
				// this case either.
				if err := os.Symlink("a.png", "link.png"); err != nil {
					t.Skipf("cannot create a symlink here: %v", err)
				}
			},
			input:   "--attach ./a.png --attach ./link.png",
			wantErr: "./a.png and ./link.png are the same file; attached files must be unique",
		},
		{
			name: "a hard link and the file it shares",
			setup: func(t *testing.T) {
				require.NoError(t, os.Link("a.png", "hard.png"))
			},
			input:   "--attach ./a.png --attach ./hard.png",
			wantErr: "./a.png and ./hard.png are the same file; attached files must be unique",
		},
		{
			name:    "reports the first invalid file",
			input:   "--attach ./a.png --attach ./notes.txt",
			wantErr: "./notes.txt is not a supported file type (supported: png, jpg, jpeg, gif, webp, svg, mp4, mov, webm)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given the named files and raw attachment arguments
			t.Chdir(t.TempDir())
			for _, name := range []string{
				"shot.png",
				"shot#dark.png",
				"shot#dark.png#first.png",
				"caption.png",
				"a.png",
				"before,after.png",
				"notes.txt",
			} {
				require.NoError(t, os.WriteFile(name, []byte("the bytes"), 0o600))
			}
			if tt.setup != nil {
				tt.setup(t)
			}

			_, attachFlag := attachCmd(t, tt.input)

			// When the attachment files are resolved
			resolved, err := attachFlag.UserAssets()

			// Then invalid inputs fail or the resolved paths retain their order
			if tt.wantErrIs != nil {
				require.ErrorIs(t, err, tt.wantErrIs)
				require.ErrorContains(t, err, tt.wantErr)
				assert.Nil(t, resolved)
				return
			}
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Nil(t, resolved)
				return
			}
			require.NoError(t, err)

			var paths, alts []string
			for _, a := range resolved {
				paths = append(paths, a.Path())
				alts = append(alts, a.getAsset().alt)
			}
			assert.Equal(t, tt.wantPaths, paths)
			if tt.wantAlts != nil {
				assert.Equal(t, tt.wantAlts, alts)
			}
		})
	}

	t.Run("preserves input order", func(t *testing.T) {
		// Given two files with different contents
		t.Chdir(t.TempDir())
		require.NoError(t, os.WriteFile("a.png", []byte("first"), 0o600))
		require.NoError(t, os.WriteFile("b.png", []byte("second"), 0o600))

		// When files are supplied in non-alphabetical order
		resolved, err := assetsFromArgs(t, "./b.png", "./a.png")

		// Then the resolved files retain that order
		require.NoError(t, err)
		require.Len(t, resolved, 2)
		assert.Equal(t, []string{"./b.png", "./a.png"}, []string{resolved[0].Path(), resolved[1].Path()})
	})

	t.Run("allows distinct files with identical contents", func(t *testing.T) {
		// Given two separate files containing the same bytes
		t.Chdir(t.TempDir())
		require.NoError(t, os.WriteFile("a.png", []byte("same contents"), 0o600))
		require.NoError(t, os.WriteFile("b.png", []byte("same contents"), 0o600))

		// When both files are supplied
		resolved, err := assetsFromArgs(t, "./a.png", "./b.png")

		// Then both files are accepted
		require.NoError(t, err)
		require.Len(t, resolved, 2)
		assert.ElementsMatch(t, []string{"./a.png", "./b.png"}, []string{resolved[0].Path(), resolved[1].Path()})
	})

	t.Run("maximum number of attachments", func(t *testing.T) {
		names := make([]string, maxAttachments)
		for i := range names {
			names[i] = fmt.Sprintf("attachment-%d.png", i)
		}
		assert.Len(t, NewTestAssets(t, names...), maxAttachments)
	})
}
