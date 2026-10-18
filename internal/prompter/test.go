package prompter

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func NewMockPrompter(t *testing.T) *MockPrompter {
	m := &MockPrompter{
		t:                    t,
		authTokenStubs:       []authTokenStub{},
		confirmDeletionStubs: []confirmDeletionStub{},
		inputHostnameStubs:   []inputHostnameStub{},
		markdownEditorStubs:  []markdownEditorStub{},
		selectStubs:          []selectStub{},
		multiSelectStubs:     []multiSelectStub{},
		inputStubs:           []inputStub{},
		passwordStubs:        []passwordStub{},
		confirmStubs:         []confirmStub{},
	}
	t.Cleanup(m.Verify)
	return m
}

type MockPrompter struct {
	t                          *testing.T
	authTokenStubs             []authTokenStub
	confirmDeletionStubs       []confirmDeletionStub
	inputHostnameStubs         []inputHostnameStub
	markdownEditorStubs        []markdownEditorStub
	multiSelectWithSearchStubs []multiSelectWithSearchStub
	selectStubs                []selectStub
	multiSelectStubs           []multiSelectStub
	inputStubs                 []inputStub
	passwordStubs              []passwordStub
	confirmStubs               []confirmStub
}

type selectStub struct {
	prompt          string
	expectedOptions []string
	fn              func(string, string, []string) (int, error)
}

type multiSelectStub struct {
	prompt          string
	expectedOptions []string
	fn              func(string, []string, []string) ([]int, error)
}

type inputStub struct {
	prompt string
	fn     func(string, string) (string, error)
}

type passwordStub struct {
	prompt string
	fn     func(string) (string, error)
}

type confirmStub struct {
	prompt string
	fn     func(string, bool) (bool, error)
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

type multiSelectWithSearchStub struct {
	fn func(string, string, []string, []string, func(string) MultiSelectSearchResult) ([]string, error)
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

func (m *MockPrompter) MultiSelectWithSearch(prompt, searchPrompt string, defaults []string, persistentOptions []string, searchFunc func(string) MultiSelectSearchResult) ([]string, error) {
	var s multiSelectWithSearchStub
	if len(m.multiSelectWithSearchStubs) == 0 {
		return nil, NoSuchPromptErr(prompt)
	}
	s = m.multiSelectWithSearchStubs[0]
	m.multiSelectWithSearchStubs = m.multiSelectWithSearchStubs[1:len(m.multiSelectWithSearchStubs)]
	return s.fn(prompt, searchPrompt, defaults, persistentOptions, searchFunc)
}

func (m *MockPrompter) Select(prompt, defaultValue string, options []string) (int, error) {
	if len(m.selectStubs) == 0 {
		return -1, NoSuchPromptErr(prompt)
	}
	stub := m.selectStubs[0]
	m.selectStubs = m.selectStubs[1:]
	if stub.prompt != prompt {
		return -1, NoSuchPromptErr(prompt)
	}
	AssertOptions(m.t, stub.expectedOptions, options)
	return stub.fn(prompt, defaultValue, options)
}

func (m *MockPrompter) MultiSelect(prompt string, defaults, options []string) ([]int, error) {
	if len(m.multiSelectStubs) == 0 {
		return []int{}, NoSuchPromptErr(prompt)
	}
	stub := m.multiSelectStubs[0]
	m.multiSelectStubs = m.multiSelectStubs[1:]
	if stub.prompt != prompt {
		return []int{}, NoSuchPromptErr(prompt)
	}
	AssertOptions(m.t, stub.expectedOptions, options)
	return stub.fn(prompt, defaults, options)
}

func (m *MockPrompter) Input(prompt, defaultValue string) (string, error) {
	if len(m.inputStubs) == 0 {
		return "", NoSuchPromptErr(prompt)
	}
	stub := m.inputStubs[0]
	m.inputStubs = m.inputStubs[1:]
	if stub.prompt != prompt {
		return "", NoSuchPromptErr(prompt)
	}
	return stub.fn(prompt, defaultValue)
}

func (m *MockPrompter) Password(prompt string) (string, error) {
	if len(m.passwordStubs) == 0 {
		return "", NoSuchPromptErr(prompt)
	}
	stub := m.passwordStubs[0]
	m.passwordStubs = m.passwordStubs[1:]
	if stub.prompt != prompt {
		return "", NoSuchPromptErr(prompt)
	}
	return stub.fn(prompt)
}

func (m *MockPrompter) Confirm(prompt string, defaultValue bool) (bool, error) {
	if len(m.confirmStubs) == 0 {
		return false, NoSuchPromptErr(prompt)
	}
	stub := m.confirmStubs[0]
	m.confirmStubs = m.confirmStubs[1:]
	if stub.prompt != prompt {
		return false, NoSuchPromptErr(prompt)
	}
	return stub.fn(prompt, defaultValue)
}

func (m *MockPrompter) RegisterSelect(prompt string, options []string, fn func(string, string, []string) (int, error)) {
	m.selectStubs = append(m.selectStubs, selectStub{prompt: prompt, expectedOptions: options, fn: fn})
}

func (m *MockPrompter) RegisterMultiSelect(prompt string, _ []string, options []string, fn func(string, []string, []string) ([]int, error)) {
	m.multiSelectStubs = append(m.multiSelectStubs, multiSelectStub{prompt: prompt, expectedOptions: options, fn: fn})
}

func (m *MockPrompter) RegisterInput(prompt string, fn func(string, string) (string, error)) {
	m.inputStubs = append(m.inputStubs, inputStub{prompt: prompt, fn: fn})
}

func (m *MockPrompter) RegisterPassword(prompt string, fn func(string) (string, error)) {
	m.passwordStubs = append(m.passwordStubs, passwordStub{prompt: prompt, fn: fn})
}

func (m *MockPrompter) RegisterConfirm(prompt string, fn func(string, bool) (bool, error)) {
	m.confirmStubs = append(m.confirmStubs, confirmStub{prompt: prompt, fn: fn})
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
	if len(m.selectStubs) > 0 {
		errs = append(errs, "Select")
	}
	if len(m.multiSelectStubs) > 0 {
		errs = append(errs, "MultiSelect")
	}
	if len(m.inputStubs) > 0 {
		errs = append(errs, "Input")
	}
	if len(m.passwordStubs) > 0 {
		errs = append(errs, "Password")
	}
	if len(m.confirmStubs) > 0 {
		errs = append(errs, "Confirm")
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

func IndexesFor(options []string, answers ...string) ([]int, error) {
	indexes := make([]int, len(answers))
	for i, answer := range answers {
		index, err := IndexFor(options, answer)
		if err != nil {
			return nil, err
		}
		indexes[i] = index
	}
	return indexes, nil
}

func NoSuchPromptErr(prompt string) error {
	return fmt.Errorf("no such prompt '%s'", prompt)
}

func NoSuchAnswerErr(answer string, options []string) error {
	return fmt.Errorf("no such answer '%s' in [%s]", answer, strings.Join(options, ", "))
}
