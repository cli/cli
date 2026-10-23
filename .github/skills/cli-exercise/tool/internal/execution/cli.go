package execution

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/cli/cli/v2/cli-exercise/internal/engine"
	"github.com/cli/cli/v2/cli-exercise/internal/terminal"
)

type runOptions struct{ contract, preflight string }
type clientOptions struct {
	directory, file string
	timeout         float64
}

func parseRun(args []string) (runOptions, error) {
	var options runOptions
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.contract, "contract", "", "Immutable case contract")
	flags.StringVar(&options.preflight, "preflight", "", "Ready prerequisite receipt")
	if err := flags.Parse(args); err != nil {
		return options, err
	}
	if options.contract == "" || options.preflight == "" || len(flags.Args()) != 0 {
		return options, fmt.Errorf("run requires --contract and --preflight")
	}
	return options, nil
}

func parseClient(args []string) (clientOptions, error) {
	var options clientOptions
	flags := flag.NewFlagSet("client", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.directory, "run-dir", "", "Private run workspace")
	flags.StringVar(&options.file, "file", "", "Request file; otherwise stdin")
	flags.Float64Var(&options.timeout, "timeout", 60, "Response timeout in seconds")
	if err := flags.Parse(args); err != nil {
		return options, err
	}
	if options.directory == "" || len(flags.Args()) != 0 || options.timeout <= 0 || options.timeout > 86400 ||
		math.IsNaN(options.timeout) || math.IsInf(options.timeout, 0) {
		return options, fmt.Errorf("client requires --run-dir and a finite response timeout")
	}
	return options, nil
}

type runtimeStatus struct {
	SchemaVersion int    `json:"schemaVersion"`
	RunID         string `json:"runId"`
	Status        string `json:"status"`
	PID           int    `json:"pid"`
	CaseID        string `json:"caseId"`
	ContractHash  string `json:"contractSha256"`
	Transport     string `json:"transport"`
	Requests      string `json:"requests"`
	Responses     string `json:"responses"`
	Result        string `json:"result,omitempty"`
}

type envelope struct {
	ID      string          `json:"id"`
	RunID   string          `json:"runId"`
	Request json.RawMessage `json:"request"`
}

var requestName = regexp.MustCompile(`^[a-f0-9]{32}\.json$`)

func serve(ctx context.Context, runtime *engine.Runtime, streams cliutil.Streams) (json.RawMessage, error) {
	workspace := runtime.Workspace()
	requests, err := cliutil.ConfinedDirectory(workspace, "ipc/requests")
	if err != nil {
		return nil, err
	}
	responses, err := cliutil.ConfinedDirectory(workspace, "ipc/responses")
	if err != nil {
		return nil, err
	}
	runID, err := cliutil.NewRequestID()
	if err != nil {
		return nil, err
	}
	status := runtimeStatus{SchemaVersion: 1, RunID: runID, Status: "starting", PID: os.Getpid(),
		CaseID: runtime.CaseID(), ContractHash: runtime.ContractSHA256(), Transport: "private-file-ipc",
		Requests: "ipc/requests", Responses: "ipc/responses"}
	statusPath := filepath.Join(workspace, "runtime.json")
	if err := cliutil.WriteJSON(statusPath, status); err != nil {
		return nil, err
	}
	if err := runtime.Start(ctx); err != nil {
		_, finishErr := runtime.Finish(context.Background(), "launch_failed", true)
		status.Status, status.Result = "finished", "result.json"
		return nil, errors.Join(err, finishErr, cliutil.WriteJSON(statusPath, status))
	}
	status.Status = "ready"
	if err := cliutil.WriteJSON(statusPath, status); err != nil {
		return nil, err
	}
	if err := cliutil.JSON(streams.Out, map[string]any{"status": "ready", "runDir": workspace, "runId": runID}); err != nil {
		return nil, err
	}
	var loopErr error
loop:
	for {
		select {
		case <-runtime.Done():
			break loop
		default:
		}
		for _, directory := range []string{requests, responses} {
			info, err := os.Lstat(directory)
			if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				loopErr = fmt.Errorf("owned transport directory changed")
				break loop
			}
		}
		entries, err := os.ReadDir(requests)
		if err != nil {
			loopErr = err
			break
		}
		for _, item := range entries {
			if !requestName.MatchString(item.Name()) {
				continue
			}
			name := filepath.Join(requests, item.Name())
			id := strings.TrimSuffix(item.Name(), ".json")
			var request envelope
			info, err := os.Lstat(name)
			if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
				err = fmt.Errorf("transport request is not a regular private file")
			}
			if err == nil {
				err = cliutil.CheckPrivate(info)
			}
			if err == nil {
				_, err = cliutil.ReadJSON(name, 1<<20, &request)
			}
			if removeErr := os.Remove(name); removeErr != nil {
				loopErr = removeErr
				break loop
			}
			if err == nil && (request.ID != id || request.RunID != runID) {
				err = fmt.Errorf("transport request belongs to a different invocation")
			}
			var response map[string]any
			if err == nil {
				raw, handleErr := runtime.Handle(ctx, request.Request)
				err = handleErr
				if err == nil {
					err = json.Unmarshal(raw, &response)
				}
			}
			if err != nil {
				response = map[string]any{"ok": false, "error": map[string]string{
					"code": "transport_failure", "message": "The private request could not be processed."}}
			}
			response["id"], response["runId"] = id, runID
			if err := cliutil.WriteJSON(filepath.Join(responses, item.Name()), response); err != nil {
				loopErr = err
				break loop
			}
		}
		select {
		case <-runtime.Done():
			break loop
		case <-ctx.Done():
			loopErr = ctx.Err()
			break loop
		case <-time.After(10 * time.Millisecond):
		}
	}
	reason, interrupted := "host_shutdown", true
	select {
	case <-runtime.Done():
		reason, interrupted = "target_exit", false
	default:
	}
	result, finishErr := runtime.Finish(context.Background(), reason, interrupted)
	status.Status, status.Result = "finished", "result.json"
	statusErr := cliutil.WriteJSON(statusPath, status)
	entries, readErr := os.ReadDir(requests)
	if readErr == nil {
		for _, entry := range entries {
			if requestName.MatchString(entry.Name()) {
				if err := os.Remove(filepath.Join(requests, entry.Name())); err != nil {
					readErr = errors.Join(readErr, err)
				}
				response := map[string]any{"id": strings.TrimSuffix(entry.Name(), ".json"), "runId": runID,
					"ok": false, "error": map[string]string{"code": "run_closed", "message": "The run closed before this request was processed."}}
				readErr = errors.Join(readErr, cliutil.WriteJSON(filepath.Join(responses, entry.Name()), response))
			}
		}
	}
	return result, errors.Join(loopErr, finishErr, statusErr, readErr)
}

func runCommand(ctx context.Context, skillRoot string, options runOptions, streams cliutil.Streams) (int, error) {
	selected, cancel := context.WithTimeout(ctx, 20*time.Second)
	adapter, err := terminal.FromReceipt(selected, skillRoot, options.preflight)
	cancel()
	if err != nil {
		return 2, err
	}
	var contract struct {
		Environment struct {
			Pass []struct {
				Name   string `json:"name"`
				Secret bool   `json:"secret"`
			} `json:"pass"`
		} `json:"environment"`
	}
	if _, err := cliutil.ReadJSON(options.contract, 1<<20, &contract); err != nil {
		return 2, fmt.Errorf("the approved contract is unavailable")
	}
	passed, system := map[string]string{}, map[string]string{}
	for _, declaration := range contract.Environment.Pass {
		if value, exists := os.LookupEnv(declaration.Name); exists {
			passed[declaration.Name] = value
		}
	}
	var paths, homes []string
	cwd, err := os.Getwd()
	if err != nil {
		return 2, err
	}
	paths = append(paths, cwd)
	for _, key := range []string{"HOME", "USERPROFILE"} {
		if value := os.Getenv(key); filepath.IsAbs(value) {
			paths, homes = append(paths, value), append(homes, value)
		}
	}
	for _, key := range []string{"SystemRoot", "SystemDrive", "WINDIR"} {
		if value := os.Getenv(key); value != "" {
			system[key] = value
		}
	}
	runtime, err := engine.New(engine.Options{SkillRoot: skillRoot, ContractPath: options.contract,
		PassedEnvironment: passed, OperatorPaths: paths, OperatorHomes: homes, SystemEnvironment: system, Terminal: adapter})
	if err != nil {
		return 2, err
	}
	result, err := serve(ctx, runtime, streams)
	if err != nil {
		_, finishErr := runtime.Finish(context.Background(), "host_failure", true)
		return 2, errors.Join(err, finishErr)
	}
	var outcome struct {
		Status string `json:"caseStatus"`
	}
	if err := json.Unmarshal(result, &outcome); err != nil {
		return 2, err
	}
	switch outcome.Status {
	case "passed", "observed":
		return 0, nil
	case "failed":
		return 1, nil
	default:
		return 2, nil
	}
}

func requestStrings(value any) []string {
	switch value := value.(type) {
	case string:
		return []string{value}
	case []any:
		var values []string
		for _, item := range value {
			values = append(values, requestStrings(item)...)
		}
		return values
	case map[string]any:
		var values []string
		for key, item := range value {
			values = append(values, key)
			values = append(values, requestStrings(item)...)
		}
		return values
	default:
		return nil
	}
}

func clientCommand(ctx context.Context, options clientOptions, streams cliutil.Streams) (int, error) {
	var raw []byte
	var request map[string]any
	var err error
	if options.file != "" {
		raw, err = cliutil.ReadJSON(options.file, 1<<20, &request)
	} else {
		raw, err = io.ReadAll(io.LimitReader(streams.In, (1<<20)+1))
		if err == nil && len(raw) <= 1<<20 {
			err = json.Unmarshal(raw, &request)
		}
	}
	if err != nil || len(raw) > 1<<20 || request == nil {
		return 2, fmt.Errorf("client request must be one bounded JSON object")
	}
	workspace, err := filepath.Abs(options.directory)
	if err != nil {
		return 2, err
	}
	info, err := os.Lstat(workspace)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return 2, fmt.Errorf("run directory must be a real private directory")
	}
	if err := cliutil.CheckPrivate(info); err != nil {
		return 2, err
	}
	var status runtimeStatus
	if _, err := cliutil.ReadJSON(filepath.Join(workspace, "runtime.json"), 4<<20, &status); err != nil {
		return 2, err
	}
	var contract struct {
		Environment struct {
			Pass []struct {
				Name   string `json:"name"`
				Secret bool   `json:"secret"`
			} `json:"pass"`
		} `json:"environment"`
	}
	if _, err := cliutil.ReadJSON(filepath.Join(workspace, "contract.json"), 1<<20, &contract); err != nil {
		return 2, err
	}
	for _, declaration := range contract.Environment.Pass {
		secret := os.Getenv(declaration.Name)
		if declaration.Secret && secret != "" {
			for _, value := range requestStrings(request) {
				if strings.Contains(value, secret) {
					return 2, fmt.Errorf("do not put declared secret values into transport requests")
				}
			}
		}
	}
	op, _ := request["op"].(string)
	if status.Status == "finished" {
		if !slices.Contains([]string{"observe", "finish", "shutdown"}, op) {
			return 2, fmt.Errorf("the run is already closed; no input was submitted")
		}
		var result map[string]any
		if _, err := cliutil.ReadJSON(filepath.Join(workspace, "result.json"), 4<<20, &result); err != nil {
			return 2, err
		}
		return 0, cliutil.JSON(streams.Out, map[string]any{"ok": true, "result": result, "observation": result["finalObservation"]})
	}
	for _, relative := range []string{"ipc", "ipc/requests", "ipc/responses"} {
		path := filepath.Join(workspace, relative)
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return 2, fmt.Errorf("transport directory is unavailable or unsafe")
		}
		if err := cliutil.CheckPrivate(info); err != nil {
			return 2, err
		}
	}
	id, err := cliutil.NewRequestID()
	if err != nil {
		return 2, err
	}
	if err := cliutil.WriteJSON(filepath.Join(workspace, "ipc/requests", id+".json"),
		envelope{ID: id, RunID: status.RunID, Request: raw}); err != nil {
		return 2, err
	}
	responsePath := filepath.Join(workspace, "ipc/responses", id+".json")
	deadline := time.Now().Add(time.Duration(options.timeout * float64(time.Second)))
	for time.Now().Before(deadline) {
		if info, err := os.Lstat(responsePath); err == nil {
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				return 2, fmt.Errorf("transport response is unsafe")
			}
			if err := cliutil.CheckPrivate(info); err != nil {
				return 2, err
			}
			var response map[string]any
			if _, err := cliutil.ReadJSON(responsePath, 4<<20, &response); err != nil {
				return 2, err
			}
			if response["id"] != id || response["runId"] != status.RunID {
				return 2, fmt.Errorf("response belongs to a different request")
			}
			if err := cliutil.JSON(streams.Out, response); err != nil {
				return 2, err
			}
			if response["ok"] == true {
				return 0, nil
			}
			return 1, nil
		} else if !os.IsNotExist(err) {
			return 2, err
		}
		select {
		case <-ctx.Done():
			return 2, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	return 2, cliutil.JSON(streams.Out, map[string]any{"ok": false, "requestId": id, "error": map[string]string{
		"code": "client_timeout", "message": "The request may still complete; observe before retrying input."}})
}

// RunMain starts one Go policy owner with a selected mechanical adapter.
func RunMain(ctx context.Context, skillRoot string, args []string, streams cliutil.Streams) int {
	options, err := parseRun(args)
	if errors.Is(err, flag.ErrHelp) {
		_, err = fmt.Fprintln(streams.Out, "Usage: cli-exercise --skill-root PATH run --contract FILE --preflight FILE")
		if err == nil {
			return 0
		}
	}
	code := 2
	if err == nil {
		code, err = runCommand(ctx, skillRoot, options, streams)
	}
	if err != nil {
		if writeErr := cliutil.Error(streams.ErrOut, "run_blocked", err.Error()); writeErr != nil {
			return 2
		}
	}
	return code
}

// ClientMain sends exactly one request without retrying uncertain input delivery.
func ClientMain(ctx context.Context, args []string, streams cliutil.Streams) int {
	options, err := parseClient(args)
	if errors.Is(err, flag.ErrHelp) {
		_, err = fmt.Fprintln(streams.Out, "Usage: cli-exercise client --run-dir PATH [--file FILE] [--timeout SECONDS]")
		if err == nil {
			return 0
		}
	}
	code := 2
	if err == nil {
		code, err = clientCommand(ctx, options, streams)
	}
	if err != nil {
		if writeErr := cliutil.Error(streams.ErrOut, "client_failure", err.Error()); writeErr != nil {
			return 2
		}
	}
	return code
}
