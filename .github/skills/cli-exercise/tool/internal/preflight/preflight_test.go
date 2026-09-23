package preflight

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/cli/cli/v2/cli-exercise/internal/terminal"
	"github.com/stretchr/testify/require"
)

type fakeRunner struct {
	nativeFailure, missingRaw, encoderFailure, presentationFailure bool
	calls                                                          [][]string
}

func (runner *fakeRunner) Run(_ context.Context, args []string, cwd string, env []string, _ time.Duration) ([]byte, error) {
	runner.calls = append(runner.calls, slices.Clone(args))
	for _, value := range env {
		for _, forbidden := range []string{"GH_TOKEN=", "GITHUB_TOKEN=", "NODE_OPTIONS=", "HTTP_PROXY="} {
			if strings.HasPrefix(value, forbidden) {
				return nil, fmt.Errorf("probe inherited unapproved environment")
			}
		}
	}
	name := filepath.Base(args[0])
	switch {
	case slices.Contains(args, "--version") && name == "node":
		return []byte("v26.7.0\n"), nil
	case slices.Contains(args, "-version"):
		return []byte(name + " version 9.0.1\n"), nil
	case name == "ffmpeg":
		if runner.encoderFailure {
			return nil, errors.New("synthetic encoder failure")
		}
		if runner.presentationFailure && strings.Contains(strings.Join(args, " "), "overlay=") {
			return nil, errors.New("synthetic presentation filter failure")
		}
		if args[len(args)-1] != "-" {
			output := args[len(args)-1]
			if !filepath.IsAbs(output) {
				output = filepath.Join(cwd, output)
			}
			if !cliutil.Inside(cwd, output) {
				return nil, errors.New("fake media output escaped its workspace")
			}
			return nil, os.WriteFile(output, []byte("encoded fixture"), 0o600)
		}
		return nil, nil
	case name == "ffprobe":
		codec := "h264"
		if filepath.Ext(args[len(args)-1]) == ".gif" {
			codec = "gif"
		}
		return json.Marshal(map[string]any{"streams": []any{map[string]any{
			"codec_name": codec, "width": 16, "height": 16, "nb_read_frames": "2",
		}}})
	default:
		return nil, fmt.Errorf("unexpected fake command")
	}
}

type testSetup struct {
	root, skill, packagePath string
	options                  options
	runner                   *fakeRunner
	factory                  func(string, options) (*service, error)
}

func setup(t *testing.T) testSetup {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	skill := filepath.Join(root, "skill")
	require.NoError(t, os.MkdirAll(filepath.Join(skill, "scripts"), 0o700))
	require.NoError(t, cliutil.WriteJSON(filepath.Join(skill, "scripts/package.json"),
		map[string]any{"dependencies": map[string]string{"tuistory": version}}))
	require.NoError(t, cliutil.WriteJSON(filepath.Join(skill, "scripts/package-lock.json"),
		map[string]any{"lockfileVersion": 3, "packages": map[string]any{
			"":                      map[string]any{"dependencies": map[string]string{"tuistory": version}},
			"node_modules/tuistory": map[string]any{"version": version},
		}}))
	binaries := map[string]string{}
	for _, name := range []string{"node", "ffmpeg", "ffprobe"} {
		binaries[name] = filepath.Join(root, name)
		require.NoError(t, os.WriteFile(binaries[name], []byte(name+" fixture"), 0o700))
	}
	modules := filepath.Join(root, "node_modules")
	require.NoError(t, os.MkdirAll(filepath.Join(modules, "tuistory"), 0o700))
	packagePath := filepath.Join(modules, "tuistory/package.json")
	require.NoError(t, cliutil.WriteJSON(packagePath, map[string]string{"name": "tuistory", "version": version, "main": "entry"}))
	require.NoError(t, os.WriteFile(filepath.Join(modules, "tuistory/entry"), []byte("fixture entry"), 0o600))
	settings := options{ModuleRoot: modules,
		Node: binaries["node"], FFmpeg: binaries["ffmpeg"], FFprobe: binaries["ffprobe"],
		Formats: []string{"gif", "mp4"}, ProbeTimeout: 5}
	runner := &fakeRunner{}
	factory := func(root string, options options) (*service, error) {
		service, err := newService(root, options)
		if err != nil {
			return nil, err
		}
		service.runner = runner
		service.nativeProbe = func(_ context.Context, _ terminal.Session, launch terminal.LaunchOptions) error {
			require.Equal(t, []string{"probe-fixture"}, launch.Args)
			require.Equal(t, "approved", launch.Env["CLI_EXERCISE_PROBE"])
			if runner.nativeFailure {
				return errors.New("synthetic native failure")
			}
			if runner.missingRaw {
				return errors.New("raw fixture output was missing")
			}
			return nil
		}
		service.lookup = func(selection, name string) (string, error) {
			path := selection
			if path == "" {
				path = binaries[name]
			}
			if _, err := os.Stat(path); err != nil {
				return "", err
			}
			return path, nil
		}
		return service, nil
	}
	return testSetup{root, skill, packagePath, settings, runner, factory}
}

func TestParseOptions(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		fail bool
	}{
		{name: "check", args: []string{"check", "--output", "receipt"}},
		{name: "selected format", args: []string{"check", "--output", "receipt", "--format", "mp4"}},
		{name: "plan is no longer a helper operation", args: []string{"plan", "--output", "plan"}, fail: true},
		{name: "installation is never a helper operation", args: []string{"install", "--plan", "plan", "--approve-plan", "id"}, fail: true},
		{name: "cache management flag removed", args: []string{"check", "--output", "receipt", "--cache-dir", "cache"}, fail: true},
		{name: "lock management flag removed", args: []string{"check", "--output", "receipt", "--lock-timeout", "1"}, fail: true},
		{name: "missing command", fail: true},
		{name: "missing output", args: []string{"check"}, fail: true},
		{name: "missing plan", args: []string{"install"}, fail: true},
		{name: "unknown format", args: []string{"check", "--output", "receipt", "--format", "webm"}, fail: true},
		{name: "blank explicit selection", args: []string{"check", "--output", "receipt", "--node", ""}, fail: true},
		{name: "font selection belongs to rendering", args: []string{"check", "--output", "receipt", "--font", "mono.ttf"}, fail: true},
		{name: "nonfinite timeout", args: []string{"check", "--output", "receipt", "--probe-timeout", "NaN"}, fail: true},
		{name: "old interpreter flag rejected", args: []string{"check", "--output", "receipt", "--python", "not-used"}, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseOptions(tc.args)
			if tc.fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRun(t *testing.T) {
	t.Run("shared adapter probe", sharedProbeCases)
	t.Run("Go probe requires owned terminal stdin", func(t *testing.T) {
		var diagnostic bytes.Buffer
		code := ProbeMain(context.Background(), nil, cliutil.Streams{
			In: &bytes.Buffer{}, Out: io.Discard, ErrOut: &diagnostic,
		})
		require.Equal(t, 1, code)
		require.Contains(t, diagnostic.String(), "probe_fixture_failed")
	})
	for _, name := range []string{
		"ready", "missing", "incompatible", "native failure", "missing raw", "encoder failure", "presentation filter failure",
		"no installation selected", "unsupported runtime", "stdout is not a file", "read only",
	} {
		t.Run(name, func(t *testing.T) {
			fixture := setup(t)
			options := fixture.options
			switch name {
			case "native failure":
				fixture.runner.nativeFailure = true
			case "missing raw":
				fixture.runner.missingRaw = true
			case "encoder failure":
				fixture.runner.encoderFailure = true
			case "presentation filter failure":
				fixture.runner.presentationFailure = true
			case "missing":
				require.NoError(t, os.Remove(fixture.packagePath))
			case "incompatible":
				require.NoError(t, cliutil.WriteJSON(fixture.packagePath, map[string]string{"name": "tuistory", "version": "0.10.0"}))
			case "no installation selected":
				options.ModuleRoot = ""
			}
			service, err := fixture.factory(fixture.skill, options)
			require.NoError(t, err)
			if name == "stdout is not a file" {
				work := t.TempDir()
				_, err := fixture.runner.Run(context.Background(), []string{options.FFmpeg, "-f", "null", "-"}, work, nil, time.Second)
				require.NoError(t, err)
				require.NoFileExists(t, filepath.Join(work, "-"))
				return
			}
			if name == "unsupported runtime" {
				service.managed = false
			}
			before := fixtureFiles(t, fixture.root)
			result, err := service.check(context.Background())
			require.NoError(t, err)
			want := "blocked"
			if name == "ready" || name == "read only" {
				want = "ready"
			} else if name == "missing" || name == "incompatible" {
				want = "needs-install"
			}
			require.Equal(t, want, result.Status)
			require.Equal(t, before, fixtureFiles(t, fixture.root), "checking must not change installations or create management state")
			for _, call := range fixture.runner.calls {
				require.NotEqual(t, "npm", filepath.Base(call[0]))
			}
			if name == "no installation selected" {
				require.Contains(t, result.Issues[0].Detail, "do not assume installation is missing")
			}
		})
	}
	t.Run("install command cannot execute or write a plan", func(t *testing.T) {
		fixture := setup(t)
		var stdout, stderr bytes.Buffer
		before := fixtureFiles(t, fixture.root)
		code := Main(context.Background(), fixture.skill, []string{"install", "--approve-plan", "anything"}, cliutil.Streams{
			Out: &stdout, ErrOut: &stderr,
		})
		require.Equal(t, 2, code)
		require.Contains(t, stderr.String(), "check only")
		require.Empty(t, stdout.String())
		require.Equal(t, before, fixtureFiles(t, fixture.root))
	})
}

func fixtureFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		require.NoError(t, err)
		if !entry.IsDir() {
			hash, err := cliutil.SHA256File(path)
			require.NoError(t, err)
			files[path] = hash
		}
		return nil
	})
	require.NoError(t, err)
	return files
}
