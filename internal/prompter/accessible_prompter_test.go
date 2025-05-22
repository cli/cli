//go:build !windows

package prompter_test

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Netflix/go-expect"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/iostreams"
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

	beforePasswordSendTimeout := 100 * time.Microsecond

	t.Run("Select", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAccessiblePrompter(t, console)

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Input a number between 1 and 3:")
			require.NoError(t, err)

			// Select option 1
			_, err = console.SendLine("1")
			require.NoError(t, err)
		}()

		selectValue, err := p.Select("Select a number", "", []string{"1", "2", "3"})
		require.NoError(t, err)
		assert.Equal(t, 0, selectValue)
	})

	t.Run("Select - blank input returns default value", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAccessiblePrompter(t, console)
		dummyDefaultValue := "12345abcdefg"
		options := []string{"1", "2", dummyDefaultValue}

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Input a number between 1 and 3:")
			require.NoError(t, err)

			// Just press enter to accept the default
			_, err = console.SendLine("")
			require.NoError(t, err)
		}()

		selectValue, err := p.Select("Select a number", dummyDefaultValue, options)
		require.NoError(t, err)

		expectedIndex := slices.Index(options, dummyDefaultValue)
		assert.Equal(t, expectedIndex, selectValue)
	})

	t.Run("Select - default value is in prompt and in readable format", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAccessiblePrompter(t, console)
		dummyDefaultValue := "12345abcdefg"
		options := []string{"1", "2", dummyDefaultValue}

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Select a number (default: 12345abcdefg)")
			require.NoError(t, err)

			// Just press enter to accept the default
			_, err = console.SendLine("")
			require.NoError(t, err)
		}()

		selectValue, err := p.Select("Select a number", dummyDefaultValue, options)
		require.NoError(t, err)

		expectedIndex := slices.Index(options, dummyDefaultValue)
		assert.Equal(t, expectedIndex, selectValue)
	})

	t.Run("Select - invalid defaults are excluded from prompt", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAccessiblePrompter(t, console)
		dummyDefaultValue := "foo"
		options := []string{"1", "2"}

		go func() {
			// Wait for prompt to appear without the invalid default value
			_, err := console.ExpectString("Select a number \r\n")
			require.NoError(t, err)

			// Select option 2
			_, err = console.SendLine("2")
			require.NoError(t, err)
		}()

		selectValue, err := p.Select("Select a number", dummyDefaultValue, options)
		require.NoError(t, err)
		assert.Equal(t, 1, selectValue)
	})

	t.Run("MultiSelect", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAccessiblePrompter(t, console)

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Input a number between 0 and 3:")
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

	t.Run("MultiSelect - default values are respected by being pre-selected", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAccessiblePrompter(t, console)

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Select a number")
			require.NoError(t, err)

			// Don't select anything because the default should be selected.

			// This confirms selections
			_, err = console.SendLine("0")
			require.NoError(t, err)
		}()

		multiSelectValue, err := p.MultiSelect("Select a number", []string{"2"}, []string{"1", "2", "3"})
		require.NoError(t, err)
		assert.Equal(t, []int{1}, multiSelectValue)
	})

	t.Run("MultiSelect - default value is in prompt and in readable format", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAccessiblePrompter(t, console)
		dummyDefaultValues := []string{"foo", "bar"}
		options := []string{"1", "2"}
		options = append(options, dummyDefaultValues...)

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Select a number (defaults: foo, bar)")
			require.NoError(t, err)

			// Don't select anything because the defaults should be selected.

			// This confirms selections
			_, err = console.SendLine("0")
			require.NoError(t, err)
		}()

		multiSelectValues, err := p.MultiSelect("Select a number", dummyDefaultValues, options)
		require.NoError(t, err)
		var expectedIndices []int

		// Get the indices of the default values within the options slice
		// as that's what we expect the prompter to return when no selections are made.
		for _, defaultValue := range dummyDefaultValues {
			expectedIndices = append(expectedIndices, slices.Index(options, defaultValue))
		}
		assert.Equal(t, expectedIndices, multiSelectValues)
	})

	t.Run("MultiSelect - invalid defaults are excluded from prompt", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAccessiblePrompter(t, console)
		dummyDefaultValues := []string{"foo", "bar"}
		options := []string{"1", "2"}

		go func() {
			// Wait for prompt to appear without the invalid default values
			_, err := console.ExpectString("Select a number \r\n")
			require.NoError(t, err)

			// Not selecting anything will fail because there are no defaults.
			_, err = console.SendLine("2")
			require.NoError(t, err)

			// This confirms selections
			_, err = console.SendLine("0")
			require.NoError(t, err)
		}()

		multiSelectValues, err := p.MultiSelect("Select a number", dummyDefaultValues, options)
		require.NoError(t, err)
		assert.Equal(t, []int{1}, multiSelectValues)
	})

	t.Run("Input", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAccessiblePrompter(t, console)
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
		p := newTestAccessiblePrompter(t, console)
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

	t.Run("Input - default value is in prompt and in readable format", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAccessiblePrompter(t, console)
		dummyDefaultValue := "12345abcdefg"

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Enter some characters (default: 12345abcdefg)")
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
		p := newTestAccessiblePrompter(t, console)
		dummyPassword := "12345abcdefg"

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Enter password")
			require.NoError(t, err)

			// Wait to ensure huh has time to set the echo mode
			time.Sleep(beforePasswordSendTimeout)

			// Enter a number
			_, err = console.SendLine(dummyPassword)
			require.NoError(t, err)
		}()

		passwordValue, err := p.Password("Enter password")
		require.NoError(t, err)
		require.Equal(t, dummyPassword, passwordValue)

		// Ensure the dummy password is not printed to the screen,
		// asserting that echo mode is disabled.
		//
		// Note that since console.ExpectString returns successful if the
		// expected string matches any part of the stream, we have to use an
		// anchored regexp (i.e., with ^ and $) to make sure the password/token
		// is not printed at all.
		_, err = console.Expect(expect.RegexpPattern("^ \r\n\r\n$"))
		require.NoError(t, err)
	})

	t.Run("Confirm", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAccessiblePrompter(t, console)

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
		p := newTestAccessiblePrompter(t, console)

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

	t.Run("Confirm - default value is in prompt and in readable format", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAccessiblePrompter(t, console)
		defaultValue := true

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Are you sure (default: yes)")
			require.NoError(t, err)

			// Enter nothing
			_, err = console.SendLine("")
			require.NoError(t, err)
		}()

		confirmValue, err := p.Confirm("Are you sure", defaultValue)
		require.NoError(t, err)
		require.Equal(t, defaultValue, confirmValue)
	})

	t.Run("AuthToken", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAccessiblePrompter(t, console)
		dummyAuthToken := "12345abcdefg"

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("Paste your authentication token:")
			require.NoError(t, err)

			// Wait to ensure huh has time to set the echo mode
			time.Sleep(beforePasswordSendTimeout)

			// Enter some dummy auth token
			_, err = console.SendLine(dummyAuthToken)
			require.NoError(t, err)
		}()

		authValue, err := p.AuthToken()
		require.NoError(t, err)
		require.Equal(t, dummyAuthToken, authValue)

		// Ensure the dummy password is not printed to the screen,
		// asserting that echo mode is disabled.
		//
		// Note that since console.ExpectString returns successful if the
		// expected string matches any part of the stream, we have to use an
		// anchored regexp (i.e., with ^ and $) to make sure the password/token
		// is not printed at all.
		_, err = console.Expect(expect.RegexpPattern("^ \r\n\r\n$"))
		require.NoError(t, err)
	})

	t.Run("AuthToken - blank input returns error", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAccessiblePrompter(t, console)
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

			// Wait for the retry prompt
			_, err = console.ExpectString("Paste your authentication token:")
			require.NoError(t, err)

			// Wait to ensure huh has time to set the echo mode
			time.Sleep(beforePasswordSendTimeout)

			// Now enter some dummy auth token to return control back to the test
			_, err = console.SendLine(dummyAuthTokenForAfterFailure)
			require.NoError(t, err)
		}()

		authValue, err := p.AuthToken()
		require.NoError(t, err)
		require.Equal(t, dummyAuthTokenForAfterFailure, authValue)

		// Ensure the dummy password is not printed to the screen,
		// asserting that echo mode is disabled.
		//
		// Note that since console.ExpectString returns successful if the
		// expected string matches any part of the stream, we have to use an
		// anchored regexp (i.e., with ^ and $) to make sure the password/token
		// is not printed at all.
		_, err = console.Expect(expect.RegexpPattern("^ \r\n\r\n$"))
		require.NoError(t, err)
	})

	t.Run("ConfirmDeletion", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		p := newTestAccessiblePrompter(t, console)

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
		p := newTestAccessiblePrompter(t, console)
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
		p := newTestAccessiblePrompter(t, console)
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
		p := newTestAccessiblePrompter(t, console)

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
		p := newTestAccessiblePrompter(t, console)
		defaultValue := "12345abcdefg"

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("How to edit?")
			require.NoError(t, err)

			// Enter number 2 to select "skip". This shouldn't be allowed.
			_, err = console.SendLine("2")
			require.NoError(t, err)

			// Expect a notice to enter something valid since blank is disallowed.
			_, err = console.ExpectString("Invalid: must be between 1 and 1")
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
		p := newTestAccessiblePrompter(t, console)

		go func() {
			// Wait for prompt to appear
			_, err := console.ExpectString("How to edit?")
			require.NoError(t, err)

			// Enter number 2 to select "skip". This shouldn't be allowed.
			_, err = console.SendLine("2")
			require.NoError(t, err)

			// Expect a notice to enter something valid since blank is disallowed.
			_, err = console.ExpectString("Invalid: must be between 1 and 1")
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

func newTestVirtualTerminalIOStreams(t *testing.T, console *expect.Console) *iostreams.IOStreams {
	t.Helper()
	io := &iostreams.IOStreams{
		In:     console.Tty(),
		Out:    console.Tty(),
		ErrOut: console.Tty(),
	}
	io.SetStdinTTY(false)
	io.SetStdoutTTY(false)
	io.SetStderrTTY(false)
	return io
}

// `echo` is chosen as the editor command because it immediately returns
// a success exit code, returns an empty string, doesn't require any user input,
// and since this file is only built on Linux, it is near guaranteed to be available.
var editorCmd = "echo"

func newTestAccessiblePrompter(t *testing.T, console *expect.Console) prompter.Prompter {
	t.Helper()

	io := newTestVirtualTerminalIOStreams(t, console)
	io.SetAccessiblePrompterEnabled(true)

	return prompter.New(editorCmd, io)
}

func newTestSurveyPrompter(t *testing.T, console *expect.Console) prompter.Prompter {
	t.Helper()

	io := newTestVirtualTerminalIOStreams(t, console)
	io.SetAccessiblePrompterEnabled(false)

	return prompter.New(editorCmd, io)
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
