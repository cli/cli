package terminal

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/cli/cli/v2/cli-exercise/internal/recording"
)

type adapterMessage struct {
	Type     string          `json:"type"`
	ID       string          `json:"id"`
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Raw      string          `json:"raw"`
	ExitCode *int            `json:"exitCode"`
	Signal   *int            `json:"signal"`
	PID      *int            `json:"pid"`
}

const adapterEventLimit = 64

// Adapter delivers mechanical requests and real events through the selected Node bridge.
type Adapter struct {
	node, entry, script string
	command             *exec.Cmd
	input               io.WriteCloser
	events              chan Event
	done                chan struct{}
	mu                  sync.Mutex
	writeMu             sync.Mutex
	pending             map[string]chan adapterMessage
	waitErr             error
	closing             atomic.Bool
	processGroup        atomic.Int64
	cleanupMu           sync.Mutex
}

// New prepares an adapter without starting a process.
func New(node, entry, script string) *Adapter {
	return &Adapter{
		node: node, entry: entry, script: script, events: make(chan Event, adapterEventLimit+1),
		done: make(chan struct{}), pending: map[string]chan adapterMessage{},
	}
}

// Events returns the ordered, bounded terminal event stream.
func (adapter *Adapter) Events() <-chan Event { return adapter.events }

// Start launches only the selected adapter and target.
func (adapter *Adapter) Start(ctx context.Context, launch LaunchOptions) error {
	if adapter.command != nil {
		return fmt.Errorf("terminal adapter was already started")
	}
	command := exec.Command(adapter.node, adapter.script, adapter.entry)
	command.Dir = launch.Cwd
	command.Env = []string{}
	for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME",
		"XDG_CACHE_HOME", "TMPDIR", "TMP", "TEMP", "SystemRoot", "SystemDrive", "WINDIR", "TERM", "COLORTERM"} {
		if value, exists := launch.Env[key]; exists {
			command.Env = append(command.Env, key+"="+value)
		}
	}
	cliutil.OwnProcessGroup(command)
	input, err := command.StdinPipe()
	if err != nil {
		return err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		return errors.Join(err, input.Close())
	}
	command.Stderr = io.Discard
	adapter.command, adapter.input = command, input
	if err := command.Start(); err != nil {
		return errors.Join(err, input.Close(), output.Close())
	}
	go adapter.read(output)
	return adapter.request(ctx, map[string]any{"op": "open", "launch": launch})
}

func (adapter *Adapter) read(output io.Reader) {
	defer close(adapter.done)
	defer close(adapter.events)
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 64<<10), 64<<20)
	var backlogErr error
	for scanner.Scan() {
		var message adapterMessage
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			adapter.waitErr = fmt.Errorf("terminal adapter emitted malformed protocol data")
			break
		}
		if message.Type == "response" {
			adapter.mu.Lock()
			response := adapter.pending[message.ID]
			adapter.mu.Unlock()
			if response != nil {
				response <- message
			}
			continue
		}
		if message.Type == "process_group" {
			if message.PID == nil || *message.PID < 0 || *message.PID == 1 || *message.PID == adapter.command.Process.Pid {
				adapter.waitErr = fmt.Errorf("terminal adapter emitted an invalid owned process group")
				break
			}
			previous := adapter.processGroup.Load()
			if *message.PID != 0 && previous != 0 && previous != int64(*message.PID) {
				adapter.waitErr = fmt.Errorf("terminal adapter changed its owned process group")
				break
			}
			adapter.processGroup.Store(int64(*message.PID))
			continue
		}
		if backlogErr != nil {
			continue
		}
		event := Event{Kind: message.Type, Data: message.Data, Raw: message.Raw,
			ExitCode: message.ExitCode, Signal: message.Signal}
		if message.Type != "data" && message.Type != "exit" {
			event.Kind, event.Err = "error", fmt.Errorf("terminal adapter could not capture its target")
		}
		// Only this reader sends events; reserve a slot for failure rather than blocking response decoding.
		if len(adapter.events) >= adapterEventLimit {
			backlogErr = fmt.Errorf("terminal event backlog exceeded %d queued events; capture is incomplete", adapterEventLimit)
			adapter.events <- Event{Kind: "error", Err: backlogErr}
		} else {
			adapter.events <- event
		}
	}
	if err := scanner.Err(); err != nil {
		adapter.waitErr = fmt.Errorf("terminal adapter protocol stream failed: %w", err)
	}
	adapter.waitErr = errors.Join(adapter.waitErr, adapter.killTerminalGroup())
	if adapter.waitErr != nil {
		adapter.waitErr = errors.Join(adapter.waitErr, cliutil.KillOwned(adapter.command))
	}
	waitErr := adapter.command.Wait()
	adapter.waitErr = errors.Join(adapter.waitErr, waitErr, backlogErr)
	if !adapter.closing.Load() && backlogErr == nil {
		adapter.events <- Event{Kind: "error", Err: fmt.Errorf("terminal adapter ended before owned cleanup")}
	}
}

func (adapter *Adapter) killTerminalGroup() error {
	adapter.cleanupMu.Lock()
	defer adapter.cleanupMu.Unlock()
	group := adapter.processGroup.Load()
	if group == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3500*time.Millisecond)
	defer cancel()
	if err := cliutil.KillProcessGroup(ctx, int(group)); err != nil {
		return err
	}
	adapter.processGroup.CompareAndSwap(group, 0)
	return nil
}

func (adapter *Adapter) request(ctx context.Context, request map[string]any) error {
	id, err := cliutil.NewRequestID()
	if err != nil {
		return err
	}
	request["id"] = id
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return fmt.Errorf("terminal adapter request exceeds its bounded size")
	}
	response := make(chan adapterMessage, 1)
	adapter.mu.Lock()
	adapter.pending[id] = response
	adapter.mu.Unlock()
	defer func() {
		adapter.mu.Lock()
		delete(adapter.pending, id)
		adapter.mu.Unlock()
	}()
	adapter.writeMu.Lock()
	_, err = adapter.input.Write(append(data, '\n'))
	adapter.writeMu.Unlock()
	if err != nil {
		return fmt.Errorf("terminal adapter input delivery failed")
	}
	select {
	case result := <-response:
		if !result.OK {
			return fmt.Errorf("terminal adapter rejected a mechanical operation")
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("terminal adapter operation did not complete: %w", ctx.Err())
	case <-adapter.done:
		return fmt.Errorf("terminal adapter closed during an operation")
	}
}

// Input delivers an already validated physical action.
func (adapter *Adapter) Input(ctx context.Context, action json.RawMessage) error {
	return adapter.request(ctx, map[string]any{"op": "input", "action": action})
}

// Reply delivers a literal terminal protocol response.
func (adapter *Adapter) Reply(ctx context.Context, text string) error {
	return adapter.request(ctx, map[string]any{"op": "reply", "text": text})
}

// Close stops and reaps the owned adapter and terminal.
func (adapter *Adapter) Close(ctx context.Context) error {
	if adapter.command == nil || adapter.command.Process == nil {
		return nil
	}
	adapter.closing.Store(true)
	select {
	case <-adapter.done:
		return adapter.waitErr
	default:
	}
	requestErr := adapter.request(ctx, map[string]any{"op": "close"})
	closeErr := adapter.input.Close()
	select {
	case <-adapter.done:
		return errors.Join(requestErr, closeErr, adapter.waitErr)
	case <-ctx.Done():
		killErr := errors.Join(adapter.killTerminalGroup(), cliutil.KillOwned(adapter.command))
		select {
		case <-adapter.done:
			return errors.Join(ctx.Err(), requestErr, closeErr, killErr, adapter.waitErr)
		case <-time.After(5 * time.Second):
			return errors.Join(ctx.Err(), killErr, fmt.Errorf("owned adapter did not stop"))
		}
	}
}

// FromReceipt validates selected tools before creating the production adapter.
func FromReceipt(ctx context.Context, skillRoot, receiptPath string) (*Adapter, error) {
	if !cliutil.ManagedProcesses() {
		return nil, fmt.Errorf("owned terminal process cleanup is not implemented on this platform")
	}
	var receipt recording.Receipt
	if _, err := cliutil.ReadJSON(receiptPath, 4<<20, &receipt); err != nil {
		return nil, fmt.Errorf("ready preflight receipt is unavailable")
	}
	hash, err := cliutil.ManifestHash(skillRoot)
	if err != nil || receipt.SchemaVersion != 1 || receipt.Status != "ready" || receipt.Manifest != hash {
		return nil, fmt.Errorf("preflight receipt does not match the pinned manifests")
	}
	node := receipt.Tools.Node
	if node == nil || receipt.Tools.Tuistory == nil || !filepath.IsAbs(node.Path) || !filepath.IsAbs(receipt.Tools.Tuistory.ModuleRoot) {
		return nil, fmt.Errorf("preflight must select absolute Node and Tuistory paths")
	}
	actual, err := cliutil.SHA256File(node.Path)
	if err != nil || node.SHA256 != "" && actual != node.SHA256 {
		return nil, fmt.Errorf("selected Node executable no longer matches preflight")
	}
	command := exec.CommandContext(ctx, node.Path, "--version")
	command.Env = []string{}
	version, err := command.Output()
	if err != nil || string(bytes.TrimSpace(version)) != node.Version {
		return nil, fmt.Errorf("selected Node version no longer matches preflight")
	}
	var pinned struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if _, err := cliutil.ReadJSON(filepath.Join(skillRoot, "scripts/package.json"), 1<<20, &pinned); err != nil {
		return nil, err
	}
	if receipt.Tools.Tuistory.Version != pinned.Dependencies["tuistory"] {
		return nil, fmt.Errorf("selected Tuistory no longer matches the pinned receipt")
	}
	entry, err := ResolveEntry(receipt.Tools.Tuistory.ModuleRoot, pinned.Dependencies["tuistory"])
	if err != nil {
		return nil, err
	}
	return New(node.Path, entry, filepath.Join(skillRoot, "scripts/terminal.mjs")), nil
}

// ResolveEntry confines module resolution to the selected, version-matched installation.
func ResolveEntry(moduleRoot, expectedVersion string) (string, error) {
	packageRoot := filepath.Join(moduleRoot, "tuistory")
	var metadata struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Main    string `json:"main"`
		Exports map[string]struct {
			Default string `json:"default"`
		} `json:"exports"`
	}
	if _, err := cliutil.ReadJSON(filepath.Join(packageRoot, "package.json"), 1<<20, &metadata); err != nil {
		return "", err
	}
	if metadata.Name != "tuistory" || metadata.Version != expectedVersion {
		return "", fmt.Errorf("selected Tuistory no longer matches the pinned version")
	}
	entry := metadata.Exports["."].Default
	if entry == "" {
		entry = metadata.Main
	}
	if entry == "" || filepath.IsAbs(entry) {
		return "", fmt.Errorf("Tuistory has no supported relative Node entry point")
	}
	packageRoot, err := filepath.EvalSymlinks(packageRoot)
	if err != nil {
		return "", err
	}
	entry, err = filepath.EvalSymlinks(filepath.Join(packageRoot, entry))
	if err != nil || !cliutil.Inside(packageRoot, entry) {
		return "", fmt.Errorf("Tuistory entry leaves its selected package")
	}
	return entry, nil
}
