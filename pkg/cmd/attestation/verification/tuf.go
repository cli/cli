package verification

import (
	_ "embed"
	"os"
	"path/filepath"

	o "github.com/cli/cli/v2/pkg/option"
	"github.com/cli/go-gh/v2/pkg/config"
	"github.com/sigstore/sigstore-go/pkg/tuf"
)

//go:embed embed/tuf-repo.github.com/root.json
var githubRoot []byte

const GitHubTUFMirror = "https://tuf-repo.github.com"

var testCacheDir string

func DefaultOptionsWithCacheSetting(tufMetadataDir o.Option[string]) *tuf.Options {
	opts := tuf.DefaultOptions()

	// The CODESPACES environment variable will be set to true in a Codespaces workspace
	if os.Getenv("CODESPACES") == "true" {
		// if the tool is being used in a Codespace, disable the local cache
		// because there is a permissions issue preventing the tuf library
		// from writing the Sigstore cache to the home directory
		opts.DisableLocalCache = true
	}

	// During tests, use a temporary directory that will be cleaned up
	if os.Getenv("GO_INTEGRATION_TEST") == "1" {
		if testCacheDir == "" {
			var err error
			testCacheDir, err = os.MkdirTemp("", "gh-tuf-cache-*")
			if err == nil {
				opts.CachePath = testCacheDir
			}
		} else {
			opts.CachePath = testCacheDir
		}
	} else {
		// Set the cache path to the provided dir, or a directory owned by the CLI
		opts.CachePath = tufMetadataDir.UnwrapOr(filepath.Join(config.CacheDir(), ".sigstore", "root"))
	}

	// Allow TUF cache for 1 day
	opts.CacheValidity = 1

	return opts
}

// CleanupTestCache removes the temporary test cache directory if it exists
func CleanupTestCache() error {
	if testCacheDir != "" {
		err := os.RemoveAll(testCacheDir)
		testCacheDir = ""
		return err
	}
	return nil
}

func GitHubTUFOptions(tufMetadataDir o.Option[string]) *tuf.Options {
	opts := DefaultOptionsWithCacheSetting(tufMetadataDir)

	opts.Root = githubRoot
	opts.RepositoryBaseURL = GitHubTUFMirror

	return opts
}
