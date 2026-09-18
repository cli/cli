package engine

import (
	"maps"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"unicode"
	"unicode/utf16"
)

var optionPattern = regexp.MustCompile(`^([ >❯›]) (?:\[([ xX✓✔])\]\s+)?(\S.*)$`)
var focusPattern = regexp.MustCompile(`^[>❯›] `)

func menuObservation(text string, columns int) document {
	unsupported := document{"supported": false, "question": nil, "selected": nil, "options": []any{}}
	lines := strings.Split(text, "\n")
	focused := -1
	for index, line := range lines {
		if focusPattern.MatchString(line) {
			if focused != -1 {
				return unsupported
			}
			focused = index
		}
	}
	if focused == -1 {
		return unsupported
	}
	start, end := focused, focused+1
	for start > 0 && optionPattern.MatchString(lines[start-1]) {
		start--
	}
	for end < len(lines) && optionPattern.MatchString(lines[end]) {
		end++
	}
	options := []any{}
	var selected any
	checkbox, hasKind := false, false
	for _, line := range lines[start:end] {
		match := optionPattern.FindStringSubmatch(line)
		if match == nil {
			return unsupported
		}
		hasCheckbox := match[2] != ""
		if hasKind && checkbox != hasCheckbox {
			return unsupported
		}
		checkbox, hasKind = hasCheckbox, true
		var checked any
		if checkbox {
			checked = match[2] != " "
		}
		label := strings.TrimRightFunc(match[3], unicode.IsSpace)
		focus := match[1] != " "
		options = append(options, document{"label": label, "focused": focus, "checked": checked})
		if focus {
			selected = label
		}
	}
	if len(options) == 0 || selected == nil {
		return unsupported
	}
	var question any
	heading := start - 1
	for heading >= 0 && strings.TrimSpace(lines[heading]) == "" {
		heading--
	}
	if heading >= 0 {
		value := lines[heading]
		for heading > 0 && len(utf16.Encode([]rune(lines[heading-1]))) == columns {
			heading--
			value = lines[heading] + value
		}
		question = strings.TrimSpace(value)
	}
	kind := "line-select"
	if checkbox {
		kind = "line-checkbox"
	}
	return document{"supported": true, "kind": kind, "question": question, "selected": selected, "options": options}
}

func matches(condition, observation document) bool {
	for key, expected := range condition {
		switch key {
		case "screenContains":
			if !strings.Contains(str(observation["text"]), str(expected)) {
				return false
			}
		case "questionPrefix":
			if observation["question"] == nil || !strings.HasPrefix(str(observation["question"]), str(expected)) {
				return false
			}
		case "selected":
			if observation["selected"] != expected {
				return false
			}
		}
	}
	return true
}

func constraintAction(action document) (document, error) {
	value, exists := action["key"]
	if !exists {
		return action, nil
	}
	key, err := normalizeKey(value)
	if err != nil {
		return nil, err
	}
	if equivalent := equivalentKey(key); equivalent != "" {
		key = equivalent
	} else if chord, ok := stringList(key); ok {
		slices.Sort(chord[:len(chord)-1])
		key = strings.Join(chord, "+")
	}
	normalized := maps.Clone(action)
	normalized["key"] = key
	return normalized, nil
}

func (contract *runContract) allowed(action, observation document) error {
	action, err := constraintAction(action)
	if err != nil {
		return err
	}
	for _, deny := range contract.denies {
		pattern, _ := object(deny["action"])
		pattern, err = constraintAction(pattern)
		if err != nil {
			return err
		}
		if !subset(pattern, action) {
			continue
		}
		condition, exists := object(deny["when"])
		if !exists {
			reason := str(deny["reason"])
			if reason == "" {
				reason = "The immutable contract denies this action."
			}
			return problem("action_denied", reason)
		}
		unknown, rejected := false, false
		menu, _ := object(observation["menu"])
		for key, value := range condition {
			if key == "selected" && menu["supported"] != true {
				unknown = true
				continue
			}
			if key == "questionPrefix" && observation["question"] == nil {
				found := false
				for line := range strings.SplitSeq(str(observation["text"]), "\n") {
					found = found || strings.HasPrefix(strings.TrimLeftFunc(line, unicode.IsSpace), str(value))
				}
				unknown = unknown || found
				rejected = rejected || !found
				continue
			}
			rejected = rejected || !matches(document{key: value}, observation)
		}
		if rejected {
			continue
		}
		if unknown {
			return problem("constraint_unobservable", "The deny constraint requires an unobservable selection.")
		}
		reason := str(deny["reason"])
		if reason == "" {
			reason = "The immutable contract denies this action."
		}
		return problem("action_denied", reason)
	}
	return nil
}

type privacyGuard struct {
	secrets, paths, allowed []string
}

func pathForm(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	for strings.Contains(value, "//") {
		value = strings.ReplaceAll(value, "//", "/")
	}
	if runtime.GOOS == "windows" {
		value = strings.ToLower(value)
	}
	return value
}

func boundary(value string, offset int) bool {
	return offset >= len(value) || strings.ContainsRune("/ \t\r\n:'\")]", rune(value[offset]))
}

func (guard *privacyGuard) check(value any, paths bool) error {
	for _, text := range allStrings(value) {
		for _, secret := range guard.secrets {
			if strings.Contains(text, secret) {
				return problem("privacy_failure", "A declared secret was detected; rejected text was not retained.")
			}
		}
		if !paths {
			continue
		}
		normalized := pathForm(text)
		for _, original := range guard.paths {
			protected := pathForm(original)
			if protected == "" {
				continue
			}
			for offset := 0; offset < len(normalized); {
				found := strings.Index(normalized[offset:], protected)
				if found < 0 {
					break
				}
				index := offset + found
				suffix := normalized[index:]
				approved := !boundary(suffix, len(protected))
				for _, original := range guard.allowed {
					allowed := pathForm(original)
					approved = approved || strings.HasPrefix(suffix, allowed) && boundary(suffix, len(allowed)) || strings.HasPrefix(allowed, suffix)
				}
				if !approved {
					return problem("privacy_failure", "An operator path outside the declared workspace or command was detected.")
				}
				offset = index + len(protected)
			}
		}
	}
	return nil
}

func (guard *privacyGuard) pending(value string) bool {
	for _, secret := range guard.secrets {
		for length := min(len(secret)-1, len(value)); length > 0; length-- {
			previous := len(value) - length - 1
			word := previous >= 0 && (value[previous] >= 'a' && value[previous] <= 'z' ||
				value[previous] >= 'A' && value[previous] <= 'Z' || value[previous] >= '0' && value[previous] <= '9' || value[previous] == '_')
			if (length > 1 || !word) && strings.HasSuffix(value, secret[:length]) {
				return true
			}
		}
	}
	normalized := pathForm(value)
	for _, protected := range guard.paths {
		index := strings.LastIndex(normalized, pathForm(protected))
		if index >= 0 {
			suffix := normalized[index:]
			for _, allowed := range guard.allowed {
				allowed = pathForm(allowed)
				if suffix != allowed && strings.HasPrefix(allowed, suffix) {
					return true
				}
			}
		}
	}
	return false
}

type plainStream struct{ state string }

func (stream *plainStream) feed(input string) string {
	var output strings.Builder
	for _, character := range input {
		switch stream.state {
		case "", "text":
			if character == '\x1b' {
				stream.state = "escape"
			} else {
				output.WriteRune(character)
			}
		case "escape":
			stream.state = "text"
			if character == '[' {
				stream.state = "csi"
			} else if character == ']' {
				stream.state = "osc"
			} else if character == 'P' {
				stream.state = "dcs"
			}
		case "csi":
			if character >= '@' && character <= '~' {
				stream.state = "text"
			}
		case "osc", "dcs":
			if character == '\x07' && stream.state == "osc" {
				stream.state = "text"
			} else if character == '\x1b' {
				stream.state += "-escape"
			}
		default:
			if character == '\\' {
				stream.state = "text"
			} else {
				stream.state = strings.Split(stream.state, "-")[0]
			}
		}
	}
	return output.String()
}

func evaluate(contract *runContract, observation document, exitCode *int, complete, interrupted bool, steps int) document {
	checks := []any{}
	conclusive := complete && !interrupted && (contract.mode != "exact" || steps == len(contract.steps))
	actual := AssertionContext{ExitCode: exitCode, Conclusive: conclusive}
	if observation != nil {
		value := str(observation["text"])
		actual.Text = &value
	}
	results := make([]AssertionResult, 0, len(contract.expectations))
	for _, expected := range contract.expectations {
		item := document{}
		maps.Copy(item, expected)
		result := EvaluateAssertion(str(expected["type"]), expected["value"], actual)
		item["status"], item["reason"], item["observed"] = result.Status, result.Reason, result.Observed
		checks = append(checks, item)
		results = append(results, result)
	}
	return document{"caseStatus": AssertionVerdict(results, conclusive), "expectations": checks}
}
