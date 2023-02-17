package prompter

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test helpers

func IndexFor(options []string, answer string) (int, error) {
	for ix, a := range options {
		if a == answer {
			return ix, nil
		}
	}
	return -1, NoSuchAnswerErr(answer)
}

func AssertOptions(t *testing.T, expected, actual []string) {
	assert.Equal(t, expected, actual)
}

func NoSuchAnswerErr(answer string) error {
	return fmt.Errorf("no such answer '%s'", answer)
}

func NoSuchPromptErr(prompt string) error {
	return fmt.Errorf("no such prompt '%s'", prompt)
}
