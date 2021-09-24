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
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
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

func newTestManager(dir string, client *http.Client, io *iostreams.IOStreams) *Manager {
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
		config: config.NewBlankConfig(),
		io:     io,
		client: client,
		platform: func() string {
			return "windows-amd64"
		},
	}
}

func TestManager_List(t *testing.T) {
	tempDir := t.TempDir()
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-hello", "gh-hello")))
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-two", "gh-two")))

	m := newTestManager(tempDir, nil, nil)
	exts := m.List(false)
	assert.Equal(t, 2, len(exts))
	assert.Equal(t, "hello", exts[0].Name())
	assert.Equal(t, "two", exts[1].Name())
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

func TestManager_Upgrade_AllExtensions(t *testing.T) {
	tempDir := t.TempDir()
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-hello", "gh-hello")))
	assert.NoError(t, stubExtension(filepath.Join(tempDir, "extensions", "gh-two", "gh-two")))
	assert.NoError(t, stubLocalExtension(tempDir, filepath.Join(tempDir, "extensions", "gh-local", "gh-local")))

	m := newTestManager(tempDir, nil, nil)

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

	m := newTestManager(tempDir, nil, nil)

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

	m := newTestManager(tempDir, nil, nil)

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

	m := newTestManager(tempDir, nil, nil)

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

	m := newTestManager(tempDir, nil, nil)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := m.Upgrade("", false, stdout, stderr)
	assert.EqualError(t, err, "no extensions installed")
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

	io, _, _, _ := iostreams.Test()
	tempDir := t.TempDir()

	m := newTestManager(tempDir, &client, io)

	err := m.Install(repo)
	assert.Error(t, err)

	errText := "gh-bin-ext unsupported for windows-amd64. Open an issue: `gh issue create -R owner/gh-bin-ext -t'Support windows-amd64'`"

	assert.Equal(t, errText, err.Error())
}

func TestManager_Install_binary(t *testing.T) {
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
						Name:   "gh-bin-ext-windows-amd64",
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
						Name:   "gh-bin-ext-windows-amd64",
						APIURL: "https://example.com/release/cool",
					},
				},
			}))
	reg.Register(
		httpmock.REST("GET", "release/cool"),
		httpmock.StringResponse("FAKE BINARY"))

	io, _, _, _ := iostreams.Test()
	tempDir := t.TempDir()

	m := newTestManager(tempDir, &client, io)

	err := m.Install(repo)
	assert.NoError(t, err)

	manifest, err := os.ReadFile(filepath.Join(tempDir, "extensions/gh-bin-ext/manifest.yml"))
	assert.NoError(t, err)

	var bm binManifest
	err = yaml.Unmarshal(manifest, &bm)
	assert.NoError(t, err)

	assert.Equal(t, binManifest{
		Name:  "gh-bin-ext",
		Owner: "owner",
		Host:  "example.com",
		Tag:   "v1.0.1",
		Path:  filepath.Join(tempDir, "extensions/gh-bin-ext/gh-bin-ext"),
	}, bm)

	fakeBin, err := os.ReadFile(filepath.Join(tempDir, "extensions/gh-bin-ext/gh-bin-ext"))
	assert.NoError(t, err)

	assert.Equal(t, "FAKE BINARY", string(fakeBin))
}

func TestManager_Create(t *testing.T) {
	tempDir := t.TempDir()
	oldWd, _ := os.Getwd()
	assert.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() { _ = os.Chdir(oldWd) })
	m := newTestManager(tempDir, nil, nil)
	err := m.Create("gh-test")
	assert.NoError(t, err)
	files, err := ioutil.ReadDir(filepath.Join(tempDir, "gh-test"))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(files))
	extFile := files[0]
	assert.Equal(t, "gh-test", extFile.Name())
	if runtime.GOOS == "windows" {
		assert.Equal(t, os.FileMode(0666), extFile.Mode())
	} else {
		assert.Equal(t, os.FileMode(0755), extFile.Mode())
	}
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
