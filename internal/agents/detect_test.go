package agents

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lookup(vars map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	}
}

func TestParseAgentName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    AgentName
		wantErr bool
	}{
		{name: "valid lowercase", input: "my-agent", want: "my-agent"},
		{name: "valid with underscore", input: "my_agent_v2", want: "my_agent_v2"},
		{name: "valid uppercase", input: "MyAgent", want: "MyAgent"},
		{name: "valid numbers", input: "agent123", want: "agent123"},
		{name: "spaces rejected", input: "my agent", wantErr: true},
		{name: "newline rejected", input: "my\nagent", wantErr: true},
		{name: "carriage return rejected", input: "my\ragent", wantErr: true},
		{name: "null byte rejected", input: "my\x00agent", wantErr: true},
		{name: "dot rejected", input: "my.agent", wantErr: true},
		{name: "slash rejected", input: "my/agent", wantErr: true},
		{name: "empty rejected", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAgentName(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestDetectWith(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		wantAgent AgentName
	}{
		{
			name:      "clean environment",
			env:       map[string]string{},
			wantAgent: "",
		},
		{
			name:      "empty var is not detected",
			env:       map[string]string{"GEMINI_CLI": ""},
			wantAgent: "",
		},
		{
			name:      "AGENT=amp detected as amp",
			env:       map[string]string{"AGENT": "amp"},
			wantAgent: "amp",
		},
		{
			name:      "AGENT with non-amp value is ignored",
			env:       map[string]string{"AGENT": "other"},
			wantAgent: "",
		},
		{
			name:      "AI_AGENT returns value as agent name",
			env:       map[string]string{"AI_AGENT": "some-agent"},
			wantAgent: "some-agent",
		},
		{
			name:      "AI_AGENT with invalid characters is ignored",
			env:       map[string]string{"AI_AGENT": "bad\nagent"},
			wantAgent: "",
		},
		{
			name:      "AI_AGENT with spaces is ignored",
			env:       map[string]string{"AI_AGENT": "bad agent"},
			wantAgent: "",
		},
		{
			name:      "AI_AGENT takes priority over AGENT",
			env:       map[string]string{"AGENT": "amp", "AI_AGENT": "other"},
			wantAgent: "other",
		},
		{
			name:      "CODEX_SANDBOX",
			env:       map[string]string{"CODEX_SANDBOX": "seatbelt"},
			wantAgent: "codex",
		},
		{
			name:      "CODEX_CI",
			env:       map[string]string{"CODEX_CI": "1"},
			wantAgent: "codex",
		},
		{
			name:      "CODEX_THREAD_ID",
			env:       map[string]string{"CODEX_THREAD_ID": "abc"},
			wantAgent: "codex",
		},
		{
			name:      "GEMINI_CLI",
			env:       map[string]string{"GEMINI_CLI": "1"},
			wantAgent: "gemini-cli",
		},
		{
			name:      "COPILOT_CLI",
			env:       map[string]string{"COPILOT_CLI": "1"},
			wantAgent: "copilot-cli",
		},
		{
			name:      "OPENCODE",
			env:       map[string]string{"OPENCODE": "1"},
			wantAgent: "opencode",
		},
		{
			name:      "CLAUDECODE",
			env:       map[string]string{"CLAUDECODE": "1"},
			wantAgent: "claude-code",
		},
		{
			name:      "AGENT=amp takes priority over CLAUDECODE",
			env:       map[string]string{"AGENT": "amp", "CLAUDECODE": "1"},
			wantAgent: "amp",
		},
		{
			name:      "invalid AI_AGENT falls through to tool-specific detection",
			env:       map[string]string{"AI_AGENT": "bad agent", "GEMINI_CLI": "1"},
			wantAgent: "gemini-cli",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectWith(lookup(tt.env))
			assert.Equal(t, tt.wantAgent, got)
		})
	}
}
