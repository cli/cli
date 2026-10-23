package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/cli/cli/v2/cli-exercise/internal/recording"
)

type document = map[string]any

type fault struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (fault *fault) Error() string { return fault.Message }
func problem(code, message string) error {
	return &fault{Code: code, Message: message}
}

func object(value any) (document, bool) { result, ok := value.(map[string]any); return result, ok }
func text(value any) (string, bool) {
	result, ok := value.(string)
	return result, ok && !strings.ContainsRune(result, 0)
}
func str(value any) string { result, _ := text(value); return result }
func number(value any) (float64, bool) {
	var result float64
	ok := true
	switch value := value.(type) {
	case float64:
		result = value
	case int:
		result = float64(value)
	case int64:
		result = float64(value)
	default:
		ok = false
	}
	return result, ok && !math.IsInf(result, 0) && !math.IsNaN(result)
}
func integer(value any) (int, bool) {
	result, ok := number(value)
	if !ok || result != math.Trunc(result) || math.Abs(result) > min(float64(math.MaxInt), 9007199254740991) {
		return 0, false
	}
	return int(result), true
}
func nonempty(value any) bool {
	result, ok := text(value)
	return ok && strings.TrimSpace(result) != ""
}
func fields(value document, allowed ...string) bool {
	for key := range value {
		if !slices.Contains(allowed, key) {
			return false
		}
	}
	return true
}
func stringList(value any) ([]string, bool) {
	list, ok := value.([]any)
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(list))
	for _, item := range list {
		value, ok := text(item)
		if !ok {
			return nil, false
		}
		result = append(result, value)
	}
	return result, true
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var checksum = regexp.MustCompile(`^[a-f0-9]{64}$`)
var hexColor = regexp.MustCompile(`^#[a-fA-F0-9]{6}$`)
var reserved = []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_CACHE_HOME",
	"XDG_DATA_HOME", "XDG_STATE_HOME", "TMPDIR", "TMP", "TEMP", "TERMCAST_DB_SUFFIX"}
var modifiers = []string{"ctrl", "alt", "shift", "meta"}
var namedKeys = []string{"enter", "return", "esc", "escape", "tab", "space", "backspace", "delete", "insert",
	"up", "down", "left", "right", "home", "end", "pageup", "pagedown", "clear", "linefeed",
	"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12"}

func normalizeKey(value any) (any, error) {
	var parts []string
	if single, ok := text(value); ok {
		parts = []string{single}
	} else {
		var ok bool
		parts, ok = stringList(value)
		if !ok {
			return nil, problem("invalid_action", "A key must be a name or modifier chord.")
		}
	}
	if len(parts) == 0 || len(parts) > 5 {
		return nil, problem("invalid_action", "Use separate actions for a sequence of keys.")
	}
	base := ""
	seen := map[string]bool{}
	var result []any
	for _, part := range parts {
		if seen[part] || part == "" {
			return nil, problem("invalid_action", "Key chords cannot repeat keys.")
		}
		seen[part] = true
		if slices.Contains(modifiers, part) {
			result = append(result, part)
		} else if base == "" {
			base = part
		} else {
			return nil, problem("invalid_action", "Use separate actions for a sequence of keys.")
		}
	}
	if !slices.Contains(namedKeys, base) && !(len(base) == 1 && base[0] >= '!' && base[0] <= '~') {
		return nil, problem("invalid_action", "The requested key is unsupported.")
	}
	if base == "return" {
		base = "enter"
	} else if base == "escape" {
		base = "esc"
	}
	if seen["meta"] {
		return nil, problem("invalid_action", "The selected terminal adapter does not encode Meta modifiers.")
	}
	letter := len(base) == 1 && (base[0] >= 'a' && base[0] <= 'z' || base[0] >= 'A' && base[0] <= 'Z')
	modifiedSpecial := slices.Contains([]string{"enter", "tab", "backspace", "esc"}, base)
	if seen["ctrl"] && !modifiedSpecial {
		if !letter || seen["alt"] || seen["shift"] {
			return nil, problem("invalid_action", "Ctrl requires a letter without other modifiers, or Enter, Tab, Backspace, or Escape.")
		}
		base = strings.ToLower(base)
	} else if seen["shift"] && !modifiedSpecial {
		if !letter {
			return nil, problem("invalid_action", "The selected terminal adapter does not encode Shift on this key.")
		}
		base = strings.ToUpper(base)
		result = slices.DeleteFunc(result, func(value any) bool { return value == "shift" })
	}
	if len(result) == 0 {
		return base, nil
	}
	return append(result, base), nil
}

var actionFields = map[string][]string{
	"text": {"type", "text", "timing"}, "key": {"type", "key", "timing"},
	"select": {"type", "label", "timing"}, "toggle": {"type", "label", "checked", "timing"},
	"resize": {"type", "columns", "rows", "timing"}, "click": {"type", "x", "y", "timing"},
}

func validateAction(action document, partial bool) error {
	fail := func() error { return problem("invalid_action", "The action contains unsupported fields or values.") }
	if action == nil {
		return fail()
	}
	kind := str(action["type"])
	if partial {
		for key := range action {
			found := false
			for _, names := range actionFields {
				found = found || slices.Contains(names, key)
			}
			if !found {
				return fail()
			}
		}
		if _, exists := action["type"]; exists && actionFields[kind] == nil {
			return fail()
		}
		if value, exists := action["key"]; exists {
			if _, err := normalizeKey(value); err != nil {
				return err
			}
		}
		for _, key := range []string{"text", "label"} {
			if value, exists := action[key]; exists {
				if _, ok := text(value); !ok {
					return fail()
				}
			}
		}
		if value, exists := action["checked"]; exists {
			if _, ok := value.(bool); !ok {
				return fail()
			}
		}
		return nil
	}
	if actionFields[kind] == nil || !fields(action, actionFields[kind]...) {
		return fail()
	}
	switch kind {
	case "text":
		value, ok := text(action["text"])
		if !ok || len(utf16.Encode([]rune(value))) > 65536 {
			return fail()
		}
		for _, character := range value {
			if character < 32 && character != '\t' && character != '\n' && character != '\r' || character == 127 {
				return fail()
			}
		}
	case "key":
		if _, err := normalizeKey(action["key"]); err != nil {
			return err
		}
	case "select", "toggle":
		if !nonempty(action["label"]) {
			return fail()
		}
		if kind == "toggle" {
			if _, ok := action["checked"].(bool); !ok {
				return fail()
			}
		}
	case "resize":
		for _, key := range []string{"columns", "rows"} {
			value, ok := integer(action[key])
			if !ok || value < 1 || value > 1000 {
				return fail()
			}
		}
	case "click":
		for _, key := range []string{"x", "y"} {
			value, ok := integer(action[key])
			if !ok || value < 0 {
				return fail()
			}
		}
	}
	if value, exists := action["timing"]; exists {
		timing, ok := object(value)
		if !ok || !fields(timing, "delayBeforeMs", "maxDelayBeforeMs", "typingDelayMs") {
			return fail()
		}
		for key, value := range timing {
			number, ok := number(value)
			if !ok || number < 0 || number > 2147483647 || key == "typingDelayMs" && kind != "text" {
				return fail()
			}
		}
		lower, _ := number(timing["delayBeforeMs"])
		if upper, ok := number(timing["maxDelayBeforeMs"]); ok && upper < lower {
			return fail()
		}
	}
	return nil
}

func validateCondition(condition document) error {
	if condition == nil || !fields(condition, "screenContains", "questionPrefix", "selected") {
		return problem("invalid_condition", "Conditions use only literal screenContains, questionPrefix, and selected strings.")
	}
	for _, value := range condition {
		if _, ok := text(value); !ok {
			return problem("invalid_condition", "Condition values must be literal strings.")
		}
	}
	return nil
}

type runContract struct {
	doc                            document
	command, terminal              document
	id, mode, goal                 string
	workspace, cwd, stateDirectory string
	env                            map[string]string
	secrets                        []string
	steps, expectations            []document
	denies                         []document
	actions                        int
	duration, idle                 float64
	bytes                          []byte
	hash                           string
}

func parseContract(options Options) (*runContract, error) {
	fail := func(message string) (*runContract, error) { return nil, problem("invalid_contract", message) }
	var source document
	raw, err := cliutil.ReadJSON(options.ContractPath, 1<<20, &source)
	if err != nil || source == nil {
		return fail("The contract must be a bounded readable JSON object.")
	}
	version, ok := integer(source["schemaVersion"])
	if !ok || version != 1 || !nonempty(source["caseId"]) || !nonempty(source["goal"]) ||
		!slices.Contains([]string{"exact", "explore"}, str(source["mode"])) {
		return fail("The original case identity and fidelity mode are required.")
	}
	origin, ok := object(source["source"])
	if !ok || !nonempty(origin["kind"]) || !nonempty(origin["text"]) {
		return fail("Preserve the original source request.")
	}
	approval, ok := object(source["authorization"])
	_, effectsOK := stringList(approval["effects"])
	if !ok || str(approval["status"]) != "approved" || !nonempty(approval["basis"]) || !effectsOK {
		return fail("An explicitly approved effect scope is required.")
	}
	command, ok := object(source["command"])
	_, argsOK := stringList(command["args"])
	if !ok || !fields(command, "executable", "args", "sha256", "version", "cwd") ||
		!filepath.IsAbs(str(command["executable"])) || !argsOK || !checksum.MatchString(str(command["sha256"])) ||
		!nonempty(command["version"]) || !nonempty(command["cwd"]) || filepath.IsAbs(str(command["cwd"])) ||
		slices.Contains(strings.FieldsFunc(str(command["cwd"]), func(r rune) bool { return r == '/' || r == '\\' }), "..") {
		return fail("The exact command identity, arguments, and confined cwd are required.")
	}
	workspace := str(source["workspace"])
	if !filepath.IsAbs(workspace) {
		return fail("The private workspace must be absolute.")
	}
	stateDirectory := workspace
	if value, exists := source["stateDirectory"]; exists {
		if !nonempty(value) || !filepath.IsAbs(str(value)) {
			return fail("Shared stateDirectory must be an explicit absolute path.")
		}
		stateDirectory = str(value)
	}
	terminal, ok := object(source["terminal"])
	if !ok || !hexColor.MatchString(str(terminal["background"])) || !hexColor.MatchString(str(terminal["foreground"])) {
		return fail("Terminal dimensions and colors are required.")
	}
	for _, key := range []string{"columns", "rows"} {
		value, ok := integer(terminal[key])
		if !ok || value < 1 || value > 1000 {
			return fail("Terminal dimensions must be between 1 and 1000 cells.")
		}
	}
	size, sizeOK := number(terminal["fontSize"])
	fps, fpsOK := integer(terminal["fps"])
	if !sizeOK || size <= 0 || size > 512 || !fpsOK || fps < 1 || fps > 60 {
		return fail("The font size and integer recording cadence must be supported.")
	}
	if _, exists := terminal["fontPath"]; exists {
		return fail("Terminal font choices belong to evidence rendering, not the execution contract.")
	}
	if _, exists := terminal["fontFallbacks"]; exists {
		return fail("Terminal font choices belong to evidence rendering, not the execution contract.")
	}
	limits, ok := object(source["limits"])
	actions, actionsOK := integer(limits["maxActions"])
	duration, durationOK := number(limits["maxDurationSeconds"])
	idle, idleOK := number(limits["idleTimeoutSeconds"])
	if !ok || !fields(limits, "maxActions", "maxDurationSeconds", "idleTimeoutSeconds") || !actionsOK || actions < 0 ||
		!durationOK || duration <= 0 || duration > 2147483 || !idleOK || idle <= 0 || idle > 2147483 {
		return fail("Finite action, duration, and idle budgets are required.")
	}
	result := &runContract{doc: source, command: command, terminal: terminal, id: str(source["caseId"]),
		mode: str(source["mode"]), goal: str(source["goal"]), workspace: workspace, actions: actions,
		duration: duration, idle: idle, bytes: raw, env: map[string]string{}, stateDirectory: stateDirectory}
	steps, ok := source["steps"].([]any)
	if !ok {
		return fail("Preserve canonical steps, including an empty list.")
	}
	for _, value := range steps {
		action, _ := object(value)
		if err := validateAction(action, false); err != nil {
			return nil, err
		}
		result.steps = append(result.steps, action)
	}
	constraints, ok := object(source["constraints"])
	denies, deniesOK := constraints["deny"].([]any)
	if !ok || !fields(constraints, "deny") || !deniesOK {
		return fail("Explicit action constraints are required.")
	}
	for _, value := range denies {
		deny, ok := object(value)
		action, _ := object(deny["action"])
		if !ok || !fields(deny, "action", "when", "reason") {
			return fail("Invalid deny constraint.")
		}
		if err := validateAction(action, true); err != nil {
			return nil, err
		}
		if value, exists := deny["when"]; exists {
			condition, _ := object(value)
			if err := validateCondition(condition); err != nil {
				return nil, err
			}
		}
		if reason, exists := deny["reason"]; exists && !nonempty(reason) {
			return fail("Denial reasons must be nonempty strings.")
		}
		result.denies = append(result.denies, deny)
	}
	expectations, ok := source["expectations"].([]any)
	if !ok {
		return fail("Preserve the supplied expectations.")
	}
	ids := map[string]bool{}
	for _, value := range expectations {
		expected, ok := object(value)
		if !ok || !nonempty(expected["id"]) || !nonempty(expected["type"]) || ids[str(expected["id"])] {
			return fail("Expectations require unique IDs and types.")
		}
		ids[str(expected["id"])] = true
		if str(expected["type"]) == "exit_code" {
			if _, ok := integer(expected["value"]); !ok {
				return fail("Exit expectations require integer values.")
			}
		}
		if slices.Contains([]string{"screen_contains", "screen_not_contains"}, str(expected["type"])) {
			if _, ok := text(expected["value"]); !ok {
				return fail("Screen expectations require literal strings.")
			}
		}
		result.expectations = append(result.expectations, expected)
	}
	output, ok := object(source["output"])
	var presentation struct {
		Output recording.OutputOptions `json:"output"`
	}
	if err := json.Unmarshal(raw, &presentation); err != nil || !ok || len(presentation.Output.Formats) == 0 {
		return fail("Preserve the requested media formats and timing.")
	}
	for _, format := range presentation.Output.Formats {
		if !slices.Contains([]string{"gif", "mp4"}, format) {
			return fail("Unsupported output format.")
		}
	}
	captions := true
	if value, exists := output["captions"]; exists {
		var ok bool
		captions, ok = value.(bool)
		if !ok {
			return fail("Captions must be boolean.")
		}
	}
	if presentation.Output.Timing == "condensed" && !captions {
		return fail("Unannotated output preserves real timing; use realtime instead of condensed.")
	}
	environment, ok := object(source["environment"])
	values, valuesOK := object(environment["values"])
	pass, passOK := environment["pass"].([]any)
	if !ok || !fields(environment, "values", "pass") || !valuesOK || !passOK {
		return fail("Explicit environment values and pass-through declarations are required.")
	}
	for name, value := range values {
		stringValue, ok := text(value)
		if !ok || !envName.MatchString(name) || slices.Contains(reserved, strings.ToUpper(name)) {
			return fail("Environment values cannot override isolation.")
		}
		result.env[name] = stringValue
	}
	for _, value := range pass {
		declaration, ok := object(value)
		name := str(declaration["name"])
		secret, secretOK := declaration["secret"].(bool)
		if _, exists := result.env[name]; exists || !ok || !secretOK || !fields(declaration, "name", "secret") ||
			!envName.MatchString(name) || slices.Contains(reserved, strings.ToUpper(name)) || slices.Contains([]string{"TERM", "COLORTERM"}, name) {
			return fail("Pass-through declarations must be explicit, unique, and isolated.")
		}
		actual, exists := options.PassedEnvironment[name]
		if !exists || strings.ContainsRune(actual, 0) || secret && actual == "" {
			return nil, problem("missing_environment", "An explicitly requested environment value is missing.")
		}
		result.env[name] = actual
		if secret {
			result.secrets = append(result.secrets, actual)
		}
	}
	for name, required := range map[string]string{"TERM": "xterm-truecolor", "COLORTERM": "truecolor"} {
		if value, exists := result.env[name]; exists && value != required {
			return fail("Tuistory cannot honor different terminal capability variables.")
		}
		result.env[name] = required
	}
	declared := append(allStrings(source), literalActions(result.steps))
	for _, secret := range result.secrets {
		for _, value := range declared {
			if strings.Contains(value, secret) {
				return nil, problem("private_contract", "Do not put declared secret values in the contract.")
			}
		}
	}
	sum := sha256.Sum256(raw)
	result.hash = hex.EncodeToString(sum[:])
	return result, nil
}

func allStrings(value any) []string {
	switch value := value.(type) {
	case string:
		return []string{value}
	case []any:
		var result []string
		for _, item := range value {
			result = append(result, allStrings(item)...)
		}
		return result
	case map[string]any:
		var result []string
		for key, item := range value {
			result = append(result, key)
			result = append(result, allStrings(item)...)
		}
		return result
	}
	return nil
}

func literalAction(action document) string {
	switch str(action["type"]) {
	case "text":
		return str(action["text"])
	case "key":
		key, _ := normalizeKey(action["key"])
		switch key {
		case "space":
			return " "
		case "enter", "linefeed":
			return "\n"
		case "tab":
			return "\t"
		}
		if value, ok := key.(string); ok && len(value) == 1 {
			return value
		}
	}
	return "\n"
}
func literalActions(actions []document) string {
	var result strings.Builder
	for _, action := range actions {
		result.WriteString(literalAction(action))
	}
	return result.String()
}

func subset(pattern, value document) bool {
	for key, expected := range pattern {
		if nested, ok := object(expected); ok {
			actual, ok := object(value[key])
			if !ok || !subset(nested, actual) {
				return false
			}
		} else if !reflect.DeepEqual(expected, value[key]) {
			return false
		}
	}
	return true
}

func (contract *runContract) verifyExecutable() error {
	hash, err := cliutil.SHA256File(str(contract.command["executable"]))
	if err != nil {
		return problem("executable_missing", "The selected executable is unavailable.")
	}
	if hash != str(contract.command["sha256"]) {
		return problem("executable_changed", "The selected executable no longer matches the approved checksum.")
	}
	return nil
}

func (contract *runContract) prepare(options Options) error {
	root, err := cliutil.ResolvePath(contract.workspace)
	if err != nil {
		return err
	}
	if filepath.Dir(root) == root {
		return problem("unsafe_workspace", "Use a dedicated workspace outside the repository.")
	}
	stateRoot, err := cliutil.ResolvePath(contract.stateDirectory)
	if err != nil {
		return err
	}
	if filepath.Dir(stateRoot) == stateRoot || stateRoot != root &&
		(cliutil.Inside(root, stateRoot) || cliutil.Inside(stateRoot, root)) {
		return problem("unsafe_workspace", "Shared state must be separate from capture workspaces and filesystem roots.")
	}
	roots := []string{}
	if options.SkillRoot != "" {
		skill, err := cliutil.ResolvePath(options.SkillRoot)
		if err != nil {
			return err
		}
		roots = append(roots, skill)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	for _, path := range append(slices.Clone(roots), cwd) {
		repository, err := cliutil.RepositoryRoot(path)
		if err != nil {
			return err
		}
		if repository != "" {
			roots = append(roots, repository)
		}
	}
	source, err := cliutil.ResolvePath(options.ContractPath)
	if err != nil {
		return err
	}
	for _, excluded := range roots {
		if cliutil.Inside(excluded, root) || cliutil.Inside(excluded, source) || cliutil.Inside(excluded, stateRoot) {
			return problem("unsafe_workspace", "Contracts and evidence must remain outside the skill and repository.")
		}
	}
	for _, home := range options.OperatorHomes {
		resolved, err := cliutil.ResolvePath(home)
		if err != nil {
			return err
		}
		if resolved == root || resolved == stateRoot {
			return problem("unsafe_workspace", "The workspace cannot be an operator profile.")
		}
	}
	if err := contract.verifyExecutable(); err != nil {
		return err
	}
	if err := cliutil.PrivateDirectory(contract.workspace); err != nil {
		return err
	}
	if err := cliutil.PrivateDirectory(contract.stateDirectory); err != nil {
		return err
	}
	contract.workspace = root
	contract.stateDirectory = stateRoot
	for _, name := range []string{"runtime.json", "capture", "result.json"} {
		if _, err := os.Lstat(filepath.Join(root, name)); err == nil {
			return problem("existing_run", "Preserve the existing run and use a new workspace.")
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	contract.cwd, err = cliutil.ConfinedDirectory(stateRoot, str(contract.command["cwd"]))
	if err != nil {
		return err
	}
	isolated, err := cliutil.IsolatedEnvironment(stateRoot, "")
	if err != nil {
		return err
	}
	for _, directory := range isolated {
		if err := cliutil.PrivateDirectory(directory); err != nil {
			return err
		}
	}
	for key, name := range map[string]string{"XDG_DATA_HOME": "data", "XDG_STATE_HOME": "state"} {
		isolated[key], err = cliutil.ConfinedDirectory(stateRoot, name)
		if err != nil {
			return err
		}
	}
	for _, key := range []string{"SystemRoot", "SystemDrive", "WINDIR"} {
		if value, exists := options.SystemEnvironment[key]; exists {
			if _, reviewed := contract.env[key]; !reviewed {
				contract.env[key] = value
			}
		}
	}
	maps.Copy(contract.env, isolated)
	saved := filepath.Join(root, "contract.json")
	if info, err := os.Lstat(saved); err == nil {
		bytes, readErr := os.ReadFile(saved)
		if info.Mode()&os.ModeSymlink != 0 || readErr != nil || string(bytes) != string(contract.bytes) {
			return problem("existing_contract", "Do not replace a different saved contract.")
		}
	} else if !os.IsNotExist(err) {
		return err
	} else if err := os.WriteFile(saved, contract.bytes, 0o400); err != nil {
		return err
	}
	return os.Chmod(saved, 0o400)
}

func marshal(value any) (json.RawMessage, error) {
	bytes, err := json.Marshal(value)
	return json.RawMessage(bytes), err
}

func safeFault(err error) *fault {
	if value, ok := errors.AsType[*fault](err); ok {
		return value
	}
	return &fault{Code: "runtime_failure", Message: "The owned runtime could not complete its operation."}
}

func invalid(message string) error { return problem("invalid_request", message) }
