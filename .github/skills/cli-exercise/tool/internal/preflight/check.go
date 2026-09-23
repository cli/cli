// Package preflight checks existing capabilities without installing dependencies.
package preflight

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"maps"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/cli/cli/v2/cli-exercise/internal/fontutil"
	"github.com/cli/cli/v2/cli-exercise/internal/recording"
	"github.com/cli/cli/v2/cli-exercise/internal/terminal"
)

const version = "0.11.0"

type options struct {
	ModuleRoot   string   `json:"module_root,omitempty"`
	Node         string   `json:"node,omitempty"`
	FFmpeg       string   `json:"ffmpeg,omitempty"`
	FFprobe      string   `json:"ffprobe,omitempty"`
	Font         string   `json:"font,omitempty"`
	Formats      []string `json:"formats"`
	ProbeTimeout float64  `json:"probe_timeout"`
}

type issue struct {
	Item         string `json:"item"`
	Kind         string `json:"kind,omitempty"`
	Detail       string `json:"detail"`
	NeedsInstall bool   `json:"needsInstall"`
	Remedy       string `json:"remedy,omitempty"`
	Source       string `json:"source,omitempty"`
}

type report struct {
	recording.Receipt
	Issues []issue `json:"issues"`
}

type runner interface {
	Run(context.Context, []string, string, []string, time.Duration) ([]byte, error)
}

type processRunner struct{}

func (processRunner) Run(ctx context.Context, args []string, cwd string, env []string, timeout time.Duration) ([]byte, error) {
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(bounded, args[0], args[1:]...)
	command.Dir, command.Env = cwd, env
	command.WaitDelay = 5 * time.Second
	cliutil.OwnProcessGroup(command)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		detail := stderr.String()
		if detail == "" {
			detail = stdout.String()
		}
		if len(detail) > 4000 {
			detail = detail[len(detail)-4000:]
		}
		return nil, fmt.Errorf("%s failed: %w: %s", filepath.Base(args[0]), err, strings.TrimSpace(detail))
	}
	return stdout.Bytes(), nil
}

type service struct {
	options        options
	root, manifest string
	helper         string
	runner         runner
	fontProbe      func(string) (fontutil.Info, error)
	lookup         func(string, string) (string, error)
	managed        bool
	nativeProbe    func(context.Context, terminal.Session, terminal.LaunchOptions) error
}

func executable(selection, fallback string) (string, error) {
	if selection == "" {
		selection = fallback
	}
	path, err := exec.LookPath(selection)
	if err != nil {
		return "", err
	}
	return filepath.Abs(path)
}

func newService(root string, selected options) (*service, error) {
	if len(selected.Formats) == 0 || selected.ProbeTimeout < 5 || selected.ProbeTimeout > 120 ||
		math.IsNaN(selected.ProbeTimeout) || math.IsInf(selected.ProbeTimeout, 0) {
		return nil, fmt.Errorf("invalid prerequisite selections or time budgets")
	}
	for _, format := range selected.Formats {
		if !slices.Contains([]string{"gif", "mp4"}, format) {
			return nil, fmt.Errorf("formats must be gif and/or mp4")
		}
	}
	selected.Formats = slices.Clone(selected.Formats)
	slices.Sort(selected.Formats)
	selected.Formats = slices.Compact(selected.Formats)
	root, err := cliutil.ResolvePath(root)
	if err != nil {
		return nil, err
	}
	for _, value := range []*string{&selected.ModuleRoot, &selected.Font} {
		if *value != "" {
			absolute, err := filepath.Abs(*value)
			if err != nil {
				return nil, err
			}
			*value = absolute
		}
	}
	manifest, err := cliutil.ManifestHash(root)
	if err != nil {
		return nil, err
	}
	helper, err := os.Executable()
	if err != nil {
		return nil, err
	}
	var packageJSON struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if _, err := cliutil.ReadJSON(filepath.Join(root, "scripts/package.json"), 1<<20, &packageJSON); err != nil {
		return nil, err
	}
	if len(packageJSON.Dependencies) != 1 || packageJSON.Dependencies["tuistory"] != version {
		return nil, fmt.Errorf("the skill manifest does not pin the supported Tuistory version")
	}
	return &service{options: selected, root: root, manifest: manifest, helper: helper,
		runner: processRunner{}, fontProbe: fontutil.Probe, lookup: executable, managed: cliutil.ManagedProcesses(),
		nativeProbe: checkTerminal}, nil
}

func isolatedEnvironment(workspace, node string) (map[string]string, error) {
	values, err := cliutil.IsolatedEnvironment(workspace, "")
	if err != nil {
		return nil, err
	}
	values["LANG"], values["LC_ALL"] = "en_US.UTF-8", "en_US.UTF-8"
	values["TERM"], values["COLORTERM"] = "xterm-truecolor", "truecolor"
	values["GIT_CONFIG_NOSYSTEM"], values["GIT_CONFIG_GLOBAL"] = "1", os.DevNull
	paths := []string{"/usr/bin", "/bin"}
	if node != "" {
		paths = append([]string{filepath.Dir(node)}, paths...)
	}
	for _, key := range []string{"SystemRoot", "WINDIR"} {
		if value := os.Getenv(key); value != "" {
			values[key] = value
			paths = append(paths, filepath.Join(value, "System32"))
		}
	}
	values["PATH"] = strings.Join(paths, string(os.PathListSeparator))
	return values, nil
}

func (service *service) check(ctx context.Context) (result report, err error) {
	result = report{SchemaVersion: 1, Manifest: service.manifest, Issues: []issue{}}
	result.Checks.Formats, result.Checks.FontAttempts = map[string]bool{}, []map[string]any{}
	problem := func(item, kind, detail string, needsInstall bool) {
		remedy := "Select a working existing capability and rerun check; system installation is a separate user decision."
		if needsInstall {
			remedy = "Propose setup commands using the pinned requirements, ask for approval, then rerun check."
		}
		result.Issues = append(result.Issues, issue{Item: item, Kind: kind, Detail: detail, NeedsInstall: needsInstall, Remedy: remedy})
	}
	if !service.managed {
		problem("runtime", "unsupported-platform", "Owned terminal process cleanup and private ownership checks are not implemented on this platform.", false)
	}
	work, err := os.MkdirTemp("", "cli-exercise-probe-")
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(work)) }()
	node, _ := service.lookup(service.options.Node, "node")
	env, err := isolatedEnvironment(work, node)
	if err != nil {
		return result, err
	}
	run := func(args []string) ([]byte, error) {
		output, err := service.runner.Run(ctx, args, work, cliutil.EnvironmentList(env), time.Duration(service.options.ProbeTimeout*float64(time.Second)))
		if err != nil {
			return nil, fmt.Errorf("%s", strings.ReplaceAll(err.Error(), work, "<probe-workspace>"))
		}
		return output, nil
	}
	for _, item := range []struct{ name, selection string }{
		{"node", service.options.Node}, {"ffmpeg", service.options.FFmpeg}, {"ffprobe", service.options.FFprobe},
	} {
		path, err := service.lookup(item.selection, item.name)
		if err != nil {
			problem(item.name, "missing", "No usable executable was selected.", false)
			continue
		}
		flag := "-version"
		if item.name == "node" {
			flag = "--version"
		}
		value, err := run([]string{path, flag})
		hash, hashErr := cliutil.SHA256File(path)
		if err != nil || hashErr != nil {
			problem(item.name, "unusable", fmt.Sprint(errors.Join(err, hashErr)), false)
			continue
		}
		observed, _, _ := strings.Cut(strings.TrimSpace(string(value)), "\n")
		if item.name == "node" && !regexp.MustCompile(`^v?\d+\.\d+\.\d+(?:[-+][\w.-]+)?$`).MatchString(observed) ||
			item.name != "node" && !strings.HasPrefix(observed, item.name+" version ") {
			problem(item.name, "unusable", "Executable returned an unrecognized version.", false)
			continue
		}
		selection := &recording.Executable{Path: path, Version: observed, SHA256: hash}
		switch item.name {
		case "node":
			result.Tools.Node = selection
		case "ffmpeg":
			result.Tools.FFmpeg = selection
		case "ffprobe":
			result.Tools.FFprobe = selection
		}
	}
	moduleRoot := service.options.ModuleRoot
	result.Tools.Tuistory = &recording.Tuistory{ModuleRoot: moduleRoot}
	packagePath := filepath.Join(moduleRoot, "tuistory/package.json")
	var packageJSON struct{ Name, Version string }
	if moduleRoot == "" {
		problem("tuistory", "unselected", "Select an installed node_modules root with --module-root; do not assume installation is missing.", false)
	} else if _, err := cliutil.ReadJSON(packagePath, 1<<20, &packageJSON); err != nil {
		if _, statErr := os.Stat(packagePath); os.IsNotExist(statErr) {
			problem("tuistory", "missing", "The selected node_modules root lacks Tuistory "+version+".", true)
		} else {
			problem("tuistory", "invalid-package", err.Error(), false)
		}
	} else {
		hash, err := cliutil.SHA256File(packagePath)
		if err != nil {
			return result, err
		}
		result.Tools.Tuistory.Version, result.Tools.Tuistory.PackageSHA256 = packageJSON.Version, hash
		switch {
		case packageJSON.Name != "tuistory" || packageJSON.Version != version:
			problem("tuistory", "incompatible", "Tuistory must be exactly "+version+".", true)
		case result.Tools.Node == nil:
			problem("tuistory", "unverified", "Node is required for the native probe.", false)
		default:
			entry, err := terminal.ResolveEntry(moduleRoot, version)
			if err == nil {
				adapter := terminal.New(result.Tools.Node.Path, entry, filepath.Join(service.root, "scripts/terminal.mjs"))
				probeEnv := maps.Clone(env)
				probeEnv["CLI_EXERCISE_PROBE"] = "approved"
				bounded, cancel := context.WithTimeout(ctx, time.Duration(service.options.ProbeTimeout*float64(time.Second)))
				err = service.nativeProbe(bounded, adapter, terminal.LaunchOptions{
					Executable: service.helper, Args: []string{"probe-fixture"}, Cwd: work, Env: probeEnv,
					Columns: 40, Rows: 10,
				})
				cancel()
			}
			if err != nil {
				problem("tuistory", "native-probe-failed", err.Error(), false)
			} else {
				result.Checks.NativePTY = true
			}
		}
	}
	if service.options.Font != "" {
		_, err := service.fontProbe(service.options.Font)
		if err != nil {
			result.Checks.FontAttempts = append(result.Checks.FontAttempts, map[string]any{"path": service.options.Font, "status": "unusable", "detail": err.Error()})
			problem("font", "unusable", "The selected rendering font did not pass Go loading and monospaced rasterization.", false)
		} else {
			result.Checks.FontAttempts = append(result.Checks.FontAttempts, map[string]any{"path": service.options.Font, "status": "usable"})
		}
	}
	if result.Tools.FFmpeg != nil && result.Tools.FFprobe != nil {
		raw := filepath.Join(work, "frames.rgb")
		if err := os.WriteFile(raw, make([]byte, 16*16*3*2), 0o600); err != nil {
			return result, err
		}
		overview := filepath.Join(work, "overview.png")
		var pixels bytes.Buffer
		if err := png.Encode(&pixels, image.NewRGBA(image.Rect(0, 0, 16, 16))); err != nil {
			return result, err
		}
		if err := os.WriteFile(overview, pixels.Bytes(), 0o600); err != nil {
			return result, err
		}
		for _, kind := range service.options.Formats {
			output := filepath.Join(work, "encoder."+kind)
			args := []string{result.Tools.FFmpeg.Path, "-v", "error", "-nostdin", "-f", "rawvideo",
				"-pixel_format", "rgb24", "-video_size", "16x16", "-framerate", "5", "-i", raw, "-frames:v", "2", "-an"}
			if kind == "mp4" {
				args = []string{result.Tools.FFmpeg.Path, "-v", "error", "-nostdin", "-loop", "1", "-framerate", "5",
					"-i", overview, "-frames:v", "2", "-an", "-filter_complex",
					"[0:v]drawbox=x=0:y=ih-4:w=iw:h=4:color=0x30363d:t=fill[base];" +
						"color=c=0x79c0ff:s=16x4:r=5:d=0.4[progress];" +
						"[base][progress]overlay=x='-w+w*(n-1)':y=H-h:eval=frame:shortest=1[video]",
					"-map", "[video]", "-c:v", "libx264", "-pix_fmt", "yuv420p"}
			} else {
				args = append(args, "-filter_complex", "[0:v]split[a][b];[a]palettegen=stats_mode=single[p];[b][p]paletteuse=new=1", "-c:v", "gif")
			}
			_, err := run(append(args, output))
			if err == nil {
				_, err = run([]string{result.Tools.FFmpeg.Path, "-v", "error", "-nostdin", "-i", output,
					"-vf", "select=eq(n\\,0)", "-fps_mode", "passthrough", "-pix_fmt", "rgb24", "-c:v", "rawvideo", "-f", "null", "-"})
			}
			var observed struct {
				Streams []struct {
					Codec  string `json:"codec_name"`
					Width  int    `json:"width"`
					Height int    `json:"height"`
					Frames string `json:"nb_read_frames"`
				} `json:"streams"`
			}
			if err == nil {
				data, runErr := run([]string{result.Tools.FFprobe.Path, "-v", "error", "-select_streams", "v:0",
					"-count_frames", "-show_entries", "stream=codec_name,width,height,nb_read_frames", "-of", "json", output})
				err = runErr
				if err == nil {
					err = json.Unmarshal(data, &observed)
				}
			}
			codec := "gif"
			if kind == "mp4" {
				codec = "h264"
			}
			if err == nil && (len(observed.Streams) != 1 || observed.Streams[0].Codec != codec ||
				observed.Streams[0].Width != 16 || observed.Streams[0].Height != 16 || observed.Streams[0].Frames != "2") {
				err = fmt.Errorf("native encoder/decoder returned unexpected frames")
			}
			result.Checks.Formats[kind] = err == nil
			if err != nil {
				problem(kind, "encoder-probe-failed", err.Error(), false)
			}
		}
	}
	result.Status = "ready"
	for _, issue := range result.Issues {
		if !issue.NeedsInstall {
			result.Status = "blocked"
			break
		}
		result.Status = "needs-install"
	}
	return result, nil
}
