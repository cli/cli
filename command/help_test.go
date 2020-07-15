package command

import (
	"testing"
)

func TestStripIndent(t *testing.T) {
	type c struct {
		input    string
		expected string
	}

	cases := []c{
		{
			input:    "      --help      Show help for command\n      --version   Show gh version",
			expected: "--help      Show help for command\n--version   Show gh version\n",
		},
		{
			input:    "      --help              Show help for command\n  -R, --repo OWNER/REPO   Select another repository using the OWNER/REPO format",
			expected: "    --help              Show help for command\n-R, --repo OWNER/REPO   Select another repository using the OWNER/REPO format",
		},
	}

	for _, kase := range cases {
		eq(t, stripIndent(kase.input), kase.expected)
	}
}
