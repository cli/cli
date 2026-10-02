package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/cli/cli/v2/cli-exercise/internal/recording"
	"github.com/stretchr/testify/require"
)

func sessionInspectionNativeCase(t *testing.T) {
	preflight := os.Getenv("CLI_EXERCISE_SESSION_MEDIA_RECEIPT")
	if preflight == "" {
		t.Skip("Select an existing ready receipt for test-owned native final inspection.")
	}
	var receipt recording.Receipt
	_, err := cliutil.ReadJSON(preflight, 4<<20, &receipt)
	require.NoError(t, err)
	fontPath := os.Getenv("CLI_EXERCISE_RENDER_FONT")
	if fontPath == "" {
		t.Skip("Select an explicit font for native rendering.")
	}
	require.True(t, receipt.Checks.Formats["mp4"] && receipt.Checks.Formats["gif"])
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()
	root, skillRoot := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(skillRoot, ".git"), []byte("test repository boundary"), 0o600))
	t.Chdir(skillRoot)
	manifest := sessionManifest{SchemaVersion: 1, ID: "inspection", Title: "Synthetic CLI checks",
		Source: "Inspect captured states without running a target.", Workspace: root}
	manifest.Output.Formats = []string{"mp4", "gif"}
	originals := map[string]string{}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for index := range 8 {
		directory := filepath.Join(root, fmt.Sprintf("case-%d", index))
		require.NoError(t, os.MkdirAll(filepath.Join(directory, "capture"), 0o700))
		source := contract{CaseID: fmt.Sprintf("case-%d", index), Mode: "exact", Goal: "Inspect synthetic output",
			Terminal: terminalConfig{Columns: 80, Rows: 12, FontPath: fontPath, FontSize: 14,
				FPS: 10, Background: "#0d1117", Foreground: "#e6edf3"},
			Output:       recording.OutputOptions{Formats: []string{"mp4"}, Timing: "condensed"},
			Expectations: []expectation{{ID: "exit", Type: "exit_code", Value: json.RawMessage("0")}},
		}
		source.Command.Executable = filepath.Join(root, "never-executed")
		source.Command.Args = []string{strconv.Itoa(index)}
		require.NoError(t, cliutil.WriteJSON(filepath.Join(directory, "contract.json"), source))
		hash, err := cliutil.SHA256File(filepath.Join(directory, "contract.json"))
		require.NoError(t, err)
		zero := 0
		require.NoError(t, cliutil.WriteJSON(filepath.Join(directory, "result.json"), result{
			ContractSHA256: hash, CaptureStatus: "complete", StepsCompleted: &zero, ExitCode: &zero, DurationSeconds: .4,
			StartedAt: start.Format(time.RFC3339Nano), FinishedAt: start.Add(time.Second).Format(time.RFC3339Nano),
		}))
		start = start.Add(2 * time.Second)
		var states bytes.Buffer
		for revision, text := range []string{"first value", "first value", "another value", "first value"} {
			state := testState(text, float64(revision)/10)
			state.Revision, state.Data.Columns, state.Data.Rows = revision, 80, 12
			if index == 0 && revision == 2 {
				state.Data.Columns, state.Data.Rows = 90, 15
			}
			require.NoError(t, json.NewEncoder(&states).Encode(state))
		}
		require.NoError(t, os.WriteFile(filepath.Join(directory, "capture/states.jsonl"), states.Bytes(), 0o600))
		noteTime, note := .15, "Check the next value"
		noteJSON, err := json.Marshal(event{Type: "note", Time: &noteTime, Text: &note})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(directory, "capture/events.jsonl"), append(noteJSON, '\n'), 0o600))
		for _, relative := range []string{"contract.json", "result.json", "capture/states.jsonl", "capture/events.jsonl"} {
			path := filepath.Join(directory, relative)
			originals[path], err = cliutil.SHA256File(path)
			require.NoError(t, err)
		}
		manifest.Cases = append(manifest.Cases, sessionCase{ID: source.CaseID, Title: fmt.Sprintf("Behavior %d remains explicit", index+1),
			Runs: []sessionRun{{ID: "exercise", Phase: "exercise", Title: "Read the captured values", RunDirectory: directory}}})
	}
	path := filepath.Join(root, "session.json")
	require.NoError(t, cliutil.WriteJSON(path, manifest))
	reports := map[string]sessionReport{}
	for _, mode := range []string{"sampled", "all"} {
		opts, err := parseOptions([]string{"--session", path, "--preflight", preflight, "--inspection", mode,
			"--font", fontPath})
		require.NoError(t, err)
		var output bytes.Buffer
		code, err := runSession(ctx, skillRoot, opts, cliutil.Streams{Out: &output}, render)
		require.NoError(t, err)
		var report sessionReport
		require.NoError(t, json.Unmarshal(output.Bytes(), &report))
		require.Zero(t, code, output.String())
		require.Equal(t, "complete", report.Rendering.Status)
		require.Equal(t, mode, report.Rendering.Inspection.Mode)
		require.Equal(t, "pending", report.Rendering.Inspection.VisualReview)
		require.Greater(t, report.Overview.Pages, 1)
		reports[mode] = report
	}
	sampled, all := reports["sampled"], reports["all"]
	require.Equal(t, sampled.Cases, all.Cases)
	require.Equal(t, sampled.Rendering.MediaDetails, all.Rendering.MediaDetails, "inspection must not change video bytes or timing")
	for path, before := range originals {
		after, err := cliutil.SHA256File(path)
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
	require.NoFileExists(t, filepath.Join(root, "never-executed"))
	require.Empty(t, htmlFiles(t, root))
	var inspection struct {
		Mode          string                    `json:"mode"`
		Frames        map[string]string         `json:"frames"`
		DecodedFrames map[string]map[int]string `json:"decodedFrames"`
	}
	_, err = cliutil.ReadJSON(all.Rendering.Inspection.Manifest, 16<<20, &inspection)
	require.NoError(t, err)
	require.Equal(t, "all", inspection.Mode)
	for _, kind := range manifest.Output.Formats {
		selected := inspection.DecodedFrames[kind]
		require.Greater(t, len(selected), 3)
		require.Less(t, len(selected), all.Rendering.Frames)
		offset, padded := 0, false
		for _, chapter := range all.Chapters {
			clip := chapter.Evidence.Rendering
			require.Contains(t, selected, offset)
			require.Contains(t, selected, offset+clip.Frames-1)
			if chapter.Kind == "overview" {
				require.Contains(t, selected, offset+clip.Frames/2)
			} else {
				for local := range 5 {
					require.Contains(t, selected, offset+local, "terminal and annotation changes must be represented")
				}
				hold := int(clip.PresentationHoldSeconds * float64(clip.FPS))
				for _, frame := range inspectionRangeSamples(clip.Frames-hold, clip.Frames-1) {
					require.Contains(t, selected, offset+frame)
				}
			}
			padded = padded || clip.Width < all.Rendering.Width || clip.Height < all.Rendering.Height
			offset += clip.Frames
		}
		require.True(t, padded)
		assertSessionInspectionSamples(t, ctx, receipt, all.Rendering, kind, selected)
	}
	t.Run("GIF-only repeated images share storage", func(t *testing.T) {
		off := false
		source := contract{CaseID: "repeated", Goal: "Repeated state",
			Terminal: terminalConfig{FontPath: fontPath, FontSize: 14, FPS: 30, Background: "#0d1117", Foreground: "#e6edf3"},
			Output:   recording.OutputOptions{Formats: []string{"mp4"}, Timing: "realtime", Captions: &off}}
		captured := testState("", 0)
		captured.Data.Columns = 4
		clip, err := render(ctx, source, result{DurationSeconds: .4}, []state{captured}, nil, receipt, t.TempDir(), "all")
		require.NoError(t, err)
		chapters := []sessionChapter{{CaseTitle: "Repeated state", Phase: "exercise", Title: "Inspect", Evidence: report{Rendering: clip}}}
		result, _, err := assembleSession(ctx, filepath.Join(t.TempDir(), "output"), receipt, chapters, []string{"gif"}, false, "all")
		require.NoError(t, err)
		require.Equal(t, 1, result.Inspection.UniqueImages)
		require.NotContains(t, result.Media, "mp4")
	})
}

func assertSessionInspectionSamples(t *testing.T, ctx context.Context, receipt recording.Receipt, rendered rendering, kind string, selected map[int]string) {
	t.Helper()
	frames := []int{}
	for frame := range selected {
		frames = append(frames, frame)
	}
	slices.Sort(frames)
	frames = []int{frames[0], frames[len(frames)/2], frames[len(frames)-1]}
	selectors := []string{}
	for _, frame := range frames {
		selectors = append(selectors, fmt.Sprintf(`eq(n\,%d)`, frame))
	}
	environment, err := rendererEnvironment(t.TempDir(), receipt.Tools)
	require.NoError(t, err)
	var pixels bytes.Buffer
	require.NoError(t, runCommand(ctx, []string{
		receipt.Tools.FFmpeg.Path, "-v", "error", "-nostdin", "-i", rendered.Media[kind],
		"-vf", "select=" + strings.Join(selectors, "+"), "-fps_mode", "passthrough", "-pix_fmt", "rgba", "-f", "rawvideo", "-",
	}, environment, filepath.Dir(rendered.Media[kind]), &pixels))
	size := rendered.Width * rendered.Height * 4
	require.Equal(t, len(frames)*size, pixels.Len())
	for index, frame := range frames {
		file, err := os.Open(filepath.Join(rendered.Inspection.Directory, selected[frame]))
		require.NoError(t, err)
		decoded, err := png.Decode(file)
		require.NoError(t, err)
		require.NoError(t, file.Close())
		expected := image.NewRGBA(decoded.Bounds())
		draw.Draw(expected, expected.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
		require.Equal(t, expected.Pix, pixels.Bytes()[index*size:(index+1)*size], "decoded %s frame %d", kind, frame)
	}
}
