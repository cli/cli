package clearcache

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func TestClearCacheRun(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()

	opts := &ClearCacheOptions{
		IO:       ios,
		CacheDir: filepath.Join(t.TempDir(), "gh-cli-cache"),
	}

	err := os.Mkdir(opts.CacheDir, 0600)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = clearCacheRun(opts)
	assert.NoError(t, err)
	assert.NoDirExistsf(t, opts.CacheDir, fmt.Sprintf("Cache dir: %s still exists", opts.CacheDir))
	assert.Equal(t, fmt.Sprintf("Cleared cache dir: %s\n", opts.CacheDir), stdout.String())
	assert.Equal(t, "", stderr.String())
}
