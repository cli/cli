package evidence

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/cli/cli/v2/cli-exercise/internal/recording"
)

func displayCommand(executable string, args []string) string {
	words := append([]string{filepath.Base(executable)}, args...)
	for index, word := range words {
		safe := word != "" && strings.IndexFunc(word, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
				strings.ContainsRune("_.,/:=@%+-", r))
		}) == -1
		if !safe {
			words[index] = "'" + strings.ReplaceAll(word, "'", "'\"'\"'") + "'"
		}
	}
	return strings.Join(words, " ")
}

func chapterEvents(chapter chapterPresentation, original []event) []event {
	start := 0.0
	phase, _ := phaseAppearance(chapter.Phase, chapter.Status)
	label := chapter.Label + " / " + phase
	initial := chapter.Title
	events := []event{{Time: &start, Type: "note", Chapter: &label, Text: &initial}}
	for _, item := range original {
		if item.Type == "note" {
			parts := []string{}
			if item.Chapter != nil && *item.Chapter != "" {
				parts = append(parts, *item.Chapter)
			}
			if item.Text != nil && *item.Text != "" {
				parts = append(parts, *item.Text)
			}
			text := strings.Join(parts, " | ")
			item.Chapter, item.Text = &label, &text
		}
		events = append(events, item)
	}
	return events
}

func loadSession(path string) (sessionManifest, []byte, error) {
	var manifest sessionManifest
	raw, err := cliutil.ReadJSON(path, 16<<20, &manifest)
	if err != nil {
		return manifest, nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return manifest, nil, err
	}
	if manifest.SchemaVersion != 1 || strings.TrimSpace(manifest.ID) == "" ||
		strings.TrimSpace(manifest.Title) == "" || strings.TrimSpace(manifest.Source) == "" ||
		!filepath.IsAbs(manifest.Workspace) || len(manifest.Cases) == 0 {
		return manifest, nil, fmt.Errorf("recording session needs an identity, title, source request, absolute workspace, and cases")
	}
	caseIDs := map[string]bool{}
	for _, item := range manifest.Cases {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Title) == "" || caseIDs[item.ID] || len(item.Runs) == 0 {
			return manifest, nil, fmt.Errorf("recording cases need unique IDs, titles, and at least one run")
		}
		caseIDs[item.ID] = true
		runIDs := map[string]bool{}
		for _, run := range item.Runs {
			if strings.TrimSpace(run.ID) == "" || runIDs[run.ID] || strings.TrimSpace(run.Title) == "" ||
				strings.TrimSpace(run.Phase) == "" || strings.TrimSpace(run.RunDirectory) == "" {
				return manifest, nil, fmt.Errorf("each case run needs a unique ID, phase, title, and recorded runDirectory")
			}
			runIDs[run.ID] = true
		}
	}
	if err := validateCaseSequence(manifest); err != nil {
		return manifest, nil, err
	}
	if manifest.Output.Timing != "" && !slices.Contains([]string{"condensed", "realtime"}, manifest.Output.Timing) {
		return manifest, nil, fmt.Errorf("session timing must be realtime or condensed")
	}
	for _, format := range manifest.Output.Formats {
		if !slices.Contains([]string{"mp4", "gif"}, format) {
			return manifest, nil, fmt.Errorf("session output formats must be mp4 and/or gif")
		}
	}
	if minimum := manifest.Output.MinimumChapterSeconds; minimum != nil && (!finite(*minimum) || *minimum < 0 || *minimum > 60) {
		return manifest, nil, fmt.Errorf("minimumChapterSeconds must be between 0 and 60")
	}
	return manifest, raw, nil
}

func sessionStatus(statuses []string) string {
	passed, blocked := false, false
	for _, status := range statuses {
		if status == "failed" {
			return "failed"
		}
		passed = passed || status == "passed"
		blocked = blocked || !slices.Contains([]string{"passed", "observed"}, status)
	}
	if blocked || len(statuses) == 0 {
		return "blocked"
	}
	if passed {
		return "passed"
	}
	return "observed"
}

func sessionPath(workspace, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(workspace, filepath.FromSlash(path))
}

func runSession(ctx context.Context, skillRoot string, opts options, streams cliutil.Streams, renderMedia renderFunc) (int, error) {
	manifest, original, err := loadSession(opts.sessionManifest)
	if err != nil {
		return 1, err
	}
	workspace, err := guardWorkspace(manifest.Workspace, skillRoot)
	if err != nil {
		return 1, err
	}
	var receipt recording.Receipt
	if _, err := cliutil.ReadJSON(opts.preflight, 4<<20, &receipt); err != nil {
		return 1, err
	}
	formats := manifest.Output.Formats
	if len(opts.formats) != 0 {
		formats = opts.formats
	}
	if len(formats) == 0 {
		formats = []string{"mp4"}
	}
	if receipt.Status != "ready" || !receipt.Checks.Formats["mp4"] {
		return 1, fmt.Errorf("session assembly requires a ready MP4 capability check for its chapter intermediates")
	}
	for _, format := range formats {
		if !receipt.Checks.Formats[format] {
			return 1, fmt.Errorf("session format %s was not confirmed by preflight", format)
		}
	}
	output, err := os.MkdirTemp(workspace, "session-render-")
	if err != nil {
		return 1, err
	}
	if err := os.WriteFile(filepath.Join(output, "session.json"), original, 0o400); err != nil {
		return 1, err
	}
	hash := sha256.Sum256(original)
	result := sessionReport{SchemaVersion: 1, ID: manifest.ID, Title: manifest.Title,
		ManifestSHA256: hex.EncodeToString(hash[:]), Cases: []sessionCaseResult{}, Chapters: []sessionChapter{}}
	complete := true
	caseStatuses := []string{}
	caseModes := []string{}
	annotated := true
	for _, item := range manifest.Cases {
		statuses := []string{}
		modes, exerciseModes := []string{}, []string{}
		for _, run := range item.Runs {
			if err := ctx.Err(); err != nil {
				return 1, err
			}
			directory := sessionPath(workspace, run.RunDirectory)
			minimum := 2.0
			if manifest.Output.MinimumChapterSeconds != nil {
				minimum = *manifest.Output.MinimumChapterSeconds
			}
			settings := options{runDir: directory, preflight: opts.preflight, inspection: opts.inspection, html: opts.html,
				formats: []string{"mp4"}, timing: manifest.Output.Timing, captions: manifest.Output.Captions,
				fontPath: opts.fontPath, fontFallbacks: opts.fontFallbacks,
				chapter: &chapterPresentation{Label: item.Title, Phase: run.Phase, Title: run.Title, MinimumDurationSeconds: minimum}}
			if opts.timing != "" {
				settings.timing, settings.timingAuthorization = opts.timing, opts.timingAuthorization
			}
			if run.Verification != "" {
				settings.verification = sessionPath(directory, run.Verification)
			}
			chapter := sessionChapter{Kind: "run", CaseID: item.ID, CaseTitle: item.Title, RunID: run.ID,
				Phase: run.Phase, Title: run.Title, RunDirectory: directory}
			chapter.Evidence, err = buildReport(ctx, skillRoot, settings, renderMedia)
			if err != nil {
				chapter.Error = err.Error()
				chapter.Evidence.CaseStatus = "blocked"
				chapter.Evidence.Rendering = rendering{Status: "failed", Error: err.Error()}
			}
			statuses = append(statuses, chapter.Evidence.CaseStatus)
			modes = append(modes, chapter.Evidence.Mode)
			if run.Phase == "exercise" {
				exerciseModes = append(exerciseModes, chapter.Evidence.Mode)
			}
			if presentation := chapter.Evidence.Presentation; presentation != nil && presentation.Captions != nil && !*presentation.Captions {
				annotated = false
			}
			complete = complete && chapter.Evidence.Rendering.Status == "complete"
			result.Chapters = append(result.Chapters, chapter)
		}
		status := sessionStatus(statuses)
		if len(exerciseModes) != 0 {
			modes = exerciseModes
		}
		mode := recordingMode(modes)
		caseStatuses = append(caseStatuses, status)
		caseModes = append(caseModes, mode)
		result.Cases = append(result.Cases, sessionCaseResult{ID: item.ID, Title: item.Title, Status: status, Mode: mode})
	}
	result.SessionStatus = sessionStatus(caseStatuses)
	result.Mode = recordingMode(caseModes)
	verified, warnings, chronologyErr := verifySessionChronology(result.Chapters)
	result.OrderVerified = verified
	if chronologyErr != nil {
		complete = false
		result.SessionStatus = "blocked"
	}
	if complete {
		planned, _, err := planSessionMedia(result.Chapters)
		if err == nil && annotated {
			overviewContext, cancel := context.WithTimeout(ctx, time.Duration(max(120, min(1800, len(result.Cases)*20+30)))*time.Second)
			var intro []sessionChapter
			intro, result.Overview, err = renderOverview(overviewContext, output, receipt, result.Chapters[0].Evidence.terminal,
				result.Title, result.Mode, result.Cases, planned.Width, planned.Height, planned.FPS)
			cancel()
			if err == nil {
				result.Chapters = append(intro, result.Chapters...)
			}
		}
		var assembled rendering
		var chapters []sessionChapter
		if err == nil {
			assembled, chapters, err = assembleSession(ctx, output, receipt, result.Chapters, formats, annotated, opts.inspection)
		}
		if err != nil {
			result.Rendering = rendering{Status: "failed", Error: err.Error(), Media: map[string]string{}}
		} else {
			result.Rendering, result.Chapters = assembled, chapters
		}
	} else {
		result.Rendering = rendering{Status: "failed", Error: "Not every requested run could be rendered; no chapter was silently omitted.", Media: map[string]string{}}
		if chronologyErr != nil {
			result.Rendering.Error = chronologyErr.Error()
		}
	}
	result.Rendering.Warnings = append(result.Rendering.Warnings, warnings...)
	if err := cliutil.WriteJSON(filepath.Join(output, "report.json"), result); err != nil {
		return 1, err
	}
	if opts.html {
		result.Report = filepath.Join(output, "report.html")
		if err := writeSessionHTML(result.Report, manifest.Source, result); err != nil {
			return 1, err
		}
	}
	if err := cliutil.JSON(streams.Out, result); err != nil {
		return 1, err
	}
	if result.Rendering.Status == "complete" && slices.Contains([]string{"passed", "observed"}, result.SessionStatus) {
		return 0, nil
	}
	return 1, nil
}

func writeSessionHTML(path, source string, report sessionReport) (err error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	writer := bufio.NewWriter(file)
	write := func(value string) {
		if err == nil {
			_, err = io.WriteString(writer, value)
		}
	}
	write(`<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>CLI recording session</title>
<style>body{max-width:1400px;margin:auto;padding:24px;font:16px/1.5 system-ui;background:#0d1117;color:#e6edf3}
video,img{width:100%;height:auto}main{display:grid;grid-template-columns:minmax(0,1fr) 300px;gap:20px}
button{display:block;width:100%;text-align:left;padding:10px;margin:6px 0;background:#161b22;color:inherit;border:1px solid #30363d;border-radius:6px;cursor:pointer}
button:hover{border-color:#58a6ff}small,p{color:#9198a1}a{color:#79c0ff}pre{white-space:pre-wrap;overflow-wrap:anywhere}details{margin-top:20px}
@media(max-width:850px){main{display:block}}</style></head><body>`)
	write("<h1>" + html.EscapeString(report.Title) + "</h1><p>Session: " + html.EscapeString(report.SessionStatus) +
		" | Rendering: " + html.EscapeString(report.Rendering.Status) + "</p><main><section>")
	if report.Rendering.Error != "" {
		write("<p>" + html.EscapeString(report.Rendering.Error) + "</p>")
	}
	for _, kind := range []string{"mp4", "gif"} {
		filename, exists := report.Rendering.Media[kind]
		if !exists {
			continue
		}
		media, openErr := os.Open(filename)
		if openErr != nil {
			return errors.Join(err, openErr)
		}
		if kind == "mp4" {
			write(`<video id="recording" controls preload="metadata" src="data:video/mp4;base64,`)
		} else {
			write(`<img alt="Recorded CLI session" src="data:image/gif;base64,`)
		}
		if err != nil {
			return errors.Join(err, media.Close())
		}
		encoder := base64.NewEncoder(base64.StdEncoding, writer)
		_, copyErr := io.Copy(encoder, media)
		if err := errors.Join(copyErr, encoder.Close(), media.Close()); err != nil {
			return err
		}
		if kind == "mp4" {
			write(`"></video>`)
		} else {
			write(`">`)
		}
	}
	write("<p>Chapters contain separately recorded commands. Captions are annotations, not simulated terminal output. Original captures and per-run reports remain unchanged.</p></section><nav aria-label=\"Video chapters\">")
	for _, chapter := range report.Chapters {
		name := chapter.CaseTitle + " / " + chapter.Phase + " / " + chapter.Title
		if chapter.Kind == "overview" {
			name = chapter.Title
		}
		write(fmt.Sprintf(`<button data-time="%.6f">%s<br><small>%.2fs - %.2fs | %s</small></button>`,
			chapter.StartSeconds, html.EscapeString(name), chapter.StartSeconds, chapter.EndSeconds, html.EscapeString(chapter.Evidence.CaseStatus)))
		if chapter.Evidence.Report != "" {
			link := (&url.URL{Scheme: "file", Path: filepath.ToSlash(chapter.Evidence.Report)}).String()
			write(`<a href="` + html.EscapeString(link) + `">Run evidence</a>`)
		}
	}
	write("</nav></main><details><summary>Requested recording</summary><pre>" + html.EscapeString(source) + "</pre></details>")
	details, marshalErr := json.MarshalIndent(report, "", "  ")
	if marshalErr != nil {
		return errors.Join(err, marshalErr)
	}
	write("<details><summary>Case outcomes and provenance</summary><pre>" + html.EscapeString(string(details)) + "</pre></details>")
	write(`<script>const player=document.getElementById('recording');document.querySelectorAll('[data-time]').forEach(button=>button.addEventListener('click',()=>{if(player){player.currentTime=Number(button.dataset.time);player.play();}}));</script></body></html>`)
	return errors.Join(err, writer.Flush())
}
