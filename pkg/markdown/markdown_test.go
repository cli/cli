package markdown

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Render(t *testing.T) {
	os.Unsetenv("GLAMOUR_STYLE")

	type input struct {
		text  string
		theme string
	}
	tests := []struct {
		name     string
		input    input
		wantsErr bool
	}{
		{
			name: "light theme",
			input: input{
				text:  "some text",
				theme: "light",
			},
			wantsErr: false,
		},
		{
			name: "dark theme",
			input: input{
				text:  "some text",
				theme: "dark",
			},
			wantsErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Render(tt.input.text, WithIO(terminalThemer(tt.input.theme)))
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

type terminalThemer string

func (tt terminalThemer) TerminalTheme() string {
	return string(tt)
}
