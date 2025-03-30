package tableprinter_test

import (
	"testing"

	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

func TestHeadersAreNotMutated(t *testing.T) {
	// Given a TTY environment so that headers are included in the table
	ios, _, _, _ := iostreams.Test()
	ios.SetStdoutTTY(true)

	// When creating a new table printer
	headers := []string{"one", "two", "three"}
	_ = tableprinter.New(ios, tableprinter.WithHeader(headers...))

	// The provided headers should not be mutated
	require.Equal(t, []string{"one", "two", "three"}, headers)
}
