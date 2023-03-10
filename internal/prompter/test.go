package prompter

import (
	"fmt"
	"strings"
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

type SelectStub struct {
	Prompt       string
	ExpectedOpts []string
	Fn           func(string, string, []string) (int, error)
}

type InputStub struct {
	Prompt string
	Fn     func(string, string) (string, error)
}

type ConfirmStub struct {
	Prompt string
	Fn     func(string, bool) (bool, error)
}

type MultiSelectStub struct {
	Prompt       string
	ExpectedOpts []string
	Fn           func(string, string, []string) ([]string, error)
}

type InputHostnameStub struct {
	Fn func() (string, error)
}

type PasswordStub struct {
	Prompt string
	Fn     func(string) (string, error)
}

type AuthTokenStub struct {
	Fn func() (string, error)
}

type ConfirmDeletionStub struct {
	Prompt string
	Fn     func(string) error
}

type MockPrompter struct {
	PrompterMock
	t                    *testing.T
	SelectStubs          []SelectStub
	InputStubs           []InputStub
	ConfirmStubs         []ConfirmStub
	MultiSelectStubs     []MultiSelectStub
	InputHostnameStubs   []InputHostnameStub
	PasswordStubs        []PasswordStub
	AuthTokenStubs       []AuthTokenStub
	ConfirmDeletionStubs []ConfirmDeletionStub
}

// TODO thread safety

func NewMockPrompter(t *testing.T) *MockPrompter {
	m := &MockPrompter{
		t:                    t,
		SelectStubs:          []SelectStub{},
		InputStubs:           []InputStub{},
		ConfirmStubs:         []ConfirmStub{},
		MultiSelectStubs:     []MultiSelectStub{},
		InputHostnameStubs:   []InputHostnameStub{},
		PasswordStubs:        []PasswordStub{},
		AuthTokenStubs:       []AuthTokenStub{},
		ConfirmDeletionStubs: []ConfirmDeletionStub{},
	}

	t.Cleanup(m.Verify)

	m.SelectFunc = func(p, d string, opts []string) (int, error) {
		var s SelectStub

		if len(m.SelectStubs) > 0 {
			s = m.SelectStubs[0]
			m.SelectStubs = m.SelectStubs[1:len(m.SelectStubs)]
		} else {
			return -1, NoSuchPromptErr(p)
		}

		if s.Prompt != p {
			return -1, NoSuchPromptErr(p)
		}

		AssertOptions(m.t, s.ExpectedOpts, opts)

		return s.Fn(p, d, opts)
	}

	m.MultiSelectFunc = func(p, d string, opts []string) ([]string, error) {
		var s MultiSelectStub
		if len(m.SelectStubs) > 0 {
			s = m.MultiSelectStubs[0]
			m.SelectStubs = m.SelectStubs[1:len(m.SelectStubs)]
		} else {
			return []string{}, NoSuchPromptErr(p)
		}

		if s.Prompt != p {
			return []string{}, NoSuchPromptErr(p)
		}

		AssertOptions(m.t, s.ExpectedOpts, opts)

		return s.Fn(p, d, opts)
	}

	m.InputFunc = func(p, d string) (string, error) {
		var s InputStub

		if len(m.InputStubs) > 0 {
			s = m.InputStubs[0]
			m.InputStubs = m.InputStubs[1:len(m.InputStubs)]
		} else {
			return "", NoSuchPromptErr(p)
		}

		if s.Prompt != p {
			return "", NoSuchPromptErr(p)
		}

		return s.Fn(p, d)
	}

	m.ConfirmFunc = func(p string, d bool) (bool, error) {
		var s ConfirmStub

		if len(m.ConfirmStubs) > 0 {
			s = m.ConfirmStubs[0]
			m.ConfirmStubs = m.ConfirmStubs[1:len(m.ConfirmStubs)]
		} else {
			return false, NoSuchPromptErr(p)
		}

		if s.Prompt != p {
			return false, NoSuchPromptErr(p)
		}

		return s.Fn(p, d)
	}

	m.InputHostnameFunc = func() (string, error) {
		var s InputHostnameStub

		if len(m.InputHostnameStubs) > 0 {
			s = m.InputHostnameStubs[0]
			m.InputHostnameStubs = m.InputHostnameStubs[1:len(m.InputHostnameStubs)]
		} else {
			return "", NoSuchPromptErr("InputHostname")
		}

		return s.Fn()
	}

	m.PasswordFunc = func(p string) (string, error) {
		var s PasswordStub

		if len(m.PasswordStubs) > 0 {
			s = m.PasswordStubs[0]
			m.PasswordStubs = m.PasswordStubs[1:len(m.PasswordStubs)]
		} else {
			return "", NoSuchPromptErr(p)
		}

		if s.Prompt != p {
			return "", NoSuchPromptErr(p)
		}

		return s.Fn(p)
	}

	m.AuthTokenFunc = func() (string, error) {
		var s AuthTokenStub

		if len(m.AuthTokenStubs) > 0 {
			s = m.AuthTokenStubs[0]
			m.AuthTokenStubs = m.AuthTokenStubs[1:len(m.AuthTokenStubs)]
		} else {
			return "", NoSuchPromptErr("AuthToken")
		}

		return s.Fn()
	}

	m.ConfirmDeletionFunc = func(p string) error {
		var s ConfirmDeletionStub

		if len(m.ConfirmDeletionStubs) > 0 {
			s = m.ConfirmDeletionStubs[0]
			m.ConfirmDeletionStubs = m.ConfirmDeletionStubs[1:len(m.ConfirmDeletionStubs)]
		} else {
			return NoSuchPromptErr("ConfirmDeletion")
		}

		return s.Fn(p)
	}

	// TODO MarkdownEditor(string, string, bool) (string, error)

	return m
}

func (m *MockPrompter) RegisterSelect(prompt string, opts []string, stub func(_, _ string, _ []string) (int, error)) {
	m.SelectStubs = append(m.SelectStubs, SelectStub{
		Prompt:       prompt,
		ExpectedOpts: opts,
		Fn:           stub})
}

func (m *MockPrompter) RegisterMultiSelect(prompt string, opts []string, stub func(_, _ string, _ []string) ([]string, error)) {
	m.MultiSelectStubs = append(m.MultiSelectStubs, MultiSelectStub{
		Prompt:       prompt,
		ExpectedOpts: opts,
		Fn:           stub})
}

func (m *MockPrompter) RegisterInput(prompt string, stub func(_, _ string) (string, error)) {
	m.InputStubs = append(m.InputStubs, InputStub{Prompt: prompt, Fn: stub})
}

func (m *MockPrompter) RegisterConfirm(prompt string, stub func(_ string, _ bool) (bool, error)) {
	m.ConfirmStubs = append(m.ConfirmStubs, ConfirmStub{Prompt: prompt, Fn: stub})
}

func (m *MockPrompter) RegisterInputHostname(stub func() (string, error)) {
	m.InputHostnameStubs = append(m.InputHostnameStubs, InputHostnameStub{Fn: stub})
}

func (m *MockPrompter) RegisterPassword(prompt string, stub func(string) (string, error)) {
	m.PasswordStubs = append(m.PasswordStubs, PasswordStub{Fn: stub})
}

func (m *MockPrompter) RegisterConfirmDeletion(prompt string, stub func(string) error) {
	m.ConfirmDeletionStubs = append(m.ConfirmDeletionStubs, ConfirmDeletionStub{Prompt: prompt, Fn: stub})
}

func (m *MockPrompter) Verify() {
	errs := []string{}
	if len(m.SelectStubs) > 0 {
		errs = append(errs, "Select")
	}
	if len(m.InputStubs) > 0 {
		errs = append(errs, "Input")
	}
	if len(m.ConfirmStubs) > 0 {
		errs = append(errs, "Confirm")
	}
	// TODO other prompt types

	if len(errs) > 0 {
		m.t.Helper()
		m.t.Errorf("%d unmatched calls to %s", len(errs), strings.Join(errs, ","))
	}
}
