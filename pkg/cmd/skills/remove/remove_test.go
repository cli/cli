package remove

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdRemove(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
	}

	cmd := NewCmdRemove(f, nil)

	assert.Equal(t, "remove <skill>", cmd.Use)
	assert.Equal(t, []string{"uninstall"}, cmd.Aliases)
	assert.NotNil(t, cmd.RunE)
}

func TestRemoveRun(t *testing.T) {
	tempDir := t.TempDir()

	// create mock skill dirs
	skillDir := filepath.Join(tempDir, "test-skill")
	err := os.MkdirAll(skillDir, 0755)
	assert.NoError(t, err)

	namespacedDir := filepath.Join(tempDir, "author", "test-skill-namespaced")
	err = os.MkdirAll(namespacedDir, 0755)
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)

	// test 1: remove flat skill
	opts := &RemoveOptions{
		IO:        ios,
		SkillName: "test-skill",
		Dir:       tempDir,
	}
	err = removeRun(opts)
	assert.NoError(t, err)
	assert.Contains(t, stdout.String(), "Removed test-skill")
	_, err = os.Stat(skillDir)
	assert.True(t, os.IsNotExist(err))

	// clear stdout
	stdout.Reset()

	// test 2: remove namespaced skill
	opts.SkillName = "test-skill-namespaced"
	err = removeRun(opts)
	assert.NoError(t, err)
	assert.Contains(t, stdout.String(), "Removed test-skill-namespaced")
	_, err = os.Stat(namespacedDir)
	assert.True(t, os.IsNotExist(err))

	// clear stdout
	stdout.Reset()

	// test 3: not found
	opts.SkillName = "non-existent"
	err = removeRun(opts)
	assert.ErrorContains(t, err, "skill \"non-existent\" not found")
}
