package extensions

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/cli/cli/internal/config"
	"github.com/cli/safeexec"
)

type Manager struct {
	dataDir  func() string
	lookPath func(string) (string, error)
}

func NewManager() *Manager {
	return &Manager{
		dataDir:  config.ConfigDir,
		lookPath: safeexec.LookPath,
	}
}

func (m *Manager) Dispatch(args []string, stdin io.Reader, stdout, stderr io.Writer) (bool, error) {
	if len(args) == 0 {
		return false, errors.New("too few arguments in list")
	}

	var exe string
	extName := "gh-" + args[0]
	forwardArgs := args[1:]

	for _, e := range m.List() {
		if filepath.Base(e) == extName {
			exe = e
			break
		}
	}
	if exe == "" {
		return false, nil
	}

	// TODO: parse the shebang on Windows and invoke the correct interpreter instead of invoking directly
	externalCmd := exec.Command(exe, forwardArgs...)
	externalCmd.Stdin = stdin
	externalCmd.Stdout = stdout
	externalCmd.Stderr = stderr
	return true, externalCmd.Run()
}

func (m *Manager) List() []string {
	dir := m.installDir()
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil
	}

	var results []string
	for _, f := range entries {
		if !strings.HasPrefix(f.Name(), "gh-") || !(f.IsDir() || f.Mode()&os.ModeSymlink != 0) {
			continue
		}
		results = append(results, filepath.Join(dir, f.Name(), f.Name()))
	}
	return results
}

func (m *Manager) InstallLocal(dir string) error {
	name := filepath.Base(dir)
	targetDir := filepath.Join(m.installDir(), name)
	return os.Symlink(dir, targetDir)
}

func (m *Manager) Install(cloneURL string, stdout, stderr io.Writer) error {
	exe, err := m.lookPath("git")
	if err != nil {
		return err
	}

	name := strings.TrimSuffix(path.Base(cloneURL), ".git")
	targetDir := filepath.Join(m.installDir(), name)

	externalCmd := exec.Command(exe, "clone", cloneURL, targetDir)
	externalCmd.Stdout = stdout
	externalCmd.Stderr = stderr
	return externalCmd.Run()
}

func (m *Manager) Upgrade(stdout, stderr io.Writer) error {
	exe, err := m.lookPath("git")
	if err != nil {
		return err
	}

	exts := m.List()
	if len(exts) == 0 {
		return errors.New("no extensions installed")
	}

	for _, f := range exts {
		fmt.Fprintf(stdout, "[%s]: ", filepath.Base(f))
		dir := filepath.Dir(f)
		externalCmd := exec.Command(exe, "-C", dir, "--git-dir="+filepath.Join(dir, ".git"), "pull", "--ff-only")
		externalCmd.Stdout = stdout
		externalCmd.Stderr = stderr
		if e := externalCmd.Run(); e != nil {
			err = e
		}
	}
	return err
}

func (m *Manager) installDir() string {
	return filepath.Join(m.dataDir(), "extensions")
}
