package jsonfieldstest

import (
	"strings"
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func jsonFieldsFor(cmd *cobra.Command) []string {
	// This annotation is added by the `cmdutil.AddJSONFlags` function.
	//
	// This is an extremely janky way to get access to this information but due to the fact we pass
	// around concrete cobra.Command structs, there's no viable way to have typed access to the fields.
	//
	// It's also kind of fragile because it's several hops away from the code that actually validates the usage
	// of these flags, so it's possible for things to get out of sync.
	stringFields, ok := cmd.Annotations["help:json-fields"]
	if !ok {
		return nil
	}

	return strings.Split(stringFields, ",")
}

// Haha bite me. I probably would just make the ListCmd take a pointer but I'm a masochist and I've never
// played with type constraints before.
type CmdRunF[T any] interface {
	func(*T) error | func(T) error
}

// NewCmdFunc represents the typical function signature we use for creating commands e.g. `NewCmdView`.
//
// It is generic over `T` as each command construction has their own Options type e.g. `ViewOptions`
type NewCmdFunc[T any, F CmdRunF[T]] func(f *cmdutil.Factory, runF F) *cobra.Command

// ExpectCommandToSupportJSONFields asserts that the provided command supports exactly the provided fields.
// Ordering of the expected fields is not important.
//
// Make sure you are not pointing to the same slice of fields in the test and the implementation.
// It can be a little tedious to rewrite the fields inline in the test but it's significantly more useful because:
//   - It forces the test author to think about and convey exactly the expected fields for a command
//   - It avoids accidentally adding fields to a command, and the test passing unintentionally
func ExpectCommandToSupportJSONFields[T any, F CmdRunF[T]](t *testing.T, fn NewCmdFunc[T, F], expectedFields []string) {
	t.Helper()

	actualFields := jsonFieldsFor(fn(&cmdutil.Factory{}, nil))
	assert.Equal(t, len(actualFields), len(expectedFields), "expected number of fields to match")
	require.ElementsMatch(t, expectedFields, actualFields)
}
