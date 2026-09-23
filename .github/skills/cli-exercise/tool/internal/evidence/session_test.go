package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/cli/cli/v2/cli-exercise/internal/recording"
	"github.com/stretchr/testify/require"
)

func sessionCases(t *testing.T) {
	for _, withHTML := range []bool{false, true} {
		name := "media-only session skips all HTML"
		if withHTML {
			name = "HTML session includes chapter reports"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			directory := filepath.Join(root, "run")
			require.NoError(t, os.MkdirAll(filepath.Join(directory, "capture"), 0o700))
			source := contract{CaseID: "fixture", Mode: "exact", Goal: "Inspect a captured fixture",
				Output: recording.OutputOptions{Formats: []string{"mp4"}, Timing: "condensed"}}
			source.Command.Executable = filepath.Join(root, "fixture")
			raw, err := json.Marshal(source)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(directory, "contract.json"), raw, 0o600))
			hash := sha256.Sum256(raw)
			zero := 0
			require.NoError(t, cliutil.WriteJSON(filepath.Join(directory, "result.json"),
				result{ContractSHA256: hex.EncodeToString(hash[:]), CaptureStatus: "complete", StepsCompleted: &zero, ExitCode: &zero}))
			recorded, err := json.Marshal(testState("captured", 0))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(directory, "capture/states.jsonl"), append(recorded, '\n'), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(directory, "capture/events.jsonl"), nil, 0o600))
			manifest := sessionManifest{SchemaVersion: 1, ID: "session", Title: "Captured case", Source: "Record the case", Workspace: root,
				Cases: []sessionCase{{ID: "case", Title: "Case", Runs: []sessionRun{{ID: "run", Phase: "exercise", Title: "Exercise", RunDirectory: "run"}}}}}
			path := filepath.Join(root, "session.json")
			require.NoError(t, cliutil.WriteJSON(path, manifest))
			receiptPath := filepath.Join(root, "preflight.json")
			require.NoError(t, cliutil.WriteJSON(receiptPath, recording.Receipt{Status: "ready",
				Checks: recording.CapabilityChecks{Formats: map[string]bool{"mp4": true}}}))
			var output bytes.Buffer
			code, err := runSession(context.Background(), t.TempDir(), options{sessionManifest: path, preflight: receiptPath,
				inspection: "all", html: withHTML, fontPath: filepath.Join(root, "selected.ttf")},
				cliutil.Streams{Out: &output}, func(context.Context, contract, result, []state, []event, recording.Receipt, string, string) (rendering, error) {
					return rendering{Status: "failed", Error: "This fixture does not encode media."}, nil
				})
			require.NoError(t, err)
			require.Equal(t, 1, code)
			var report sessionReport
			require.NoError(t, json.Unmarshal(output.Bytes(), &report))
			require.Len(t, report.Chapters, 1)
			if withHTML {
				require.FileExists(t, report.Report)
				require.FileExists(t, report.Chapters[0].Evidence.Report)
				require.Len(t, htmlFiles(t, root), 2)
			} else {
				require.Empty(t, report.Report)
				require.Empty(t, report.Chapters[0].Evidence.Report)
				require.Empty(t, htmlFiles(t, root))
			}
			reports, err := filepath.Glob(filepath.Join(root, "session-render-*", "report.json"))
			require.NoError(t, err)
			require.Len(t, reports, 1)
		})
	}
	for _, tc := range []struct {
		name   string
		modify func(*sessionManifest)
		err    string
	}{
		{name: "multiple cases and phases"},
		{name: "exercise-only coverage", modify: func(m *sessionManifest) { m.Cases = m.Cases[1:]; m.Cases[0].Runs = m.Cases[0].Runs[1:2] }},
		{name: "duplicate case", modify: func(m *sessionManifest) { m.Cases[1].ID = m.Cases[0].ID }, err: "unique IDs"},
		{name: "duplicate run", modify: func(m *sessionManifest) { m.Cases[0].Runs[1].ID = m.Cases[0].Runs[0].ID }, err: "unique ID"},
		{name: "missing phase", modify: func(m *sessionManifest) { m.Cases[0].Runs[0].Phase = "" }, err: "phase"},
		{name: "invalid reading hold", modify: func(m *sessionManifest) { duration := -1.0; m.Output.MinimumChapterSeconds = &duration }, err: "minimumChapterSeconds"},
		{name: "invalid timing", modify: func(m *sessionManifest) { m.Output.Timing = "sped-up" }, err: "timing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			manifest := sessionManifest{SchemaVersion: 1, ID: "session", Title: "Two cases", Source: "Record the supplied cases.", Workspace: root}
			for _, id := range []string{"config", "alias"} {
				item := sessionCase{ID: id, Title: id}
				for _, phase := range []string{"setup", "exercise", "validation"} {
					item.Runs = append(item.Runs, sessionRun{ID: phase, Phase: phase, Title: phase, RunDirectory: id + "/" + phase})
				}
				manifest.Cases = append(manifest.Cases, item)
			}
			if tc.modify != nil {
				tc.modify(&manifest)
			}
			path := filepath.Join(root, "session.json")
			require.NoError(t, cliutil.WriteJSON(path, manifest))
			loaded, raw, err := loadSession(path)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, manifest.Cases, loaded.Cases)
			saved, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, saved, raw)
		})
	}
	t.Run("chapter notes are separate presentation metadata", func(t *testing.T) {
		time := 1.0
		chapter, text := "Typing", "Enter a title"
		original := []event{{Time: &time, Type: "note", Chapter: &chapter, Text: &text}}
		rendered := chapterEvents(chapterPresentation{Label: "Case", Phase: "exercise", Title: "Inspect the requested issue", Command: `gh issue view 'with spaces'`}, original)
		require.Equal(t, "Typing", *original[0].Chapter)
		require.Len(t, rendered, 2)
		require.Equal(t, "Case / Running", *rendered[1].Chapter)
		require.NotContains(t, *rendered[1].Text, "Command:")
		require.NotContains(t, *rendered[1].Text, "gh issue view")
		require.Contains(t, *rendered[1].Text, "Enter a title")
		require.Equal(t, `gh alias set 'name with space' 'config get prompt'`, displayCommand("/selected/gh", []string{"alias", "set", "name with space", "config get prompt"}))
	})
	for _, tc := range []struct {
		statuses []string
		want     string
	}{
		{[]string{"observed", "passed", "passed"}, "passed"},
		{[]string{"passed", "failed"}, "failed"},
		{[]string{"observed", "blocked"}, "blocked"},
		{[]string{"observed"}, "observed"},
		{nil, "blocked"},
	} {
		require.Equal(t, tc.want, sessionStatus(tc.statuses))
	}
	t.Run("missing requested run remains blocked", func(t *testing.T) {
		root := t.TempDir()
		manifest := sessionManifest{SchemaVersion: 1, ID: "missing", Title: "Missing validation", Source: "Keep validation in the recording.", Workspace: root,
			Cases: []sessionCase{{ID: "case", Title: "Case", Runs: []sessionRun{{ID: "verify", Phase: "validation", Title: "Verify", RunDirectory: "not-recorded"}}}}}
		path := filepath.Join(root, "session.json")
		require.NoError(t, cliutil.WriteJSON(path, manifest))
		receiptPath := filepath.Join(root, "preflight.json")
		receipt := recording.Receipt{Status: "ready", Checks: recording.CapabilityChecks{Formats: map[string]bool{"mp4": true}}}
		require.NoError(t, cliutil.WriteJSON(receiptPath, receipt))
		var output bytes.Buffer
		code, err := runSession(context.Background(), t.TempDir(), options{sessionManifest: path, preflight: receiptPath,
			inspection: "all", fontPath: filepath.Join(root, "selected.ttf")},
			cliutil.Streams{Out: &output}, render)
		require.NoError(t, err)
		require.Equal(t, 1, code)
		var report sessionReport
		require.NoError(t, json.Unmarshal(output.Bytes(), &report))
		require.Equal(t, "blocked", report.SessionStatus)
		require.Equal(t, "failed", report.Rendering.Status)
		require.Len(t, report.Chapters, 1)
		require.NotEmpty(t, report.Chapters[0].Error)
	})
}
