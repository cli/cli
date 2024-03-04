package verification

import (
	"embed"
	"fmt"
	"os"

	"github.com/sigstore/sigstore-go/pkg/tuf"
)

//go:embed embed
var embeddedRepos embed.FS

const GitHubTUFMirror = "https://tuf-repo.github.com"

// readEmbeddedRoot reads the embedded trust anchor for the given URL
func readEmbeddedRoot(url string) ([]byte, error) {
	// the embed file system always uses forward slashes, even on Windows
	p := fmt.Sprintf("embed/%s/root.json", tuf.URLToPath(url))

	b, err := embeddedRepos.ReadFile(p)
	if err != nil {
		return nil, err
	}

	return b, nil
}

func DefaultOptionsWithCacheSetting() *tuf.Options {
	opts := tuf.DefaultOptions()

	// The CODESPACES environment variable will be set to true in a Codespaces workspace
	if os.Getenv("CODESPACES") == "true" {
		// if the tool is being used in a Codespace, disable the local cache
		// because there is a permissions issue preventing the tuf library
		// from writing the Sigstore cache to the home directory
		opts.DisableLocalCache = true
	}

	return opts
}

func GitHubTUFOptions() (*tuf.Options, error) {
	opts := DefaultOptionsWithCacheSetting()

	// replace root and mirror url
	root, err := readEmbeddedRoot(GitHubTUFMirror)
	if err != nil {
		return nil, err
	}
	opts.Root = root
	opts.RepositoryBaseURL = GitHubTUFMirror

	return opts, nil
}
