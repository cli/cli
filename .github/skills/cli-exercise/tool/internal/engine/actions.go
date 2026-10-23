package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"
)

func (runtime *Runtime) readyLocked(revision int, reason string) error {
	if !runtime.active || runtime.exited {
		return problem("target_exited", "Input after the target exits or closes is forbidden.")
	}
	if revision != runtime.revision {
		return problem("stale_observation", "Observe the current UI before an effectful request.")
	}
	if strings.TrimSpace(reason) == "" {
		return problem("missing_reason", "Effectful requests need a reason.")
	}
	bytes, err := os.ReadFile(filepath.Join(runtime.Workspace(), "contract.json"))
	if err != nil || string(bytes) != string(runtime.contract.bytes) {
		return problem("contract_modified", "The immutable contract changed on disk.")
	}
	if runtime.data == nil {
		return problem("observation_unavailable", "Wait for the initial real terminal observation.")
	}
	return nil
}

func (runtime *Runtime) sleep(ctx context.Context, milliseconds float64) error {
	deadline := time.Now().Add(time.Duration(milliseconds * float64(time.Millisecond)))
	for time.Now().Before(deadline) {
		runtime.mu.Lock()
		active := runtime.active && !runtime.exited
		runtime.mu.Unlock()
		if !active {
			return problem("target_exited", "The target exited while waiting.")
		}
		timer := time.NewTimer(min(10*time.Millisecond, time.Until(deadline)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

func equivalentKey(key any) string {
	if str(key) == "linefeed" {
		return "enter"
	}
	chord, ok := stringList(key)
	if !ok || len(chord) != 2 || !slices.Contains(chord, "ctrl") {
		return ""
	}
	base := chord[0]
	if base == "ctrl" {
		base = chord[1]
	}
	return map[string]string{"m": "enter", "j": "enter", "i": "tab", "h": "backspace"}[base]
}

func (runtime *Runtime) physical(ctx context.Context, action, requested document, mode, reason string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	runtime.mu.Lock()
	if !runtime.active || runtime.exited {
		runtime.mu.Unlock()
		return problem("target_exited", "The target is unavailable for input.")
	}
	view, err := runtime.observationLocked()
	if err == nil {
		err = runtime.privacy.check(view, true)
	}
	if err == nil {
		err = runtime.contract.allowed(requested, view)
	}
	if err == nil {
		err = runtime.contract.allowed(action, view)
	}
	kind := str(action["type"])
	if err == nil && kind == "text" && slices.Contains([]string{"\r", "\n", "\t"}, str(action["text"])) {
		key := "enter"
		if str(action["text"]) == "\t" {
			key = "tab"
		}
		err = runtime.contract.allowed(document{"type": "key", "key": key}, view)
	}
	value := ""
	if kind == "text" {
		value = str(action["text"])
	} else if kind == "key" {
		key := str(action["key"])
		switch key {
		case "enter":
			value = "\r"
		case "linefeed":
			value = "\n"
		case "space":
			value = " "
		case "tab":
			value = "\t"
		default:
			if len(key) == 1 {
				value = key
			}
		}
	}
	if err == nil {
		candidate := runtime.inputWindow + value
		err = runtime.privacy.check([]any{candidate, strings.ReplaceAll(strings.ReplaceAll(candidate, "\r\n", "\n"), "\r", "\n")}, false)
		if err == nil {
			retained := 4096
			for _, secret := range runtime.contract.secrets {
				retained = max(retained, 2*len(secret))
			}
			runtime.inputWindow = candidate[max(0, len(candidate)-retained):]
			runtime.inputPending = runtime.privacy.pending(runtime.inputWindow)
			if !runtime.privacyPending() {
				err = runtime.flushLocked()
			}
		}
	}
	if err == nil {
		err = runtime.eventLocked("input", mode, document{"action": action, "requestedAction": requested,
			"reason": reason, "observation": view, "stepIndex": runtime.steps, "delivery": "attempted"})
	}
	if err == nil {
		runtime.inputAttempts++
	}
	runtime.mu.Unlock()
	if err != nil {
		if safeFault(err).Code == "privacy_failure" {
			runtime.fail(err)
		}
		return err
	}
	raw, err := marshal(action)
	if err == nil {
		err = runtime.terminal.Input(ctx, raw)
	}
	if err != nil {
		fault := problem("input_failed", "The terminal adapter could not deliver the requested input.")
		runtime.fail(fault)
		return fault
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	runtime.lastActivity = time.Now()
	return runtime.eventLocked("input_delivered", mode, document{"action": action, "stepIndex": runtime.steps})
}

func (runtime *Runtime) current() (document, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.observationLocked()
}

func uniqueOption(menu document, label string) (document, int, bool) {
	values, ok := menu["options"].([]any)
	if !ok || menu["supported"] != true {
		return nil, 0, false
	}
	index := -1
	var found document
	for position, value := range values {
		option, _ := object(value)
		if str(option["label"]) == label {
			if index != -1 {
				return nil, 0, false
			}
			found, index = option, position
		}
	}
	return found, index, found != nil
}

func (runtime *Runtime) semantic(ctx context.Context, action document, mode, reason string) error {
	view, err := runtime.current()
	if err != nil {
		return err
	}
	menu, _ := object(view["menu"])
	label, kind := str(action["label"]), str(action["type"])
	_, _, ok := uniqueOption(menu, label)
	if !ok || kind == "select" && str(menu["kind"]) != "line-select" || kind == "toggle" && str(menu["kind"]) != "line-checkbox" {
		return problem("unsupported_selector", "The label is not unique in a compatible line menu.")
	}
	options, _ := menu["options"].([]any)
	bound := min(100, 2*len(options))
	for moves := 0; str(menu["selected"]) != label; moves++ {
		if moves >= bound {
			return problem("unsupported_selector", "The bounded selector did not reach its label.")
		}
		_, to, ok := uniqueOption(menu, label)
		_, from, fromOK := uniqueOption(menu, str(menu["selected"]))
		if !ok || !fromOK {
			return problem("unsupported_selector", "Menu selection became ambiguous.")
		}
		key := "down"
		if to < from {
			key = "up"
		}
		previous := menu["selected"]
		if err := runtime.physical(ctx, document{"type": "key", "key": key}, action, mode, reason); err != nil {
			return err
		}
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			if err := runtime.sleep(ctx, 10); err != nil {
				return err
			}
			view, err = runtime.current()
			if err != nil {
				return err
			}
			menu, _ = object(view["menu"])
			if menu["supported"] == true && menu["selected"] != previous {
				break
			}
		}
		if _, _, ok := uniqueOption(menu, label); !ok || menu["selected"] == previous {
			return problem("unsupported_selector", "Menu movement could not be verified.")
		}
	}
	if kind == "select" {
		return runtime.physical(ctx, document{"type": "key", "key": "enter"}, action, mode, reason)
	}
	target, _, _ := uniqueOption(menu, label)
	if target["checked"] == action["checked"] {
		return nil
	}
	if err := runtime.physical(ctx, document{"type": "key", "key": "space"}, action, mode, reason); err != nil {
		return err
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if err := runtime.sleep(ctx, 10); err != nil {
			return err
		}
		view, err := runtime.current()
		if err != nil {
			return err
		}
		menu, _ := object(view["menu"])
		target, _, ok := uniqueOption(menu, label)
		if ok && menu["selected"] == label && target["checked"] == action["checked"] {
			return nil
		}
	}
	return problem("unsupported_selector", "The requested checkbox state could not be verified.")
}

func (runtime *Runtime) act(ctx context.Context, action document, revision int, reason, mode string) (document, error) {
	if err := validateAction(action, false); err != nil {
		return nil, err
	}
	if err := runtime.privacy.check(document{"action": action, "reason": reason}, true); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	err := runtime.readyLocked(revision, reason)
	if err == nil && runtime.actions >= runtime.contract.actions {
		err = problem("action_limit", "The approved action budget is exhausted.")
	}
	if err == nil && runtime.contract.mode == "exact" &&
		(runtime.steps >= len(runtime.contract.steps) || !reflect.DeepEqual(action, runtime.contract.steps[runtime.steps])) {
		err = problem("exact_mismatch", "The action differs from the next canonical step.")
	}
	timing, _ := object(action["timing"])
	lower, _ := number(timing["delayBeforeMs"])
	earliest := runtime.lastAction.Add(time.Duration(lower * float64(time.Millisecond)))
	runtime.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if delay := time.Until(earliest); delay > 0 {
		if err := runtime.sleep(ctx, float64(delay)/float64(time.Millisecond)); err != nil {
			return nil, err
		}
	}
	runtime.mu.Lock()
	err = runtime.readyLocked(revision, reason)
	if upper, ok := number(timing["maxDelayBeforeMs"]); err == nil && ok && float64(time.Since(runtime.lastAction))/float64(time.Millisecond) > upper {
		err = problem("timing_mismatch", "The caller's action timing window has elapsed.")
	}
	view, viewErr := runtime.observationLocked()
	err = errors.Join(err, viewErr)
	if err == nil {
		err = runtime.contract.allowed(action, view)
	}
	if err == nil && str(action["type"]) == "click" {
		capabilities, _ := object(view["capabilities"])
		x, _ := integer(action["x"])
		y, _ := integer(action["y"])
		columns, _ := integer(view["columns"])
		rows, _ := integer(view["rows"])
		if capabilities["click"] != true {
			err = problem("unsupported_action", "The target has not enabled supported mouse reporting.")
		} else if x >= columns || y >= rows {
			err = problem("invalid_action", "Click is outside the terminal.")
		}
	}
	attempts := runtime.inputAttempts
	if err == nil {
		runtime.actions++
	}
	runtime.mu.Unlock()
	if err != nil {
		return nil, err
	}
	switch str(action["type"]) {
	case "select", "toggle":
		err = runtime.semantic(ctx, action, mode, reason)
	case "text":
		delay, _ := number(timing["typingDelayMs"])
		for index, character := range []rune(str(action["text"])) {
			if index > 0 && delay > 0 {
				err = runtime.sleep(ctx, delay)
			}
			if err == nil {
				err = runtime.physical(ctx, document{"type": "text", "text": string(character)}, action, mode, reason)
			}
			if err != nil {
				break
			}
		}
	default:
		physical := document{}
		for key, value := range action {
			if key != "timing" {
				physical[key] = value
			}
		}
		if str(action["type"]) == "key" {
			physical["key"], err = normalizeKey(action["key"])
		}
		if err == nil {
			err = runtime.physical(ctx, physical, action, mode, reason)
		}
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err != nil {
		if runtime.contract.mode == "exact" && runtime.inputAttempts > attempts {
			runtime.actionError = safeFault(err)
			runtime.active = false
			go func() { _, _ = runtime.Finish(context.Background(), "partial_action", true) }()
		}
		return nil, err
	}
	runtime.lastAction = time.Now()
	if runtime.contract.mode == "exact" {
		runtime.steps++
	}
	if err := runtime.eventLocked("action_completed", mode, document{"action": action, "reason": reason, "stepsCompleted": runtime.steps}); err != nil {
		return nil, err
	}
	view, err = runtime.observationLocked()
	if runtime.privacyPending() || runtime.exited {
		view = runtime.lastSafe
	}
	return document{"ok": true, "observation": view, "revision": runtime.revision,
		"privacyPending": runtime.privacyPending(), "stepsCompleted": runtime.steps}, err
}

func (runtime *Runtime) controller(ctx context.Context, request document) (document, error) {
	source, ok := object(request["controller"])
	if !ok || !fields(source, "id", "rules", "maxActions", "maxDurationSeconds", "waitForMatchMs") {
		return nil, problem("invalid_controller", "Controllers are bounded declarative rule objects.")
	}
	rules, ok := source["rules"].([]any)
	if !ok || len(rules) > 1000 {
		return nil, problem("invalid_controller", "Invalid controller rules.")
	}
	maxActions := 100
	seconds := min(30.0, runtime.contract.duration)
	waitForMatch := 1000.0
	if value, exists := source["maxActions"]; exists {
		var ok bool
		maxActions, ok = integer(value)
		if !ok || maxActions < 1 || maxActions > 10000 {
			return nil, problem("invalid_controller", "Invalid controller action budget.")
		}
	}
	for key, target := range map[string]*float64{"maxDurationSeconds": &seconds, "waitForMatchMs": &waitForMatch} {
		if value, exists := source[key]; exists {
			parsed, ok := number(value)
			if !ok {
				return nil, problem("invalid_controller", "Invalid controller timing.")
			}
			*target = parsed
		}
	}
	if seconds <= 0 || seconds > runtime.contract.duration || waitForMatch < 0 || waitForMatch > 60000 {
		return nil, problem("invalid_controller", "Invalid controller limits.")
	}
	var actions []document
	ids := map[string]bool{}
	for _, value := range rules {
		rule, ok := object(value)
		action, _ := object(rule["action"])
		condition, _ := object(rule["when"])
		if !ok || !fields(rule, "id", "when", "action", "reason", "maxMatches") || !nonempty(rule["id"]) ||
			ids[str(rule["id"])] || !nonempty(rule["reason"]) {
			return nil, problem("invalid_controller", "Rules need unique IDs, conditions, actions, and reasons.")
		}
		ids[str(rule["id"])] = true
		if err := validateAction(action, false); err != nil {
			return nil, err
		}
		if err := validateCondition(condition); err != nil {
			return nil, err
		}
		if value, exists := rule["maxMatches"]; exists {
			count, ok := integer(value)
			if !ok || count < 1 || count > 10000 {
				return nil, problem("invalid_controller", "Invalid rule match budget.")
			}
		}
		actions = append(actions, action)
	}
	if err := runtime.privacy.check(source, true); err != nil {
		return nil, err
	}
	if err := runtime.privacy.check(literalActions(actions), false); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(source, "", "  ")
	if err != nil {
		return nil, err
	}
	raw = append(raw, '\n')
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	path := filepath.Join(runtime.Workspace(), "capture/controllers", hash+".json")
	if saved, err := os.ReadFile(path); err == nil {
		if string(saved) != string(raw) {
			return nil, problem("controller_modified", "A retained controller source changed.")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	} else {
		runtime.mu.Lock()
		if runtime.savedBytes+runtime.pendingBytes+len(raw) > 64<<20 {
			runtime.mu.Unlock()
			return nil, problem("capture_limit", "The controller exceeds the capture budget.")
		}
		runtime.savedBytes += len(raw)
		runtime.mu.Unlock()
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			return nil, err
		}
	}
	counts := runtime.controllers[hash]
	if counts == nil {
		counts = map[string]int{}
		runtime.controllers[hash] = counts
	}
	runtime.mu.Lock()
	err = runtime.eventLocked("controller_started", "controller", document{"sourceSha256": hash, "reason": request["reason"]})
	runtime.mu.Unlock()
	if err != nil {
		return nil, err
	}
	bounded, cancel := context.WithTimeout(ctx, time.Duration(seconds*float64(time.Second)))
	defer cancel()
	matchDeadline := time.Now().Add(time.Duration(waitForMatch * float64(time.Millisecond)))
	performed, pause := 0, "unhandled_state"
	for bounded.Err() == nil && performed < maxActions {
		runtime.mu.Lock()
		active := runtime.active && !runtime.exited
		view, err := runtime.observationLocked()
		runtime.mu.Unlock()
		if err != nil {
			return nil, err
		}
		if !active {
			pause = "target_exited"
			break
		}
		var matched document
		if view != nil {
			for _, value := range rules {
				rule, _ := object(value)
				limit := 1
				if configured, ok := integer(rule["maxMatches"]); ok {
					limit = configured
				}
				condition, _ := object(rule["when"])
				if counts[str(rule["id"])] < limit && matches(condition, view) {
					matched = rule
					break
				}
			}
		}
		if matched == nil {
			if !time.Now().Before(matchDeadline) {
				break
			}
			if err := runtime.sleep(bounded, 10); err != nil {
				pause = "target_exited"
				break
			}
			continue
		}
		runtime.mu.Lock()
		attempts := runtime.inputAttempts
		err = runtime.eventLocked("controller_matched", "controller", document{"sourceSha256": hash, "ruleId": matched["id"], "observation": view})
		runtime.mu.Unlock()
		if err != nil {
			return nil, err
		}
		action, _ := object(matched["action"])
		revision, _ := integer(view["revision"])
		if _, err := runtime.act(bounded, action, revision, str(matched["reason"]), "controller"); err != nil {
			if errors.Is(bounded.Err(), context.DeadlineExceeded) {
				runtime.mu.Lock()
				partial := runtime.inputAttempts > attempts
				runtime.mu.Unlock()
				if partial {
					return nil, problem("controller_limit", "The controller deadline interrupted an action; input may be partially delivered. Do not replay it blindly.")
				}
				pause = "controller_limit"
				break
			}
			return nil, err
		}
		counts[str(matched["id"])]++
		performed++
		matchDeadline = time.Now().Add(time.Duration(waitForMatch * float64(time.Millisecond)))
	}
	if performed == maxActions || errors.Is(bounded.Err(), context.DeadlineExceeded) {
		pause = "controller_limit"
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.active {
		if err := runtime.eventLocked("controller_paused", "controller", document{"sourceSha256": hash, "reason": pause, "actions": performed}); err != nil {
			return nil, err
		}
	}
	return document{"ok": true, "controllerStatus": "paused", "reason": pause, "sourceSha256": hash,
		"actions": performed, "observation": runtime.lastSafe, "revision": runtime.revision, "privacyPending": runtime.privacyPending()}, nil
}

// Handle serializes a non-secret external request through the shared Go policy gates.
func (runtime *Runtime) Handle(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	runtime.opMu.Lock()
	runtime.mu.Lock()
	runContext := runtime.runContext
	runtime.mu.Unlock()
	if runContext != nil {
		bounded, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(runContext, cancel)
		defer func() {
			stop()
			cancel()
		}()
		ctx = bounded
	}
	var request document
	err := json.Unmarshal(raw, &request)
	var response document
	if err != nil || request == nil {
		err = invalid("Requests must be bounded JSON objects.")
	} else if len(raw) > 1<<20 {
		err = invalid("The request exceeds its bounded size.")
	} else {
		err = runtime.privacy.check(request, true)
	}
	op := str(request["op"])
	allowed := map[string][]string{
		"observe": {"op"}, "wait": {"op", "until", "milliseconds", "timeoutMs"},
		"act": {"op", "revision", "reason", "action"}, "note": {"op", "chapter", "text"},
		"run_controller": {"op", "revision", "reason", "controller"},
		"finish":         {"op", "revision", "reason"}, "shutdown": {"op", "revision", "reason"},
	}
	if err == nil && (allowed[op] == nil || !fields(request, allowed[op]...)) {
		err = invalid("The request contains unsupported operations or fields.")
	}
	if err == nil {
		runtime.mu.Lock()
		active := runtime.active
		if op == "observe" {
			view := runtime.lastSafe
			if active && !runtime.privacyPending() {
				view, err = runtime.observationLocked()
			}
			response = document{"ok": true, "observation": view, "revision": runtime.revision,
				"privacyPending": runtime.privacyPending(), "result": runtime.result}
		} else if !active {
			err = problem("run_closed", "The bounded invocation is closed.")
		} else {
			runtime.lastActivity = time.Now()
		}
		runtime.mu.Unlock()
	}
	if err == nil && op != "observe" {
		switch op {
		case "act", "run_controller", "finish", "shutdown":
			revision, ok := integer(request["revision"])
			runtime.mu.Lock()
			if !ok {
				err = problem("stale_observation", "Use the latest real observation revision.")
			} else {
				err = runtime.readyLocked(revision, str(request["reason"]))
			}
			if err == nil && (op == "finish" || op == "shutdown") {
				runtime.active = false
			}
			runtime.mu.Unlock()
			if err == nil {
				if op == "act" {
					action, _ := object(request["action"])
					response, err = runtime.act(ctx, action, revision, str(request["reason"]), "ai")
				} else if op == "run_controller" {
					response, err = runtime.controller(ctx, request)
				} else {
					runtime.opMu.Unlock()
					result, finishErr := runtime.Finish(ctx, op, true)
					if finishErr != nil {
						return nil, finishErr
					}
					return marshal(document{"ok": true, "result": result})
				}
			}
		case "note":
			if _, ok := text(request["chapter"]); !ok {
				err = invalid("Notes require chapter and text strings.")
			} else if _, ok := text(request["text"]); !ok {
				err = invalid("Notes require chapter and text strings.")
			} else {
				runtime.mu.Lock()
				err = runtime.eventLocked("note", "ai", document{"chapter": request["chapter"], "text": request["text"]})
				runtime.mu.Unlock()
				response = document{"ok": true}
			}
		case "wait":
			response, err = runtime.wait(ctx, request)
		}
	}
	if err != nil {
		failure := safeFault(err)
		if slices.Contains([]string{"capture_limit", "contract_modified"}, failure.Code) {
			runtime.fail(err)
		}
		runtime.mu.Lock()
		if runtime.active {
			if recordErr := runtime.eventLocked("request_rejected", "host", document{"code": failure.Code}); recordErr != nil {
				runtime.mu.Unlock()
				runtime.fail(recordErr)
				runtime.opMu.Unlock()
				return nil, recordErr
			}
		}
		runtime.mu.Unlock()
		response = document{"ok": false, "error": failure}
	}
	runtime.opMu.Unlock()
	return marshal(response)
}

func (runtime *Runtime) wait(ctx context.Context, request document) (document, error) {
	timeout := 5000.0
	if value, exists := request["timeoutMs"]; exists {
		var ok bool
		timeout, ok = number(value)
		if !ok || timeout < 0 || timeout > 2147483647 {
			return nil, invalid("Invalid wait timeout.")
		}
	}
	until, hasUntil := request["until"]
	milliseconds, hasDuration := request["milliseconds"]
	if hasUntil == hasDuration {
		return nil, invalid("Wait for one literal condition or duration.")
	}
	condition, _ := object(until)
	duration := 0.0
	if hasUntil {
		if err := validateCondition(condition); err != nil {
			return nil, err
		}
	} else {
		var ok bool
		duration, ok = number(milliseconds)
		if !ok || duration < 0 || duration > timeout {
			return nil, invalid("Invalid wait duration.")
		}
	}
	start := time.Now()
	for time.Since(start) <= time.Duration(timeout*float64(time.Millisecond)) {
		runtime.mu.Lock()
		view, err := runtime.observationLocked()
		active, pending, revision := runtime.active, runtime.privacyPending(), runtime.revision
		runtime.mu.Unlock()
		if err != nil {
			return nil, err
		}
		if !pending && view != nil && (hasUntil && matches(condition, view) || hasDuration && float64(time.Since(start))/float64(time.Millisecond) >= duration) {
			return document{"ok": true, "observation": view, "revision": revision, "privacyPending": false}, nil
		}
		if !active {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	return nil, problem("wait_timeout", "The requested wait did not complete.")
}
