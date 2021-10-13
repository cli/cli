package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_fileConfig_Hosts(t *testing.T) {
	c := NewBlankConfig()
	hosts, err := c.Hosts()
	require.NoError(t, err)
	assert.Equal(t, []string{}, hosts)
}
