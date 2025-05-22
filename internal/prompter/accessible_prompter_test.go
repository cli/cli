//go:build !windows

package prompter_test

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Netflix/go-expect"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The following tests are broadly testing the accessible prompter, and NOT asserting
// on the prompter's complete and exact output strings.
//
// These tests generally operate with this logic:
// - Wait for a particular substring (a portion of the prompt) to appear
// - Send input
// - Wait for another substring to appear or for control to return to the test
// - Assert that the input value was returned from the prompter function

// In the future, expanding these tests to assert on the exact prompt strings
// would help build confidence in `huh` upgrades, but for now these tests
// are sufficient to ensure that the accessible prompter behaves roughly as expected
// but doesn't mandate that prompts always look exactly the same.
func TestAccessiblePrompter(t *testing.T) {
	t.Run("Select", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAcessiblePrompter(t, console)

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Choose:")
			require.NoError(t, err)

			// Select option 1
			_, err = console.SendLine("1")
			require.NoError(t, err)
		}()

		selectValue, err := p.Select("Select a number", "", []string{"1", "2", "3"})
		require.NoError(t, err)
		assert.Equal(t, 0, selectValue)
	})

	t.Run("MultiSelect", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAcessiblePrompter(t, console)

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Select a number")
			require.NoError(t, err)

			// Select options 1 and 2
			_, err = console.SendLine("1")
			require.NoError(t, err)
			_, err = console.SendLine("2")
			require.NoError(t, err)

			// This confirms selections
			_, err = console.SendLine("0")
			require.NoError(t, err)
		}()

		multiSelectValue, err := p.MultiSelect("Select a number", []string{}, []string{"1", "2", "3"})
		require.NoError(t, err)
		assert.Equal(t, []int{0, 1}, multiSelectValue)
	})

	t.Run("Input", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAcessiblePrompter(t, console)
		dummyText := "12345abcdefg"

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Enter some characters")
			require.NoError(t, err)

			// Enter a number
			_, err = console.SendLine(dummyText)
			require.NoError(t, err)
		}()

		inputValue, err := p.Input("Enter some characters", "")
		require.NoError(t, err)
		assert.Equal(t, dummyText, inputValue)
	})

	t.Run("Input - blank input returns default value", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAcessiblePrompter(t, console)
		dummyDefaultValue := "12345abcdefg"

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Enter some characters")
			require.NoError(t, err)

			// Enter nothing
			_, err = console.SendLine("")
			require.NoError(t, err)
		}()

		inputValue, err := p.Input("Enter some characters", dummyDefaultValue)
		require.NoError(t, err)
		assert.Equal(t, dummyDefaultValue, inputValue)
	})

	t.Run("Password", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAcessiblePrompter(t, console)
		dummyPassword := "12345abcdefg"

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Enter password")
			require.NoError(t, err)

			// Enter a number
			_, err = console.SendLine(dummyPassword)
			require.NoError(t, err)
		}()

		passwordValue, err := p.Password("Enter password")
		require.NoError(t, err)
		require.Equal(t, dummyPassword, passwordValue)
	})

	t.Run("Confirm", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAcessiblePrompter(t, console)

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Are you sure")
			require.NoError(t, err)

			// Confirm
			_, err = console.SendLine("y")
			require.NoError(t, err)
		}()

		confirmValue, err := p.Confirm("Are you sure", false)
		require.NoError(t, err)
		require.Equal(t, true, confirmValue)
	})

	t.Run("Confirm - blank input returns default", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAcessiblePrompter(t, console)

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Are you sure")
			require.NoError(t, err)

			// Enter nothing
			_, err = console.SendLine("")
			require.NoError(t, err)
		}()

		confirmValue, err := p.Confirm("Are you sure", false)
		require.NoError(t, err)
		require.Equal(t, false, confirmValue)
	})

	t.Run("AuthToken", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAcessiblePrompter(t, console)
		dummyAuthToken := "12345abcdefg"

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Paste your authentication token:")
			require.NoError(t, err)

			// Enter some dummy auth token
			_, err = console.SendLine(dummyAuthToken)
			require.NoError(t, err)
		}()

		authValue, err := p.AuthToken()
		require.NoError(t, err)
		require.Equal(t, dummyAuthToken, authValue)
	})

	t.Run("AuthToken - blank input returns error", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAcessiblePrompter(t, console)
		dummyAuthTokenForAfterFailure := "12345abcdefg"

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Paste your authentication token:")
			require.NoError(t, err)

			// Enter nothing
			_, err = console.SendLine("")
			require.NoError(t, err)

			// Expect an error message
			_, err = console.ExpectString("token is required")
			require.NoError(t, err)

			// Now enter some dummy auth token to return control back to the test
			_, err = console.SendLine(dummyAuthTokenForAfterFailure)
			require.NoError(t, err)
		}()

		authValue, err := p.AuthToken()
		require.NoError(t, err)
		require.Equal(t, dummyAuthTokenForAfterFailure, authValue)
	})

	t.Run("ConfirmDeletion", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAcessiblePrompter(t, console)

		requiredValue := "test"
		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString(fmt.Sprintf("Type %q to confirm deletion", requiredValue))
			require.NoError(t, err)

			// Confirm
			_, err = console.SendLine(requiredValue)
			require.NoError(t, err)
		}()

		// An err indicates that the confirmation text sent did not match
		err := p.ConfirmDeletion(requiredValue)
		require.NoError(t, err)
	})

	t.Run("ConfirmDeletion - bad input", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAcessiblePrompter(t, console)
		requiredValue := "test"
		badInputValue := "garbage"

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString(fmt.Sprintf("Type %q to confirm deletion", requiredValue))
			require.NoError(t, err)

			// Confirm with bad input
			_, err = console.SendLine(badInputValue)
			require.NoError(t, err)

			// Expect an error message and loop back to the prompt
			_, err = console.ExpectString(fmt.Sprintf("You entered: %q", badInputValue))
			require.NoError(t, err)

			// Confirm with the correct input to return control back to the test
			_, err = console.SendLine(requiredValue)
			require.NoError(t, err)
		}()

		// An err indicates that the confirmation text sent did not match
		err := p.ConfirmDeletion(requiredValue)
		require.NoError(t, err)
	})

	t.Run("InputHostname", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAcessiblePrompter(t, console)
		hostname := "example.com"

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Hostname:")
			require.NoError(t, err)

			// Enter the hostname
			_, err = console.SendLine(hostname)
			require.NoError(t, err)
		}()

		inputValue, err := p.InputHostname()
		require.NoError(t, err)
		require.Equal(t, hostname, inputValue)
	})

	t.Run("MarkdownEditor - blank allowed with blank input returns blank", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAcessiblePrompter(t, console)

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("How to edit?")
			require.NoError(t, err)

			// Enter 2, to select "skip"
			_, err = console.SendLine("2")
			require.NoError(t, err)
		}()

		inputValue, err := p.MarkdownEditor("How to edit?", "", true)
		require.NoError(t, err)
		require.Equal(t, "", inputValue)
	})

	t.Run("MarkdownEditor - blank disallowed with default value returns default value", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAcessiblePrompter(t, console)
		defaultValue := "12345abcdefg"

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("How to edit?")
			require.NoError(t, err)

			// Enter number 2 to select "skip". This shouldn't be allowed.
			_, err = console.SendLine("2")
			require.NoError(t, err)

			// Expect a notice to enter something valid since blank is disallowed.
			_, err = console.ExpectString("invalid input. please try again")
			require.NoError(t, err)

			// Send a 1 to select to open the editor. This will immediately exit
			_, err = console.SendLine("1")
			require.NoError(t, err)
		}()

		inputValue, err := p.MarkdownEditor("How to edit?", defaultValue, false)
		require.NoError(t, err)
		require.Equal(t, defaultValue, inputValue)
	})

	t.Run("MarkdownEditor - blank disallowed no default value returns error", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAcessiblePrompter(t, console)

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("How to edit?")
			require.NoError(t, err)

			// Enter number 2 to select "skip". This shouldn't be allowed.
			_, err = console.SendLine("2")
			require.NoError(t, err)

			// Expect a notice to enter something valid since blank is disallowed.
			_, err = console.ExpectString("invalid input. please try again")
			require.NoError(t, err)

			// Send a 1 to select to open the editor since skip is invalid and
			// we need to return control back to the test.
			_, err = console.SendLine("1")
			require.NoError(t, err)
		}()

		inputValue, err := p.MarkdownEditor("How to edit?", "", false)
		require.NoError(t, err)
		require.Equal(t, "", inputValue)
	})
}

func TestSurveyPrompter(t *testing.T) {
	// This not a comprehensive test of the survey prompter, but it does
	// demonstrate that the survey prompter is used when the
	// accessible prompter is disabled.
	t.Run("Select uses survey prompter when accessible prompter is disabled", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestSurveyPrompter(t, console)

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Select a number")
			require.NoError(t, err)

			// Send a newline to select the first option
			// Note: This would not work with the accessible prompter
			// because it would requires sending a 1 to select the first option.
			// So it proves we are seeing a survey prompter.
			_, err = console.SendLine("")
			require.NoError(t, err)
		}()

		selectValue, err := p.Select("Select a number", "", []string{"1", "2", "3"})
		require.NoError(t, err)
		assert.Equal(t, 0, selectValue)
	})
}

func newTestVirtualTerminal(t *testing.T) *expect.Console {
	t.Helper()

	// Create a PTY and hook up a virtual terminal emulator
	ptm, pts, err := pty.Open()
	require.NoError(t, err)

	term := vt10x.New(vt10x.WithWriter(pts))

	// Create a console via Expect that allows scripting against the terminal
	consoleOpts := []expect.ConsoleOpt{
		expect.WithStdin(ptm),
		expect.WithStdout(term),
		expect.WithCloser(ptm, pts),
		failOnExpectError(t),
		failOnSendError(t),
		expect.WithDefaultTimeout(time.Second),
	}

	console, err := expect.NewConsole(consoleOpts...)
	require.NoError(t, err)
	t.Cleanup(func() { testCloser(t, console) })

	return console
}

func newTestAcessiblePrompter(t *testing.T, console *expect.Console) prompter.Prompter {
	t.Helper()

	t.Setenv("GH_ACCESSIBLE_PROMPTER", "true")
	// `echo`` is chose as the editor command because it immediately returns
	// a success exit code, returns an empty string, doesn't require any user input,
	// and since this file is only built on Linux, it is near guaranteed to be available.
	return prompter.New("echo", console.Tty(), console.Tty(), console.Tty())
}

func newTestSurveyPrompter(t *testing.T, console *expect.Console) prompter.Prompter {
	t.Helper()

	t.Setenv("GH_ACCESSIBLE_PROMPTER", "false")
	return prompter.New("echo", console.Tty(), console.Tty(), console.Tty())
}

// failOnExpectError adds an observer that will fail the test in a standardised way
// if any expectation on the command output fails, without requiring an explicit
// assertion.
//
// Use WithRelaxedIO to disable this behaviour.
func failOnExpectError(t *testing.T) expect.ConsoleOpt {
	t.Helper()
	return expect.WithExpectObserver(
		func(matchers []expect.Matcher, buf string, err error) {
			t.Helper()

			if err == nil {
				return
			}

			if len(matchers) == 0 {
				t.Fatalf("Error occurred while matching %q: %s\n", buf, err)
			}

			var criteria []string
			for _, matcher := range matchers {
				criteria = append(criteria, fmt.Sprintf("%q", matcher.Criteria()))
			}
			t.Fatalf("Failed to find [%s] in %q: %s\n", strings.Join(criteria, ", "), buf, err)
		},
	)
}

// failOnSendError adds an observer that will fail the test in a standardised way
// if any sending of input fails, without requiring an explicit assertion.
//
// Use WithRelaxedIO to disable this behaviour.
func failOnSendError(t *testing.T) expect.ConsoleOpt {
	t.Helper()
	return expect.WithSendObserver(
		func(msg string, n int, err error) {
			t.Helper()

			if err != nil {
				t.Fatalf("Failed to send %q: %s\n", msg, err)
			}
			if len(msg) != n {
				t.Fatalf("Only sent %d of %d bytes for %q\n", n, len(msg), msg)
			}
		},
	)
}

// testCloser is a helper to fail the test if a Closer fails to close.
func testCloser(t *testing.T, closer io.Closer) {
	t.Helper()
	if err := closer.Close(); err != nil {
		t.Errorf("Close failed: %s", err)
	}
}
