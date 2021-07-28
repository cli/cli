package shared

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
)

func PreserveInput(io *iostreams.IOStreams, state *IssueMetadataState, createErr *error) func() {
	return func() {
		if !state.IsDirty() {
			return
		}

		if *createErr == nil {
			return
		}

		if cmdutil.IsUserCancellation(*createErr) {
			// these errors are user-initiated cancellations
			return
		}

		out := io.ErrOut

		// this extra newline guards against appending to the end of a survey line
		fmt.Fprintln(out)

		data, err := json.Marshal(state)
		if err != nil {
			fmt.Fprintf(out, "failed to save input to file: %s\n", err)
			fmt.Fprintln(out, "would have saved:")
			fmt.Fprintf(out, "%v\n", state)
			return
		}

		tmpfile, err := io.TempFile(os.TempDir(), "gh*.json")
		if err != nil {
			fmt.Fprintf(out, "failed to save input to file: %s\n", err)
			fmt.Fprintln(out, "would have saved:")
			fmt.Fprintf(out, "%v\n", state)
			return
		}

		_, err = tmpfile.Write(data)
		if err != nil {
			fmt.Fprintf(out, "failed to save input to file: %s\n", err)
			fmt.Fprintln(out, "would have saved:")
			fmt.Fprintln(out, string(data))
			return
		}

		cs := io.ColorScheme()

		issueType := "pr"
		if state.Type == IssueMetadata {
			issueType = "issue"
		}

		fmt.Fprintf(out, "%s operation failed. To restore: gh %s create --recover %s\n", cs.FailureIcon(), issueType, tmpfile.Name())

		// some whitespace before the actual error
		fmt.Fprintln(out)
	}
}
