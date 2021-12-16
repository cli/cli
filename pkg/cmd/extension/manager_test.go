package extension

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GH_WANT_HELPER_PROCESS") != "1" {
		return
	}
	if err := func(args []string) error {
		// git init should create the directory named by argument
		if len(args) > 2 && strings.HasPrefix(strings.Join(args, " "), "git init") {
			dir := args[len(args)-1]
			if !strings.HasPrefix(dir, "-") {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return err
				}
			}
		}
		fmt.Fprintf(os.Stdout, "%v\n", args)
		return nil
	}(os.Args[3:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func newTestManager(dir string, client *http.Client, io *iostreams.IOStreams) *Manager {
	return &Manager{
		dataDir:  func() string { return dir },
		lookPath: func(exe string) (string, error) { return exe, nil },
		findSh:   func() (string, error) { return "sh", nil },
		newCommand: func(exe string, args ...string) *exec.Cmd {
			args = append([]string{os.Args[0], "-test.run=TestHelperProcess", "--", exe}, args...)
			cmd := exec.Command(args[0], args[1:]...)
			if io != nil {
				cmd.Stdout = io.Out
				cmd.Stderr = io.ErrOut
			}
			cmd.Env = []string{"GH_WANT_HELPER_PROCESS=1"}
			return cmd
		},
		config: config.NewBlankConfig(),
		io:     io,
		client: client,
		platform: func() (string, string) {
			return "windows-amd64", ".exe"
		},
	}
}

func TestManager_List(t *testing.T) {
	tempDir := t.TempDir()
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-hello", "gh-hello")))
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-two", "gh-two")))

	assert.NoError(t, stubBinaryExtension(
		filepath.Join(tempDir, "extensions", "gh-bin-ext"),
		binManifest{
			Owner: "owner",
			Name:  "gh-bin-ext",
			Host:  "example.com",
			Tag:   "v1.0.1",
		}))

	m := newTestManager(tempDir, nil, nil)
	exts := m.List(false)
	assert.Equal(t, 3, len(exts))
	assert.Equal(t, "bin-ext", exts[0].Name())
	assert.Equal(t, "hello", exts[1].Name())
	assert.Equal(t, "two", exts[2].Name())
}

func TestManager_List_binary_update(t *testing.T) {
	tempDir := t.TempDir()

	assert.NoError(t, stubBinaryExtension(
		filepath.Join(tempDir, "extensions", "gh-bin-ext"),
		binManifest{
			Owner: "owner",
			Name:  "gh-bin-ext",
			Host:  "example.com",
			Tag:   "v1.0.1",
		}))

	reg := httpmock.Registry{}
	defer reg.Verify(t)
	client := http.Client{Transport: &reg}

	reg.Register(
		httpmock.REST("GET", "api/v3/repos/owner/gh-bin-ext/releases/latest"),
		httpmock.JSONResponse(
			release{
				Tag: "v1.0.2",
				Assets: []releaseAsset{
					{
						Name:   "gh-bin-ext-windows-amd64",
						APIURL: "https://example.com/release/cool2",
					},
				},
			}))

	m := newTestManager(tempDir, &client, nil)

	exts := m.List(true)
	assert.Equal(t, 1, len(exts))
	assert.Equal(t, "bin-ext", exts[0].Name())
	assert.True(t, exts[0].UpdateAvailable())
	assert.Equal(t, "https://example.com/owner/gh-bin-ext", exts[0].URL())
}

func TestManager_Dispatch(t *testing.T) {
	tempDir := t.TempDir()
	extPath := filepath.Join(tempDir, "extensions", "gh-hello", "gh-hello")
	assert.NoError(t, stubExtension(extPath))

	m := newTestManager(tempDir, nil, nil)

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

func TestManager_Dispatch_binary(t *testing.T) {
	tempDir := t.TempDir()
	extPath := filepath.Join(tempDir, "extensions", "gh-hello")
	exePath := filepath.Join(extPath, "gh-hello")
	bm := binManifest{
		Owner: "owner",
		Name:  "gh-hello",
		Host:  "github.com",
		Tag:   "v1.0.0",
	}
	assert.NoError(t, stubBinaryExtension(extPath, bm))

	m := newTestManager(tempDir, nil, nil)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	found, err := m.Dispatch([]string{"hello", "one", "two"}, nil, stdout, stderr)
	assert.NoError(t, err)
	assert.True(t, found)

	assert.Equal(t, fmt.Sprintf("[%s one two]\n", exePath), stdout.String())
	assert.Equal(t, "", stderr.String())
}

func TestManager_Remove(t *testing.T) {
	tempDir := t.TempDir()
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-hello", "gh-hello")))
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-two", "gh-two")))

	m := newTestManager(tempDir, nil, nil)
	err := m.Remove("hello")
	assert.NoError(t, err)

	items, err := ioutil.ReadDir(filepath.Join(tempDir, "extensions"))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(items))
	assert.Equal(t, "gh-two", items[0].Name())
}

func TestManager_Upgrade_NoExtensions(t *testing.T) {
	tempDir := t.TempDir()
	io, _, stdout, stderr := iostreams.Test()
	m := newTestManager(tempDir, nil, io)
	err := m.Upgrade("", false)
	assert.EqualError(t, err, "no extensions installed")
	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "", stderr.String())
}

func TestManager_Upgrade_NoMatchingExtension(t *testing.T) {
	tempDir := t.TempDir()
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-hello", "gh-hello")))
	io, _, stdout, stderr := iostreams.Test()
	m := newTestManager(tempDir, nil, io)
	err := m.Upgrade("invalid", false)
	assert.EqualError(t, err, `no extension matched "invalid"`)
	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "", stderr.String())
}

func TestManager_UpgradeExtensions(t *testing.T) {
	tempDir := t.TempDir()
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-hello", "gh-hello")))
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-two", "gh-two")))
	assert.NoError(t, stubLocalExtension(tempDir, filepath.Join(tempDir, "extensions", "gh-local", "gh-local")))
	io, _, stdout, stderr := iostreams.Test()
	m := newTestManager(tempDir, nil, io)
	exts, err := m.list(false)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(exts))
	for i := 0; i < 3; i++ {
		exts[i].currentVersion = "old version"
		exts[i].latestVersion = "new version"
	}
	err = m.upgradeExtensions(exts, false)
	assert.NoError(t, err)
	assert.Equal(t, heredoc.Docf(
		`
		[hello]: [git -C %s pull --ff-only]
		upgrade complete
		[local]: local extensions can not be upgraded
		[two]: [git -C %s pull --ff-only]
		upgrade complete
		`,
		filepath.Join(tempDir, "extensions", "gh-hello"),
		filepath.Join(tempDir, "extensions", "gh-two"),
	), stdout.String())
	assert.Equal(t, "", stderr.String())
}

func TestManager_UpgradeExtension_LocalExtension(t *testing.T) {
	tempDir := t.TempDir()
	assert.NoError(t, stubLocalExtension(tempDir, filepath.Join(tempDir, "extensions", "gh-local", "gh-local")))

	io, _, stdout, stderr := iostreams.Test()
	m := newTestManager(tempDir, nil, io)
	exts, err := m.list(false)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(exts))
	err = m.upgradeExtension(exts[0], false)
	assert.EqualError(t, err, "local extensions can not be upgraded")
	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "", stderr.String())
}

func TestManager_UpgradeExtension_GitExtension(t *testing.T) {
	tempDir := t.TempDir()
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-remote", "gh-remote")))
	io, _, stdout, stderr := iostreams.Test()
	m := newTestManager(tempDir, nil, io)
	exts, err := m.list(false)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(exts))
	ext := exts[0]
	ext.currentVersion = "old version"
	ext.latestVersion = "new version"
	err = m.upgradeExtension(ext, false)
	assert.NoError(t, err)
	assert.Equal(t, heredoc.Docf(
		`
		[git -C %s pull --ff-only]
		`,
		filepath.Join(tempDir, "extensions", "gh-remote"),
	), stdout.String())
	assert.Equal(t, "", stderr.String())
}

func TestManager_UpgradeExtension_GitExtension_Force(t *testing.T) {
	tempDir := t.TempDir()
	extensionDir := filepath.Join(tempDir, "extensions", "gh-remote")
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-remote", "gh-remote")))
	io, _, stdout, stderr := iostreams.Test()
	m := newTestManager(tempDir, nil, io)
	exts, err := m.list(false)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(exts))
	ext := exts[0]
	ext.currentVersion = "old version"
	ext.latestVersion = "new version"
	err = m.upgradeExtension(ext, true)
	assert.NoError(t, err)
	assert.Equal(t, heredoc.Docf(
		`
		[git -C %[1]s fetch origin HEAD]
		[git -C %[1]s reset --hard origin/HEAD]
		`,
		extensionDir,
	), stdout.String())
	assert.Equal(t, "", stderr.String())
}

func TestManager_MigrateToBinaryExtension(t *testing.T) {
	tempDir := t.TempDir()
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-remote", "gh-remote")))
	io, _, stdout, stderr := iostreams.Test()

	reg := httpmock.Registry{}
	defer reg.Verify(t)
	client := http.Client{Transport: &reg}
	m := newTestManager(tempDir, &client, io)
	exts, err := m.list(false)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(exts))
	ext := exts[0]
	ext.currentVersion = "old version"
	ext.latestVersion = "new version"

	rs, restoreRun := run.Stub()
	defer restoreRun(t)

	rs.Register(`git -C.*?gh-remote remote -v`, 0, "origin  git@github.com:owner/gh-remote.git (fetch)\norigin  git@github.com:owner/gh-remote.git (push)")
	rs.Register(`git -C.*?gh-remote config --get-regexp \^.*`, 0, "remote.origin.gh-resolve base")

	reg.Register(
		httpmock.REST("GET", "repos/owner/gh-remote/releases/latest"),
		httpmock.JSONResponse(
			release{
				Tag: "v1.0.2",
				Assets: []releaseAsset{
					{
						Name:   "gh-remote-windows-amd64.exe",
						APIURL: "/release/cool",
					},
				},
			}))
	reg.Register(
		httpmock.REST("GET", "repos/owner/gh-remote/releases/latest"),
		httpmock.JSONResponse(
			release{
				Tag: "v1.0.2",
				Assets: []releaseAsset{
					{
						Name:   "gh-remote-windows-amd64.exe",
						APIURL: "/release/cool",
					},
				},
			}))
	reg.Register(
		httpmock.REST("GET", "release/cool"),
		httpmock.StringResponse("FAKE UPGRADED BINARY"))

	err = m.upgradeExtension(ext, false)
	assert.NoError(t, err)

	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "", stderr.String())

	manifest, err := os.ReadFile(filepath.Join(tempDir, "extensions/gh-remote", manifestName))
	assert.NoError(t, err)

	var bm binManifest
	err = yaml.Unmarshal(manifest, &bm)
	assert.NoError(t, err)

	assert.Equal(t, binManifest{
		Name:  "gh-remote",
		Owner: "owner",
		Host:  "github.com",
		Tag:   "v1.0.2",
		Path:  filepath.Join(tempDir, "extensions/gh-remote/gh-remote.exe"),
	}, bm)

	fakeBin, err := os.ReadFile(filepath.Join(tempDir, "extensions/gh-remote/gh-remote.exe"))
	assert.NoError(t, err)

	assert.Equal(t, "FAKE UPGRADED BINARY", string(fakeBin))
}

func TestManager_UpgradeExtension_BinaryExtension(t *testing.T) {
	tempDir := t.TempDir()

	reg := httpmock.Registry{}
	defer reg.Verify(t)

	assert.NoError(t, stubBinaryExtension(
		filepath.Join(tempDir, "extensions", "gh-bin-ext"),
		binManifest{
			Owner: "owner",
			Name:  "gh-bin-ext",
			Host:  "example.com",
			Tag:   "v1.0.1",
		}))

	io, _, stdout, stderr := iostreams.Test()
	m := newTestManager(tempDir, &http.Client{Transport: &reg}, io)
	reg.Register(
		httpmock.REST("GET", "api/v3/repos/owner/gh-bin-ext/releases/latest"),
		httpmock.JSONResponse(
			release{
				Tag: "v1.0.2",
				Assets: []releaseAsset{
					{
						Name:   "gh-bin-ext-windows-amd64.exe",
						APIURL: "https://example.com/release/cool2",
					},
				},
			}))
	reg.Register(
		httpmock.REST("GET", "release/cool2"),
		httpmock.StringResponse("FAKE UPGRADED BINARY"))

	exts, err := m.list(false)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(exts))
	ext := exts[0]
	ext.latestVersion = "v1.0.2"
	err = m.upgradeExtension(ext, false)
	assert.NoError(t, err)

	manifest, err := os.ReadFile(filepath.Join(tempDir, "extensions/gh-bin-ext", manifestName))
	assert.NoError(t, err)

	var bm binManifest
	err = yaml.Unmarshal(manifest, &bm)
	assert.NoError(t, err)

	assert.Equal(t, binManifest{
		Name:  "gh-bin-ext",
		Owner: "owner",
		Host:  "example.com",
		Tag:   "v1.0.2",
		Path:  filepath.Join(tempDir, "extensions/gh-bin-ext/gh-bin-ext.exe"),
	}, bm)

	fakeBin, err := os.ReadFile(filepath.Join(tempDir, "extensions/gh-bin-ext/gh-bin-ext.exe"))
	assert.NoError(t, err)
	assert.Equal(t, "FAKE UPGRADED BINARY", string(fakeBin))

	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "", stderr.String())
}

func TestManager_Install_git(t *testing.T) {
	tempDir := t.TempDir()

	reg := httpmock.Registry{}
	defer reg.Verify(t)
	client := http.Client{Transport: &reg}

	io, _, stdout, stderr := iostreams.Test()
	m := newTestManager(tempDir, &client, io)

	reg.Register(
		httpmock.REST("GET", "repos/owner/gh-some-ext/releases/latest"),
		httpmock.JSONResponse(
			release{
				Assets: []releaseAsset{
					{
						Name:   "not-a-binary",
						APIURL: "https://example.com/release/cool",
					},
				},
			}))
	reg.Register(
		httpmock.REST("GET", "repos/owner/gh-some-ext/contents/gh-some-ext"),
		httpmock.StringResponse("script"))

	repo := ghrepo.New("owner", "gh-some-ext")

	err := m.Install(repo)
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("[git clone https://github.com/owner/gh-some-ext.git %s]\n", filepath.Join(tempDir, "extensions", "gh-some-ext")), stdout.String())
	assert.Equal(t, "", stderr.String())
}

func TestManager_Install_binary_unsupported(t *testing.T) {
	repo := ghrepo.NewWithHost("owner", "gh-bin-ext", "example.com")

	reg := httpmock.Registry{}
	defer reg.Verify(t)
	client := http.Client{Transport: &reg}

	reg.Register(
		httpmock.REST("GET", "api/v3/repos/owner/gh-bin-ext/releases/latest"),
		httpmock.JSONResponse(
			release{
				Assets: []releaseAsset{
					{
						Name:   "gh-bin-ext-linux-amd64",
						APIURL: "https://example.com/release/cool",
					},
				},
			}))
	reg.Register(
		httpmock.REST("GET", "api/v3/repos/owner/gh-bin-ext/releases/latest"),
		httpmock.JSONResponse(
			release{
				Tag: "v1.0.1",
				Assets: []releaseAsset{
					{
						Name:   "gh-bin-ext-linux-amd64",
						APIURL: "https://example.com/release/cool",
					},
				},
			}))

	io, _, stdout, stderr := iostreams.Test()
	tempDir := t.TempDir()

	m := newTestManager(tempDir, &client, io)

	err := m.Install(repo)
	assert.EqualError(t, err, "gh-bin-ext unsupported for windows-amd64. Open an issue: `gh issue create -R owner/gh-bin-ext -t'Support windows-amd64'`")

	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "", stderr.String())
}

func TestManager_Install_binary(t *testing.T) {
	repo := ghrepo.NewWithHost("owner", "gh-bin-ext", "example.com")

	reg := httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "api/v3/repos/owner/gh-bin-ext/releases/latest"),
		httpmock.JSONResponse(
			release{
				Assets: []releaseAsset{
					{
						Name:   "gh-bin-ext-windows-amd64.exe",
						APIURL: "https://example.com/release/cool",
					},
				},
			}))
	reg.Register(
		httpmock.REST("GET", "api/v3/repos/owner/gh-bin-ext/releases/latest"),
		httpmock.JSONResponse(
			release{
				Tag: "v1.0.1",
				Assets: []releaseAsset{
					{
						Name:   "gh-bin-ext-windows-amd64.exe",
						APIURL: "https://example.com/release/cool",
					},
				},
			}))
	reg.Register(
		httpmock.REST("GET", "release/cool"),
		httpmock.StringResponse("FAKE BINARY"))

	io, _, stdout, stderr := iostreams.Test()
	tempDir := t.TempDir()

	m := newTestManager(tempDir, &http.Client{Transport: &reg}, io)

	err := m.Install(repo)
	assert.NoError(t, err)

	manifest, err := os.ReadFile(filepath.Join(tempDir, "extensions/gh-bin-ext", manifestName))
	assert.NoError(t, err)

	var bm binManifest
	err = yaml.Unmarshal(manifest, &bm)
	assert.NoError(t, err)

	assert.Equal(t, binManifest{
		Name:  "gh-bin-ext",
		Owner: "owner",
		Host:  "example.com",
		Tag:   "v1.0.1",
		Path:  filepath.Join(tempDir, "extensions/gh-bin-ext/gh-bin-ext.exe"),
	}, bm)

	fakeBin, err := os.ReadFile(filepath.Join(tempDir, "extensions/gh-bin-ext/gh-bin-ext.exe"))
	assert.NoError(t, err)
	assert.Equal(t, "FAKE BINARY", string(fakeBin))

	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "", stderr.String())
}

func TestManager_Create(t *testing.T) {
	chdirTemp(t)
	io, _, stdout, stderr := iostreams.Test()
	m := newTestManager(".", nil, io)

	err := m.Create("gh-test", extensions.GitTemplateType)
	assert.NoError(t, err)
	files, err := ioutil.ReadDir("gh-test")
	assert.NoError(t, err)
	assert.Equal(t, []string{"gh-test"}, fileNames(files))

	assert.Equal(t, heredoc.Doc(`
		[git init --quiet gh-test]
		[git -C gh-test add gh-test --chmod=+x]
	`), stdout.String())
	assert.Equal(t, "", stderr.String())
}

func TestManager_Create_go_binary(t *testing.T) {
	chdirTemp(t)
	reg := httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`{"data":{"viewer":{"login":"jillv"}}}`))

	io, _, stdout, stderr := iostreams.Test()
	m := newTestManager(".", &http.Client{Transport: &reg}, io)

	err := m.Create("gh-test", extensions.GoBinTemplateType)
	require.NoError(t, err)

	files, err := ioutil.ReadDir("gh-test")
	require.NoError(t, err)
	assert.Equal(t, []string{".github", ".gitignore", "main.go"}, fileNames(files))

	gitignore, err := os.ReadFile(filepath.Join("gh-test", ".gitignore"))
	require.NoError(t, err)
	assert.Equal(t, heredoc.Doc(`
		/gh-test
		/gh-test.exe
	`), string(gitignore))

	files, err = ioutil.ReadDir(filepath.Join("gh-test", ".github", "workflows"))
	require.NoError(t, err)
	assert.Equal(t, []string{"release.yml"}, fileNames(files))

	assert.Equal(t, heredoc.Doc(`
		[git init --quiet gh-test]
		[go mod init github.com/jillv/gh-test]
		[go mod tidy]
		[go build]
		[git -C gh-test add .]
	`), stdout.String())
	assert.Equal(t, "", stderr.String())
}

func TestManager_Create_other_binary(t *testing.T) {
	chdirTemp(t)
	io, _, stdout, stderr := iostreams.Test()
	m := newTestManager(".", nil, io)

	err := m.Create("gh-test", extensions.OtherBinTemplateType)
	assert.NoError(t, err)

	files, err := ioutil.ReadDir("gh-test")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(files))

	files, err = ioutil.ReadDir(filepath.Join("gh-test", ".github", "workflows"))
	assert.NoError(t, err)
	assert.Equal(t, []string{"release.yml"}, fileNames(files))

	files, err = ioutil.ReadDir(filepath.Join("gh-test", "script"))
	assert.NoError(t, err)
	assert.Equal(t, []string{"build.sh"}, fileNames(files))

	assert.Equal(t, heredoc.Docf(`
		[git init --quiet gh-test]
		[git -C gh-test add %s --chmod=+x]
		[git -C gh-test add .]
	`, filepath.FromSlash("script/build.sh")), stdout.String())
	assert.Equal(t, "", stderr.String())
}

// chdirTemp changes the current working directory to a temporary directory for the duration of the test.
func chdirTemp(t *testing.T) {
	oldWd, _ := os.Getwd()
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWd)
	})
}

func fileNames(files []os.FileInfo) []string {
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Name()
	}
	sort.Strings(names)
	return names
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

// Given the path where an extension should be installed and a manifest struct, creates a fake binary extension on disk
func stubBinaryExtension(installPath string, bm binManifest) error {
	if err := os.MkdirAll(installPath, 0755); err != nil {
		return err
	}
	fakeBinaryPath := filepath.Join(installPath, filepath.Base(installPath))
	fb, err := os.OpenFile(fakeBinaryPath, os.O_CREATE, 0755)
	if err != nil {
		return err
	}
	err = fb.Close()
	if err != nil {
		return err
	}

	bs, err := yaml.Marshal(bm)
	if err != nil {
		return fmt.Errorf("failed to serialize manifest: %w", err)
	}

	manifestPath := filepath.Join(installPath, manifestName)

	fm, err := os.OpenFile(manifestPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open manifest for writing: %w", err)
	}
	_, err = fm.Write(bs)
	if err != nil {
		return fmt.Errorf("failed write manifest file: %w", err)
	}

	return fm.Close()
}
