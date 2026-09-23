package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/cli/cli/v2/cli-exercise/internal/recording"
	"github.com/stretchr/testify/require"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
)

func TestParseOptions(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		err  string
		html bool
	}{
		{name: "defaults", args: []string{"--run-dir", "run", "--preflight", "receipt"}},
		{name: "recording session", args: []string{"--session", "session.json", "--preflight", "receipt"}},
		{name: "recording session all inspection", args: []string{"--session", "session.json", "--preflight", "receipt", "--inspection", "all"}},
		{name: "recording session sampled inspection", args: []string{"--session", "session.json", "--preflight", "receipt", "--inspection", "sampled"}},
		{name: "HTML report", args: []string{"--run-dir", "run", "--preflight", "receipt", "--html"}, html: true},
		{name: "HTML explicitly disabled", args: []string{"--run-dir", "run", "--preflight", "receipt", "--html=false"}},
		{name: "recording session with HTML", args: []string{"--session", "session.json", "--preflight", "receipt", "--html"}, html: true},
		{name: "mutually exclusive recording inputs", args: []string{"--run-dir", "run", "--session", "session.json", "--preflight", "receipt"}, err: "choose only one"},
		{name: "session manual evidence is per run", args: []string{"--session", "session.json", "--preflight", "receipt", "--verification", "one.json"}, err: "individual manifest runs"},
		{name: "all and manual", args: []string{"--run-dir", "run", "--preflight", "receipt", "--inspection", "all", "--verification", "manual"}},
		{name: "companion MP4", args: []string{"--run-dir", "run", "--preflight", "receipt", "--format", "mp4"}},
		{name: "companion formats", args: []string{"--run-dir", "run", "--preflight", "receipt", "--format", "gif", "--format", "mp4"}},
		{name: "condensed companion", args: []string{"--run-dir", "run", "--preflight", "receipt", "--timing", "condensed", "--timing-authorization", "The user requested a condensed version."}},
		{name: "condensed needs no extra approval", args: []string{"--run-dir", "run", "--preflight", "receipt", "--timing", "condensed"}},
		{name: "explicit real-time companion", args: []string{"--run-dir", "run", "--preflight", "receipt", "--timing", "realtime"}},
		{name: "authorization without timing", args: []string{"--run-dir", "run", "--preflight", "receipt", "--timing-authorization", "unused"}, err: "applies only"},
		{name: "unsupported format", args: []string{"--run-dir", "run", "--preflight", "receipt", "--format", "webm"}, err: "must be gif or mp4"},
		{name: "unsupported timing", args: []string{"--run-dir", "run", "--preflight", "receipt", "--timing", "fast"}, err: "must be realtime or condensed"},
		{name: "missing run", args: []string{"--preflight", "receipt"}, err: "requires --run-dir"},
		{name: "missing preflight", args: []string{"--run-dir", "run"}, err: "requires --run-dir"},
		{name: "invalid inspection", args: []string{"--run-dir", "run", "--preflight", "receipt", "--inspection", "none"}, err: "sampled or all"},
		{name: "recording session invalid inspection", args: []string{"--session", "session.json", "--preflight", "receipt", "--inspection", "frames"}, err: "sampled or all"},
		{name: "recording session blank inspection", args: []string{"--session", "session.json", "--preflight", "receipt", "--inspection", ""}, err: "sampled or all"},
		{name: "extra argument", args: []string{"--run-dir", "run", "--preflight", "receipt", "extra"}, err: "no positional"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := parseOptions(tc.args)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.html, opts.html)
			if strings.HasPrefix(tc.name, "recording session") {
				require.Equal(t, "session.json", opts.sessionManifest)
				require.Empty(t, opts.runDir)
			} else {
				require.Equal(t, "run", opts.runDir)
			}
			if tc.name == "all and manual" || tc.name == "recording session all inspection" {
				require.Equal(t, "all", opts.inspection)
				if tc.name == "all and manual" {
					require.Equal(t, "manual", opts.verification)
				}
			} else {
				require.Equal(t, "sampled", opts.inspection)
			}
			switch tc.name {
			case "defaults":
				require.Empty(t, opts.timing, "an omitted flag must not override recorded timing")
			case "companion MP4":
				require.Equal(t, []string{"mp4"}, opts.formats)
			case "companion formats":
				require.Equal(t, []string{"gif", "mp4"}, opts.formats)
			case "condensed companion":
				require.Equal(t, "condensed", opts.timing)
				require.NotEmpty(t, opts.timingAuthorization)
			case "condensed needs no extra approval":
				require.Equal(t, "condensed", opts.timing)
				require.Empty(t, opts.timingAuthorization)
			case "explicit real-time companion":
				require.Equal(t, "realtime", opts.timing)
			}
		})
	}
	for _, tc := range []struct {
		name, input, timing, err string
	}{
		{name: "omitted contract timing", input: `{"formats":["mp4"]}`, timing: "condensed"},
		{name: "explicit full timing", input: `{"formats":["gif"],"timing":"realtime"}`, timing: "realtime"},
		{name: "explicit condensed", input: `{"formats":["mp4"],"timing":"condensed"}`, timing: "condensed"},
		{name: "blank contract timing", input: `{"formats":["mp4"],"timing":""}`, err: "output timing"},
		{name: "null contract timing", input: `{"formats":["mp4"],"timing":null}`, err: "output timing"},
		{name: "invalid contract timing type", input: `{"formats":["mp4"],"timing":42}`, err: "output timing"},
		{name: "unknown contract timing", input: `{"formats":["mp4"],"timing":"fast"}`, err: "output timing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output recording.OutputOptions
			err := json.Unmarshal([]byte(tc.input), &output)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.timing, output.Timing)
			require.Len(t, output.Formats, 1)
		})
	}
}

func testState(text string, timestamp float64) state {
	length := len(text)
	return state{Time: &timestamp, Data: recording.TerminalData{
		Rows: 2, Columns: 40, Lines: []recording.Line{{Spans: []recording.Span{{Text: text, Width: &length}}}},
	}}
}

func TestRun(t *testing.T) {
	t.Run("recording sessions", sessionCases)
	t.Run("case sequencing", sessionOrderCases)
	t.Run("recording presentation", presentationCases)
	t.Run("session media", sessionMediaCases)
	t.Run("session inspection", sessionInspectionCases)
	for _, tc := range []struct {
		name            string
		expected        string
		capture         string
		interrupted     bool
		incomplete      bool
		renderError     bool
		companion       bool
		recordedTiming  string
		companionTiming string
		timingNote      string
		wantStatus      string
		wantCode        int
		html            bool
	}{
		{name: "explicit real-time contract is preserved", expected: "hello", capture: "complete", recordedTiming: "realtime", wantStatus: "passed"},
		{name: "default condensed output", expected: "hello", capture: "complete", wantStatus: "passed"},
		{name: "requested HTML report", expected: "hello", capture: "complete", wantStatus: "passed", html: true},
		{name: "requested HTML retains rendering failure", expected: "hello", capture: "complete", renderError: true, wantStatus: "passed", wantCode: 1, html: true},
		{name: "explicit condensed needs no extra approval", expected: "hello", capture: "complete", recordedTiming: "condensed", wantStatus: "passed"},
		{name: "case failure with valid media", expected: "missing", capture: "complete", wantStatus: "failed", wantCode: 1},
		{name: "render failure preserves case outcome", expected: "hello", capture: "complete", renderError: true, wantStatus: "passed", wantCode: 1},
		{name: "capture failure blocks partial assertions", expected: "missing", capture: "failed", wantStatus: "blocked", wantCode: 1},
		{name: "interrupted is not a product failure", expected: "missing", capture: "complete", interrupted: true, wantStatus: "blocked", wantCode: 1},
		{name: "incomplete exact steps cannot pass", expected: "hello", capture: "complete", incomplete: true, wantStatus: "blocked", wantCode: 1},
		{name: "companion does not alter the recorded contract", expected: "hello", capture: "complete", recordedTiming: "realtime", companion: true, wantStatus: "passed"},
		{name: "companion cannot rewrite a failed case", expected: "missing", capture: "complete", companion: true, wantStatus: "failed", wantCode: 1},
		{name: "optional companion request note", expected: "hello", capture: "complete", companion: true, timingNote: "The user requested a condensed companion.", wantStatus: "passed"},
		{name: "real-time companion overrides default", expected: "hello", capture: "complete", companion: true, companionTiming: "realtime", wantStatus: "passed"},
		{name: "real-time companion overrides recorded condensed", expected: "hello", capture: "complete", recordedTiming: "condensed", companion: true, companionTiming: "realtime", wantStatus: "passed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(root, "capture"), 0o700))
			source := contract{
				CaseID: "fixture", Mode: "exact", Goal: "<script>not executable</script>",
				Output: recording.OutputOptions{Formats: []string{"mp4"}, Timing: tc.recordedTiming},
				Steps:  []json.RawMessage{}, Expectations: []expectation{
					{ID: "output", Type: "screen_contains", Value: json.RawMessage(jsonString(t, tc.expected))},
				},
			}
			if tc.incomplete {
				source.Steps = []json.RawMessage{json.RawMessage(`{"type":"key","key":"enter"}`)}
			}
			raw, err := json.Marshal(source)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(root, "contract.json"), raw, 0o600))
			hash := sha256.Sum256(raw)
			zero := 0
			outcome := result{
				ExitCode: &zero, CaptureStatus: tc.capture, StepsCompleted: &zero,
				Interrupted: tc.interrupted, ContractSHA256: hex.EncodeToString(hash[:]),
			}
			require.NoError(t, cliutil.WriteJSON(filepath.Join(root, "result.json"), outcome))
			encodedState, err := json.Marshal(testState("hello", 0))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(root, "capture/states.jsonl"), append(encodedState, '\n'), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(root, "capture/events.jsonl"), nil, 0o600))
			receiptPath := filepath.Join(root, "preflight.json")
			require.NoError(t, cliutil.WriteJSON(receiptPath, map[string]string{"status": "ready"}))
			var output bytes.Buffer
			opts := options{runDir: root, preflight: receiptPath, inspection: "sampled", html: tc.html}
			wantTiming := tc.recordedTiming
			if wantTiming == "" {
				wantTiming = "condensed"
			}
			if tc.companion {
				opts.formats = []string{"gif"}
				opts.timing = tc.companionTiming
				if opts.timing == "" {
					opts.timing = "condensed"
				}
				opts.timingAuthorization = tc.timingNote
				wantTiming = opts.timing
			}
			code, err := run(context.Background(), t.TempDir(), opts,
				cliutil.Streams{Out: &output, ErrOut: io.Discard}, func(_ context.Context, received contract, _ result, _ []state, _ []event, _ recording.Receipt, _ string, _ string) (rendering, error) {
					require.Equal(t, wantTiming, received.Output.Timing)
					if tc.companion {
						require.Equal(t, []string{"gif"}, received.Output.Formats)
						require.Equal(t, opts.timingAuthorization, received.Output.TimingAuthorization)
					}
					if tc.renderError {
						return rendering{}, errors.New("encoder failed")
					}
					return rendering{Status: "complete", Media: map[string]string{}, Timing: received.Output.Timing,
						Inspection: &inspectionInfo{VisualReview: "pending"}}, nil
				})
			require.NoError(t, err)
			require.Equal(t, tc.wantCode, code)
			var report report
			require.NoError(t, json.Unmarshal(output.Bytes(), &report))
			require.Equal(t, tc.wantStatus, report.CaseStatus)
			require.Equal(t, tc.renderError, report.Rendering.Status == "failed")
			unchanged, err := os.ReadFile(filepath.Join(root, "contract.json"))
			require.NoError(t, err)
			require.Equal(t, raw, unchanged)
			require.Equal(t, tc.recordedTiming, source.Output.Timing)
			if tc.companion {
				require.NotNil(t, report.Presentation)
				require.Equal(t, wantTiming, report.Presentation.Timing)
				require.Equal(t, opts.timingAuthorization, report.Presentation.TimingAuthorization)
			} else {
				require.Nil(t, report.Presentation)
			}
			if !tc.renderError {
				require.Equal(t, wantTiming, report.Rendering.Timing)
			}
			reports, err := filepath.Glob(filepath.Join(root, "artifacts", "render-*", "report.json"))
			require.NoError(t, err)
			require.Len(t, reports, 1, "machine-readable evidence is retained for either deliverable")
			if tc.html {
				page, err := os.ReadFile(report.Report)
				require.NoError(t, err)
				require.Contains(t, string(page), "&lt;script&gt;")
				require.NotContains(t, string(page), "<script>")
				require.Len(t, htmlFiles(t, root), 1)
			} else {
				require.Empty(t, report.Report)
				require.Empty(t, htmlFiles(t, root), "media-only output must skip HTML creation")
			}
		})
	}
	for _, tc := range []struct {
		name     string
		expected []expectation
		manual   *verification
		exit     int
		want     string
		err      string
	}{
		{name: "no assertions is observed", want: "observed"},
		{name: "expected nonzero exit", exit: 2, expected: []expectation{{ID: "exit", Type: "exit_code", Value: json.RawMessage("2")}}, want: "passed"},
		{name: "forbidden output", expected: []expectation{{ID: "text", Type: "screen_not_contains", Value: json.RawMessage(`"hello"`)}}, want: "failed"},
		{name: "unsupported expectation", expected: []expectation{{ID: "unsupported", Type: "stdout_equals"}}, want: "blocked"},
		{name: "manual needs evidence", expected: []expectation{{ID: "manual", Type: "manual"}}, want: "blocked"},
		{name: "manual cannot override automatic check", expected: []expectation{{ID: "exit", Type: "exit_code", Value: json.RawMessage("0")}},
			manual: manualResult("exit", "readback.json"), err: "unknown manual"},
		{name: "manual readback", expected: []expectation{{ID: "manual", Type: "manual"}},
			manual: manualResult("manual", "readback.json"), want: "passed"},
		{name: "manual path escape", expected: []expectation{{ID: "manual", Type: "manual"}},
			manual: manualResult("manual", "../outside.json"), err: "verification evidence"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "readback.json"), []byte(`{"observed":true}`), 0o600))
			completed := 0
			outcome, checks, err := evaluate(contract{Mode: "exact", Expectations: tc.expected}, result{
				CaptureStatus: "complete", StepsCompleted: &completed, ExitCode: &tc.exit,
			}, []state{testState("hello", 0)}, tc.manual, root, "hash")
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, outcome)
			if tc.name == "unsupported expectation" {
				require.Contains(t, checks[0].Reason, "Unsupported expectation")
			}
		})
	}
	for _, tc := range []struct {
		name     string
		states   []state
		events   []event
		duration float64
		timing   string
		frames   int
		err      string
	}{
		{name: "real time", states: []state{testState("one", 0), testState("two", .5)}, duration: 1, timing: "realtime", frames: 30},
		{name: "authorized condensation", states: []state{testState("one", 0), testState("two", 5)}, duration: 6, timing: "condensed", frames: 90},
		{name: "last frame between ticks", states: []state{testState("one", 0), testState("two", .019)}, duration: .02, timing: "realtime", frames: 2},
		{name: "condensed final boundary", states: []state{testState("one", 0), testState("two", 1)}, duration: 1, timing: "condensed", frames: 31},
		{name: "no states", duration: 1, timing: "realtime", err: "no recorded"},
		{name: "out of order", states: []state{testState("one", .5), testState("two", .1)}, duration: 1, timing: "realtime", err: "monotonic"},
		{name: "outside duration", states: []state{testState("one", 0), testState("two", 2)}, duration: 1, timing: "realtime", err: "duration"},
		{name: "invalid note", states: []state{testState("one", 0)}, events: []event{{Type: "note"}}, duration: 1, timing: "realtime", err: "annotation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := framePlan(tc.states, tc.events, tc.duration, 30, tc.timing)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			count := 0
			for _, entry := range plan {
				count += entry.Repeats
			}
			require.Equal(t, tc.frames, count)
			require.Equal(t, len(tc.states)-1, plan[len(plan)-1].State)
		})
	}
	t.Run("isolated encoder environment", func(t *testing.T) {
		t.Setenv("GH_TOKEN", "synthetic-must-not-pass")
		t.Setenv("HTTP_PROXY", "http://not-used.invalid")
		root := t.TempDir()
		environment, err := rendererEnvironment(root, recording.Tools{
			FFmpeg: &recording.Executable{Path: "/tools/ffmpeg"}, FFprobe: &recording.Executable{Path: "/tools/ffprobe"},
		})
		require.NoError(t, err)
		require.NotContains(t, strings.Join(environment, "\n"), "synthetic-must-not-pass")
		require.NotContains(t, strings.Join(environment, "\n"), "not-used.invalid")
		require.Contains(t, environment, "HOME="+filepath.Join(root, ".home"))
		for _, kind := range []string{"gif", "mp4"} {
			args, err := encodingArguments("/tools/ffmpeg", "terminal."+kind, 600, 274, 30, kind)
			require.NoError(t, err)
			if kind == "gif" {
				require.NotContains(t, args, "libx264")
				require.Contains(t, strings.Join(args, " "), "stats_mode=single")
			} else {
				require.Contains(t, args, "libx264")
			}
		}
	})
	t.Run("actual Go terminal and annotation raster", func(t *testing.T) {
		root := t.TempDir()
		font := filepath.Join(root, "Mono-Regular.ttf")
		require.NoError(t, os.WriteFile(font, gomono.TTF, 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(root, "Mono-Bold.ttf"), gomonobold.TTF, 0o600))
		config := terminalConfig{FontPath: font, FontSize: 18, FPS: 30, Background: "#0d1117", Foreground: "#e6edf3"}
		raster, err := newRasterizer(config)
		require.NoError(t, err)
		defer func() { require.NoError(t, raster.fonts.Close()) }()
		data := testState("Mona", 0).Data
		data.Lines[0].Spans[0].Flags = 1 | 4
		visible := true
		data.CursorVisible, data.Cursor, data.CursorStyle = &visible, []int{4, 0}, "block"
		top, err := raster.draw(data)
		require.NoError(t, err)
		require.Equal(t, image.Rect(0, 0, 40*raster.fonts.CellWidth, 2*raster.fonts.CellHeight), top.Bounds())
		source := contract{CaseID: "fixture", Goal: "Inspect the actual source cells.", Terminal: config}
		source.Output.Timing = "realtime"
		canvas, err := raster.compose(top, source, frame{}, top.Bounds().Dx(), 6*raster.fonts.CellHeight, 2*raster.fonts.CellHeight)
		require.NoError(t, err)
		require.Equal(t, 6*raster.fonts.CellHeight+2*presentationMargin, canvas.Bounds().Dy())
		store, err := newImageStore(t.TempDir())
		require.NoError(t, err)
		original, err := store.save(top)
		require.NoError(t, err)
		repeated, err := raster.draw(data)
		require.NoError(t, err)
		repeatedPath, err := store.save(repeated)
		require.NoError(t, err)
		require.Equal(t, original, repeatedPath, "identical pixels are saved once")
		data.Lines[0].Spans[0].FG = "#ff0000"
		colored, err := raster.draw(data)
		require.NoError(t, err)
		colorPath, err := store.save(colored)
		require.NoError(t, err)
		require.NotEqual(t, original, colorPath, "matching text with different colors must stay distinct")
		data.Lines[0].Spans[0].FG = ""
		data.Cursor = []int{3, 0}
		cursor, err := raster.draw(data)
		require.NoError(t, err)
		cursorPath, err := store.save(cursor)
		require.NoError(t, err)
		require.NotEqual(t, original, cursorPath, "cursor changes must stay distinct")
		captionPath, err := store.save(canvas)
		require.NoError(t, err)
		source.Goal = "A different caption."
		changedCaption, err := raster.compose(top, source, frame{}, top.Bounds().Dx(), 6*raster.fonts.CellHeight, 2*raster.fonts.CellHeight)
		require.NoError(t, err)
		changedPath, err := store.save(changedCaption)
		require.NoError(t, err)
		require.NotEqual(t, captionPath, changedPath, "caption changes must stay distinct")
		geometryA := image.NewRGBA(image.Rect(0, 0, 2, 1))
		geometryB := image.NewRGBA(image.Rect(0, 0, 1, 2))
		require.NotEqual(t, pixelHash(geometryA), pixelHash(geometryB), "geometry is part of visual identity")
	})
	t.Run("explicit existing capture and media tools", func(t *testing.T) {
		root := os.Getenv("CLI_EXERCISE_EVIDENCE_RUN")
		preflight := os.Getenv("CLI_EXERCISE_EVIDENCE_PREFLIGHT")
		if root == "" || preflight == "" {
			t.Skip("Supply a test-owned capture and an explicitly selected ready receipt for native rendering.")
		}
		skillRoot, err := filepath.Abs("../../..")
		require.NoError(t, err)
		var output bytes.Buffer
		code, err := run(context.Background(), skillRoot, options{
			runDir: root, preflight: preflight, inspection: "all",
		}, cliutil.Streams{Out: &output, ErrOut: io.Discard}, render)
		require.NoError(t, err)
		require.Zero(t, code, output.String())
		var report report
		require.NoError(t, json.Unmarshal(output.Bytes(), &report))
		require.Equal(t, "passed", report.CaseStatus)
		require.Equal(t, "complete", report.Rendering.Status)
		require.NotNil(t, report.Rendering.Font)
		require.NotEmpty(t, report.Rendering.Media)
		require.NotEmpty(t, report.Rendering.Inspection.EncodedSamples)
		require.Empty(t, report.Report)
		t.Logf("Native Go media: %v", report.Rendering.Media)
	})
}

func htmlFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && filepath.Ext(path) == ".html" {
			files = append(files, path)
		}
		return nil
	})
	require.NoError(t, err)
	return files
}

func jsonString(t *testing.T, value string) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}

func manualResult(id, path string) *verification {
	return &verification{
		ContractSHA256: "hash",
		Results: []verificationResult{{
			ID: id, Status: "passed", Reason: "Independent fixture readback.", Evidence: []string{path},
		}},
	}
}
