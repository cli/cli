package command

import (
	"testing"
)

func TestChangelogURL(t *testing.T) {
	tag := "0.3.2"
	url := "https://github.com/cli/cli/releases/tag/v0.3.2"
	result := changelogURL(tag)
	if result != url {
		t.Errorf("expected %s to create url %s but got %s", tag, url, result)
	}

	tag = "v0.3.2"
	url = "https://github.com/cli/cli/releases/tag/v0.3.2"
	result = changelogURL(tag)
	if result != url {
		t.Errorf("expected %s to create url %s but got %s", tag, url, result)
	}

	tag = "0.3.2-pre.1"
	url = "https://github.com/cli/cli/releases/tag/v0.3.2-pre.1"
	result = changelogURL(tag)
	if result != url {
		t.Errorf("expected %s to create url %s but got %s", tag, url, result)
	}

	tag = "0.3.5-90-gdd3f0e0"
	url = "https://github.com/cli/cli/releases/latest"
	result = changelogURL(tag)
	if result != url {
		t.Errorf("expected %s to create url %s but got %s", tag, url, result)
	}

	tag = "deadbeef"
	url = "https://github.com/cli/cli/releases/latest"
	result = changelogURL(tag)
	if result != url {
		t.Errorf("expected %s to create url %s but got %s", tag, url, result)
	}
}

func TestRemoteURLFormatting_no_config(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	result := formatRemoteURL(repoForkCmd, "OWNER/REPO")
	eq(t, result, "https://github.com/OWNER/REPO.git")
}

func TestRemoteURLFormatting_ssh_config(t *testing.T) {
	cfg := `---
hosts:
  github.com:
    user: OWNER
    oauth_token: MUSTBEHIGHCUZIMATOKEN
git_protocol: ssh
`
	initBlankContext(cfg, "OWNER/REPO", "master")
	result := formatRemoteURL(repoForkCmd, "OWNER/REPO")
	eq(t, result, "git@github.com:OWNER/REPO.git")
}
