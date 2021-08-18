package extension

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/stretchr/testify/assert"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GH_WANT_HELPER_PROCESS") != "1" {
		return
	}
	if err := func(args []string) error {
		fmt.Fprintf(os.Stdout, "%v\n", args)
		return nil
	}(os.Args[3:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func newTestManager(dir string) *Manager {
	return &Manager{
		dataDir:  func() string { return dir },
		lookPath: func(exe string) (string, error) { return exe, nil },
		findSh:   func() (string, error) { return "sh", nil },
		newCommand: func(exe string, args ...string) *exec.Cmd {
			args = append([]string{os.Args[0], "-test.run=TestHelperProcess", "--", exe}, args...)
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Env = []string{"GH_WANT_HELPER_PROCESS=1"}
			return cmd
		},
	}
}

func TestManager_List(t *testing.T) {
	tempDir := t.TempDir()
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-hello", "gh-hello")))
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-two", "gh-two")))

	m := newTestManager(tempDir)
	exts := m.List(false)
	assert.Equal(t, 2, len(exts))
	assert.Equal(t, "hello", exts[0].Name())
	assert.Equal(t, "two", exts[1].Name())
}

func TestManager_Dispatch(t *testing.T) {
	tempDir := t.TempDir()
	extPath := filepath.Join(tempDir, "extensions", "gh-hello", "gh-hello")
	assert.NoError(t, stubExtension(extPath))

	m := newTestManager(tempDir)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	found, err := m.Dispatch([]string{"hello", "one", "two"}, nil, stdout, stderr)
	assert.NoError(t, err)
	assert.True(t, found)

	if runtime.GOOS == "windows" {
		assert.Equal(t, fmt.Sprintf("[sh -c command \"$@\" -- %s one two]\n", extPath), stdout.String())
	} else {
		assert.Equal(t, fmt.Sprintf("[%s one two]\n", extPath), stdout.String())
	}
	assert.Equal(t, "", stderr.String())
}

func TestManager_Remove(t *testing.T) {
	tempDir := t.TempDir()
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-hello", "gh-hello")))
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-two", "gh-two")))

	m := newTestManager(tempDir)
	err := m.Remove("hello")
	assert.NoError(t, err)

	items, err := ioutil.ReadDir(filepath.Join(tempDir, "extensions"))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(items))
	assert.Equal(t, "gh-two", items[0].Name())
}

func TestManager_Upgrade_AllExtensions(t *testing.T) {
	tempDir := t.TempDir()
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-hello", "gh-hello")))
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-two", "gh-two")))
	assert.NoError(t, stubLocalExtension(tempDir, filepath.Join(tempDir, "extensions", "gh-local", "gh-local")))

	m := newTestManager(tempDir)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := m.Upgrade("", false, stdout, stderr)
	assert.NoError(t, err)

	assert.Equal(t, heredoc.Docf(
		`
		[hello]: [git -C %s --git-dir=%s pull --ff-only]
		[local]: local extensions can not be upgraded
		[two]: [git -C %s --git-dir=%s pull --ff-only]
		`,
		filepath.Join(tempDir, "extensions", "gh-hello"),
		filepath.Join(tempDir, "extensions", "gh-hello", ".git"),
		filepath.Join(tempDir, "extensions", "gh-two"),
		filepath.Join(tempDir, "extensions", "gh-two", ".git"),
	), stdout.String())
	assert.Equal(t, "", stderr.String())
}

func TestManager_Upgrade_RemoteExtension(t *testing.T) {
	tempDir := t.TempDir()
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-remote", "gh-remote")))

	m := newTestManager(tempDir)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := m.Upgrade("remote", false, stdout, stderr)
	assert.NoError(t, err)
	assert.Equal(t, heredoc.Docf(
		`
		[git -C %s --git-dir=%s pull --ff-only]
		`,
		filepath.Join(tempDir, "extensions", "gh-remote"),
		filepath.Join(tempDir, "extensions", "gh-remote", ".git"),
	), stdout.String())
	assert.Equal(t, "", stderr.String())
}

func TestManager_Upgrade_LocalExtension(t *testing.T) {
	tempDir := t.TempDir()
	assert.NoError(t, stubLocalExtension(tempDir, filepath.Join(tempDir, "extensions", "gh-local", "gh-local")))

	m := newTestManager(tempDir)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := m.Upgrade("local", false, stdout, stderr)
	assert.EqualError(t, err, "local extensions can not be upgraded")
	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "", stderr.String())
}

func TestManager_Upgrade_Force(t *testing.T) {
	tempDir := t.TempDir()
	extensionDir := filepath.Join(tempDir, "extensions", "gh-remote")
	gitDir := filepath.Join(tempDir, "extensions", "gh-remote", ".git")

	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-remote", "gh-remote")))

	m := newTestManager(tempDir)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := m.Upgrade("remote", true, stdout, stderr)
	assert.NoError(t, err)
	assert.Equal(t, heredoc.Docf(
		`
		[git -C %s --git-dir=%s fetch origin HEAD]
		[git -C %s --git-dir=%s reset --hard origin/HEAD]
		`,
		extensionDir,
		gitDir,
		extensionDir,
		gitDir,
	), stdout.String())
	assert.Equal(t, "", stderr.String())
}

func TestManager_Upgrade_NoExtensions(t *testing.T) {
	tempDir := t.TempDir()

	m := newTestManager(tempDir)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := m.Upgrade("", false, stdout, stderr)
	assert.EqualError(t, err, "no extensions installed")
	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "", stderr.String())
}

func TestManager_Install(t *testing.T) {
	tempDir := t.TempDir()
	m := newTestManager(tempDir)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := m.Install("https://github.com/owner/gh-some-ext.git", stdout, stderr)
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("[git clone https://github.com/owner/gh-some-ext.git %s]\n", filepath.Join(tempDir, "extensions", "gh-some-ext")), stdout.String())
	assert.Equal(t, "", stderr.String())
}

func stubExtension(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE, 0755)
	if err != nil {
		return err
	}
	return f.Close()
}

func stubLocalExtension(tempDir, path string) error {
	extDir, err := ioutil.TempDir(tempDir, "local-ext")
	if err != nil {
		return err
	}
	extFile, err := os.OpenFile(filepath.Join(extDir, filepath.Base(path)), os.O_CREATE, 0755)
	if err != nil {
		return err
	}
	if err := extFile.Close(); err != nil {
		return err
	}

	linkPath := filepath.Dir(path)
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(linkPath, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	_, err = f.WriteString(extDir)
	if err != nil {
		return err
	}
	return f.Close()
}
