package verification

import (
	_ "embed"
	"os"
	"path/filepath"

	"github.com/cli/go-gh/v2/pkg/config"
	"github.com/sigstore/sigstore-go/pkg/tuf"
)

//go:embed embed/tuf-repo.github.com/root.json
var githubRoot []byte

const GitHubTUFMirror = "https://tuf-repo.github.com"

func DefaultOptionsWithCacheSetting() *tuf.Options {
	opts := tuf.DefaultOptions()

	// The CODESPACES environment variable will be set to true in a Codespaces workspace
	if os.Getenv("CODESPACES") == "true" {
		// if the tool is being used in a Codespace, disable the local cache
		// because there is a permissions issue preventing the tuf library
		// from writing the Sigstore cache to the home directory
		opts.DisableLocalCache = true
	}

	// Set the cache path to a directory owned by the CLI
	opts.CachePath = filepath.Join(config.CacheDir(), ".sigstore", "root")

	// Allow TUF cache for 1 day
	opts.CacheValidity = 1

	return opts
}

func GitHubTUFOptions() *tuf.Options {
	opts := DefaultOptionsWithCacheSetting()

	opts.Root = githubRoot
	opts.RepositoryBaseURL = GitHubTUFMirror

	return opts
}
