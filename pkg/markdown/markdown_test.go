package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Render(t *testing.T) {
	type input struct {
		text  string
		style string
	}
	type output struct {
		wantsErr bool
	}
	tests := []struct {
		name   string
		input  input
		output output
	}{
		{
			name: "invalid glamour style",
			input: input{
				text:  "some text",
				style: "invalid",
			},
			output: output{
				wantsErr: true,
			},
		},
		{
			name: "valid glamour style",
			input: input{
				text:  "some text",
				style: "dark",
			},
			output: output{
				wantsErr: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Render(tt.input.text, tt.input.style, "")
			if tt.output.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func Test_GetStyle(t *testing.T) {
	type input struct {
		glamourStyle  string
		terminalTheme string
	}
	type output struct {
		style string
	}
	tests := []struct {
		name   string
		input  input
		output output
	}{
		{
			name: "no glamour style and no terminal theme",
			input: input{
				glamourStyle:  "",
				terminalTheme: "none",
			},
			output: output{
				style: "notty",
			},
		},
		{
			name: "auto glamour style and no terminal theme",
			input: input{
				glamourStyle:  "auto",
				terminalTheme: "none",
			},
			output: output{
				style: "notty",
			},
		},
		{
			name: "user glamour style and no terminal theme",
			input: input{
				glamourStyle:  "somestyle",
				terminalTheme: "none",
			},
			output: output{
				style: "somestyle",
			},
		},
		{
			name: "no glamour style and light terminal theme",
			input: input{
				glamourStyle:  "",
				terminalTheme: "light",
			},
			output: output{
				style: "light",
			},
		},
		{
			name: "no glamour style and dark terminal theme",
			input: input{
				glamourStyle:  "",
				terminalTheme: "dark",
			},
			output: output{
				style: "dark",
			},
		},
		{
			name: "no glamour style and unknown terminal theme",
			input: input{
				glamourStyle:  "",
				terminalTheme: "unknown",
			},
			output: output{
				style: "notty",
			},
		},
	}

	for _, tt := range tests {
		fromEnv = func() string {
			return tt.input.glamourStyle
		}

		t.Run(tt.name, func(t *testing.T) {
			style := GetStyle(tt.input.terminalTheme)
			assert.Equal(t, tt.output.style, style)
		})
	}
}
