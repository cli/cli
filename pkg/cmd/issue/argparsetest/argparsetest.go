package argparsetest

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/google/shlex"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCmdFunc represents the typical function signature we use for creating commands e.g. `NewCmdView`.
//
// It is generic over `T` as each command construction has their own Options type e.g. `ViewOptions`
type newCmdFunc[T any] func(f *cmdutil.Factory, runF func(*T) error) *cobra.Command

// TestArgParsing is a test helper that verifies that issue commands correctly parse the `{issue number | url}`
// positional arg into an issue number and base repo.
//
// Looking through the existing tests, I noticed that the coverage was pretty smattered.
// Since nearly all issue commands only accept a single positional argument, we are able to reuse this test helper.
// Commands with no further flags or args can use this solely.
// Commands with extra flags use this and further table tests.
// Commands with extra required positional arguments (like `transfer`) cannot use this. They duplicate these cases inline.
func TestArgParsing[T any](t *testing.T, fn newCmdFunc[T]) {
	tests := []struct {
		name                string
		input               string
		expectedissueNumber int
		expectedBaseRepo    ghrepo.Interface
		expectErr           bool
	}{
		{
			name:      "no argument",
			input:     "",
			expectErr: true,
		},
		{
			name:                "issue number argument",
			input:               "23 --repo owner/repo",
			expectedissueNumber: 23,
			expectedBaseRepo:    ghrepo.New("owner", "repo"),
		},
		{
			name: "argument is hash prefixed number",
			// Escaping is required here to avoid what I think is shellex treating it as a comment.
			input:               "\\#23 --repo owner/repo",
			expectedissueNumber: 23,
			expectedBaseRepo:    ghrepo.New("owner", "repo"),
		},
		{
			name:                "argument is a URL",
			input:               "https://github.com/cli/cli/issues/23",
			expectedissueNumber: 23,
			expectedBaseRepo:    ghrepo.New("cli", "cli"),
		},
		{
			name:      "argument cannot be parsed to an issue",
			input:     "unparseable",
			expectErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{}

			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)

			var gotOpts T
			cmd := fn(f, func(opts *T) error {
				gotOpts = *opts
				return nil
			})

			cmdutil.EnableRepoOverride(cmd, f)

			// TODO: remember why we do this
			cmd.Flags().BoolP("help", "x", false, "")

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()

			if tt.expectErr {
				require.Error(t, err)
				return
			} else {
				require.NoError(t, err)
			}

			actualIssueNumber := issueNumberFromOpts(t, gotOpts)
			assert.Equal(t, tt.expectedissueNumber, actualIssueNumber)

			actualBaseRepo := baseRepoFromOpts(t, gotOpts)
			assert.True(
				t,
				ghrepo.IsSame(tt.expectedBaseRepo, actualBaseRepo),
				"expected base repo %+v, got %+v", tt.expectedBaseRepo, actualBaseRepo,
			)
		})
	}
}

func issueNumberFromOpts(t *testing.T, v any) int {
	rv := reflect.ValueOf(v)
	field := rv.FieldByName("IssueNumber")
	if !field.IsValid() || field.Kind() != reflect.Int {
		t.Fatalf("Type %T does not have IssueNumber int field", v)
	}
	return int(field.Int())
}

func baseRepoFromOpts(t *testing.T, v any) ghrepo.Interface {
	rv := reflect.ValueOf(v)
	field := rv.FieldByName("BaseRepo")
	// check whether the field is valid and of type func() (ghrepo.Interface, error)
	if !field.IsValid() || field.Kind() != reflect.Func {
		t.Fatalf("Type %T does not have BaseRepo func field", v)
	}
	// call the function and check the return value
	results := field.Call([]reflect.Value{})
	if len(results) != 2 {
		t.Fatalf("%T.BaseRepo() does not return two values", v)
	}
	if !results[1].IsNil() {
		t.Fatalf("%T.BaseRepo() returned an error: %v", v, results[1].Interface())
	}
	return results[0].Interface().(ghrepo.Interface)
}
