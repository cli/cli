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

	buf := bytes.NewBufferString("")
	defer config.StubWriteConfig(buf)()
	output, err := RunCommand("config set editor ed")
	if err != nil {
		t.Fatalf("error running command `config set editor ed`: %v", err)
	}

	eq(t, output.String(), "")

	expected := `hosts:
    github.com:
        user: OWNER
        oauth_token: 1234567890
editor: ed
`

	eq(t, buf.String(), expected)
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

	buf := bytes.NewBufferString("")
	defer config.StubWriteConfig(buf)()

	output, err := RunCommand("config set editor vim")
	if err != nil {
		t.Fatalf("error running command `config get editor`: %v", err)
	}

	eq(t, output.String(), "")

	expected := `hosts:
    github.com:
        user: OWNER
        oauth_token: MUSTBEHIGHCUZIMATOKEN
editor: vim
`
	eq(t, buf.String(), expected)
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

	buf := bytes.NewBufferString("")
	defer config.StubWriteConfig(buf)()
	output, err := RunCommand("config set -hgithub.com git_protocol ssh")
	if err != nil {
		t.Fatalf("error running command `config set editor ed`: %v", err)
	}

	eq(t, output.String(), "")

	expected := `hosts:
    github.com:
        user: OWNER
        oauth_token: 1234567890
        git_protocol: ssh
`

	eq(t, buf.String(), expected)
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

	buf := bytes.NewBufferString("")
	defer config.StubWriteConfig(buf)()

	output, err := RunCommand("config set -hgithub.com git_protocol https")
	if err != nil {
		t.Fatalf("error running command `config get editor`: %v", err)
	}

	eq(t, output.String(), "")

	expected := `hosts:
    github.com:
        git_protocol: https
        user: OWNER
        oauth_token: MUSTBEHIGHCUZIMATOKEN
`
	eq(t, buf.String(), expected)
}
