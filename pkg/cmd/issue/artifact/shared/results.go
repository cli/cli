package shared

import (
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/iostreams"
)

// Result identifies the artifact on a result line. A command that acts on
// several artifacts reports one line for each, with PrintSuccess, PrintSkip
// and PrintFailure, so every such command shares one piped format.
type Result struct {
	// Number is the artifact's number. 0 leaves its column empty, for an
	// artifact that has no number.
	Number int
	// Name is the artifact's name as the command prints it, with its
	// whitespace cleaned up. A command that writes files gives the path it
	// wrote instead.
	Name string
	// Source is the input as typed, such as a file path, or empty when there
	// is none.
	Source string
}

// PrintSuccess prints the line for an artifact the command acted on. In a
// terminal, the line is icon and message on stdout. Otherwise it is a piped
// line on stdout whose status is status, such as "deleted", with an empty
// reason.
func PrintSuccess(ios *iostreams.IOStreams, status string, r Result, icon, message string) {
	if !ios.IsStdoutTTY() {
		printResultLine(ios.Out, status, r, "")
		return
	}
	fmt.Fprintf(ios.Out, "%s %s\n", icon, message)
}

// PrintSkip prints the line for an artifact the command left alone as asked,
// such as a download whose file already exists with --skip-existing. A skip
// isn't a failure, so in a terminal it goes to stdout with the successes,
// reading "- Skipped <subject>: <reason>" with a muted dash. Otherwise it is a
// piped line on stdout whose status is "skipped".
func PrintSkip(ios *iostreams.IOStreams, r Result, subject, reason string) {
	if !ios.IsStdoutTTY() {
		printResultLine(ios.Out, "skipped", r, reason)
		return
	}
	fmt.Fprintf(ios.Out, "%s Skipped %s: %s\n", ios.ColorScheme().Muted("-"), subject, reason)
}

// PrintFailure prints the line for an artifact the command couldn't act on,
// with err's reason. In a terminal, it reads "X Failed to <action>: <reason>"
// on stderr. Otherwise it is a piped line on stdout whose status is "failed".
// The command exits 1 after any failure.
func PrintFailure(ios *iostreams.IOStreams, r Result, action string, err error) {
	reason := failureReason(err)
	if !ios.IsStdoutTTY() {
		printResultLine(ios.Out, "failed", r, reason)
		return
	}
	fmt.Fprintf(ios.ErrOut, "%s Failed to %s: %s\n", ios.ColorScheme().FailureIcon(), action, reason)
}

// printResultLine prints a piped result line. Every line has all five
// tab-separated columns, even empty ones, like gh pr checks rows.
func printResultLine(w io.Writer, status string, r Result, reason string) {
	var number string
	if r.Number > 0 {
		number = strconv.Itoa(r.Number)
	}
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", status, number, r.Name, r.Source, reason)
}

// failureReason returns what a result line says about err. The line already
// names the artifact, so an API error gives only the server's message. Other
// errors, and API errors without a message, give their whole text. Whitespace
// is cleaned up so the result stays on one line.
func failureReason(err error) string {
	if httpErr, ok := errors.AsType[api.HTTPError](err); ok && httpErr.HTTPError != nil && httpErr.Message != "" {
		return text.RemoveExcessiveWhitespace(httpErr.Message)
	}
	return text.RemoveExcessiveWhitespace(err.Error())
}
