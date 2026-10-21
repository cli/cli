package atomicfile

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteReplacesFileWithCompleteContents(t *testing.T) {
	t.Parallel()

	// Given an existing file
	path := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))

	// When it is atomically rewritten
	err := Write(path, []byte("new contents"))

	// Then readers see the complete replacement
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new contents", string(data))
}

func TestConcurrentReadersOnlyObserveCompleteWrites(t *testing.T) {
	t.Parallel()

	// Given a file whose complete contents alternate between two values
	path := filepath.Join(t.TempDir(), "state.json")
	first := bytes.Repeat([]byte("a"), 64<<10)
	second := bytes.Repeat([]byte("b"), 64<<10)
	require.NoError(t, Write(path, first))
	var workers sync.WaitGroup
	results := make(chan []byte, 200)
	errs := make(chan error, 200)

	// When reads race with atomic replacements
	for range 4 {
		workers.Go(func() {
			for range 50 {
				data, err := os.ReadFile(path)
				if err != nil {
					errs <- err
					continue
				}
				results <- data
			}
		})
	}
	for i := range 50 {
		contents := first
		if i%2 == 0 {
			contents = second
		}
		require.NoError(t, Write(path, contents))
	}
	workers.Wait()
	close(results)
	close(errs)

	// Then no reader observes a partial file
	for err := range errs {
		require.NoError(t, err)
	}
	for data := range results {
		assert.True(t, bytes.Equal(data, first) || bytes.Equal(data, second), "reader observed incomplete contents")
	}
}
