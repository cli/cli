package attachments

import (
	"io/fs"
	"os"
	"testing"

	"github.com/google/shlex"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// attachCmd builds a command shaped like the ones that take --attach.
// withAttach is false for a command that has not registered the flag.
func attachCmd(t *testing.T, withAttach bool, input string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "comment"}
	cmd.Flags().Bool("web", false, "")
	cmd.Flags().Bool("delete-last", false, "")
	cmd.Flags().Bool("dry-run", false, "")

	if withAttach {
		AddFlag(cmd)
	}

	argv, err := shlex.Split(input)
	require.NoError(t, err)
	require.NoError(t, cmd.Flags().Parse(argv))

	return cmd
}

// Resolved through the public entry point, so a fixture is built the way a
// command builds one.
func assetsFromArgs(t *testing.T, args ...string) ([]UserAsset, error) {
	t.Helper()

	cmd := &cobra.Command{}
	AddFlag(cmd)

	argv := make([]string, 0, len(args)*2)
	for _, arg := range args {
		argv = append(argv, "--attach", arg)
	}
	require.NoError(t, cmd.Flags().Parse(argv))

	return FromFlagValues(cmd)
}

func TestAddFlag(t *testing.T) {
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
			cmd := attachCmd(t, true, tt.input)

			// Read the way FromFlagValues does, so this cannot pass while
			// the flag it describes reads differently.
			slice, ok := cmd.Flags().Lookup(flagName).Value.(pflag.SliceValue)
			require.True(t, ok)
			assert.Equal(t, tt.want, slice.GetSlice())
			assert.Empty(t, cmd.Flags().Lookup(flagName).Shorthand)
			assert.Equal(t, "Attach an image or video `file`, in '<file>#<image alt text>' format", cmd.Flags().Lookup(flagName).Usage)
		})
	}
}

func TestFromFlagValues(t *testing.T) {
	tests := []struct {
		name       string
		withAttach bool
		setup      func(t *testing.T)
		input      string
		wantPaths  []string
		wantAlts   []string
		wantErr    string
		// wantErrIs covers an error whose text the operating system words
		// differently, so the assertion cannot be on the message.
		wantErrIs error
	}{
		{
			name:       "not registered",
			withAttach: false,
			wantErr:    "comment does not register --attach",
		},
		{
			name:       "registered but not passed",
			withAttach: true,
			input:      "",
		},
		{
			name:       "one file",
			withAttach: true,
			input:      "--attach ./shot.png",
			wantPaths:  []string{"./shot.png"},
		},
		{
			name:       "a filename containing a hash stays whole",
			withAttach: true,
			input:      "--attach './shot#dark.png'",
			wantPaths:  []string{"./shot#dark.png"},
			wantAlts:   []string{"shot#dark"},
		},
		{
			name:       "alt text can contain a hash",
			withAttach: true,
			input:      "--attach './caption.png#first#second'",
			wantPaths:  []string{"./caption.png"},
			wantAlts:   []string{"first#second"},
		},
		{
			name:       "the longest existing path wins",
			withAttach: true,
			input:      "--attach './shot#dark.png#first.png#second'",
			wantPaths:  []string{"./shot#dark.png#first.png"},
			wantAlts:   []string{"second"},
		},
		{
			name:       "a missing path falls back at the last hash",
			withAttach: true,
			input:      "--attach './gone.png#caption'",
			wantErr:    "./gone.png: ",
			wantErrIs:  fs.ErrNotExist,
		},
		{
			name:       "several files, in the order written",
			withAttach: true,
			input:      "--attach ./b.png --attach ./a.png",
			wantPaths:  []string{"./b.png", "./a.png"},
		},
		{
			name:       "a file that does not exist",
			withAttach: true,
			input:      "--attach ./gone.png",
			wantErr:    "./gone.png: ",
			wantErrIs:  fs.ErrNotExist,
		},
		{
			// pflag reads this flag back as holding nothing at all, so
			// without the length check the command would post with no
			// attachment and no error.
			name:       "a lone empty value",
			withAttach: true,
			input:      `--attach ""`,
			wantErr:    "cannot attach an empty path; --attach needs a file path",
		},
		{
			name:       "an empty value beside a real one",
			withAttach: true,
			input:      `--attach ./shot.png --attach ""`,
			wantErr:    "cannot attach an empty path; --attach needs a file path",
		},
		{
			name:       "standard input",
			withAttach: true,
			input:      "--attach -",
			wantErr:    "cannot attach standard input; --attach needs a file path",
		},
		{
			name:       "a value holding a comma stays one path",
			withAttach: true,
			input:      `--attach ./before,after.png`,
			wantPaths:  []string{"./before,after.png"},
		},
		{
			name:       "web without attach",
			withAttach: true,
			input:      "--web",
		},
		{
			name:       "delete-last without attach",
			withAttach: true,
			input:      "--delete-last",
		},
		{
			name:       "dry-run without attach",
			withAttach: true,
			input:      "--dry-run",
		},
		{
			name:       "web on a command with no attach flag",
			withAttach: false,
			input:      "--web",
			wantErr:    "comment does not register --attach",
		},
		{
			name:       "attach with web",
			withAttach: true,
			input:      "--attach ./shot.png --web",
			wantErr:    "`--attach` is not supported when using `--web`",
		},
		{
			name:       "attach with delete-last",
			withAttach: true,
			input:      "--attach ./shot.png --delete-last",
			wantErr:    "`--attach` is not supported when using `--delete-last`",
		},
		{
			name:       "attach with dry-run",
			withAttach: true,
			input:      "--attach ./shot.png --dry-run",
			wantErr:    "`--attach` is not supported when using `--dry-run`",
		},
		{
			// The conflict is checked before the paths are read, so a caller
			// cannot reach the disk in a mode the asset could never be written
			// in.
			name:       "a flag conflict is reported before a missing file",
			withAttach: true,
			input:      "--attach ./gone.png --web",
			wantErr:    "`--attach` is not supported when using `--web`",
		},
		{
			name:       "keeps the order the arguments were written in",
			withAttach: true,
			input:      "--attach './b.png#Second' --attach ./a.png --attach ./c.mp4",
			wantPaths:  []string{"./b.png", "./a.png", "./c.mp4"},
		},
		{
			name:       "the same file twice",
			withAttach: true,
			input:      "--attach ./a.png --attach './a.png#Another caption'",
			wantErr:    "./a.png and ./a.png are the same file; attached files must be unique",
		},
		{
			name:       "the same file under two different paths",
			withAttach: true,
			input:      "--attach ./a.png --attach a.png",
			wantErr:    "./a.png and a.png are the same file; attached files must be unique",
		},
		{
			name:       "a symlink and the file it points at",
			withAttach: true,
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
			name:       "a hard link and the file it shares",
			withAttach: true,
			setup: func(t *testing.T) {
				require.NoError(t, os.Link("a.png", "hard.png"))
			},
			input:   "--attach ./a.png --attach ./hard.png",
			wantErr: "./a.png and ./hard.png are the same file; attached files must be unique",
		},
		{
			// GitHub gives each its own asset URL.
			name:       "two separate files with identical contents",
			withAttach: true,
			input:      "--attach ./a.png --attach ./b.png",
			wantPaths:  []string{"./a.png", "./b.png"},
		},
		{
			name:       "reports the first invalid file",
			withAttach: true,
			input:      "--attach ./a.png --attach ./notes.txt",
			wantErr:    "./notes.txt is not a supported file type (supported: png, jpg, jpeg, gif, webp, svg, mp4, mov, webm)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			for _, name := range []string{
				"shot.png",
				"shot#dark.png",
				"shot#dark.png#first.png",
				"caption.png",
				"a.png",
				"b.png",
				"c.mp4",
				"before,after.png",
				"notes.txt",
			} {
				require.NoError(t, os.WriteFile(name, []byte("the bytes"), 0o600))
			}
			if tt.setup != nil {
				tt.setup(t)
			}

			cmd := attachCmd(t, tt.withAttach, tt.input)

			resolved, err := FromFlagValues(cmd)

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
}
