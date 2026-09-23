package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/cli/cli/v2/cli-exercise/internal/recording"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/gofont/goregular"
)

func fontCapture(t *testing.T) (string, string, map[string][]byte) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "capture"), 0o700))
	source := contract{
		CaseID: "font-fixture", Mode: "exact", Goal: "Read the captured output",
		Terminal: terminalConfig{
			Columns: 40, Rows: 2, FontSize: 18, FPS: 10, Background: "#0d1117", Foreground: "#e6edf3",
		},
		Output:       recording.OutputOptions{Formats: []string{"mp4"}, Timing: "realtime"},
		Expectations: []expectation{{ID: "output", Type: "screen_contains", Value: json.RawMessage(`"hello"`)}},
	}
	source.Command.Executable = filepath.Join(root, "never-executed")
	raw, err := json.Marshal(source)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "fontPath")
	require.NotContains(t, string(raw), "fontFallbacks")
	require.NoError(t, os.WriteFile(filepath.Join(root, "contract.json"), raw, 0o400))
	sum := sha256.Sum256(raw)
	zero := 0
	require.NoError(t, cliutil.WriteJSON(filepath.Join(root, "result.json"), result{
		ContractSHA256: hex.EncodeToString(sum[:]), ExitCode: &zero, StepsCompleted: &zero,
		CaptureStatus: "complete", DurationSeconds: .1,
		StartedAt: "2026-01-01T00:00:00Z", FinishedAt: "2026-01-01T00:00:01Z",
	}))
	state, err := json.Marshal(testState("hello", 0))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "capture/states.jsonl"), append(state, '\n'), 0o400))
	require.NoError(t, os.WriteFile(filepath.Join(root, "capture/events.jsonl"), nil, 0o400))
	require.NoError(t, os.WriteFile(filepath.Join(root, "capture/raw.jsonl"), []byte("{\"t\":0,\"type\":\"output\",\"data\":\"hello\"}\n"), 0o400))
	receipt := filepath.Join(root, "preflight.json")
	require.NoError(t, cliutil.WriteJSON(receipt, recording.Receipt{Status: "ready",
		Checks: recording.CapabilityChecks{Formats: map[string]bool{"mp4": true}}}))
	originals := map[string][]byte{}
	for _, name := range []string{"contract.json", "result.json", "capture/states.jsonl", "capture/events.jsonl", "capture/raw.jsonl"} {
		path := filepath.Join(root, name)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		originals[path] = data
	}
	return root, receipt, originals
}

func TestRenderFontOverridesPreserveCapture(t *testing.T) {
	t.Parallel()

	// Given a capture whose original fonts are no longer available.
	root, receipt, originals := fontCapture(t)
	font := filepath.Join(root, "replacement.ttf")
	fallback := filepath.Join(root, "symbols.ttf")
	require.NoError(t, os.WriteFile(font, gomono.TTF, 0o600))
	for name, data := range map[string][]byte{
		"replacement-Bold.ttf": gomonobold.TTF, "replacement-Italic.ttf": gomonoitalic.TTF,
		"replacement-BoldItalic.ttf": gomonobolditalic.TTF,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), data, 0o600))
	}
	require.NoError(t, os.WriteFile(fallback, goregular.TTF, 0o600))

	// When rendering requests a different primary font and fallback chain.
	opts, err := parseOptions([]string{"--run-dir", root, "--preflight", receipt, "--font", font, "--font-fallback", fallback})
	require.NoError(t, err)
	report, err := buildReport(t.Context(), t.TempDir(), opts, func(_ context.Context, c contract, _ result, states []state, _ []event, r recording.Receipt, _, _ string) (rendering, error) {
		assert.Equal(t, font, c.Terminal.FontPath)
		assert.Equal(t, []string{fallback}, c.Terminal.FontFallbacks)
		raster, err := newRasterizer(c.Terminal)
		require.NoError(t, err)
		defer func() { require.NoError(t, raster.fonts.Close()) }()
		_, err = raster.draw(states[0].Data)
		require.NoError(t, err)
		return rendering{
			Status: "complete", Font: &raster.fonts.Info,
			FontStyles: raster.fonts.Styles, FontFallbacks: raster.fonts.Fallbacks,
		}, nil
	})

	// Then rendering succeeds with separately recorded choices and unchanged evidence.
	require.NoError(t, err)
	assert.Equal(t, "complete", report.Rendering.Status)
	assert.Equal(t, "passed", report.CaseStatus)
	require.NotNil(t, report.Rendering.Font)
	assert.Equal(t, font, report.Rendering.Font.Path)
	require.Len(t, report.Rendering.FontStyles, 3)
	for index, name := range []string{"replacement-Bold.ttf", "replacement-Italic.ttf", "replacement-BoldItalic.ttf"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		sum := sha256.Sum256(data)
		assert.Equal(t, filepath.Join(root, name), report.Rendering.FontStyles[index].Path)
		assert.NotEmpty(t, report.Rendering.FontStyles[index].Family)
		assert.Equal(t, hex.EncodeToString(sum[:]), report.Rendering.FontStyles[index].SHA256)
	}
	require.Len(t, report.Rendering.FontFallbacks, 1)
	assert.Equal(t, fallback, report.Rendering.FontFallbacks[0].Path)
	assert.Equal(t, font, report.terminal.FontPath, "session overview must use the rendering font too")
	assert.Equal(t, []string{fallback}, report.terminal.FontFallbacks)
	require.NotNil(t, report.Presentation)
	assert.Equal(t, font, report.Presentation.FontPath)
	assert.Equal(t, []string{fallback}, report.Presentation.FontFallbacks)
	for path, before := range originals {
		after, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, before, after, path)
	}
}

func TestParseRenderingFonts(t *testing.T) {
	t.Parallel()
	primary, err := filepath.Abs("primary.ttf")
	require.NoError(t, err)
	first, err := filepath.Abs("first.ttf")
	require.NoError(t, err)
	second, err := filepath.Abs("second.ttf")
	require.NoError(t, err)
	for _, tc := range []struct {
		name      string
		flags     []string
		primary   string
		fallbacks []string
	}{
		{name: "omitted choices remain empty"},
		{name: "primary font only", flags: []string{"--font", "primary.ttf"}, primary: primary},
		{name: "ordered fallbacks only", flags: []string{"--font-fallback", "first.ttf", "--font-fallback", "second.ttf"},
			fallbacks: []string{first, second}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Given an evidence request with optional font choices.
			args := append([]string{"--run-dir", "capture", "--preflight", "receipt"}, tc.flags...)

			// When parsing the request.
			opts, err := parseOptions(args)

			// Then only explicit choices override the contract, with absolute paths and preserved order.
			require.NoError(t, err)
			assert.Equal(t, tc.primary, opts.fontPath)
			assert.Equal(t, tc.fallbacks, opts.fontFallbacks)
		})
	}
}

func TestRejectBlankRenderingFonts(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"--font", "--font-fallback"} {
		t.Run(flag, func(t *testing.T) {
			// Given an explicitly empty font path.
			args := []string{"--run-dir", "capture", "--preflight", "receipt", flag, ""}

			// When parsing the request.
			_, err := parseOptions(args)

			// Then the request is rejected rather than silently using the recorded font.
			require.ErrorContains(t, err, "font path must not be empty")
		})
	}
}

func TestUnavailableRenderingFallbackPreservesCaseOutcome(t *testing.T) {
	t.Parallel()

	// Given a successful capture and an unavailable requested rendering fallback.
	root, receipt, originals := fontCapture(t)
	primary := filepath.Join(root, "primary.ttf")
	require.NoError(t, os.WriteFile(primary, gomono.TTF, 0o600))
	missing := filepath.Join(root, "missing.ttf")
	opts, err := parseOptions([]string{"--run-dir", root, "--preflight", receipt,
		"--font", primary, "--font-fallback", missing})
	require.NoError(t, err)

	// When the renderer attempts to open the requested fonts.
	report, err := buildReport(t.Context(), t.TempDir(), opts, func(_ context.Context, c contract, _ result, _ []state, _ []event, r recording.Receipt, _, _ string) (rendering, error) {
		raster, err := newRasterizer(c.Terminal)
		if err != nil {
			return rendering{}, err
		}
		defer func() { require.NoError(t, raster.fonts.Close()) }()
		return rendering{Status: "complete"}, nil
	})

	// Then rendering fails explicitly without turning the successful case into a failure.
	require.NoError(t, err)
	assert.Equal(t, "passed", report.CaseStatus)
	assert.Equal(t, "failed", report.Rendering.Status)
	assert.Contains(t, report.Rendering.Error, "load explicit fallback")
	assert.Contains(t, report.Rendering.Error, missing)
	require.NotNil(t, report.Presentation)
	assert.Equal(t, []string{missing}, report.Presentation.FontFallbacks)
	for path, before := range originals {
		after, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, before, after, path)
	}
}

func TestRenderingRequiresExplicitFont(t *testing.T) {
	t.Parallel()

	// Given an execution capture without presentation font paths.
	root, receipt, _ := fontCapture(t)
	replacement := filepath.Join(root, "replacement.ttf")
	require.NoError(t, os.WriteFile(replacement, gomono.TTF, 0o600))
	for _, tc := range []struct {
		name      string
		flags     []string
		primary   string
		fallbacks []string
		status    string
	}{
		{name: "missing primary", status: "failed"},
		{name: "selected primary", flags: []string{"--font", replacement}, primary: replacement, status: "complete"},
		{name: "fallback still needs primary", flags: []string{"--font-fallback", replacement},
			fallbacks: []string{replacement}, status: "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// When rendering uses only the explicit presentation choices.
			args := append([]string{"--run-dir", root, "--preflight", receipt}, tc.flags...)
			opts, err := parseOptions(args)
			require.NoError(t, err)
			report, err := buildReport(t.Context(), t.TempDir(), opts, func(_ context.Context, c contract, _ result, _ []state, _ []event, _ recording.Receipt, _, _ string) (rendering, error) {
				assert.Equal(t, tc.primary, c.Terminal.FontPath)
				assert.Equal(t, tc.fallbacks, c.Terminal.FontFallbacks)
				raster, err := newRasterizer(c.Terminal)
				if err != nil {
					return rendering{}, err
				}
				defer func() { require.NoError(t, raster.fonts.Close()) }()
				return rendering{Status: "complete", Font: &raster.fonts.Info, FontFallbacks: raster.fonts.Fallbacks}, nil
			})

			// Then no font path is inherited from execution or preflight.
			if tc.status == "failed" {
				require.ErrorContains(t, err, "--font")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.status, report.Rendering.Status)
			assert.Equal(t, tc.primary, report.terminal.FontPath)
			assert.Equal(t, tc.fallbacks, report.terminal.FontFallbacks)
			require.NotNil(t, report.Rendering.Font)
			assert.Equal(t, tc.primary, report.Rendering.Font.Path)
		})
	}
}
