package config

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_fileConfig_Set(t *testing.T) {
	cb := bytes.Buffer{}
	StubWriteConfig(&cb, nil)

	c := NewBlankConfig()
	assert.NoError(t, c.Set("", "editor", "nano"))
	assert.NoError(t, c.Set("github.com", "git_protocol", "ssh"))
	assert.NoError(t, c.Set("example.com", "editor", "vim"))
	assert.NoError(t, c.Set("github.com", "user", "hubot"))
	assert.NoError(t, c.Write())

	assert.Equal(t, `editor: nano
hosts:
    github.com:
        git_protocol: ssh
        user: hubot
    example.com:
        editor: vim
`, cb.String())
}

func Test_fileConfig_Write(t *testing.T) {
	cb := bytes.Buffer{}
	StubWriteConfig(&cb, nil)

	c := NewBlankConfig()
	assert.NoError(t, c.Write())

	assert.Equal(t, "", cb.String())
}
