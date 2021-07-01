package extensions

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/extensions"
	"github.com/cli/cli/pkg/findsh"
	"github.com/cli/safeexec"
)

type Manager struct {
	dataDir    func() string
	lookPath   func(string) (string, error)
	findSh     func() (string, error)
	newCommand func(string, ...string) *exec.Cmd
}

func NewManager() *Manager {
	return &Manager{
		dataDir:    config.DataDir,
		lookPath:   safeexec.LookPath,
		findSh:     findsh.Find,
		newCommand: exec.Command,
	}
}

func (m *Manager) Dispatch(args []string, stdin io.Reader, stdout, stderr io.Writer) (bool, error) {
	if len(args) == 0 {
		return false, errors.New("too few arguments in list")
	}

	var exe string
	extName := args[0]
	forwardArgs := args[1:]

	for _, e := range m.list(false) {
		if e.Name() == extName {
			exe = e.Path()
			break
		}
	}
	if exe == "" {
		return false, nil
	}

	var externalCmd *exec.Cmd

	if runtime.GOOS == "windows" {
		// Dispatch all extension calls through the `sh` interpreter to support executable files with a
		// shebang line on Windows.
		shExe, err := m.findSh()
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return true, errors.New("the `sh.exe` interpreter is required. Please install Git for Windows and try again")
			}
			return true, err
		}
		forwardArgs = append([]string{"-c", `command "$@"`, "--", exe}, forwardArgs...)
		externalCmd = m.newCommand(shExe, forwardArgs...)
	} else {
		externalCmd = m.newCommand(exe, forwardArgs...)
	}
	externalCmd.Stdin = stdin
	externalCmd.Stdout = stdout
	externalCmd.Stderr = stderr
	return true, externalCmd.Run()
}

func (m *Manager) List() []extensions.Extension {
	return m.list(true)
}

func (m *Manager) list(includeMetadata bool) []extensions.Extension {
	dir := m.installDir()
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil
	}

	var gitExe string
	if includeMetadata {
		gitExe, _ = m.lookPath("git")
	}

	var results []extensions.Extension
	for _, f := range entries {
		if !strings.HasPrefix(f.Name(), "gh-") || !(f.IsDir() || f.Mode()&os.ModeSymlink != 0) {
			continue
		}
		var remoteURL string
		if gitExe != "" {
			stdout := bytes.Buffer{}
			cmd := m.newCommand(gitExe, "--git-dir="+filepath.Join(dir, f.Name(), ".git"), "config", "remote.origin.url")
			cmd.Stdout = &stdout
			if err := cmd.Run(); err == nil {
				remoteURL = strings.TrimSpace(stdout.String())
			}
		}
		results = append(results, &Extension{
			path: filepath.Join(dir, f.Name(), f.Name()),
			url:  remoteURL,
		})
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

	externalCmd := m.newCommand(exe, "clone", cloneURL, targetDir)
	externalCmd.Stdout = stdout
	externalCmd.Stderr = stderr
	return externalCmd.Run()
}

var localExtensionUpgradeError = errors.New("local extensions can not be upgraded")

func (m *Manager) Upgrade(name string, stdout, stderr io.Writer) error {
	exe, err := m.lookPath("git")
	if err != nil {
		return err
	}

	exts := m.List()
	if len(exts) == 0 {
		return errors.New("no extensions installed")
	}

	someUpgraded := false
	for _, f := range exts {
		if name == "" {
			fmt.Fprintf(stdout, "[%s]: ", f.Name())
		} else if f.Name() != name {
			continue
		}

		dir := filepath.Dir(f.Path())
		fileInfo, e := os.Lstat(dir)
		if e != nil {
			err = e
			if name == "" {
				fmt.Fprintf(stdout, "%s\n", err)
			}
			continue
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			err = localExtensionUpgradeError
			if name == "" {
				fmt.Fprintf(stdout, "%s\n", err)
			}
			continue
		}
		if _, err = os.Stat(filepath.Join(dir, ".git")); err != nil {
			err = localExtensionUpgradeError
			if name == "" {
				fmt.Fprintf(stdout, "%s\n", err)
			}
			continue
		}

		externalCmd := m.newCommand(exe, "-C", dir, "--git-dir="+filepath.Join(dir, ".git"), "pull", "--ff-only")
		externalCmd.Stdout = stdout
		externalCmd.Stderr = stderr
		if e := externalCmd.Run(); e != nil {
			err = e
		}
		someUpgraded = true
	}
	if err == nil && !someUpgraded {
		err = fmt.Errorf("no extension matched %q", name)
	}
	return err
}

func (m *Manager) Remove(name string) error {
	targetDir := filepath.Join(m.installDir(), "gh-"+name)
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return fmt.Errorf("no extension found: %q", targetDir)
	}
	return os.RemoveAll(targetDir)
}

func (m *Manager) installDir() string {
	return filepath.Join(m.dataDir(), "extensions")
}
