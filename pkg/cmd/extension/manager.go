package extension

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/findsh"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/safeexec"
	"gopkg.in/yaml.v3"
)

// ErrInitialCommitFailed indicates the initial commit when making a new extension failed.
var ErrInitialCommitFailed = errors.New("initial commit failed")

type Manager struct {
	dataDir    func() string
	lookPath   func(string) (string, error)
	findSh     func() (string, error)
	newCommand func(string, ...string) *exec.Cmd
	platform   func() (string, string)
	client     *http.Client
	gitClient  gitClient
	config     config.Config
	io         *iostreams.IOStreams
	dryRunMode bool
}

func NewManager(ios *iostreams.IOStreams, gc *git.Client) *Manager {
	return &Manager{
		dataDir:    config.DataDir,
		lookPath:   safeexec.LookPath,
		findSh:     findsh.Find,
		newCommand: exec.Command,
		platform: func() (string, string) {
			ext := ""
			if runtime.GOOS == "windows" {
				ext = ".exe"
			}
			return fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH), ext
		},
		io:        ios,
		gitClient: &gitExecuter{client: gc},
	}
}

func (m *Manager) SetConfig(cfg config.Config) {
	m.config = cfg
}

func (m *Manager) SetClient(client *http.Client) {
	m.client = client
}

func (m *Manager) EnableDryRunMode() {
	m.dryRunMode = true
}

func (m *Manager) Dispatch(args []string, stdin io.Reader, stdout, stderr io.Writer) (bool, error) {
	if len(args) == 0 {
		return false, errors.New("too few arguments in list")
	}

	var exe string
	extName := args[0]
	forwardArgs := args[1:]

	exts, _ := m.list(false)
	var ext Extension
	for _, e := range exts {
		if e.Name() == extName {
			ext = e
			exe = ext.Path()
			break
		}
	}
	if exe == "" {
		return false, nil
	}

	var externalCmd *exec.Cmd

	if ext.IsBinary() || runtime.GOOS != "windows" {
		externalCmd = m.newCommand(exe, forwardArgs...)
	} else if runtime.GOOS == "windows" {
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
	}
	externalCmd.Stdin = stdin
	externalCmd.Stdout = stdout
	externalCmd.Stderr = stderr
	return true, externalCmd.Run()
}

func (m *Manager) List() []extensions.Extension {
	exts, _ := m.list(false)
	r := make([]extensions.Extension, len(exts))
	for i, v := range exts {
		val := v
		r[i] = &val
	}
	return r
}

func (m *Manager) list(includeMetadata bool) ([]Extension, error) {
	dir := m.installDir()
	entries, err := os.ReadDir(dir)
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

func (m *Manager) parseExtensionFile(fi fs.DirEntry) (Extension, error) {
	ext := Extension{isLocal: true}
	id := m.installDir()
	exePath := filepath.Join(id, fi.Name(), fi.Name())
	if !isSymlink(fi.Type()) {
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

func (m *Manager) parseExtensionDir(fi fs.DirEntry) (Extension, error) {
	id := m.installDir()
	if _, err := os.Stat(filepath.Join(id, fi.Name(), manifestName)); err == nil {
		return m.parseBinaryExtensionDir(fi)
	}

	return m.parseGitExtensionDir(fi)
}

func (m *Manager) parseBinaryExtensionDir(fi fs.DirEntry) (Extension, error) {
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
	ext.isPinned = bm.IsPinned
	return ext, nil
}

func (m *Manager) parseGitExtensionDir(fi fs.DirEntry) (Extension, error) {
	id := m.installDir()
	exePath := filepath.Join(id, fi.Name(), fi.Name())
	remoteUrl := m.getRemoteUrl(fi.Name())
	currentVersion := m.getCurrentVersion(fi.Name())

	var isPinned bool
	pinPath := filepath.Join(id, fi.Name(), fmt.Sprintf(".pin-%s", currentVersion))
	if _, err := os.Stat(pinPath); err == nil {
		isPinned = true
	}

	return Extension{
		path:           exePath,
		url:            remoteUrl,
		isLocal:        false,
		currentVersion: currentVersion,
		kind:           GitKind,
		isPinned:       isPinned,
	}, nil
}

// getCurrentVersion determines the current version for non-local git extensions.
func (m *Manager) getCurrentVersion(extension string) string {
	dir := filepath.Join(m.installDir(), extension)
	scopedClient := m.gitClient.ForRepo(dir)
	localSha, err := scopedClient.CommandOutput([]string{"rev-parse", "HEAD"})
	if err != nil {
		return ""
	}
	return string(bytes.TrimSpace(localSha))
}

// getRemoteUrl determines the remote URL for non-local git extensions.
func (m *Manager) getRemoteUrl(extension string) string {
	dir := filepath.Join(m.installDir(), extension)
	scopedClient := m.gitClient.ForRepo(dir)
	url, err := scopedClient.Config("remote.origin.url")
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
		return "", localExtensionUpgradeError
	}
	if ext.IsBinary() {
		repo, err := ghrepo.FromFullName(ext.url)
		if err != nil {
			return "", err
		}
		r, err := fetchLatestRelease(m.client, repo)
		if err != nil {
			return "", err
		}
		return r.Tag, nil
	} else {
		extDir := filepath.Dir(ext.path)
		scopedClient := m.gitClient.ForRepo(extDir)
		lsRemote, err := scopedClient.CommandOutput([]string{"ls-remote", "origin", "HEAD"})
		if err != nil {
			return "", err
		}
		remoteSha := bytes.SplitN(lsRemote, []byte("\t"), 2)[0]
		return string(remoteSha), nil
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
	Owner    string
	Name     string
	Host     string
	Tag      string
	IsPinned bool
	// TODO I may end up not using this; just thinking ahead to local installs
	Path string
}

// Install installs an extension from repo, and pins to commitish if provided
func (m *Manager) Install(repo ghrepo.Interface, target string) error {
	isBin, err := isBinExtension(m.client, repo)
	if err != nil {
		if errors.Is(err, releaseNotFoundErr) {
			if ok, err := repoExists(m.client, repo); err != nil {
				return err
			} else if !ok {
				return repositoryNotFoundErr
			}
		} else {
			return fmt.Errorf("could not check for binary extension: %w", err)
		}
	}
	if isBin {
		return m.installBin(repo, target)
	}

	hs, err := hasScript(m.client, repo)
	if err != nil {
		return err
	}
	if !hs {
		return errors.New("extension is not installable: missing executable")
	}

	return m.installGit(repo, target)
}

func (m *Manager) installBin(repo ghrepo.Interface, target string) error {
	var r *release
	var err error
	isPinned := target != ""
	if isPinned {
		r, err = fetchReleaseFromTag(m.client, repo, target)
	} else {
		r, err = fetchLatestRelease(m.client, repo)
	}
	if err != nil {
		return err
	}

	platform, ext := m.platform()
	isMacARM := platform == "darwin-arm64"
	trueARMBinary := false

	var asset *releaseAsset
	for _, a := range r.Assets {
		if strings.HasSuffix(a.Name, platform+ext) {
			asset = &a
			trueARMBinary = isMacARM
			break
		}
	}

	// if an arm64 binary is unavailable, fall back to amd64 if it can be executed through Rosetta 2
	if asset == nil && isMacARM && hasRosetta() {
		for _, a := range r.Assets {
			if strings.HasSuffix(a.Name, "darwin-amd64") {
				asset = &a
				break
			}
		}
	}

	if asset == nil {
		return fmt.Errorf(
			"%[1]s unsupported for %[2]s. Open an issue: `gh issue create -R %[3]s/%[1]s -t'Support %[2]s'`",
			repo.RepoName(), platform, repo.RepoOwner())
	}

	name := repo.RepoName()
	targetDir := filepath.Join(m.installDir(), name)

	// TODO clean this up if function errs?
	if !m.dryRunMode {
		err = os.MkdirAll(targetDir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create installation directory: %w", err)
		}
	}

	binPath := filepath.Join(targetDir, name)
	binPath += ext

	if !m.dryRunMode {
		err = downloadAsset(m.client, *asset, binPath)
		if err != nil {
			return fmt.Errorf("failed to download asset %s: %w", asset.Name, err)
		}
		if trueARMBinary {
			if err := codesignBinary(binPath); err != nil {
				return fmt.Errorf("failed to codesign downloaded binary: %w", err)
			}
		}
	}

	manifest := binManifest{
		Name:     name,
		Owner:    repo.RepoOwner(),
		Host:     repo.RepoHost(),
		Path:     binPath,
		Tag:      r.Tag,
		IsPinned: isPinned,
	}

	bs, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("failed to serialize manifest: %w", err)
	}

	if !m.dryRunMode {
		if err := writeManifest(targetDir, manifestName, bs); err != nil {
			return err
		}
	}

	return nil
}

func writeManifest(dir, name string, data []byte) (writeErr error) {
	path := filepath.Join(dir, name)
	var f *os.File
	if f, writeErr = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600); writeErr != nil {
		writeErr = fmt.Errorf("failed to open manifest for writing: %w", writeErr)
		return
	}
	defer func() {
		if err := f.Close(); writeErr == nil && err != nil {
			writeErr = err
		}
	}()
	if _, writeErr = f.Write(data); writeErr != nil {
		writeErr = fmt.Errorf("failed write manifest file: %w", writeErr)
	}
	return
}

func (m *Manager) installGit(repo ghrepo.Interface, target string) error {
	protocol, _ := m.config.GetOrDefault(repo.RepoHost(), "git_protocol")
	cloneURL := ghrepo.FormatRemoteURL(repo, protocol)

	var commitSHA string
	if target != "" {
		var err error
		commitSHA, err = fetchCommitSHA(m.client, repo, target)
		if err != nil {
			return err
		}
	}

	name := strings.TrimSuffix(path.Base(cloneURL), ".git")
	targetDir := filepath.Join(m.installDir(), name)

	_, err := m.gitClient.Clone(cloneURL, []string{targetDir})
	if err != nil {
		return err
	}
	if commitSHA == "" {
		return nil
	}

	scopedClient := m.gitClient.ForRepo(targetDir)
	err = scopedClient.CheckoutBranch(commitSHA)
	if err != nil {
		return err
	}

	pinPath := filepath.Join(targetDir, fmt.Sprintf(".pin-%s", commitSHA))
	f, err := os.OpenFile(pinPath, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("failed to create pin file in directory: %w", err)
	}
	return f.Close()
}

var pinnedExtensionUpgradeError = errors.New("pinned extensions can not be upgraded")
var localExtensionUpgradeError = errors.New("local extensions can not be upgraded")
var upToDateError = errors.New("already up to date")
var noExtensionsInstalledError = errors.New("no extensions installed")

func (m *Manager) Upgrade(name string, force bool) error {
	// Fetch metadata during list only when upgrading all extensions.
	// This is a performance improvement so that we don't make a
	// bunch of unnecessary network requests when trying to upgrade a single extension.
	fetchMetadata := name == ""
	exts, _ := m.list(fetchMetadata)
	if len(exts) == 0 {
		return noExtensionsInstalledError
	}
	if name == "" {
		return m.upgradeExtensions(exts, force)
	}
	for _, f := range exts {
		if f.Name() != name {
			continue
		}
		var err error
		// For single extensions manually retrieve latest version since we forgo
		// doing it during list.
		f.latestVersion, err = m.getLatestVersion(f)
		if err != nil {
			return err
		}
		return m.upgradeExtensions([]Extension{f}, force)
	}
	return fmt.Errorf("no extension matched %q", name)
}

func (m *Manager) upgradeExtensions(exts []Extension, force bool) error {
	var failed bool
	for _, f := range exts {
		fmt.Fprintf(m.io.Out, "[%s]: ", f.Name())
		err := m.upgradeExtension(f, force)
		if err != nil {
			if !errors.Is(err, localExtensionUpgradeError) &&
				!errors.Is(err, upToDateError) &&
				!errors.Is(err, pinnedExtensionUpgradeError) {
				failed = true
			}
			fmt.Fprintf(m.io.Out, "%s\n", err)
			continue
		}
		currentVersion := displayExtensionVersion(&f, f.currentVersion)
		latestVersion := displayExtensionVersion(&f, f.latestVersion)
		if m.dryRunMode {
			fmt.Fprintf(m.io.Out, "would have upgraded from %s to %s\n", currentVersion, latestVersion)
		} else {
			fmt.Fprintf(m.io.Out, "upgraded from %s to %s\n", currentVersion, latestVersion)
		}
	}
	if failed {
		return errors.New("some extensions failed to upgrade")
	}
	return nil
}

func (m *Manager) upgradeExtension(ext Extension, force bool) error {
	if ext.isLocal {
		return localExtensionUpgradeError
	}
	if !force && ext.IsPinned() {
		return pinnedExtensionUpgradeError
	}
	if !ext.UpdateAvailable() {
		return upToDateError
	}
	var err error
	if ext.IsBinary() {
		err = m.upgradeBinExtension(ext)
	} else {
		// Check if git extension has changed to a binary extension
		var isBin bool
		repo, repoErr := repoFromPath(m.gitClient, filepath.Join(ext.Path(), ".."))
		if repoErr == nil {
			isBin, _ = isBinExtension(m.client, repo)
		}
		if isBin {
			if err := m.Remove(ext.Name()); err != nil {
				return fmt.Errorf("failed to migrate to new precompiled extension format: %w", err)
			}
			return m.installBin(repo, "")
		}
		err = m.upgradeGitExtension(ext, force)
	}
	return err
}

func (m *Manager) upgradeGitExtension(ext Extension, force bool) error {
	if m.dryRunMode {
		return nil
	}
	dir := filepath.Dir(ext.path)
	scopedClient := m.gitClient.ForRepo(dir)
	if force {
		err := scopedClient.Fetch("origin", "HEAD")
		if err != nil {
			return err
		}

		_, err = scopedClient.CommandOutput([]string{"reset", "--hard", "origin/HEAD"})
		return err
	}

	return scopedClient.Pull("", "")
}

func (m *Manager) upgradeBinExtension(ext Extension) error {
	repo, err := ghrepo.FromFullName(ext.url)
	if err != nil {
		return fmt.Errorf("failed to parse URL %s: %w", ext.url, err)
	}
	return m.installBin(repo, "")
}

func (m *Manager) Remove(name string) error {
	targetDir := filepath.Join(m.installDir(), "gh-"+name)
	if _, err := os.Lstat(targetDir); os.IsNotExist(err) {
		return fmt.Errorf("no extension found: %q", targetDir)
	}
	if m.dryRunMode {
		return nil
	}
	return os.RemoveAll(targetDir)
}

func (m *Manager) installDir() string {
	return filepath.Join(m.dataDir(), "extensions")
}

//go:embed ext_tmpls/goBinMain.go.txt
var mainGoTmpl string

//go:embed ext_tmpls/goBinWorkflow.yml
var goBinWorkflow []byte

//go:embed ext_tmpls/otherBinWorkflow.yml
var otherBinWorkflow []byte

//go:embed ext_tmpls/script.sh
var scriptTmpl string

//go:embed ext_tmpls/buildScript.sh
var buildScript []byte

func (m *Manager) Create(name string, tmplType extensions.ExtTemplateType) error {
	if _, err := m.gitClient.CommandOutput([]string{"init", "--quiet", name}); err != nil {
		return err
	}

	if tmplType == extensions.GoBinTemplateType {
		return m.goBinScaffolding(name)
	} else if tmplType == extensions.OtherBinTemplateType {
		return m.otherBinScaffolding(name)
	}

	script := fmt.Sprintf(scriptTmpl, name)
	if err := writeFile(filepath.Join(name, name), []byte(script), 0755); err != nil {
		return err
	}

	scopedClient := m.gitClient.ForRepo(name)
	if _, err := scopedClient.CommandOutput([]string{"add", name, "--chmod=+x"}); err != nil {
		return err
	}

	if _, err := scopedClient.CommandOutput([]string{"commit", "-m", "initial commit"}); err != nil {
		return ErrInitialCommitFailed
	}

	return nil
}

func (m *Manager) otherBinScaffolding(name string) error {
	if err := writeFile(filepath.Join(name, ".github", "workflows", "release.yml"), otherBinWorkflow, 0644); err != nil {
		return err
	}
	buildScriptPath := filepath.Join("script", "build.sh")
	if err := writeFile(filepath.Join(name, buildScriptPath), buildScript, 0755); err != nil {
		return err
	}

	scopedClient := m.gitClient.ForRepo(name)
	if _, err := scopedClient.CommandOutput([]string{"add", buildScriptPath, "--chmod=+x"}); err != nil {
		return err
	}

	if _, err := scopedClient.CommandOutput([]string{"add", "."}); err != nil {
		return err
	}

	if _, err := scopedClient.CommandOutput([]string{"commit", "-m", "initial commit"}); err != nil {
		return ErrInitialCommitFailed
	}

	return nil
}

func (m *Manager) goBinScaffolding(name string) error {
	goExe, err := m.lookPath("go")
	if err != nil {
		return fmt.Errorf("go is required for creating Go extensions: %w", err)
	}

	if err := writeFile(filepath.Join(name, ".github", "workflows", "release.yml"), goBinWorkflow, 0644); err != nil {
		return err
	}

	mainGo := fmt.Sprintf(mainGoTmpl, name)
	if err := writeFile(filepath.Join(name, "main.go"), []byte(mainGo), 0644); err != nil {
		return err
	}

	host, _ := m.config.Authentication().DefaultHost()

	currentUser, err := api.CurrentLoginName(api.NewClientFromHTTP(m.client), host)
	if err != nil {
		return err
	}

	goCmds := [][]string{
		{"mod", "init", fmt.Sprintf("%s/%s/%s", host, currentUser, name)},
		{"mod", "tidy"},
		{"build"},
	}

	ignore := fmt.Sprintf("/%[1]s\n/%[1]s.exe\n", name)
	if err := writeFile(filepath.Join(name, ".gitignore"), []byte(ignore), 0644); err != nil {
		return err
	}

	for _, args := range goCmds {
		goCmd := m.newCommand(goExe, args...)
		goCmd.Dir = name
		if err := goCmd.Run(); err != nil {
			return fmt.Errorf("failed to set up go module: %w", err)
		}
	}

	scopedClient := m.gitClient.ForRepo(name)
	if _, err := scopedClient.CommandOutput([]string{"add", "."}); err != nil {
		return err
	}

	if _, err := scopedClient.CommandOutput([]string{"commit", "-m", "initial commit"}); err != nil {
		return ErrInitialCommitFailed
	}

	return nil
}

func isSymlink(m os.FileMode) bool {
	return m&os.ModeSymlink != 0
}

func writeFile(p string, contents []byte, mode os.FileMode) error {
	if dir := filepath.Dir(p); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return os.WriteFile(p, contents, mode)
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
		return
	}

	for _, a := range r.Assets {
		dists := possibleDists()
		for _, d := range dists {
			suffix := d
			if strings.HasPrefix(d, "windows") {
				suffix += ".exe"
			}
			if strings.HasSuffix(a.Name, suffix) {
				isBin = true
				break
			}
		}
	}

	return
}

func repoFromPath(gitClient gitClient, path string) (ghrepo.Interface, error) {
	scopedClient := gitClient.ForRepo(path)
	remotes, err := scopedClient.Remotes()
	if err != nil {
		return nil, err
	}

	if len(remotes) == 0 {
		return nil, fmt.Errorf("no remotes configured for %s", path)
	}

	var remote *git.Remote

	for _, r := range remotes {
		if r.Name == "origin" {
			remote = r
			break
		}
	}

	if remote == nil {
		remote = remotes[0]
	}

	return ghrepo.FromURL(remote.FetchURL)
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
		"windows-arm64",
	}
}

func hasRosetta() bool {
	_, err := os.Stat("/Library/Apple/usr/libexec/oah/libRosettaRuntime")
	return err == nil
}

func codesignBinary(binPath string) error {
	codesignExe, err := safeexec.LookPath("codesign")
	if err != nil {
		return err
	}
	cmd := exec.Command(codesignExe, "--sign", "-", "--force", "--preserve-metadata=entitlements,requirements,flags,runtime", binPath)
	return cmd.Run()
}
