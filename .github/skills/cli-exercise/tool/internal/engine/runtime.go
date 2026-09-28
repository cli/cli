package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/cli/cli/v2/cli-exercise/internal/recording"
	"github.com/cli/cli/v2/cli-exercise/internal/terminal"
)

type pendingRecord struct {
	file string
	data []byte
	view document
}

// Runtime owns one immutable invocation, its policy, and its recorded evidence.
type Runtime struct {
	contract                                                       *runContract
	terminal                                                       terminal.Session
	runContext                                                     context.Context
	cancelRun                                                      context.CancelFunc
	privacy                                                        privacyGuard
	mu                                                             sync.Mutex
	opMu                                                           sync.Mutex
	finishMu                                                       sync.Mutex
	done                                                           chan struct{}
	files                                                          map[string]*os.File
	origin, lastAction, lastActivity                               time.Time
	startedAt                                                      time.Time
	active, exited, alternate, inputPending, outputPending         bool
	exitCode, exitSignal                                           *int
	revision, steps, actions, inputAttempts                        int
	data                                                           *recording.TerminalData
	dataJSON                                                       json.RawMessage
	lastSafe                                                       document
	fingerprint, rawWindow, plainWindow, inputWindow, protocolTail string
	plain                                                          plainStream
	mouse                                                          map[string]bool
	pending                                                        []pendingRecord
	savedBytes, pendingBytes, rawBytes, protocolReplies            int
	captureError, actionError                                      *fault
	result                                                         json.RawMessage
	finishError                                                    error
	controllers                                                    map[string]map[string]int
}

// New validates and stages an invocation without launching its executable.
func New(options Options) (*Runtime, error) {
	if options.Terminal == nil {
		return nil, problem("invalid_contract", "A mechanical terminal adapter is required.")
	}
	contract, err := parseContract(options)
	if err != nil {
		return nil, err
	}
	if err := contract.prepare(options); err != nil {
		return nil, err
	}
	capture, err := cliutil.ConfinedDirectory(contract.workspace, "capture")
	if err != nil {
		return nil, err
	}
	if _, err := cliutil.ConfinedDirectory(contract.workspace, "capture/controllers"); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(capture, "contract.json"), contract.bytes, 0o400); err != nil {
		return nil, err
	}
	now := time.Now()
	runtime := &Runtime{
		contract: contract, terminal: options.Terminal, done: make(chan struct{}), files: map[string]*os.File{},
		origin: now, lastAction: now, lastActivity: now, mouse: map[string]bool{}, controllers: map[string]map[string]int{},
		savedBytes: len(contract.bytes),
		privacy: privacyGuard{secrets: slices.Clone(contract.secrets), paths: slices.Clone(options.OperatorPaths),
			allowed: []string{contract.workspace, contract.stateDirectory, str(contract.command["executable"])}},
	}
	args, _ := stringList(contract.command["args"])
	for _, arg := range args {
		if filepath.IsAbs(arg) {
			runtime.privacy.allowed = append(runtime.privacy.allowed, arg)
		}
	}
	for _, name := range []string{"states", "events", "raw"} {
		file, err := os.OpenFile(filepath.Join(capture, name+".jsonl"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			for _, opened := range runtime.files {
				err = errors.Join(err, opened.Close())
			}
			return nil, err
		}
		runtime.files[name] = file
	}
	return runtime, nil
}

// Workspace returns the canonical private run directory.
func (runtime *Runtime) Workspace() string { return runtime.contract.workspace }

// CaseID preserves the caller's case identity.
func (runtime *Runtime) CaseID() string { return runtime.contract.id }

// ContractSHA256 identifies the immutable source bytes.
func (runtime *Runtime) ContractSHA256() string { return runtime.contract.hash }

// Done closes only after result persistence and owned terminal cleanup.
func (runtime *Runtime) Done() <-chan struct{} { return runtime.done }

// Result returns a copy of the finished result, when available.
func (runtime *Runtime) Result() (json.RawMessage, bool) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return slices.Clone(runtime.result), runtime.result != nil
}

func (runtime *Runtime) stamp() float64       { return time.Since(runtime.origin).Seconds() }
func (runtime *Runtime) privacyPending() bool { return runtime.inputPending || runtime.outputPending }

func (runtime *Runtime) writeLocked(record pendingRecord, final bool) error {
	if !final && runtime.savedBytes+len(record.data) > 64<<20 {
		return problem("capture_limit", "The bounded capture evidence limit was reached.")
	}
	if _, err := runtime.files[record.file].Write(record.data); err != nil {
		return err
	}
	runtime.savedBytes += len(record.data)
	if record.view != nil {
		runtime.lastSafe = record.view
	}
	return nil
}

func (runtime *Runtime) recordLocked(file string, value any, view document, deferRecord, final bool) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	record := pendingRecord{file: file, data: append(data, '\n'), view: view}
	if deferRecord {
		if runtime.savedBytes+runtime.pendingBytes+len(record.data) > 64<<20 {
			return problem("capture_limit", "The bounded pending evidence limit was reached.")
		}
		runtime.pending = append(runtime.pending, record)
		runtime.pendingBytes += len(record.data)
		return nil
	}
	return runtime.writeLocked(record, final)
}

func (runtime *Runtime) eventLocked(kind, mode string, fields document) error {
	value := document{"t": runtime.stamp(), "type": kind, "mode": mode}
	maps.Copy(value, fields)
	return runtime.recordLocked("events", value, nil, runtime.privacyPending() && kind != "finished", kind == "finished")
}

func (runtime *Runtime) flushLocked() error {
	for _, record := range runtime.pending {
		if err := runtime.writeLocked(record, false); err != nil {
			return err
		}
	}
	runtime.pending, runtime.pendingBytes = nil, 0
	runtime.outputPending, runtime.inputPending = false, false
	return nil
}

func (runtime *Runtime) observationLocked() (document, error) {
	if runtime.data == nil {
		return nil, nil
	}
	text, err := runtime.data.VisibleText()
	if err != nil {
		return nil, err
	}
	rows, columns := runtime.data.Rows, runtime.data.Columns
	cursor := runtime.data.Cursor
	if len(cursor) != 2 {
		return nil, problem("capture_failed", "Terminal cursor must contain real cell coordinates.")
	}
	column, row := cursor[0], cursor[1]
	menu := menuObservation(text, columns)
	click := runtime.mouse["1006"] && (runtime.mouse["1000"] || runtime.mouse["1002"] || runtime.mouse["1003"])
	return document{
		"t": runtime.stamp(), "revision": runtime.revision, "text": text, "columns": columns, "rows": rows,
		"cursor":          document{"column": column, "row": row, "visible": runtime.data.CursorVisible, "style": runtime.data.CursorStyle},
		"alternateScreen": runtime.alternate, "question": menu["question"], "selected": menu["selected"], "menu": menu,
		"capabilities": document{"text": true, "key": true, "resize": true, "click": click,
			"select": menu["supported"] == true && menu["kind"] == "line-select",
			"toggle": menu["supported"] == true && menu["kind"] == "line-checkbox"},
		"exited": runtime.exited, "exitCode": runtime.exitCode,
	}, nil
}

var queries = regexp.MustCompile("\x1b\\[\\?(1049|1047|47|1000|1002|1003|1006)([hl])|\x1b\\](10|11);\\?(?:\x07|\x1b\\\\)|\x1b\\[(6n|18t|c|0c|>c|>0c)")

func (runtime *Runtime) protocolLocked(raw string) ([]string, error) {
	runtime.protocolTail += raw
	if len(runtime.protocolTail) > 4096 {
		runtime.protocolTail = runtime.protocolTail[len(runtime.protocolTail)-4096:]
	}
	var replies []string
	consumed := 0
	for _, match := range queries.FindAllStringSubmatchIndex(runtime.protocolTail, -1) {
		group := func(index int) string {
			if match[index*2] < 0 {
				return ""
			}
			return runtime.protocolTail[match[index*2]:match[index*2+1]]
		}
		reply := ""
		if mode := group(1); mode != "" {
			if slices.Contains([]string{"1049", "1047", "47"}, mode) {
				runtime.alternate = group(2) == "h"
			} else if group(2) == "h" {
				runtime.mouse[mode] = true
			} else {
				delete(runtime.mouse, mode)
			}
		} else if selector := group(3); selector != "" {
			color := str(runtime.contract.terminal["background"])
			if selector == "10" {
				color = str(runtime.contract.terminal["foreground"])
			}
			reply = fmt.Sprintf("\x1b]%s;rgb:%s%s/%s%s/%s%s\x1b\\",
				selector, color[1:3], color[1:3], color[3:5], color[3:5], color[5:7], color[5:7])
		} else {
			switch value := group(4); value {
			case "6n":
				cursor := runtime.data.Cursor
				if len(cursor) != 2 {
					return nil, problem("capture_failed", "Cursor query cannot be answered without real coordinates.")
				}
				reply = fmt.Sprintf("\x1b[%d;%dR", cursor[1]+1, cursor[0]+1)
			case "18t":
				reply = fmt.Sprintf("\x1b[8;%d;%dt", runtime.data.Rows, runtime.data.Columns)
			default:
				reply = "\x1b[?1;2c"
				if strings.HasPrefix(value, ">") {
					reply = "\x1b[>0;11;0c"
				}
			}
		}
		if reply != "" && !runtime.exited {
			runtime.protocolReplies++
			if runtime.protocolReplies > 10000 {
				return nil, problem("capture_limit", "The terminal query budget was exhausted.")
			}
			if err := runtime.eventLocked("protocol_reply", "host", document{"request": group(0), "data": reply}); err != nil {
				return nil, err
			}
			replies = append(replies, reply)
		}
		consumed = match[1]
	}
	runtime.protocolTail = runtime.protocolTail[consumed:]
	return replies, nil
}

func (runtime *Runtime) captureLocked(data *recording.TerminalData, original json.RawMessage, raw string) ([]string, error) {
	runtime.rawBytes += len(raw)
	if runtime.rawBytes > 64<<20 {
		return nil, problem("capture_limit", "The bounded raw-output limit was reached.")
	}
	runtime.rawWindow += raw
	plain := runtime.plain.feed(raw)
	runtime.plainWindow += plain
	if err := runtime.privacy.check([]any{runtime.rawWindow, runtime.plainWindow,
		strings.ReplaceAll(runtime.plainWindow, "\r\n", "\n")}, true); err != nil {
		return nil, err
	}
	text, err := data.VisibleText()
	if err != nil {
		return nil, err
	}
	if err := runtime.privacy.check(text, true); err != nil {
		return nil, err
	}
	runtime.data = data
	runtime.dataJSON = slices.Clone(original)
	replies, err := runtime.protocolLocked(raw)
	if err != nil {
		return nil, err
	}
	fingerprint, err := json.Marshal(document{"data": original, "alternateScreen": runtime.alternate, "mouse": runtime.mouse})
	if err != nil {
		return nil, err
	}
	if runtime.fingerprint != "" && runtime.fingerprint != string(fingerprint) {
		runtime.revision++
	}
	runtime.fingerprint = string(fingerprint)
	view, err := runtime.observationLocked()
	if err != nil {
		return nil, err
	}
	runtime.outputPending = runtime.privacy.pending(runtime.plainWindow) ||
		plain != "" && !strings.HasSuffix(plain, "\n") && !strings.HasSuffix(plain, " ") && runtime.privacy.pending(text) ||
		runtime.plain.state != "" && runtime.plain.state != "text" && runtime.privacy.pending(runtime.rawWindow)
	if raw != "" {
		if err := runtime.recordLocked("raw", document{"t": runtime.stamp(), "type": "output", "data": raw}, nil, true, false); err != nil {
			return nil, err
		}
	}
	if err := runtime.recordLocked("states", document{"t": runtime.stamp(), "revision": runtime.revision,
		"alternateScreen": runtime.alternate, "data": original}, view, true, false); err != nil {
		return nil, err
	}
	if !runtime.privacyPending() {
		if err := runtime.flushLocked(); err != nil {
			return nil, err
		}
	}
	retained := 4096
	for _, secret := range runtime.contract.secrets {
		retained = max(retained, 2*len(secret))
	}
	if len(runtime.rawWindow) > retained {
		runtime.rawWindow = runtime.rawWindow[len(runtime.rawWindow)-retained:]
	}
	if len(runtime.plainWindow) > retained {
		runtime.plainWindow = runtime.plainWindow[len(runtime.plainWindow)-retained:]
	}
	runtime.lastActivity = time.Now()
	return replies, nil
}

// Start launches the selected target and begins ordered capture and budget checks.
func (runtime *Runtime) Start(ctx context.Context) error {
	if err := runtime.contract.verifyExecutable(); err != nil {
		return err
	}
	runtime.mu.Lock()
	runtime.active = true
	runtime.runContext, runtime.cancelRun = context.WithCancel(ctx)
	runContext := runtime.runContext
	runtime.origin, runtime.lastAction, runtime.lastActivity = time.Now(), time.Now(), time.Now()
	runtime.startedAt = runtime.origin
	runtime.mu.Unlock()
	go runtime.consume(runContext)
	args, _ := stringList(runtime.contract.command["args"])
	columns, _ := integer(runtime.contract.terminal["columns"])
	rows, _ := integer(runtime.contract.terminal["rows"])
	err := runtime.terminal.Start(runContext, terminal.LaunchOptions{Executable: str(runtime.contract.command["executable"]),
		Args: args, Cwd: runtime.contract.cwd, Env: runtime.contract.env, Columns: columns, Rows: rows})
	if err != nil {
		runtime.fail(err)
		return err
	}
	runtime.mu.Lock()
	err = runtime.eventLocked("started", "host", document{"caseId": runtime.CaseID(), "contractSha256": runtime.ContractSHA256()})
	runtime.mu.Unlock()
	if err != nil {
		runtime.fail(err)
		return err
	}
	go runtime.watch(ctx)
	return nil
}

func (runtime *Runtime) consume(ctx context.Context) {
	for event := range runtime.terminal.Events() {
		runtime.mu.Lock()
		if !runtime.active {
			runtime.mu.Unlock()
			continue
		}
		var err error
		var replies []string
		switch event.Kind {
		case "data":
			var data recording.TerminalData
			err = json.Unmarshal(event.Data, &data)
			if err == nil {
				replies, err = runtime.captureLocked(&data, event.Data, event.Raw)
			}
		case "exit":
			runtime.exited = true
			runtime.exitCode, runtime.exitSignal = event.ExitCode, event.Signal
			if event.Signal != nil && *event.Signal != 0 {
				runtime.exitCode = nil
			} else {
				runtime.exitSignal = nil
			}
			go func() {
				time.Sleep(30 * time.Millisecond)
				_, _ = runtime.Finish(context.Background(), "target_exit", false)
			}()
		default:
			err = event.Err
			if err == nil {
				err = problem("capture_failed", "The mechanical terminal adapter failed.")
			}
		}
		runtime.mu.Unlock()
		if err != nil {
			runtime.fail(err)
			continue
		}
		for _, reply := range replies {
			if err := runtime.terminal.Reply(ctx, reply); err != nil {
				runtime.fail(err)
				break
			}
		}
	}
}

func (runtime *Runtime) watch(ctx context.Context) {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-runtime.done:
			return
		case <-ctx.Done():
			_, _ = runtime.Finish(context.Background(), "signal", true)
			return
		case <-ticker.C:
			runtime.mu.Lock()
			reason := ""
			if runtime.active && runtime.stamp() >= runtime.contract.duration {
				reason = "duration_limit"
			} else if runtime.active && time.Since(runtime.lastActivity).Seconds() >= runtime.contract.idle {
				reason = "idle_limit"
			}
			runtime.mu.Unlock()
			if reason != "" {
				_, _ = runtime.Finish(context.Background(), reason, true)
				return
			}
		}
	}
}

func (runtime *Runtime) fail(err error) {
	runtime.mu.Lock()
	if runtime.captureError == nil {
		runtime.captureError = safeFault(err)
	}
	runtime.active = false
	runtime.pending, runtime.pendingBytes = nil, 0
	runtime.mu.Unlock()
	go func() { _, _ = runtime.Finish(context.Background(), "capture_failure", true) }()
}

// Finish stops only the owned terminal, then persists the actual case and capture outcomes.
func (runtime *Runtime) Finish(ctx context.Context, reason string, interrupted bool) (json.RawMessage, error) {
	runtime.finishMu.Lock()
	defer runtime.finishMu.Unlock()
	runtime.mu.Lock()
	if runtime.result != nil {
		result, err := slices.Clone(runtime.result), runtime.finishError
		runtime.mu.Unlock()
		return result, err
	}
	runtime.active = false
	cancel := runtime.cancelRun
	runtime.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	runtime.opMu.Lock()
	defer runtime.opMu.Unlock()
	runtime.mu.Lock()
	saved, err := os.ReadFile(filepath.Join(runtime.Workspace(), "contract.json"))
	if err != nil || string(saved) != string(runtime.contract.bytes) {
		runtime.captureError = &fault{Code: "contract_modified", Message: "The immutable contract no longer matches its saved bytes."}
	}
	if runtime.captureError == nil {
		if runtime.data != nil {
			_, err = runtime.captureLocked(runtime.data, runtime.dataJSON, "")
		}
		if err == nil {
			err = runtime.flushLocked()
		}
		if err != nil {
			runtime.captureError = safeFault(err)
		}
	}
	duration := runtime.stamp()
	runtime.mu.Unlock()
	cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	closeErr := runtime.terminal.Close(cleanup)
	cancel()
	finishedAt := time.Now()
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if closeErr != nil {
		runtime.captureError = &fault{Code: "cleanup_failed", Message: "The owned terminal did not close cleanly."}
	}
	if runtime.captureError == nil && (interrupted || runtime.exitCode == nil || runtime.exitSignal != nil) {
		runtime.captureError = &fault{Code: "incomplete_exit", Message: "Capture ended without a fully captured normal exit."}
	}
	status := "complete"
	if runtime.captureError != nil {
		status = "failed"
	}
	outcome := evaluate(runtime.contract, runtime.lastSafe, runtime.exitCode, status == "complete", interrupted, runtime.steps)
	value := document{
		"schemaVersion": 1, "caseId": runtime.CaseID(), "command": runtime.contract.command,
		"contractSha256": runtime.ContractSHA256(), "originalContract": "capture/contract.json",
		"finishedAt":      finishedAt.UTC().Format(time.RFC3339Nano),
		"durationSeconds": duration, "exitCode": runtime.exitCode, "exitSignal": runtime.exitSignal,
		"captureStatus": status, "caseStatus": outcome["caseStatus"], "expectations": outcome["expectations"],
		"stepsCompleted": runtime.steps, "expectedSteps": len(runtime.contract.steps), "actionsCompleted": runtime.actions,
		"stopReason": reason, "interrupted": interrupted, "captureError": runtime.captureError,
		"actionError": runtime.actionError, "cleanupError": closeErr != nil,
		"cleanupDurationSeconds": runtime.stamp() - duration, "finalObservation": runtime.lastSafe,
	}
	if !runtime.startedAt.IsZero() {
		value["startedAt"] = runtime.startedAt.UTC().Format(time.RFC3339Nano)
	}
	recordErr := runtime.eventLocked("finished", "host", document{"stopReason": reason, "captureStatus": status, "caseStatus": outcome["caseStatus"]})
	runtime.result, err = json.Marshal(value)
	if err == nil {
		err = cliutil.WriteJSON(filepath.Join(runtime.Workspace(), "result.json"), value)
	}
	for _, file := range runtime.files {
		err = errors.Join(err, file.Close())
	}
	runtime.finishError = errors.Join(err, recordErr)
	close(runtime.done)
	return slices.Clone(runtime.result), runtime.finishError
}
