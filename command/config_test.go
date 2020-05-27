package command

import (
	"bytes"
	"testing"

	"github.com/cli/cli/internal/config"
)

func TestConfigGet(t *testing.T) {
	cfg := `---
hosts:
  github.com:
    user: OWNER
    oauth_token: MUSTBEHIGHCUZIMATOKEN
editor: ed
`
	initBlankContext(cfg, "OWNER/REPO", "master")

	output, err := RunCommand("config get editor")
	if err != nil {
		t.Fatalf("error running command `config get editor`: %v", err)
	}

	eq(t, output.String(), "ed\n")
}

func TestConfigGet_default(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	output, err := RunCommand("config get git_protocol")
	if err != nil {
		t.Fatalf("error running command `config get git_protocol`: %v", err)
	}

	eq(t, output.String(), "https\n")
}

func TestConfigGet_not_found(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")

	output, err := RunCommand("config get missing")
	if err != nil {
		t.Fatalf("error running command `config get missing`: %v", err)
	}

	eq(t, output.String(), "")
}

func TestConfigSet(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")

	mainBuf := bytes.Buffer{}
	hostsBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, &hostsBuf)()

	output, err := RunCommand("config set editor ed")
	if err != nil {
		t.Fatalf("error running command `config set editor ed`: %v", err)
	}

	if len(output.String()) > 0 {
		t.Errorf("expected output to be blank: %q", output.String())
	}

	expectedMain := "editor: ed\n"
	expectedHosts := `github.com:
    user: OWNER
    oauth_token: "1234567890"
`

	if mainBuf.String() != expectedMain {
		t.Errorf("expected config.yml to be %q, got %q", expectedMain, mainBuf.String())
	}
	if hostsBuf.String() != expectedHosts {
		t.Errorf("expected hosts.yml to be %q, got %q", expectedHosts, hostsBuf.String())
	}
}

func TestConfigSet_update(t *testing.T) {
	cfg := `---
hosts:
  github.com:
    user: OWNER
    oauth_token: MUSTBEHIGHCUZIMATOKEN
editor: ed
`

	initBlankContext(cfg, "OWNER/REPO", "master")

	mainBuf := bytes.Buffer{}
	hostsBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, &hostsBuf)()

	output, err := RunCommand("config set editor vim")
	if err != nil {
		t.Fatalf("error running command `config get editor`: %v", err)
	}

	if len(output.String()) > 0 {
		t.Errorf("expected output to be blank: %q", output.String())
	}

	expectedMain := "editor: vim\n"
	expectedHosts := `github.com:
    user: OWNER
    oauth_token: MUSTBEHIGHCUZIMATOKEN
`

	if mainBuf.String() != expectedMain {
		t.Errorf("expected config.yml to be %q, got %q", expectedMain, mainBuf.String())
	}
	if hostsBuf.String() != expectedHosts {
		t.Errorf("expected hosts.yml to be %q, got %q", expectedHosts, hostsBuf.String())
	}
}

func TestConfigGetHost(t *testing.T) {
	cfg := `---
hosts:
  github.com:
    git_protocol: ssh
    user: OWNER
    oauth_token: MUSTBEHIGHCUZIMATOKEN
editor: ed
git_protocol: https
`
	initBlankContext(cfg, "OWNER/REPO", "master")

	output, err := RunCommand("config get -hgithub.com git_protocol")
	if err != nil {
		t.Fatalf("error running command `config get editor`: %v", err)
	}

	eq(t, output.String(), "ssh\n")
}

func TestConfigGetHost_unset(t *testing.T) {
	cfg := `---
hosts:
  github.com:
    user: OWNER
    oauth_token: MUSTBEHIGHCUZIMATOKEN

editor: ed
git_protocol: ssh
`
	initBlankContext(cfg, "OWNER/REPO", "master")

	output, err := RunCommand("config get -hgithub.com git_protocol")
	if err != nil {
		t.Fatalf("error running command `config get -hgithub.com git_protocol`: %v", err)
	}

	eq(t, output.String(), "ssh\n")
}

func TestConfigSetHost(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")

	mainBuf := bytes.Buffer{}
	hostsBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, &hostsBuf)()

	output, err := RunCommand("config set -hgithub.com git_protocol ssh")
	if err != nil {
		t.Fatalf("error running command `config set editor ed`: %v", err)
	}

	if len(output.String()) > 0 {
		t.Errorf("expected output to be blank: %q", output.String())
	}

	expectedMain := ""
	expectedHosts := `github.com:
    user: OWNER
    oauth_token: "1234567890"
    git_protocol: ssh
`

	if mainBuf.String() != expectedMain {
		t.Errorf("expected config.yml to be %q, got %q", expectedMain, mainBuf.String())
	}
	if hostsBuf.String() != expectedHosts {
		t.Errorf("expected hosts.yml to be %q, got %q", expectedHosts, hostsBuf.String())
	}
}

func TestConfigSetHost_update(t *testing.T) {
	cfg := `---
hosts:
  github.com:
    git_protocol: ssh
    user: OWNER
    oauth_token: MUSTBEHIGHCUZIMATOKEN
`

	initBlankContext(cfg, "OWNER/REPO", "master")

	mainBuf := bytes.Buffer{}
	hostsBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, &hostsBuf)()

	output, err := RunCommand("config set -hgithub.com git_protocol https")
	if err != nil {
		t.Fatalf("error running command `config get editor`: %v", err)
	}

	if len(output.String()) > 0 {
		t.Errorf("expected output to be blank: %q", output.String())
	}

	expectedMain := ""
	expectedHosts := `github.com:
    git_protocol: https
    user: OWNER
    oauth_token: MUSTBEHIGHCUZIMATOKEN
`

	if mainBuf.String() != expectedMain {
		t.Errorf("expected config.yml to be %q, got %q", expectedMain, mainBuf.String())
	}
	if hostsBuf.String() != expectedHosts {
		t.Errorf("expected hosts.yml to be %q, got %q", expectedHosts, hostsBuf.String())
	}
}
