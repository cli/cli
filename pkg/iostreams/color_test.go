package iostreams

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLabel(t *testing.T) {
	tests := []struct {
		name  string
		hex   string
		text  string
		wants string
		cs    *ColorScheme
	}{
		{
			name:  "truecolor",
			hex:   "fc0303",
			text:  "red",
			wants: "\033[38;2;252;3;3mred\033[0m",
			cs: &ColorScheme{
				Enabled:       true,
				EightBitColor: true,
				TrueColor:     true,
				ColorLabels:   true,
			},
		},
		{
			name:  "no truecolor",
			hex:   "fc0303",
			text:  "red",
			wants: "red",
			cs: &ColorScheme{
				Enabled:       true,
				EightBitColor: true,
				ColorLabels:   true,
			},
		},
		{
			name:  "no color",
			hex:   "fc0303",
			text:  "red",
			wants: "red",
			cs: &ColorScheme{
				ColorLabels: true,
			},
		},
		{
			name:  "invalid hex",
			hex:   "fc0",
			text:  "red",
			wants: "red",
			cs: &ColorScheme{
				ColorLabels: true,
			},
		},
		{
			name:  "no color labels",
			hex:   "fc0303",
			text:  "red",
			wants: "red",
			cs: &ColorScheme{
				Enabled:       true,
				EightBitColor: true,
				ColorLabels:   true,
			},
		},
	}

	for _, tt := range tests {
		output := tt.cs.Label(tt.hex, tt.text)
		assert.Equal(t, tt.wants, output)
	}
}

func TestTableHeader(t *testing.T) {
	reset := "\x1b[0m"
	defaultUnderline := "\x1b[0;4;39m"
	brightBlackUnderline := "\x1b[0;4;90m"
	dimBlackUnderline := "\x1b[0;2;4;37m"

	tests := []struct {
		name     string
		cs       *ColorScheme
		input    string
		expected string
	}{
		{
			name: "when color is disabled, text is not stylized",
			cs: &ColorScheme{
				Accessible: true,
				Theme:      NoTheme,
			},
			input:    "this should not be stylized",
			expected: "this should not be stylized",
		},
		{
			name: "when 4-bit color is enabled but no theme, 4-bit default color and underline are used",
			cs: &ColorScheme{
				Enabled:    true,
				Accessible: true,
				Theme:      NoTheme,
			},
			input:    "this should have no explicit color but underlined",
			expected: fmt.Sprintf("%sthis should have no explicit color but underlined%s", defaultUnderline, reset),
		},
		{
			name: "when 4-bit color is enabled and theme is light, 4-bit dark color and underline are used",
			cs: &ColorScheme{
				Enabled:    true,
				Accessible: true,
				Theme:      LightTheme,
			},
			input:    "this should have dark foreground color and underlined",
			expected: fmt.Sprintf("%sthis should have dark foreground color and underlined%s", brightBlackUnderline, reset),
		},
		{
			name: "when 4-bit color is enabled and theme is dark, 4-bit light color and underline are used",
			cs: &ColorScheme{
				Enabled:    true,
				Accessible: true,
				Theme:      DarkTheme,
			},
			input:    "this should have light foreground color and underlined",
			expected: fmt.Sprintf("%sthis should have light foreground color and underlined%s", dimBlackUnderline, reset),
		},
		{
			name: "when 8-bit color is enabled but no theme, 4-bit default color and underline are used",
			cs: &ColorScheme{
				Enabled:       true,
				EightBitColor: true,
				Accessible:    true,
				Theme:         NoTheme,
			},
			input:    "this should have no explicit color but underlined",
			expected: fmt.Sprintf("%sthis should have no explicit color but underlined%s", defaultUnderline, reset),
		},
		{
			name: "when 8-bit color is enabled and theme is light, 4-bit dark color and underline are used",
			cs: &ColorScheme{
				Enabled:       true,
				EightBitColor: true,
				Accessible:    true,
				Theme:         LightTheme,
			},
			input:    "this should have dark foreground color and underlined",
			expected: fmt.Sprintf("%sthis should have dark foreground color and underlined%s", brightBlackUnderline, reset),
		},
		{
			name: "when 8-bit color is true and theme is dark, 4-bit light color and underline are used",
			cs: &ColorScheme{
				Enabled:       true,
				EightBitColor: true,
				Accessible:    true,
				Theme:         DarkTheme,
			},
			input:    "this should have light foreground color and underlined",
			expected: fmt.Sprintf("%sthis should have light foreground color and underlined%s", dimBlackUnderline, reset),
		},
		{
			name: "when 24-bit color is enabled but no theme, 4-bit default color and underline are used",
			cs: &ColorScheme{
				Enabled:       true,
				EightBitColor: true,
				TrueColor:     true,
				Accessible:    true,
				Theme:         NoTheme,
			},
			input:    "this should have no explicit color but underlined",
			expected: fmt.Sprintf("%sthis should have no explicit color but underlined%s", defaultUnderline, reset),
		},
		{
			name: "when 24-bit color is enabled and theme is light, 4-bit dark color and underline are used",
			cs: &ColorScheme{
				Enabled:       true,
				EightBitColor: true,
				TrueColor:     true,
				Accessible:    true,
				Theme:         LightTheme,
			},
			input:    "this should have dark foreground color and underlined",
			expected: fmt.Sprintf("%sthis should have dark foreground color and underlined%s", brightBlackUnderline, reset),
		},
		{
			name: "when 24-bit color is true and theme is dark, 4-bit light color and underline are used",
			cs: &ColorScheme{
				Enabled:       true,
				EightBitColor: true,
				TrueColor:     true,
				Accessible:    true,
				Theme:         DarkTheme,
			},
			input:    "this should have light foreground color and underlined",
			expected: fmt.Sprintf("%sthis should have light foreground color and underlined%s", dimBlackUnderline, reset),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.cs.TableHeader(tt.input))
		})
	}
}

func TestMuted(t *testing.T) {
	reset := "\x1b[0m"
	gray4bit := "\x1b[0;90m"
	gray8bit := "\x1b[38;5;242m"
	brightBlack4bit := "\x1b[0;90m"
	dimBlack4bit := "\x1b[0;2;37m"

	tests := []struct {
		name     string
		cs       *ColorScheme
		input    string
		expected string
	}{
		{
			name:     "when color is disabled but accessible colors are disabled, text is not stylized",
			cs:       &ColorScheme{},
			input:    "this should not be stylized",
			expected: "this should not be stylized",
		},
		{
			name: "when 4-bit color is enabled but accessible colors are disabled, legacy 4-bit gray color is used",
			cs: &ColorScheme{
				Enabled: true,
			},
			input:    "this should be 4-bit gray",
			expected: fmt.Sprintf("%sthis should be 4-bit gray%s", gray4bit, reset),
		},
		{
			name: "when 8-bit color is enabled but accessible colors are disabled, legacy 8-bit gray color is used",
			cs: &ColorScheme{
				Enabled:       true,
				EightBitColor: true,
			},
			input:    "this should be 8-bit gray",
			expected: fmt.Sprintf("%sthis should be 8-bit gray%s", gray8bit, reset),
		},
		{
			name: "when 24-bit color is enabled but accessible colors are disabled, legacy 8-bit gray color is used",
			cs: &ColorScheme{
				Enabled:       true,
				EightBitColor: true,
				TrueColor:     true,
			},
			input:    "this should be 8-bit gray",
			expected: fmt.Sprintf("%sthis should be 8-bit gray%s", gray8bit, reset),
		},
		{
			name: "when 4-bit color is enabled and theme is dark, 4-bit light color is used",
			cs: &ColorScheme{
				Enabled:       true,
				EightBitColor: true,
				TrueColor:     true,
				Accessible:    true,
				Theme:         DarkTheme,
			},
			input:    "this should be 4-bit dim black",
			expected: fmt.Sprintf("%sthis should be 4-bit dim black%s", dimBlack4bit, reset),
		},
		{
			name: "when 4-bit color is enabled and theme is light, 4-bit dark color is used",
			cs: &ColorScheme{
				Enabled:       true,
				EightBitColor: true,
				TrueColor:     true,
				Accessible:    true,
				Theme:         LightTheme,
			},
			input:    "this should be 4-bit bright black",
			expected: fmt.Sprintf("%sthis should be 4-bit bright black%s", brightBlack4bit, reset),
		},
		{
			name: "when 4-bit color is enabled but no theme, 4-bit default color is used",
			cs: &ColorScheme{
				Enabled:       true,
				EightBitColor: true,
				TrueColor:     true,
				Accessible:    true,
				Theme:         NoTheme,
			},
			input:    "this should have no explicit color",
			expected: "this should have no explicit color",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.cs.Muted(tt.input))
		})
	}
}
