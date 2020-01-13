package command

import (
	"fmt"
	"testing"
)

func TestChangelogURL(t *testing.T) {
	tag := "v0.3.2"
	url := fmt.Sprintf("https://github.com/github/homebrew-gh/releases/tag/v0.3.2")
	result := changelogURL(tag)
	if result != url {
		t.Errorf("expected %s to create url %s but got %s", tag, url, result)
	}

	tag = "0.3.5-90-gdd3f0e0"
	url = fmt.Sprintf("https://github.com/github/homebrew-gh/releases/latest")
	result = changelogURL(tag)
	if result != url {
		t.Errorf("expected %s to create url %s but got %s", tag, url, result)
	}

	tag = "deadbeef"
	url = fmt.Sprintf("https://github.com/github/homebrew-gh/releases/latest")
	result = changelogURL(tag)
	if result != url {
		t.Errorf("expected %s to create url %s but got %s", tag, url, result)
	}
}
