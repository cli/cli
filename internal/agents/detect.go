package agents

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// AgentName is a validated agent identifier safe for use in HTTP headers.
type AgentName string

const (
	agentAmp         AgentName = "amp"
	agentClaudeCode  AgentName = "claude-code"
	agentCodex       AgentName = "codex"
	agentCopilotCLI  AgentName = "copilot-cli"
	agentGeminiCLI   AgentName = "gemini-cli"
	agentOpencode    AgentName = "opencode"
	agentAntigravity AgentName = "antigravity"
	agentAugmentCLI  AgentName = "augment-cli"
	agentReplit      AgentName = "replit"
	agentGoose       AgentName = "goose"
	agentCowork      AgentName = "cowork"
	agentCursor      AgentName = "cursor"
	agentCursorCLI   AgentName = "cursor-cli"
	agentKiro        AgentName = "kiro"
	agentPi          AgentName = "pi"
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

	// Generic agent identifiers - checked first because they are the most specific signal.
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

	// OpenAI Codex CLI - https://github.com/openai/codex
	// CODEX_SANDBOX: https://github.com/openai/codex/blob/95e1d5993985019ce0ce0d10689caf1375f95120/codex-rs/core/src/spawn.rs#L25
	// CODEX_THREAD_ID: https://github.com/openai/codex/blob/95e1d5993985019ce0ce0d10689caf1375f95120/codex-rs/core/src/exec_env.rs#L8
	// CODEX_CI: https://github.com/openai/codex/blob/95e1d5993985019ce0ce0d10689caf1375f95120/codex-rs/core/src/unified_exec/process_manager.rs#L64
	if isSet("CODEX_SANDBOX") || isSet("CODEX_CI") || isSet("CODEX_THREAD_ID") {
		return agentCodex
	}

	// Google Gemini CLI - https://github.com/google-gemini/gemini-cli
	// GEMINI_CLI: https://github.com/google-gemini/gemini-cli/blob/46fd7b4864111032a1c7dfa1821b2000fc7531da/docs/tools/shell.md#L96-L97
	if isSet("GEMINI_CLI") {
		return agentGeminiCLI
	}

	// GitHub Copilot CLI
	// No first-party docs
	if isSet("COPILOT_CLI") {
		return agentCopilotCLI
	}

	// OpenCode - https://github.com/anomalyco/opencode
	// OPENCODE: https://github.com/anomalyco/opencode/blob/fde201c286a83ff32dda9b41d61d734a4449fe70/packages/opencode/src/index.ts#L78-L80
	// Not OPENCODE_CALLER or OPENCODE_CLIENT: they name the client that launched
	// opencode (e.g. the VS Code extension), not the running agent.
	if isSet("OPENCODE") {
		return agentOpencode
	}

	// Antigravity
	// No first-party docs
	if isSet("ANTIGRAVITY_AGENT") {
		return agentAntigravity
	}

	// Augment CLI
	// No first-party docs
	if isSet("AUGMENT_AGENT") {
		return agentAugmentCLI
	}

	// Replit
	// REPL_ID is present throughout any Replit environment, not only when a
	// Replit agent is driving the CLI, so it is a broad, low-confidence signal.
	// REPL_ID: https://github.com/replit/go-replidentity/blob/2966ea2d227d572f6054ee8f077ad16a1be02663/examples/extract.go#L25
	if isSet("REPL_ID") {
		return agentReplit
	}

	// Anthropic Claude Code - https://docs.anthropic.com/en/docs/agents-and-tools/claude-code/overview
	// CLAUDECODE: https://code.claude.com/docs/en/env-vars (CLAUDECODE section)
	// CLAUDE_CODE, CLAUDE_CODE_IS_COWORK: no first-party docs
	//
	// Cowork is a Claude Code mode that also sets CLAUDECODE, so it is checked
	// first to win over the generic Claude Code signal below.
	if isSet("CLAUDE_CODE_IS_COWORK") {
		return agentCowork
	}

	// Claude Code is checked after Amp and Cowork, which also set CLAUDECODE, so
	// those more specific agents are detected first.
	if isSet("CLAUDECODE") || isSet("CLAUDE_CODE") {
		// There is a CLAUDE_CODE_ENTRYPOINT env var that is set to `cli` or `desktop` etc, but it's not documented
		// so we don't want to rely on it too heavily. We'll just return a generic claude-code agent name.
		return agentClaudeCode
	}

	// Cursor
	// No first-party docs
	// CURSOR_TRACE_ID (IDE) takes precedence over the Cursor CLI signal below.
	if isSet("CURSOR_TRACE_ID") {
		return agentCursor
	}

	// Cursor CLI
	// No first-party docs
	if isSet("CURSOR_AGENT") || valueOf("CURSOR_EXTENSION_HOST_ROLE") == "agent-exec" {
		return agentCursorCLI
	}

	// Single-source signals matched against one environment variable. These
	// carry lower corroboration than the presence-based agents above, so they
	// are checked after them.

	// Kiro
	// No first-party docs
	if valueOf("TERM_PROGRAM") == "kiro" {
		return agentKiro
	}

	// Pi
	// No first-party docs
	// Anchored to a path separator so it only matches ".pi/agent" as a real
	// path segment, not an incidental substring. The Windows separator is
	// matched too, though confidence there is lower since it is unconfirmed
	// that pi uses this layout on Windows.
	if strings.Contains(valueOf("PATH"), "/.pi/agent") || strings.Contains(valueOf("PATH"), `\.pi\agent`) {
		return agentPi
	}

	// Goose is checked last because GOOSE_PROVIDER only indicates that Goose is
	// configured as a model provider, not that it is driving the CLI, so any
	// more specific signal above should win.
	// GOOSE_PROVIDER: https://github.com/aaif-goose/goose/blob/48a2a3d1804ae75eb7b208a5d0d73fd976511b80/crates/goose/src/config/providers.rs#L93
	if isSet("GOOSE_PROVIDER") {
		return agentGoose
	}

	return ""
}
