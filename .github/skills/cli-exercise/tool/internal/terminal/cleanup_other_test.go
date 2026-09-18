//go:build !unix

package terminal

import (
	"fmt"
	"testing"
)

func descendantFixture(string) error {
	return fmt.Errorf("owned process-group fixtures require Unix")
}

func nativeCleanupCases(t *testing.T) {
	t.Skip("Owned process-group cleanup requires Unix.")
}
