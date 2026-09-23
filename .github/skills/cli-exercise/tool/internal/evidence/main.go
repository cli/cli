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
	"flag"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/cli/cli/v2/cli-exercise/internal/recording"
)

type options struct {
	runDir              string
	sessionManifest     string
	preflight           string
	verification        string
	inspection          string
	formats             []string
	timing              string
	timingAuthorization string
	chapter             *chapterPresentation
	captions            *bool
	html                bool
	fontPath            string
	fontFallbacks       []string
}

func parseOptions(args []string) (options, error) {
	var opts options
	flags := flag.NewFlagSet("evidence", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.runDir, "run-dir", "", "Private capture workspace")
	flags.StringVar(&opts.sessionManifest, "session", "", "Ordered multi-case recording manifest")
	flags.StringVar(&opts.preflight, "preflight", "", "Ready preflight receipt")
	flags.StringVar(&opts.verification, "verification", "", "Evidence-backed manual expectations")
	flags.StringVar(&opts.inspection, "inspection", "sampled", "sampled or all")
	flags.BoolVar(&opts.html, "html", false, "Also generate an interactive HTML report")
	fontPath := func(value string) (string, error) {
		if strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("font path must not be empty")
		}
		return filepath.Abs(value)
	}
	flags.Func("font", "Primary rendering font; leaves the recorded contract unchanged", func(value string) error {
		path, err := fontPath(value)
		if err == nil {
			opts.fontPath = path
		}
		return err
	})
	flags.Func("font-fallback", "Rendering fallback font; repeat to replace the recorded fallback chain in order", func(value string) error {
		path, err := fontPath(value)
		if err == nil {
			opts.fontFallbacks = append(opts.fontFallbacks, path)
		}
		return err
	})
	flags.Func("format", "Additional presentation format: gif or mp4; repeat for both", func(value string) error {
		if !slices.Contains([]string{"gif", "mp4"}, value) {
			return fmt.Errorf("--format must be gif or mp4")
		}
		if !slices.Contains(opts.formats, value) {
			opts.formats = append(opts.formats, value)
		}
		return nil
	})
	flags.Func("timing", "Presentation timing: use the contract's timing, or condensed when unspecified", func(value string) error {
		if !slices.Contains([]string{"realtime", "condensed"}, value) {
			return fmt.Errorf("--timing must be realtime or condensed")
		}
		opts.timing = value
		return nil
	})
	flags.StringVar(&opts.timingAuthorization, "timing-authorization", "", "Optional caller-request note for a condensed presentation")
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	if (opts.runDir == "" && opts.sessionManifest == "") || opts.preflight == "" || len(flags.Args()) != 0 {
		return opts, fmt.Errorf("evidence requires --run-dir or --session and --preflight, with no positional arguments")
	}
	if opts.runDir != "" && opts.sessionManifest != "" {
		return opts, fmt.Errorf("choose only one of --run-dir and --session")
	}
	if opts.sessionManifest != "" && opts.verification != "" {
		return opts, fmt.Errorf("session verification files belong to individual manifest runs")
	}
	if !slices.Contains([]string{"sampled", "all"}, opts.inspection) {
		return opts, fmt.Errorf("--inspection must be sampled or all")
	}
	if opts.timingAuthorization != "" && opts.timing != "condensed" {
		return opts, fmt.Errorf("--timing-authorization applies only to --timing condensed")
	}
	return opts, nil
}

func inside(directory, candidate string) bool {
	relative, err := filepath.Rel(directory, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && !filepath.IsAbs(relative)
}

func guardWorkspace(workspace, skillRoot string) (string, error) {
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	if filepath.Dir(root) == root {
		return "", fmt.Errorf("evidence requires a dedicated private workspace")
	}
	protected := []string{}
	if skillRoot != "" {
		skill, err := filepath.Abs(skillRoot)
		if err != nil {
			return "", err
		}
		skill, err = filepath.EvalSymlinks(skill)
		if err != nil {
			return "", err
		}
		protected = append(protected, skill)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for _, origin := range append(slices.Clone(protected), cwd) {
		for current := origin; ; current = filepath.Dir(current) {
			if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
				protected = append(protected, current)
				break
			} else if !os.IsNotExist(err) {
				return "", err
			}
			if filepath.Dir(current) == current {
				break
			}
		}
	}
	for _, directory := range protected {
		if inside(directory, root) {
			return "", fmt.Errorf("generated evidence must be outside the skill and repository")
		}
	}
	return root, nil
}

func readRecords[T any](path string) ([]T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	const limit = 64 << 20
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("capture journal must be a bounded regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("capture journal exceeds the 64 MiB limit")
	}
	values := make([]T, 0)
	for index, line := range bytes.Split(data, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if line[0] != '{' {
			return nil, fmt.Errorf("capture line %d must contain a JSON object", index+1)
		}
		var value T
		if err := json.Unmarshal(line, &value); err != nil {
			return nil, fmt.Errorf("decode capture line %d: %w", index+1, err)
		}
		values = append(values, value)
	}
	return values, nil
}

// Presentation output is already resolved, so it must not inherit the contract's input unmarshaler.
type presentationOutput recording.OutputOptions

type presentationOptions struct {
	presentationOutput
	FontPath      string   `json:"fontPath,omitempty"`
	FontFallbacks []string `json:"fontFallbacks,omitempty"`
}

type report struct {
	SchemaVersion  int                  `json:"schemaVersion"`
	CaseID         string               `json:"caseId"`
	Mode           string               `json:"mode"`
	StartedAt      string               `json:"startedAt,omitempty"`
	FinishedAt     string               `json:"finishedAt,omitempty"`
	CaseStatus     string               `json:"caseStatus"`
	CaptureStatus  string               `json:"captureStatus"`
	ContractSHA256 string               `json:"contractSha256"`
	Expectations   []check              `json:"expectations"`
	Rendering      rendering            `json:"rendering"`
	Presentation   *presentationOptions `json:"presentation,omitempty"`
	Report         string               `json:"report,omitempty"`
	Chapter        *chapterPresentation `json:"chapter,omitempty"`
	terminal       terminalConfig
}

type renderFunc func(context.Context, contract, result, []state, []event, recording.Receipt, string, string) (rendering, error)

func run(ctx context.Context, skillRoot string, opts options, streams cliutil.Streams, renderMedia renderFunc) (int, error) {
	if opts.sessionManifest != "" {
		return runSession(ctx, skillRoot, opts, streams, renderMedia)
	}
	report, err := buildReport(ctx, skillRoot, opts, renderMedia)
	if err != nil {
		return 1, err
	}
	if err := cliutil.JSON(streams.Out, report); err != nil {
		return 1, err
	}
	if report.Rendering.Status == "complete" && slices.Contains([]string{"passed", "observed"}, report.CaseStatus) {
		return 0, nil
	}
	return 1, nil
}

func buildReport(ctx context.Context, skillRoot string, opts options, renderMedia renderFunc) (report, error) {
	workspace, err := guardWorkspace(opts.runDir, skillRoot)
	if err != nil {
		return report{}, err
	}
	contractPath, err := containedFile(workspace, "contract.json")
	if err != nil {
		return report{}, err
	}
	var contract contract
	raw, err := cliutil.ReadJSON(contractPath, 1<<20, &contract)
	if err != nil {
		return report{}, err
	}
	if contract.CaseID == "" || !slices.Contains([]string{"exact", "explore"}, contract.Mode) {
		return report{}, fmt.Errorf("contract must identify its original case and fidelity mode")
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	resultPath, err := containedFile(workspace, "result.json")
	if err != nil {
		return report{}, err
	}
	var result result
	if _, err := cliutil.ReadJSON(resultPath, 4<<20, &result); err != nil {
		return report{}, err
	}
	if result.ContractSHA256 != hash {
		return report{}, fmt.Errorf("runtime result does not match the immutable contract")
	}
	var receipt recording.Receipt
	if _, err := cliutil.ReadJSON(opts.preflight, 4<<20, &receipt); err != nil {
		return report{}, err
	}
	if receipt.Status != "ready" {
		return report{}, fmt.Errorf("a ready preflight receipt is required")
	}
	statePath, err := containedFile(workspace, "capture/states.jsonl")
	if err != nil {
		return report{}, err
	}
	eventPath, err := containedFile(workspace, "capture/events.jsonl")
	if err != nil {
		return report{}, err
	}
	states, err := readRecords[state](statePath)
	if err != nil {
		return report{}, err
	}
	events, err := readRecords[event](eventPath)
	if err != nil {
		return report{}, err
	}
	var manual *verification
	if opts.verification != "" {
		manual = &verification{}
		if _, err := cliutil.ReadJSON(opts.verification, 1<<20, manual); err != nil {
			return report{}, err
		}
	}
	outcome, checks, err := evaluate(contract, result, states, manual, workspace, hash)
	if err != nil {
		return report{}, err
	}
	presentation := contract
	if opts.chapter != nil {
		if !filepath.IsAbs(contract.Command.Executable) {
			return report{}, fmt.Errorf("session chapters require the recorded command identity")
		}
		chapter := *opts.chapter
		chapter.Command = displayCommand(contract.Command.Executable, contract.Command.Args)
		chapter.Status = outcome
		presentation.Chapter = &chapter
	}
	var requestedOutput *presentationOptions
	if len(opts.formats) > 0 || opts.timing != "" || opts.captions != nil {
		if len(opts.formats) > 0 {
			presentation.Output.Formats = slices.Clone(opts.formats)
		}
		if opts.timing != "" {
			presentation.Output.Timing = opts.timing
			presentation.Output.TimingAuthorization = opts.timingAuthorization
		}
		if opts.captions != nil {
			presentation.Output.Captions = opts.captions
		}
		if presentation.Output.Timing == "condensed" && presentation.Output.Captions != nil && !*presentation.Output.Captions {
			return report{}, fmt.Errorf("unannotated output preserves real timing; use realtime instead of condensed")
		}
		requestedOutput = &presentationOptions{presentationOutput: presentationOutput(presentation.Output)}
	}
	if opts.fontPath != "" || len(opts.fontFallbacks) > 0 {
		if opts.fontPath != "" {
			presentation.Terminal.FontPath = opts.fontPath
		}
		if len(opts.fontFallbacks) > 0 {
			presentation.Terminal.FontFallbacks = slices.Clone(opts.fontFallbacks)
		}
		if requestedOutput == nil {
			requestedOutput = &presentationOptions{presentationOutput: presentationOutput(presentation.Output)}
		}
		requestedOutput.FontPath = presentation.Terminal.FontPath
		requestedOutput.FontFallbacks = slices.Clone(presentation.Terminal.FontFallbacks)
	}
	artifacts := filepath.Join(workspace, "artifacts")
	if info, err := os.Lstat(artifacts); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return report{}, fmt.Errorf("artifact directory must be a real directory, not a symlink")
	} else if err != nil && !os.IsNotExist(err) {
		return report{}, err
	}
	if err := os.MkdirAll(artifacts, 0o700); err != nil {
		return report{}, err
	}
	output, err := os.MkdirTemp(artifacts, "render-")
	if err != nil {
		return report{}, err
	}
	rendered, renderErr := renderMedia(ctx, presentation, result, states, events, receipt, output, opts.inspection)
	if renderErr != nil {
		rendered = rendering{Status: "failed", Error: renderErr.Error(), Media: map[string]string{}}
	}
	resultReport := report{
		SchemaVersion: 1, CaseID: contract.CaseID, CaseStatus: outcome, CaptureStatus: result.CaptureStatus,
		Mode: contract.Mode, StartedAt: result.StartedAt, FinishedAt: result.FinishedAt, terminal: presentation.Terminal,
		ContractSHA256: hash, Expectations: checks, Rendering: rendered,
		Presentation: requestedOutput,
		Chapter:      presentation.Chapter,
	}
	if err := cliutil.WriteJSON(filepath.Join(output, "report.json"), resultReport); err != nil {
		return report{}, err
	}
	if opts.html {
		resultReport.Report = filepath.Join(output, "report.html")
		if err := writeHTML(resultReport.Report, raw, contract.Goal, resultReport); err != nil {
			return report{}, err
		}
	}
	return resultReport, nil
}

func writeHTML(path string, contract []byte, goal string, report report) (err error) {
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
<meta name="viewport" content="width=device-width, initial-scale=1"><title>CLI exercise evidence</title>
<style>body{max-width:1200px;margin:auto;padding:24px;font:16px/1.5 system-ui;background:#0d1117;color:#e6edf3}
video,img{width:100%;height:auto}pre{white-space:pre-wrap;overflow-wrap:anywhere;font:12px/1.5 monospace}
td,th{padding:12px;border-bottom:1px solid #30363d;text-align:left;vertical-align:top}table{width:100%}
p{color:#9198a1}details{margin:16px 0}</style></head><body>`)
	write("<h1>" + html.EscapeString(report.CaseID+": "+report.CaseStatus) + "</h1><p>" + html.EscapeString(goal) + "</p>")
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
			write(`<video controls preload="metadata" src="data:video/mp4;base64,`)
		} else {
			write(`<img alt="Recorded terminal case" src="data:image/gif;base64,`)
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
	review := "not available"
	if report.Rendering.Inspection != nil {
		review = report.Rendering.Inspection.VisualReview
	}
	write("<p>Case outcome and recording status are separate. Timing: " + html.EscapeString(report.Rendering.Timing) +
		". Visual review: " + html.EscapeString(review) + ". This report requires no playback server.</p>")
	write("<table><thead><tr><th>Expectation</th><th>Status</th><th>Evidence</th></tr></thead><tbody>")
	for _, check := range report.Expectations {
		value, marshalErr := json.MarshalIndent(check, "", "  ")
		if marshalErr != nil {
			return errors.Join(err, marshalErr)
		}
		write("<tr><td>" + html.EscapeString(check.ID) + "</td><td>" + html.EscapeString(check.Status) +
			"</td><td><pre>" + html.EscapeString(string(value)) + "</pre></td></tr>")
	}
	details, marshalErr := json.MarshalIndent(report.Rendering, "", "  ")
	if marshalErr != nil {
		return errors.Join(err, marshalErr)
	}
	write("</tbody></table><details><summary>Recorded contract</summary><pre>" + html.EscapeString(string(contract)) +
		"</pre></details><details><summary>Rendering and inspection details</summary><pre>" +
		html.EscapeString(string(details)) + "</pre></details>")
	if report.Presentation != nil {
		value, marshalErr := json.MarshalIndent(report.Presentation, "", "  ")
		if marshalErr != nil {
			return errors.Join(err, marshalErr)
		}
		write("<details open><summary>Additional presentation request</summary><p>The execution contract and original recording are unchanged.</p><pre>" +
			html.EscapeString(string(value)) + "</pre></details>")
	}
	write("</body></html>")
	return errors.Join(err, writer.Flush())
}

// Main evaluates and renders one completed capture using the supplied ready tools.
func Main(ctx context.Context, skillRoot string, args []string, streams cliutil.Streams) int {
	opts, err := parseOptions(args)
	if errors.Is(err, flag.ErrHelp) {
		if _, err := fmt.Fprintln(streams.Out,
			"Usage: cli-exercise --skill-root PATH evidence {--run-dir PATH|--session FILE} --preflight FILE [--html] [--verification FILE] [--inspection sampled|all] [--format gif|mp4] [--timing realtime|condensed] [--timing-authorization TEXT] [--font FILE] [--font-fallback FILE ...]"); err != nil {
			return 1
		}
		return 0
	}
	code := 1
	if err == nil {
		code, err = run(ctx, skillRoot, opts, streams, render)
	}
	if err != nil {
		if writeErr := cliutil.Error(streams.ErrOut, "evidence_failed", err.Error()); writeErr != nil {
			return 1
		}
	}
	return code
}
