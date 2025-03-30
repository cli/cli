package iostreams

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestColorFromRGB(t *testing.T) {
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
			cs:    NewColorScheme(true, true, true, NoTheme),
		},
		{
			name:  "no truecolor",
			hex:   "fc0303",
			text:  "red",
			wants: "red",
			cs:    NewColorScheme(true, true, false, NoTheme),
		},
		{
			name:  "no color",
			hex:   "fc0303",
			text:  "red",
			wants: "red",
			cs:    NewColorScheme(false, false, false, NoTheme),
		},
		{
			name:  "invalid hex",
			hex:   "fc0",
			text:  "red",
			wants: "red",
			cs:    NewColorScheme(false, false, false, NoTheme),
		},
	}

	for _, tt := range tests {
		fn := tt.cs.ColorFromRGB(tt.hex)
		assert.Equal(t, tt.wants, fn(tt.text))
	}
}

func TestHexToRGB(t *testing.T) {
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
			cs:    NewColorScheme(true, true, true, NoTheme),
		},
		{
			name:  "no truecolor",
			hex:   "fc0303",
			text:  "red",
			wants: "red",
			cs:    NewColorScheme(true, true, false, NoTheme),
		},
		{
			name:  "no color",
			hex:   "fc0303",
			text:  "red",
			wants: "red",
			cs:    NewColorScheme(false, false, false, NoTheme),
		},
		{
			name:  "invalid hex",
			hex:   "fc0",
			text:  "red",
			wants: "red",
			cs:    NewColorScheme(false, false, false, NoTheme),
		},
	}

	for _, tt := range tests {
		output := tt.cs.HexToRGB(tt.hex, tt.text)
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
			name:     "when color is disabled, text is not stylized",
			cs:       NewColorScheme(false, false, false, NoTheme),
			input:    "this should not be stylized",
			expected: "this should not be stylized",
		},
		{
			name:     "when 4-bit color is enabled but no theme, 4-bit default color and underline are used",
			cs:       NewColorScheme(true, false, false, NoTheme),
			input:    "this should have no explicit color but underlined",
			expected: fmt.Sprintf("%sthis should have no explicit color but underlined%s", defaultUnderline, reset),
		},
		{
			name:     "when 4-bit color is enabled and theme is light, 4-bit dark color and underline are used",
			cs:       NewColorScheme(true, false, false, LightTheme),
			input:    "this should have dark foreground color and underlined",
			expected: fmt.Sprintf("%sthis should have dark foreground color and underlined%s", brightBlackUnderline, reset),
		},
		{
			name:     "when 4-bit color is enabled and theme is dark, 4-bit light color and underline are used",
			cs:       NewColorScheme(true, false, false, DarkTheme),
			input:    "this should have light foreground color and underlined",
			expected: fmt.Sprintf("%sthis should have light foreground color and underlined%s", dimBlackUnderline, reset),
		},
		{
			name:     "when 8-bit color is enabled but no theme, 4-bit default color and underline are used",
			cs:       NewColorScheme(true, true, false, NoTheme),
			input:    "this should have no explicit color but underlined",
			expected: fmt.Sprintf("%sthis should have no explicit color but underlined%s", defaultUnderline, reset),
		},
		{
			name:     "when 8-bit color is enabled and theme is light, 4-bit dark color and underline are used",
			cs:       NewColorScheme(true, true, false, LightTheme),
			input:    "this should have dark foreground color and underlined",
			expected: fmt.Sprintf("%sthis should have dark foreground color and underlined%s", brightBlackUnderline, reset),
		},
		{
			name:     "when 8-bit color is true and theme is dark, 4-bit light color and underline are used",
			cs:       NewColorScheme(true, true, false, DarkTheme),
			input:    "this should have light foreground color and underlined",
			expected: fmt.Sprintf("%sthis should have light foreground color and underlined%s", dimBlackUnderline, reset),
		},
		{
			name:     "when 24-bit color is enabled but no theme, 4-bit default color and underline are used",
			cs:       NewColorScheme(true, true, true, NoTheme),
			input:    "this should have no explicit color but underlined",
			expected: fmt.Sprintf("%sthis should have no explicit color but underlined%s", defaultUnderline, reset),
		},
		{
			name:     "when 24-bit color is enabled and theme is light, 4-bit dark color and underline are used",
			cs:       NewColorScheme(true, true, true, LightTheme),
			input:    "this should have dark foreground color and underlined",
			expected: fmt.Sprintf("%sthis should have dark foreground color and underlined%s", brightBlackUnderline, reset),
		},
		{
			name:     "when 24-bit color is true and theme is dark, 4-bit light color and underline are used",
			cs:       NewColorScheme(true, true, true, DarkTheme),
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
