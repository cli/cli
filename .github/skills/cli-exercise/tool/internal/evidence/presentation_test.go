package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/cli/cli/v2/cli-exercise/internal/fontutil"
	"github.com/cli/cli/v2/cli-exercise/internal/recording"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/image/font/gofont/gomono"
)

func presentationRaster(t *testing.T) (*rasterizer, terminalConfig) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Mono.ttf")
	require.NoError(t, os.WriteFile(path, gomono.TTF, 0o600))
	config := terminalConfig{FontPath: path, FontSize: 18, FPS: 10, Background: "#0d1117", Foreground: "#e6edf3"}
	raster, err := newRasterizer(config, recording.Receipt{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, raster.fonts.Close()) })
	return raster, config
}

func presentationCases(t *testing.T) {
	t.Run("terminal presentation keeps content inside a framed safe area", func(t *testing.T) {
		raster, config := presentationRaster(t)

		// Given a terminal frame whose first cell starts at the source image edge.
		top, err := raster.draw(testState("edge content", 0).Data)
		require.NoError(t, err)
		source := contract{CaseID: "case", Goal: "Keep playback controls clear of terminal content", Terminal: config}
		contentHeight := top.Bounds().Dy() + 4*raster.fonts.CellHeight

		// When the terminal presentation is composed.
		canvas, err := raster.compose(top, source, frame{}, top.Bounds().Dx(), contentHeight, top.Bounds().Dy())
		require.NoError(t, err)

		// Then the frame provides a 48px safe area and a visible panel boundary.
		assert.Equal(t, image.Rect(0, 0, top.Bounds().Dx()+96, contentHeight+96), canvas.Bounds())
		assert.Equal(t, color.RGBA{0x01, 0x04, 0x09, 0xff}, canvas.RGBAAt(0, 0))
		assert.Equal(t, color.RGBA{0x30, 0x36, 0x3d, 0xff}, canvas.RGBAAt(47, 47))
		assert.Equal(t, raster.background, canvas.RGBAAt(48, 48))
	})
	t.Run("phase colors and readable result labels", func(t *testing.T) {
		raster, config := presentationRaster(t)
		for _, tc := range []struct {
			phase, status, label string
			ink                  color.RGBA
		}{
			{"setup", "passed", "Preparation", preparationInk},
			{"exercise", "failed", "Running", runningInk},
			{"validation", "passed", "Validation - Passed", passedInk},
			{"validation", "failed", "Validation - Failed", failedInk},
			{"validation", "blocked", "Validation - Blocked", blockedInk},
			{"validation", "observed", "Validation - Observed", mutedInk},
			{"validation", "", "Validation - Unverified", mutedInk},
			{"cleanup", "", "Cleanup", mutedInk},
		} {
			t.Run(tc.phase+"/"+tc.status, func(t *testing.T) {
				label, ink := phaseAppearance(tc.phase, tc.status)
				require.Equal(t, tc.label, label)
				require.Equal(t, tc.ink, ink)
				state := testState("real command output", 0)
				state.Data.Columns = 90
				top, err := raster.draw(state.Data)
				require.NoError(t, err)
				source := contract{CaseID: "case", Goal: "Check the requested behavior", Terminal: config}
				source.Output.Timing = "realtime"
				source.Chapter = &chapterPresentation{Label: "One behavior", Phase: tc.phase, Status: tc.status,
					Title: "Check the requested behavior", Command: "gh config get prompt"}
				notes := chapterEvents(*source.Chapter, nil)
				offset := raster.fonts.CellHeight
				height := top.Bounds().Dy() + offset
				canvas, err := raster.compose(top, source, frame{Note: &notes[0]}, top.Bounds().Dx(),
					height+4*raster.fonts.CellHeight, height)
				require.NoError(t, err)
				require.Equal(t, tc.ink, canvas.RGBAAt(presentationMargin+5, presentationMargin+height+1),
					"the phase accent must be in the actual raster")
				require.Contains(t, *notes[0].Chapter, tc.label)
				require.Equal(t, "Check the requested behavior", *notes[0].Text)
			})
		}
	})
	t.Run("command header preserves invocation and captured cells", func(t *testing.T) {
		raster, config := presentationRaster(t)
		state := testState("unchanged output", 0)
		top, err := raster.draw(state.Data)
		require.NoError(t, err)
		source := contract{CaseID: "case", Goal: "Inspect the output", Terminal: config}
		source.Output.Timing = "realtime"
		source.Command.Executable = "/selected/gh"
		source.Command.Args = []string{"api", "$literal", "a'b", "", "two  spaces"}
		want := `$ gh api '$literal' 'a'"'"'b' '' 'two  spaces'`
		lines, err := raster.commandLines(source, top.Bounds().Dx())
		require.NoError(t, err)
		require.Equal(t, want, strings.Join(lines, ""), "soft wrapping must not alter arguments or whitespace")
		before := pixelHash(top)
		offset := len(lines) * raster.fonts.CellHeight
		height := top.Bounds().Dy() + offset
		canvas, err := raster.compose(top, source, frame{}, top.Bounds().Dx(), height+4*raster.fonts.CellHeight, height)
		require.NoError(t, err)
		captured := image.NewRGBA(top.Bounds())
		draw.Draw(captured, captured.Bounds(), canvas, image.Pt(presentationMargin, presentationMargin+offset), draw.Src)
		require.Equal(t, before, pixelHash(captured), "the terminal must retain every original output cell below the invocation")
		require.Equal(t, before, pixelHash(top), "raw source images remain unchanged")
		off := false
		source.Output.Captions = &off
		lines, err = raster.commandLines(source, top.Bounds().Dx())
		require.NoError(t, err)
		require.Empty(t, lines)
		plain, err := raster.compose(top, source, frame{}, top.Bounds().Dx(), top.Bounds().Dy(), top.Bounds().Dy())
		require.NoError(t, err)
		captured = image.NewRGBA(top.Bounds())
		draw.Draw(captured, captured.Bounds(), plain, image.Pt(presentationMargin, presentationMargin), draw.Src)
		require.Equal(t, before, pixelHash(captured), "an explicit unannotated capture must not gain an artificial prompt")
	})
	t.Run("timing adjustments do not add a footer", func(t *testing.T) {
		raster, config := presentationRaster(t)
		top, err := raster.draw(testState("observed output", 0).Data)
		require.NoError(t, err)
		source := contract{CaseID: "case", Goal: "Check the observed value", Terminal: config}
		height := top.Bounds().Dy() + 4*raster.fonts.CellHeight
		var previous [32]byte
		for _, timing := range []string{"realtime", "condensed"} {
			source.Output.Timing = timing
			canvas, err := raster.compose(top, source, frame{}, top.Bounds().Dx(), height, top.Bounds().Dy())
			require.NoError(t, err)
			for y := presentationMargin + height - raster.fonts.CellHeight; y < presentationMargin+height; y++ {
				for x := presentationMargin; x < presentationMargin+top.Bounds().Dx(); x++ {
					require.Equal(t, raster.background, canvas.RGBAAt(x, y), "bottom padding must not contain a repeated timing label")
				}
			}
			if timing == "condensed" {
				require.Equal(t, previous, pixelHash(canvas), "presentation timing belongs in metadata, not repeated captions")
			}
			previous = pixelHash(canvas)
		}
	})
	t.Run("fidelity derives from captured exercise modes", func(t *testing.T) {
		for _, tc := range []struct {
			modes       []string
			mode, label string
		}{
			{[]string{"exact", "exact"}, "exact", "Exact run"},
			{[]string{"explore", "explore"}, "explore", "Exploration"},
			{[]string{"exact", "explore"}, "mixed", "Mixed run"},
			{[]string{"mixed", "exact"}, "mixed", "Mixed run"},
			{[]string{"exact", ""}, "unknown", "Recorded run"},
			{nil, "unknown", "Recorded run"},
		} {
			require.Equal(t, tc.mode, recordingMode(tc.modes))
			require.Equal(t, tc.label, fidelityLabel(tc.mode))
		}
	})
	t.Run("overview pagination keeps readable rows and all results", func(t *testing.T) {
		raster, config := presentationRaster(t)
		heading, err := fontutil.Open(config.FontPath, config.FontSize*1.4, nil)
		require.NoError(t, err)
		defer func() { require.NoError(t, heading.Close()) }()

		// Given an overview rendered into the same video geometry as terminal chapters.
		for _, count := range []int{1, 4, 31} {
			cases := make([]sessionCaseResult, count)
			for index := range cases {
				cases[index] = sessionCaseResult{ID: fmt.Sprint(index), Title: "Setting changes are read back",
					Status: []string{"passed", "failed", "blocked", "observed"}[index%4]}
			}
			layout, err := planOverview(raster.fonts, heading, "Configuration checks", cases, 1000, 440)
			require.NoError(t, err)
			shown := []sessionCaseResult{}
			for page, rows := range layout.pages {
				// When an overview page is drawn.
				canvas, err := drawOverview(raster, heading, layout, page, 1000, 440, "exact")
				require.NoError(t, err)

				// Then its content uses the same framed 48px safe area.
				require.Equal(t, image.Rect(0, 0, 1000, 440), canvas.Bounds())
				require.Equal(t, presentationInk, canvas.RGBAAt(0, 0))
				require.Equal(t, panelBorderInk, canvas.RGBAAt(47, 47))
				require.Equal(t, raster.background, canvas.RGBAAt(48, 48))
				for _, row := range rows {
					shown = append(shown, row.result)
				}
			}
			require.Equal(t, cases, shown)
			require.Equal(t, 10, overviewPageSeconds)
			if count == 1 {
				require.Len(t, layout.pages, 1)
			}
			if count == 31 {
				require.Greater(t, len(layout.pages), 1)
			}
		}
		_, err = planOverview(raster.fonts, heading, "Too small", []sessionCaseResult{{ID: "x", Title: "Behavior"}}, 100, 100)
		require.Error(t, err)
		_, err = planOverview(raster.fonts, heading, "Brief title", []sessionCaseResult{{ID: "x", Title: strings.Repeat("word ", 100)}}, 1000, 440)
		require.ErrorContains(t, err, "brief natural-language")
	})
	t.Run("native overview and complete case sections", presentationNativeCase)
}

func presentationNativeCase(t *testing.T) {
	receiptPath := os.Getenv("CLI_EXERCISE_SESSION_MEDIA_RECEIPT")
	if receiptPath == "" {
		t.Skip("Select an existing ready receipt for native presentation checks.")
	}
	var receipt recording.Receipt
	_, err := cliutil.ReadJSON(receiptPath, 4<<20, &receipt)
	require.NoError(t, err)
	require.NotNil(t, receipt.Tools.Font)
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	root := t.TempDir()
	fallback := filepath.Join(root, "fallback.ttf")
	require.NoError(t, os.WriteFile(fallback, gomono.TTF, 0o600))
	manifest := sessionManifest{SchemaVersion: 1, ID: "presentation", Title: "Focused CLI checks",
		Source: "Record independent synthetic cases with immediate validation.", Workspace: root}
	manifest.Output.Formats = []string{"mp4"}
	expectedTimeline := []string{"overview"}
	originals := map[string]string{}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for index, mode := range []string{"exact", "explore"} {
		item := sessionCase{ID: fmt.Sprintf("case-%d", index), Title: []string{"Setting changes round-trip", "An intentional failing check"}[index]}
		for _, phase := range []string{"setup", "exercise", "validation"} {
			directory := filepath.Join(root, item.ID, phase)
			require.NoError(t, os.MkdirAll(filepath.Join(directory, "capture"), 0o700))
			recorded := contract{CaseID: item.ID + "-" + phase, Mode: mode, Goal: "Inspect only the synthetic fixture",
				Terminal: terminalConfig{Columns: 90, Rows: 10, FontPath: filepath.Join(root, "unavailable-original.ttf"),
					FontFallbacks: []string{filepath.Join(root, "unavailable-fallback.ttf")}, FontSize: 18,
					FPS: 10, Background: "#0d1117", Foreground: "#e6edf3"},
				Output:       recording.OutputOptions{Formats: []string{"mp4"}, Timing: "condensed"},
				Expectations: []expectation{{ID: "exit", Type: "exit_code", Value: json.RawMessage("0")}},
			}
			recorded.Command.Executable = filepath.Join(root, "fixture")
			recorded.Command.Args = []string{phase}
			if index == 1 && phase == "validation" {
				recorded.Expectations = append(recorded.Expectations,
					expectation{ID: "output", Type: "screen_contains", Value: json.RawMessage(`"missing expected value"`)})
			}
			raw, err := json.Marshal(recorded)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(directory, "contract.json"), raw, 0o600))
			hash := sha256.Sum256(raw)
			originals[directory] = hex.EncodeToString(hash[:])
			zero := 0
			require.NoError(t, cliutil.WriteJSON(filepath.Join(directory, "result.json"), result{
				ContractSHA256: originals[directory], CaptureStatus: "complete", StepsCompleted: &zero, ExitCode: &zero,
				DurationSeconds: .1, StartedAt: start.Format(time.RFC3339Nano), FinishedAt: start.Add(time.Second).Format(time.RFC3339Nano),
			}))
			start = start.Add(2 * time.Second)
			state := testState("actual captured output", 0)
			state.Data.Columns, state.Data.Rows = 90, 10
			data, err := json.Marshal(state)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(directory, "capture/states.jsonl"), append(data, '\n'), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(directory, "capture/events.jsonl"), nil, 0o600))
			item.Runs = append(item.Runs, sessionRun{ID: phase, Phase: phase, Title: map[string]string{
				"setup":      "Establish this case's starting value",
				"exercise":   "Run the requested behavior",
				"validation": "Compare the saved result with its expectation",
			}[phase], RunDirectory: directory})
			expectedTimeline = append(expectedTimeline, phase)
		}
		manifest.Cases = append(manifest.Cases, item)
	}
	path := filepath.Join(root, "session.json")
	require.NoError(t, cliutil.WriteJSON(path, manifest))
	var output bytes.Buffer
	// Given recorded chapters whose original font files are unavailable.
	opts, err := parseOptions([]string{"--session", path, "--preflight", receiptPath, "--inspection", "all",
		"--font", receipt.Tools.Font.Path, "--font-fallback", fallback})
	require.NoError(t, err)

	// When a session is rendered with replacement fonts.
	code, err := runSession(ctx, t.TempDir(), opts,
		cliutil.Streams{Out: &output}, render)
	require.NoError(t, err)

	// Then every chapter and the overview render without rewriting the recorded contracts.
	var report sessionReport
	require.NoError(t, json.Unmarshal(output.Bytes(), &report))
	require.Equal(t, 1, code, output.String())
	require.Equal(t, "failed", report.SessionStatus)
	require.Equal(t, "complete", report.Rendering.Status, report.Rendering.Error)
	fonts, err := fontutil.Open(receipt.Tools.Font.Path, 18, nil)
	require.NoError(t, err)
	expectedHeight := 15 * fonts.CellHeight
	expectedWidth := 90 * fonts.CellWidth
	require.NoError(t, fonts.Close())
	require.Equal(t, expectedHeight+expectedHeight%2+96, report.Rendering.Height, "the framed terminal keeps a 48px vertical safe area")
	require.Equal(t, expectedWidth+expectedWidth%2+96, report.Rendering.Width, "the framed terminal keeps a 48px horizontal safe area")
	require.True(t, report.OrderVerified)
	require.Equal(t, "mixed", report.Mode)
	require.NotNil(t, report.Overview)
	require.Equal(t, 10.0, report.Overview.DurationSeconds)
	require.Equal(t, 1, report.Overview.Pages)
	require.Equal(t, 0.0, report.Chapters[0].StartSeconds)
	require.Equal(t, 10.0, report.Chapters[0].EndSeconds)
	require.Equal(t, 10.0, report.Chapters[1].StartSeconds)
	require.Equal(t, "passed", report.Cases[0].Status)
	require.Equal(t, "failed", report.Cases[1].Status)
	actualTimeline := []string{}
	for index, chapter := range report.Chapters {
		kind := chapter.Phase
		if chapter.Kind == "overview" {
			kind = "overview"
		} else {
			require.NotNil(t, chapter.Evidence.Presentation)
			assert.Equal(t, receipt.Tools.Font.Path, chapter.Evidence.Presentation.FontPath)
			assert.Equal(t, []string{fallback}, chapter.Evidence.Presentation.FontFallbacks)
			require.Len(t, chapter.Evidence.Rendering.FontFallbacks, 1)
			sum := sha256.Sum256(gomono.TTF)
			assert.Equal(t, fallback, chapter.Evidence.Rendering.FontFallbacks[0].Path)
			assert.Equal(t, hex.EncodeToString(sum[:]), chapter.Evidence.Rendering.FontFallbacks[0].SHA256)
			require.Equal(t, "condensed", chapter.Evidence.Rendering.Timing)
			require.Positive(t, chapter.Evidence.Rendering.PresentationHoldSeconds, "reading holds remain recorded in metadata")
		}
		actualTimeline = append(actualTimeline, kind)
		if index > 0 {
			require.Equal(t, report.Chapters[index-1].EndSeconds, chapter.StartSeconds)
		}
	}
	require.Equal(t, expectedTimeline, actualTimeline)
	require.Empty(t, htmlFiles(t, root), "video remains the default deliverable")
	for directory, before := range originals {
		after, err := cliutil.SHA256File(filepath.Join(directory, "contract.json"))
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
	t.Run("every overview page lasts ten seconds", func(t *testing.T) {
		cases := make([]sessionCaseResult, 8)
		for index := range cases {
			cases[index] = sessionCaseResult{ID: fmt.Sprint(index), Title: "A brief independent behavior", Status: "passed", Mode: "exact"}
		}
		directory := t.TempDir()
		config := terminalConfig{FontPath: receipt.Tools.Font.Path, FontSize: 18, FPS: 10, Background: "#0d1117", Foreground: "#e6edf3"}
		pages, overview, err := renderOverview(ctx, directory, receipt, config, "Exact CLI checks", "exact", cases,
			report.Rendering.Width, report.Rendering.Height, report.Rendering.FPS)
		require.NoError(t, err)
		require.Greater(t, overview.Pages, 1)
		require.Equal(t, float64(10*overview.Pages), overview.DurationSeconds)
		environment, err := rendererEnvironment(directory, receipt.Tools)
		require.NoError(t, err)
		for _, page := range pages {
			actual, err := inspectSessionMediaClip(ctx, page.Evidence.Rendering.Media["mp4"], receipt.Tools, environment,
				report.Rendering.Width, report.Rendering.Height, 10*report.Rendering.FPS, report.Rendering.FPS)
			require.NoError(t, err)
			require.InDelta(t, 10.0, actual.DurationSeconds, 0.001)
		}
		timeline, ordered, err := planSessionMedia(append(pages, report.Chapters[1:]...))
		require.NoError(t, err)
		require.Equal(t, overview.DurationSeconds, ordered[len(pages)].StartSeconds)
		require.Equal(t, overview.DurationSeconds+12, timeline.DurationSeconds)
	})
	t.Run("unannotated realtime preserves the raw-only timeline", func(t *testing.T) {
		plain := manifest
		off := false
		plain.Output.Captions, plain.Output.Timing = &off, "realtime"
		path := filepath.Join(root, "plain.json")
		require.NoError(t, cliutil.WriteJSON(path, plain))
		var output bytes.Buffer
		plainOptions := opts
		plainOptions.sessionManifest, plainOptions.inspection = path, "sampled"
		code, err := runSession(ctx, t.TempDir(), plainOptions,
			cliutil.Streams{Out: &output}, render)
		require.NoError(t, err)
		require.Equal(t, 1, code)
		var plainReport sessionReport
		require.NoError(t, json.Unmarshal(output.Bytes(), &plainReport))
		require.Equal(t, "complete", plainReport.Rendering.Status, plainReport.Rendering.Error)
		require.Nil(t, plainReport.Overview)
		require.Len(t, plainReport.Chapters, 6)
		require.Zero(t, plainReport.Chapters[0].StartSeconds)
		require.Less(t, plainReport.Rendering.Height, report.Rendering.Height)
		var inspection struct {
			Progress bool `json:"continuousProgress"`
		}
		_, err = cliutil.ReadJSON(plainReport.Rendering.Inspection.Manifest, 4<<20, &inspection)
		require.NoError(t, err)
		require.False(t, inspection.Progress)
	})
	t.Run("reordered captures do not become a successful edit", func(t *testing.T) {
		reordered := manifest
		reordered.Cases = []sessionCase{manifest.Cases[1], manifest.Cases[0]}
		path := filepath.Join(root, "reordered.json")
		require.NoError(t, cliutil.WriteJSON(path, reordered))
		var output bytes.Buffer
		code, err := runSession(ctx, t.TempDir(), options{sessionManifest: path, preflight: receiptPath},
			cliutil.Streams{Out: &output}, func(context.Context, contract, result, []state, []event, recording.Receipt, string, string) (rendering, error) {
				return rendering{Status: "complete"}, nil
			})
		require.NoError(t, err)
		require.Equal(t, 1, code)
		var blocked sessionReport
		require.NoError(t, json.Unmarshal(output.Bytes(), &blocked))
		require.Equal(t, "blocked", blocked.SessionStatus)
		require.Equal(t, "failed", blocked.Rendering.Status)
		require.False(t, blocked.OrderVerified)
		require.Nil(t, blocked.Overview)
		require.Empty(t, blocked.Rendering.Media)
	})
}
