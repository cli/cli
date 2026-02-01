package dirs

import (
	"bytes"
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func TestNewCmdDirs(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
	}

	cmd := NewCmdDirs(f)

	if cmd.Use != "dirs" {
		t.Errorf("expected use to be 'dirs', got %s", cmd.Use)
	}

	if cmd.Short != "Print the directories gh uses for configuration, data, and state" {
		t.Errorf("unexpected short description: %s", cmd.Short)
	}

	// Test that the command runs without error
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

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
}
