package agents

import (
	"fmt"
	"os"
	"regexp"
)

// AgentName is a validated agent identifier safe for use in HTTP headers.
type AgentName string

const (
	agentAmp        AgentName = "amp"
	agentClaudeCode AgentName = "claude-code"
	agentCodex      AgentName = "codex"
	agentCopilotCLI AgentName = "copilot-cli"
	agentGeminiCLI  AgentName = "gemini-cli"
	agentOpencode   AgentName = "opencode"
)

var validAgentName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// parseAgentName validates and returns an AgentName from a raw string.
// Only alphanumeric characters, hyphens, and underscores are allowed.
func parseAgentName(s string) (AgentName, error) {
	if !validAgentName.MatchString(s) {
		return "", fmt.Errorf("invalid agent name %q: must match [a-zA-Z0-9_-]+", s)
	}
	return AgentName(s), nil
}

// Detect returns the name of the AI coding agent driving the CLI,
// or an empty AgentName if none is detected.
func Detect() AgentName {
	return detectWith(os.LookupEnv)
}

func detectWith(lookup func(string) (string, bool)) AgentName {
	isSet := func(key string) bool {
		v, ok := lookup(key)
		return ok && v != ""
	}

	valueOf := func(key string) string {
		v, _ := lookup(key)
		return v
	}

	// Generic agent identifiers — checked first because they are the most specific signal.
	if v, ok := lookup("AI_AGENT"); ok && v != "" {
		if name, err := parseAgentName(v); err == nil {
			return name
		}
	}

	// Tool-specific variables.

	// Check AGENT=amp before the more generic CLAUDECODE=1 since Amp sets both.
	if valueOf("AGENT") == "amp" {
		return agentAmp
	}

	// OpenAI Codex CLI — https://github.com/openai/codex
	// CODEX_SANDBOX: https://github.com/openai/codex/blob/95e1d5993985019ce0ce0d10689caf1375f95120/codex-rs/core/src/spawn.rs#L25
	// CODEX_THREAD_ID: https://github.com/openai/codex/blob/95e1d5993985019ce0ce0d10689caf1375f95120/codex-rs/core/src/exec_env.rs#L8
	// CODEX_CI: https://github.com/openai/codex/blob/95e1d5993985019ce0ce0d10689caf1375f95120/codex-rs/core/src/unified_exec/process_manager.rs#L64
	if isSet("CODEX_SANDBOX") || isSet("CODEX_CI") || isSet("CODEX_THREAD_ID") {
		return agentCodex
	}

	// Google Gemini CLI — https://github.com/google-gemini/gemini-cli
	// GEMINI_CLI: https://github.com/google-gemini/gemini-cli/blob/46fd7b4864111032a1c7dfa1821b2000fc7531da/docs/tools/shell.md#L96-L97
	if isSet("GEMINI_CLI") {
		return agentGeminiCLI
	}

	// GitHub Copilot CLI
	// No first-party docs
	if isSet("COPILOT_CLI") {
		return agentCopilotCLI
	}

	// OpenCode — https://github.com/anomalyco/opencode
	// OPENCODE: https://github.com/anomalyco/opencode/blob/fde201c286a83ff32dda9b41d61d734a4449fe70/packages/opencode/src/index.ts#L78-L80
	if isSet("OPENCODE") {
		return agentOpencode
	}

	// Anthropic Claude Code — https://docs.anthropic.com/en/docs/agents-and-tools/claude-code/overview
	// CLAUDECODE: https://code.claude.com/docs/en/env-vars (CLAUDECODE section)
	// Checked last because other agents (e.g. Amp) set CLAUDECODE=1 alongside their own vars.
	if isSet("CLAUDECODE") {
		// There is a CLAUDE_CODE_ENTRYPOINT env var that is set to `cli` or `desktop` etc, but it's not documented
		// so we don't want to rely on it too heavily. We'll just return a generic claude-code agent name.
		return agentClaudeCode
	}

	return ""
}
