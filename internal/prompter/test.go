package prompter

import (
	"fmt"
	"strings"
	"testing"

	ghPrompter "github.com/cli/go-gh/v2/pkg/prompter"
	"github.com/stretchr/testify/assert"
)

func NewMockPrompter(t *testing.T) *MockPrompter {
	m := &MockPrompter{
		t:                    t,
		PrompterMock:         *ghPrompter.NewMock(t),
		authTokenStubs:       []authTokenStub{},
		confirmDeletionStubs: []confirmDeletionStub{},
		inputHostnameStubs:   []inputHostnameStub{},
		markdownEditorStubs:  []markdownEditorStub{},
	}
	t.Cleanup(m.Verify)
	return m
}

type MockPrompter struct {
	t *testing.T
	ghPrompter.PrompterMock
	authTokenStubs       []authTokenStub
	confirmDeletionStubs []confirmDeletionStub
	inputHostnameStubs   []inputHostnameStub
	markdownEditorStubs  []markdownEditorStub
}

type authTokenStub struct {
	fn func() (string, error)
}

type confirmDeletionStub struct {
	prompt string
	fn     func(string) error
}

type inputHostnameStub struct {
	fn func() (string, error)
}

type markdownEditorStub struct {
	prompt string
	fn     func(string, string, bool) (string, error)
}

func (m *MockPrompter) AuthToken() (string, error) {
	var s authTokenStub
	if len(m.authTokenStubs) == 0 {
		return "", NoSuchPromptErr("AuthToken")
	}
	s = m.authTokenStubs[0]
	m.authTokenStubs = m.authTokenStubs[1:len(m.authTokenStubs)]
	return s.fn()
}

func (m *MockPrompter) ConfirmDeletion(prompt string) error {
	var s confirmDeletionStub
	if len(m.confirmDeletionStubs) == 0 {
		return NoSuchPromptErr("ConfirmDeletion")
	}
	s = m.confirmDeletionStubs[0]
	m.confirmDeletionStubs = m.confirmDeletionStubs[1:len(m.confirmDeletionStubs)]
	return s.fn(prompt)
}

func (m *MockPrompter) InputHostname() (string, error) {
	var s inputHostnameStub
	if len(m.inputHostnameStubs) == 0 {
		return "", NoSuchPromptErr("InputHostname")
	}
	s = m.inputHostnameStubs[0]
	m.inputHostnameStubs = m.inputHostnameStubs[1:len(m.inputHostnameStubs)]
	return s.fn()
}

func (m *MockPrompter) MarkdownEditor(prompt, defaultValue string, blankAllowed bool) (string, error) {
	var s markdownEditorStub
	if len(m.markdownEditorStubs) == 0 {
		return "", NoSuchPromptErr(prompt)
	}
	s = m.markdownEditorStubs[0]
	m.markdownEditorStubs = m.markdownEditorStubs[1:len(m.markdownEditorStubs)]
	if s.prompt != prompt {
		return "", NoSuchPromptErr(prompt)
	}
	return s.fn(prompt, defaultValue, blankAllowed)
}

func (m *MockPrompter) RegisterAuthToken(stub func() (string, error)) {
	m.authTokenStubs = append(m.authTokenStubs, authTokenStub{fn: stub})
}

func (m *MockPrompter) RegisterConfirmDeletion(prompt string, stub func(string) error) {
	m.confirmDeletionStubs = append(m.confirmDeletionStubs, confirmDeletionStub{prompt: prompt, fn: stub})
}

func (m *MockPrompter) RegisterInputHostname(stub func() (string, error)) {
	m.inputHostnameStubs = append(m.inputHostnameStubs, inputHostnameStub{fn: stub})
}

func (m *MockPrompter) RegisterMarkdownEditor(prompt string, stub func(string, string, bool) (string, error)) {
	m.markdownEditorStubs = append(m.markdownEditorStubs, markdownEditorStub{prompt: prompt, fn: stub})
}

func (m *MockPrompter) Verify() {
	errs := []string{}
	if len(m.authTokenStubs) > 0 {
		errs = append(errs, "AuthToken")
	}
	if len(m.confirmDeletionStubs) > 0 {
		errs = append(errs, "ConfirmDeletion")
	}
	if len(m.inputHostnameStubs) > 0 {
		errs = append(errs, "inputHostname")
	}
	if len(m.markdownEditorStubs) > 0 {
		errs = append(errs, "markdownEditorStubs")
	}
	if len(errs) > 0 {
		m.t.Helper()
		m.t.Errorf("%d unmatched calls to %s", len(errs), strings.Join(errs, ","))
	}
}

func AssertOptions(t *testing.T, expected, actual []string) {
	assert.Equal(t, expected, actual)
}

func IndexFor(options []string, answer string) (int, error) {
	for ix, a := range options {
		if a == answer {
			return ix, nil
		}
	}
	return -1, NoSuchAnswerErr(answer, options)
}

func NoSuchPromptErr(prompt string) error {
	return fmt.Errorf("no such prompt '%s'", prompt)
}

func NoSuchAnswerErr(answer string, options []string) error {
	return fmt.Errorf("no such answer '%s' in [%s]", answer, strings.Join(options, ", "))
}
