package dirs

import (
	"bytes"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdDirs(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
	}

	cmd := NewCmdDirs(f)

	assert.Equal(t, "dirs", cmd.Use)

	assert.Equal(t, "Print the directories gh uses for configuration, data, and state", cmd.Short)

	// Test that the command runs without error
	cmd.SetArgs([]string{})
	require.NoError(t, cmd.Execute())

	// Check that output contains expected directories
	output := stdout.String()
	if !bytes.Contains([]byte(output), []byte("CONFIG DIR")) {
		t.Errorf("output does not contain CONFIG DIR: %s", output)
	}
	if !bytes.Contains([]byte(output), []byte("DATA DIR")) {
		t.Errorf("output does not contain DATA DIR: %s", output)
	}
	if !bytes.Contains([]byte(output), []byte("STATE DIR")) {
		t.Errorf("output does not contain STATE DIR: %s", output)
	}
	if !bytes.Contains([]byte(output), []byte("CACHE DIR")) {
		t.Errorf("output does not contain CACHE DIR: %s", output)
	}

	// Check that output contains the actual directory paths
	if !bytes.Contains([]byte(output), []byte(config.ConfigDir())) {
		t.Errorf("output does not contain config dir path: %s", output)
	}
	if !bytes.Contains([]byte(output), []byte(config.DataDir())) {
		t.Errorf("output does not contain data dir path: %s", output)
	}
	if !bytes.Contains([]byte(output), []byte(config.StateDir())) {
		t.Errorf("output does not contain state dir path: %s", output)
	}
	if !bytes.Contains([]byte(output), []byte(config.CacheDir())) {
		t.Errorf("output does not contain cache dir path: %s", output)
	}
}
