package extension

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

	"github.com/MakeNowJust/heredoc"
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

	exts, _ := m.list(false)
	for _, e := range exts {
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

func (m *Manager) List(includeMetadata bool) []extensions.Extension {
	exts, _ := m.list(includeMetadata)
	return exts
}

func (m *Manager) list(includeMetadata bool) ([]extensions.Extension, error) {
	dir := m.installDir()
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var results []extensions.Extension
	for _, f := range entries {
		if !strings.HasPrefix(f.Name(), "gh-") {
			continue
		}
		var remoteUrl string
		updateAvailable := false
		isLocal := false
		exePath := filepath.Join(dir, f.Name(), f.Name())
		if f.IsDir() {
			if includeMetadata {
				remoteUrl = m.getRemoteUrl(f.Name())
				updateAvailable = m.checkUpdateAvailable(f.Name())
			}
		} else {
			isLocal = true
			if !isSymlink(f.Mode()) {
				// if this is a regular file, its contents is the local directory of the extension
				p, err := readPathFromFile(filepath.Join(dir, f.Name()))
				if err != nil {
					return nil, err
				}
				exePath = filepath.Join(p, f.Name())
			}
		}
		results = append(results, &Extension{
			path:            exePath,
			url:             remoteUrl,
			isLocal:         isLocal,
			updateAvailable: updateAvailable,
		})
	}
	return results, nil
}

func (m *Manager) getRemoteUrl(extension string) string {
	gitExe, err := m.lookPath("git")
	if err != nil {
		return ""
	}
	dir := m.installDir()
	gitDir := "--git-dir=" + filepath.Join(dir, extension, ".git")
	cmd := m.newCommand(gitExe, gitDir, "config", "remote.origin.url")
	url, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(url))
}

func (m *Manager) checkUpdateAvailable(extension string) bool {
	gitExe, err := m.lookPath("git")
	if err != nil {
		return false
	}
	dir := m.installDir()
	gitDir := "--git-dir=" + filepath.Join(dir, extension, ".git")
	cmd := m.newCommand(gitExe, gitDir, "ls-remote", "origin", "HEAD")
	lsRemote, err := cmd.Output()
	if err != nil {
		return false
	}
	remoteSha := bytes.SplitN(lsRemote, []byte("\t"), 2)[0]
	cmd = m.newCommand(gitExe, gitDir, "rev-parse", "HEAD")
	localSha, err := cmd.Output()
	if err != nil {
		return false
	}
	localSha = bytes.TrimSpace(localSha)
	return !bytes.Equal(remoteSha, localSha)
}

func (m *Manager) InstallLocal(dir string) error {
	name := filepath.Base(dir)
	targetLink := filepath.Join(m.installDir(), name)
	if err := os.MkdirAll(filepath.Dir(targetLink), 0755); err != nil {
		return err
	}
	return makeSymlink(dir, targetLink)
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

func (m *Manager) Upgrade(name string, force bool, stdout, stderr io.Writer) error {
	exe, err := m.lookPath("git")
	if err != nil {
		return err
	}

	exts := m.List(false)
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

		if f.IsLocal() {
			if name == "" {
				fmt.Fprintf(stdout, "%s\n", localExtensionUpgradeError)
			} else {
				err = localExtensionUpgradeError
			}
			continue
		}

		var cmds []*exec.Cmd
		dir := filepath.Dir(f.Path())
		if force {
			fetchCmd := m.newCommand(exe, "-C", dir, "--git-dir="+filepath.Join(dir, ".git"), "fetch", "origin", "HEAD")
			resetCmd := m.newCommand(exe, "-C", dir, "--git-dir="+filepath.Join(dir, ".git"), "reset", "--hard", "origin/HEAD")
			cmds = []*exec.Cmd{fetchCmd, resetCmd}
		} else {
			pullCmd := m.newCommand(exe, "-C", dir, "--git-dir="+filepath.Join(dir, ".git"), "pull", "--ff-only")
			cmds = []*exec.Cmd{pullCmd}
		}
		if e := runCmds(cmds, stdout, stderr); e != nil {
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
	if _, err := os.Lstat(targetDir); os.IsNotExist(err) {
		return fmt.Errorf("no extension found: %q", targetDir)
	}
	return os.RemoveAll(targetDir)
}

func (m *Manager) installDir() string {
	return filepath.Join(m.dataDir(), "extensions")
}

func (m *Manager) Create(name string) error {
	exe, err := m.lookPath("git")
	if err != nil {
		return err
	}

	err = os.Mkdir(name, 0755)
	if err != nil {
		return err
	}

	initCmd := m.newCommand(exe, "init", "--quiet", name)
	err = initCmd.Run()
	if err != nil {
		return err
	}

	fileTmpl := heredoc.Docf(`
		#!/usr/bin/env bash
		set -e

		echo "Hello %[1]s!"

		# Snippets to help get started:

		# Determine if an executable is in the PATH
		# if ! type -p ruby >/dev/null; then
		#   echo "Ruby not found on the system" >&2
		#   exit 1
		# fi

		# Pass arguments through to another command
		# gh issue list "$@" -R cli/cli

		# Using the gh api command to retrieve and format information
		# QUERY='
		#   query($endCursor: String) {
		#     viewer {
		#       repositories(first: 100, after: $endCursor) {
		#         nodes {
		#           nameWithOwner
		#           stargazerCount
		#         }
		#       }
		#     }
		#   }
		# '
		# TEMPLATE='
		#   {{- range $repo := .data.viewer.repositories.nodes -}}
		#     {{- printf "name: %[2]s - stargazers: %[3]s\n" $repo.nameWithOwner $repo.stargazerCount -}}
		#   {{- end -}}
		# '
		# exec gh api graphql -f query="${QUERY}" --paginate --template="${TEMPLATE}"
	`, name, "%s", "%v")
	filePath := filepath.Join(name, name)
	err = ioutil.WriteFile(filePath, []byte(fileTmpl), 0755)
	if err != nil {
		return err
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	dir := filepath.Join(wd, name)
	addCmd := m.newCommand(exe, "-C", dir, "--git-dir="+filepath.Join(dir, ".git"), "add", name, "--chmod=+x")
	err = addCmd.Run()
	return err
}

func runCmds(cmds []*exec.Cmd, stdout, stderr io.Writer) error {
	for _, cmd := range cmds {
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

func isSymlink(m os.FileMode) bool {
	return m&os.ModeSymlink != 0
}

// reads the product of makeSymlink on Windows
func readPathFromFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b := make([]byte, 1024)
	n, err := f.Read(b)
	return strings.TrimSpace(string(b[:n])), err
}
