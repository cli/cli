package config

import (
	"bytes"
	"testing"

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

	expected := "# What protocol to use when performing git operations. Supported values: ssh, https\ngit_protocol: https\n# What editor gh should run when creating issues, pull requests, etc. If blank, will refer to environment.\neditor: nano\n# When to interactively prompt. This is a global config that cannot be overriden by hostname. Supported values: auto, never\nprompt: auto\n# Aliases allow you to create nicknames for gh commands\naliases:\n    co: pr checkout\n"
	assert.Equal(t, expected, mainBuf.String())
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

	expected := "# What protocol to use when performing git operations. Supported values: ssh, https\ngit_protocol: https\n# What editor gh should run when creating issues, pull requests, etc. If blank, will refer to environment.\neditor:\n# When to interactively prompt. This is a global config that cannot be overriden by hostname. Supported values: auto, never\nprompt: auto\n# Aliases allow you to create nicknames for gh commands\naliases:\n    co: pr checkout\n"
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
