package extension

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/findsh"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/safeexec"
	"gopkg.in/yaml.v3"
)

type Manager struct {
	dataDir    func() string
	lookPath   func(string) (string, error)
	findSh     func() (string, error)
	newCommand func(string, ...string) *exec.Cmd
	platform   func() string
	client     *http.Client
	config     config.Config
	io         *iostreams.IOStreams
}

func NewManager(io *iostreams.IOStreams) *Manager {
	return &Manager{
		dataDir:    config.DataDir,
		lookPath:   safeexec.LookPath,
		findSh:     findsh.Find,
		newCommand: exec.Command,
		platform: func() string {
			return fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
		},
		io: io,
	}
}

func (m *Manager) SetConfig(cfg config.Config) {
	m.config = cfg
}

func (m *Manager) SetClient(client *http.Client) {
	m.client = client
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
	r := make([]extensions.Extension, len(exts))
	for i, v := range exts {
		val := v
		r[i] = &val
	}
	return r
}

func (m *Manager) list(includeMetadata bool) ([]Extension, error) {
	dir := m.installDir()
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var results []Extension
	for _, f := range entries {
		if !strings.HasPrefix(f.Name(), "gh-") {
			continue
		}
		var ext Extension
		var err error
		if f.IsDir() {
			ext, err = m.parseExtensionDir(f)
			if err != nil {
				return nil, err
			}
			results = append(results, ext)
		} else {
			ext, err = m.parseExtensionFile(f)
			if err != nil {
				return nil, err
			}
			results = append(results, ext)
		}
	}

	if includeMetadata {
		m.populateLatestVersions(results)
	}

	return results, nil
}

func (m *Manager) parseExtensionFile(fi fs.FileInfo) (Extension, error) {
	ext := Extension{isLocal: true}
	id := m.installDir()
	exePath := filepath.Join(id, fi.Name(), fi.Name())
	if !isSymlink(fi.Mode()) {
		// if this is a regular file, its contents is the local directory of the extension
		p, err := readPathFromFile(filepath.Join(id, fi.Name()))
		if err != nil {
			return ext, err
		}
		exePath = filepath.Join(p, fi.Name())
	}
	ext.path = exePath
	return ext, nil
}

func (m *Manager) parseExtensionDir(fi fs.FileInfo) (Extension, error) {
	id := m.installDir()
	if _, err := os.Stat(filepath.Join(id, fi.Name(), manifestName)); err == nil {
		return m.parseBinaryExtensionDir(fi)
	}

	return m.parseGitExtensionDir(fi)
}

func (m *Manager) parseBinaryExtensionDir(fi fs.FileInfo) (Extension, error) {
	id := m.installDir()
	exePath := filepath.Join(id, fi.Name(), fi.Name())
	ext := Extension{path: exePath, kind: BinaryKind}
	manifestPath := filepath.Join(id, fi.Name(), manifestName)
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return ext, fmt.Errorf("could not open %s for reading: %w", manifestPath, err)
	}
	var bm binManifest
	err = yaml.Unmarshal(manifest, &bm)
	if err != nil {
		return ext, fmt.Errorf("could not parse %s: %w", manifestPath, err)
	}
	repo := ghrepo.NewWithHost(bm.Owner, bm.Name, bm.Host)
	remoteURL := ghrepo.GenerateRepoURL(repo, "")
	ext.url = remoteURL
	ext.currentVersion = bm.Tag
	return ext, nil
}

func (m *Manager) parseGitExtensionDir(fi fs.FileInfo) (Extension, error) {
	id := m.installDir()
	exePath := filepath.Join(id, fi.Name(), fi.Name())
	remoteUrl := m.getRemoteUrl(fi.Name())
	currentVersion := m.getCurrentVersion(fi.Name())
	return Extension{
		path:           exePath,
		url:            remoteUrl,
		isLocal:        false,
		currentVersion: currentVersion,
		kind:           GitKind,
	}, nil
}

// getCurrentVersion determines the current version for non-local git extensions.
func (m *Manager) getCurrentVersion(extension string) string {
	gitExe, err := m.lookPath("git")
	if err != nil {
		return ""
	}
	dir := m.installDir()
	gitDir := "--git-dir=" + filepath.Join(dir, extension, ".git")
	cmd := m.newCommand(gitExe, gitDir, "rev-parse", "HEAD")
	localSha, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(bytes.TrimSpace(localSha))
}

// getRemoteUrl determines the remote URL for non-local git extensions.
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

func (m *Manager) populateLatestVersions(exts []Extension) {
	size := len(exts)
	type result struct {
		index   int
		version string
	}
	ch := make(chan result, size)
	var wg sync.WaitGroup
	wg.Add(size)
	for idx, ext := range exts {
		go func(i int, e Extension) {
			defer wg.Done()
			version, _ := m.getLatestVersion(e)
			ch <- result{index: i, version: version}
		}(idx, ext)
	}
	wg.Wait()
	close(ch)
	for r := range ch {
		ext := &exts[r.index]
		ext.latestVersion = r.version
	}
}

func (m *Manager) getLatestVersion(ext Extension) (string, error) {
	if ext.isLocal {
		return "", fmt.Errorf("unable to get latest version for local extensions")
	}
	if ext.kind == GitKind {
		gitExe, err := m.lookPath("git")
		if err != nil {
			return "", err
		}
		extDir := filepath.Dir(ext.path)
		gitDir := "--git-dir=" + filepath.Join(extDir, ".git")
		cmd := m.newCommand(gitExe, gitDir, "ls-remote", "origin", "HEAD")
		lsRemote, err := cmd.Output()
		if err != nil {
			return "", err
		}
		remoteSha := bytes.SplitN(lsRemote, []byte("\t"), 2)[0]
		return string(remoteSha), nil
	} else {
		repo, err := ghrepo.FromFullName(ext.url)
		if err != nil {
			return "", err
		}
		r, err := fetchLatestRelease(m.client, repo)
		if err != nil {
			return "", err
		}
		return r.Tag, nil
	}
}

func (m *Manager) InstallLocal(dir string) error {
	name := filepath.Base(dir)
	targetLink := filepath.Join(m.installDir(), name)
	if err := os.MkdirAll(filepath.Dir(targetLink), 0755); err != nil {
		return err
	}
	return makeSymlink(dir, targetLink)
}

type binManifest struct {
	Owner string
	Name  string
	Host  string
	Tag   string
	// TODO I may end up not using this; just thinking ahead to local installs
	Path string
}

func (m *Manager) Install(repo ghrepo.Interface) error {
	isBin, err := isBinExtension(m.client, repo)
	if err != nil {
		return fmt.Errorf("could not check for binary extension: %w", err)
	}
	if isBin {
		return m.installBin(repo)
	}

	hs, err := hasScript(m.client, repo)
	if err != nil {
		return err
	}
	if !hs {
		// TODO open an issue hint, here?
		return errors.New("extension is uninstallable: missing executable")
	}

	protocol, _ := m.config.Get(repo.RepoHost(), "git_protocol")
	return m.installGit(ghrepo.FormatRemoteURL(repo, protocol), m.io.Out, m.io.ErrOut)
}

func (m *Manager) installBin(repo ghrepo.Interface) error {
	var r *release
	r, err := fetchLatestRelease(m.client, repo)
	if err != nil {
		return err
	}

	suffix := m.platform()
	var asset *releaseAsset
	for _, a := range r.Assets {
		if strings.HasSuffix(a.Name, suffix) {
			asset = &a
			break
		}
	}

	if asset == nil {
		return fmt.Errorf("%s unsupported for %s. Open an issue: `gh issue create -R %s/%s -t'Support %s'`",
			repo.RepoName(),
			suffix, repo.RepoOwner(), repo.RepoName(), suffix)
	}

	name := repo.RepoName()
	targetDir := filepath.Join(m.installDir(), name)
	// TODO clean this up if function errs?
	err = os.MkdirAll(targetDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create installation directory: %w", err)
	}

	binPath := filepath.Join(targetDir, name)

	err = downloadAsset(m.client, *asset, binPath)
	if err != nil {
		return fmt.Errorf("failed to download asset %s: %w", asset.Name, err)
	}

	manifest := binManifest{
		Name:  name,
		Owner: repo.RepoOwner(),
		Host:  repo.RepoHost(),
		Path:  binPath,
		Tag:   r.Tag,
	}

	bs, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("failed to serialize manifest: %w", err)
	}

	manifestPath := filepath.Join(targetDir, manifestName)

	f, err := os.OpenFile(manifestPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open manifest for writing: %w", err)
	}
	defer f.Close()

	_, err = f.Write(bs)
	if err != nil {
		return fmt.Errorf("failed write manifest file: %w", err)
	}

	return nil
}

func (m *Manager) installGit(cloneURL string, stdout, stderr io.Writer) error {
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

func (m *Manager) Upgrade(name string, force bool) error {
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
			fmt.Fprintf(m.io.Out, "[%s]: ", f.Name())
		} else if f.Name() != name {
			continue
		}

		if f.IsLocal() {
			if name == "" {
				fmt.Fprintf(m.io.Out, "%s\n", localExtensionUpgradeError)
			} else {
				err = localExtensionUpgradeError
			}
			continue
		}

		binManifestPath := filepath.Join(filepath.Dir(f.Path()), manifestName)
		if _, e := os.Stat(binManifestPath); e == nil {
			err = m.upgradeBin(f)
			someUpgraded = true
			continue
		}

		if e := m.upgradeGit(f, exe, force); e != nil {
			err = e
		}
		someUpgraded = true
	}

	if err == nil && !someUpgraded {
		err = fmt.Errorf("no extension matched %q", name)
	}

	return err
}

func (m *Manager) upgradeGit(ext extensions.Extension, exe string, force bool) error {
	var cmds []*exec.Cmd
	dir := filepath.Dir(ext.Path())
	if force {
		fetchCmd := m.newCommand(exe, "-C", dir, "--git-dir="+filepath.Join(dir, ".git"), "fetch", "origin", "HEAD")
		resetCmd := m.newCommand(exe, "-C", dir, "--git-dir="+filepath.Join(dir, ".git"), "reset", "--hard", "origin/HEAD")
		cmds = []*exec.Cmd{fetchCmd, resetCmd}
	} else {
		pullCmd := m.newCommand(exe, "-C", dir, "--git-dir="+filepath.Join(dir, ".git"), "pull", "--ff-only")
		cmds = []*exec.Cmd{pullCmd}
	}

	return runCmds(cmds, m.io.Out, m.io.ErrOut)
}

func (m *Manager) upgradeBin(ext extensions.Extension) error {
	manifestPath := filepath.Join(filepath.Dir(ext.Path()), manifestName)
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("could not open %s for reading: %w", manifestPath, err)
	}

	var bm binManifest
	err = yaml.Unmarshal(manifest, &bm)
	if err != nil {
		return fmt.Errorf("could not parse %s: %w", manifestPath, err)
	}
	repo := ghrepo.NewWithHost(bm.Owner, bm.Name, bm.Host)
	var r *release

	r, err = fetchLatestRelease(m.client, repo)
	if err != nil {
		return fmt.Errorf("failed to get release info for %s: %w", ghrepo.FullName(repo), err)
	}

	if bm.Tag == r.Tag {
		return nil
	}

	return m.installBin(repo)
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

func isBinExtension(client *http.Client, repo ghrepo.Interface) (isBin bool, err error) {
	var r *release
	r, err = fetchLatestRelease(client, repo)
	if err != nil {
		httpErr, ok := err.(api.HTTPError)
		if ok && httpErr.StatusCode == 404 {
			err = nil
			return
		}
		return
	}

	for _, a := range r.Assets {
		dists := possibleDists()
		for _, d := range dists {
			if strings.HasSuffix(a.Name, d) {
				isBin = true
				break
			}
		}
	}

	return
}

func possibleDists() []string {
	return []string{
		"aix-ppc64",
		"android-386",
		"android-amd64",
		"android-arm",
		"android-arm64",
		"darwin-amd64",
		"darwin-arm64",
		"dragonfly-amd64",
		"freebsd-386",
		"freebsd-amd64",
		"freebsd-arm",
		"freebsd-arm64",
		"illumos-amd64",
		"ios-amd64",
		"ios-arm64",
		"js-wasm",
		"linux-386",
		"linux-amd64",
		"linux-arm",
		"linux-arm64",
		"linux-mips",
		"linux-mips64",
		"linux-mips64le",
		"linux-mipsle",
		"linux-ppc64",
		"linux-ppc64le",
		"linux-riscv64",
		"linux-s390x",
		"netbsd-386",
		"netbsd-amd64",
		"netbsd-arm",
		"netbsd-arm64",
		"openbsd-386",
		"openbsd-amd64",
		"openbsd-arm",
		"openbsd-arm64",
		"openbsd-mips64",
		"plan9-386",
		"plan9-amd64",
		"plan9-arm",
		"solaris-amd64",
		"windows-386",
		"windows-amd64",
		"windows-arm",
	}
}
