package config

import (
	"bytes"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/stretchr/testify/assert"
)

func Test_fileConfig_Set(t *testing.T) {
	mainBuf := bytes.Buffer{}
	hostsBuf := bytes.Buffer{}
	defer StubWriteConfig(&mainBuf, &hostsBuf)()

	c := NewBlankConfig()
	assert.NoError(t, c.Set("", "editor", "nano"))
	assert.NoError(t, c.Set("github.com", "git_protocol", "ssh"))
	assert.NoError(t, c.Set("example.com", "editor", "vim"))
	assert.NoError(t, c.Set("github.com", "user", "hubot"))
	assert.NoError(t, c.Write())

	assert.Contains(t, mainBuf.String(), "editor: nano")
	assert.Contains(t, mainBuf.String(), "git_protocol: https")
	assert.Equal(t, `github.com:
    git_protocol: ssh
    user: hubot
example.com:
    editor: vim
`, hostsBuf.String())
}

func Test_defaultConfig(t *testing.T) {
	mainBuf := bytes.Buffer{}
	hostsBuf := bytes.Buffer{}
	defer StubWriteConfig(&mainBuf, &hostsBuf)()

	cfg := NewBlankConfig()
	assert.NoError(t, cfg.Write())

	expected := heredoc.Doc(`
		# What protocol to use when performing git operations. Supported values: ssh, https
		git_protocol: https
		# What editor gh should run when creating issues, pull requests, etc. If blank, will refer to environment.
		editor:
		# When to interactively prompt. This is a global config that cannot be overridden by hostname. Supported values: enabled, disabled
		prompt: enabled
		# A pager program to send command output to, e.g. "less". Set the value to "cat" to disable the pager.
		pager:
		# Aliases allow you to create nicknames for gh commands
		aliases:
		    co: pr checkout
	`)
	assert.Equal(t, expected, mainBuf.String())
	assert.Equal(t, "", hostsBuf.String())

	proto, err := cfg.Get("", "git_protocol")
	assert.Nil(t, err)
	assert.Equal(t, "https", proto)

	editor, err := cfg.Get("", "editor")
	assert.Nil(t, err)
	assert.Equal(t, "", editor)

	aliases, err := cfg.Aliases()
	assert.Nil(t, err)
	assert.Equal(t, len(aliases.All()), 1)
	expansion, _ := aliases.Get("co")
	assert.Equal(t, expansion, "pr checkout")
}

func Test_ValidateValue(t *testing.T) {
	err := ValidateValue("git_protocol", "sshpps")
	assert.EqualError(t, err, "invalid value")

	err = ValidateValue("git_protocol", "ssh")
	assert.Nil(t, err)

	err = ValidateValue("editor", "vim")
	assert.Nil(t, err)

	err = ValidateValue("got", "123")
	assert.Nil(t, err)
}

func Test_ValidateKey(t *testing.T) {
	err := ValidateKey("invalid")
	assert.EqualError(t, err, "invalid key")

	err = ValidateKey("git_protocol")
	assert.NoError(t, err)

	err = ValidateKey("editor")
	assert.NoError(t, err)

	err = ValidateKey("prompt")
	assert.NoError(t, err)

	err = ValidateKey("pager")
	assert.NoError(t, err)
}
