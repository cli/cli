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

func outputGridFixture() error {
	return fmt.Errorf("owned output-buffer fixtures require Unix")
}

func nativeOutputBudgetCases(t *testing.T) {
	t.Skip("Owned output-buffer fixtures require Unix.")
}
