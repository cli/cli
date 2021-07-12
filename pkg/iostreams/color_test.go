package iostreams

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvColorDisabled(t *testing.T) {
	orig_NO_COLOR := os.Getenv("NO_COLOR")
	orig_CLICOLOR := os.Getenv("CLICOLOR")
	orig_CLICOLOR_FORCE := os.Getenv("CLICOLOR_FORCE")
	t.Cleanup(func() {
		os.Setenv("NO_COLOR", orig_NO_COLOR)
		os.Setenv("CLICOLOR", orig_CLICOLOR)
		os.Setenv("CLICOLOR_FORCE", orig_CLICOLOR_FORCE)
	})

	tests := []struct {
		name           string
		NO_COLOR       string
		CLICOLOR       string
		CLICOLOR_FORCE string
		want           bool
	}{
		{
			name:           "pristine env",
			NO_COLOR:       "",
			CLICOLOR:       "",
			CLICOLOR_FORCE: "",
			want:           false,
		},
		{
			name:           "NO_COLOR enabled",
			NO_COLOR:       "1",
			CLICOLOR:       "",
			CLICOLOR_FORCE: "",
			want:           true,
		},
		{
			name:           "CLICOLOR disabled",
			NO_COLOR:       "",
			CLICOLOR:       "0",
			CLICOLOR_FORCE: "",
			want:           true,
		},
		{
			name:           "CLICOLOR enabled",
			NO_COLOR:       "",
			CLICOLOR:       "1",
			CLICOLOR_FORCE: "",
			want:           false,
		},
		{
			name:           "CLICOLOR_FORCE has no effect",
			NO_COLOR:       "",
			CLICOLOR:       "",
			CLICOLOR_FORCE: "1",
			want:           false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("NO_COLOR", tt.NO_COLOR)
			os.Setenv("CLICOLOR", tt.CLICOLOR)
			os.Setenv("CLICOLOR_FORCE", tt.CLICOLOR_FORCE)

			if got := EnvColorDisabled(); got != tt.want {
				t.Errorf("EnvColorDisabled(): want %v, got %v", tt.want, got)
			}
		})
	}
}

func TestEnvColorForced(t *testing.T) {
	orig_NO_COLOR := os.Getenv("NO_COLOR")
	orig_CLICOLOR := os.Getenv("CLICOLOR")
	orig_CLICOLOR_FORCE := os.Getenv("CLICOLOR_FORCE")
	t.Cleanup(func() {
		os.Setenv("NO_COLOR", orig_NO_COLOR)
		os.Setenv("CLICOLOR", orig_CLICOLOR)
		os.Setenv("CLICOLOR_FORCE", orig_CLICOLOR_FORCE)
	})

	tests := []struct {
		name           string
		NO_COLOR       string
		CLICOLOR       string
		CLICOLOR_FORCE string
		want           bool
	}{
		{
			name:           "pristine env",
			NO_COLOR:       "",
			CLICOLOR:       "",
			CLICOLOR_FORCE: "",
			want:           false,
		},
		{
			name:           "NO_COLOR enabled",
			NO_COLOR:       "1",
			CLICOLOR:       "",
			CLICOLOR_FORCE: "",
			want:           false,
		},
		{
			name:           "CLICOLOR disabled",
			NO_COLOR:       "",
			CLICOLOR:       "0",
			CLICOLOR_FORCE: "",
			want:           false,
		},
		{
			name:           "CLICOLOR enabled",
			NO_COLOR:       "",
			CLICOLOR:       "1",
			CLICOLOR_FORCE: "",
			want:           false,
		},
		{
			name:           "CLICOLOR_FORCE enabled",
			NO_COLOR:       "",
			CLICOLOR:       "",
			CLICOLOR_FORCE: "1",
			want:           true,
		},
		{
			name:           "CLICOLOR_FORCE disabled",
			NO_COLOR:       "",
			CLICOLOR:       "",
			CLICOLOR_FORCE: "0",
			want:           false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("NO_COLOR", tt.NO_COLOR)
			os.Setenv("CLICOLOR", tt.CLICOLOR)
			os.Setenv("CLICOLOR_FORCE", tt.CLICOLOR_FORCE)

			if got := EnvColorForced(); got != tt.want {
				t.Errorf("EnvColorForced(): want %v, got %v", tt.want, got)
			}
		})
	}
}

func Test_HextoRGB(t *testing.T) {
	tests := []struct {
		name           string
		hex            string
		arg            string
		expectedOutput string
		expectedError  bool
		cs             *ColorScheme
	}{
		{
			name:           "Colored red enabled color",
			hex:            "fc0303",
			arg:            "red",
			expectedOutput: "\033[38;2;252;3;3mred\033[0m",
			cs:             NewColorScheme(true, true),
		},
		{
			name:           "Failed colored red enabled color",
			hex:            "fc0303",
			arg:            "red",
			expectedOutput: "\033[38;2;252;2;3mred\033[0m",
			expectedError:  true,
			cs:             NewColorScheme(true, true),
		},
		{
			name:           "Colored red disabled color",
			hex:            "fc0303",
			arg:            "red",
			expectedOutput: "red",
			cs:             NewColorScheme(false, false),
		},
		{
			name:           "Failed colored red disabled color",
			hex:            "fc0303",
			arg:            "red",
			expectedOutput: "\033[38;2;252;3;3mred\033[0m",
			expectedError:  true,
			cs:             NewColorScheme(false, false),
		},
	}

	for _, tt := range tests {
		output := tt.cs.HexToRGB(tt.hex, tt.arg)
		if tt.expectedError {
			assert.NotEqual(t, tt.expectedOutput, output)
		} else {
			assert.Equal(t, tt.expectedOutput, output)
		}
	}
}
