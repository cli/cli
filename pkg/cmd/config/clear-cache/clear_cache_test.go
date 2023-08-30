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
	cacheDir := filepath.Join(t.TempDir(), "gh-cli-cache")
	ios, _, stdout, stderr := iostreams.Test()
	opts := &ClearCacheOptions{
		IO:       ios,
		CacheDir: cacheDir,
	}

	if err := os.Mkdir(opts.CacheDir, 0600); err != nil {
		assert.NoError(t, err)
	}

	if err := clearCacheRun(opts); err != nil {
		assert.NoError(t, err)
	}

	assert.NoDirExistsf(t, opts.CacheDir, fmt.Sprintf("Cache dir: %s still exists", opts.CacheDir))
	assert.Equal(t, "✓ Cleared the cache\n", stdout.String())
	assert.Equal(t, "", stderr.String())
}
