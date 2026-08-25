package attachments

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

const flagName = "attach"

// errEmptyPath is reported for a --attach that named no file, whether the flag
// held nothing else or the empty value sat beside a real one.
var errEmptyPath = errors.New("cannot attach an empty path; --attach needs a file path")

// AddFlag registers the repeatable --attach flag on cmd.
func AddFlag(cmd *cobra.Command) {
	// A string slice would split on commas, which are legal in filenames.
	cmd.Flags().StringArray(flagName, nil, "Attach an image or video `file`, in '<file>#<image alt text>' format")
}

// FromFlagValues validates every file --attach named on cmd, keeping them in
// the order they were written. It returns nothing when the flag was passed no
// values, so a caller reads the length rather than asking a second question.
//
// A command that does not register --attach cannot have any, and asking for
// them is a mistake in the command rather than in what the user typed.
func FromFlagValues(cmd *cobra.Command) ([]UserAsset, error) {
	flag := cmd.Flags().Lookup(flagName)
	if flag == nil {
		return nil, fmt.Errorf("%s does not register --attach", cmd.Name())
	}
	if !flag.Changed {
		return nil, nil
	}
	if err := checkFlagConflicts(cmd); err != nil {
		return nil, err
	}

	args, err := cmd.Flags().GetStringArray(flagName)
	if err != nil {
		return nil, err
	}

	// --attach "" is a user error.
	if len(args) == 0 {
		return nil, errEmptyPath
	}

	resolvedAssets := make([]UserAsset, 0, len(args))
	for _, arg := range args {
		a, err := assetFromArg(arg)
		if err != nil {
			return nil, err
		}

		// The same file under two names is a user error.
		for _, seen := range resolvedAssets {
			if os.SameFile(seen.getAsset().info, a.getAsset().info) {
				return nil, fmt.Errorf("%s and %s are the same file; attached files must be unique", seen.Path(), a.Path())
			}
		}
		resolvedAssets = append(resolvedAssets, a)
	}

	return resolvedAssets, nil
}

// assetFromArg turns one --attach argument into a UserAsset.
func assetFromArg(arg string) (UserAsset, error) {
	path, alt := parseArg(arg)

	if path == "" {
		return nil, errEmptyPath
	}

	if path == "-" {
		return nil, errors.New("cannot attach standard input; --attach needs a file path")
	}

	return newAsset(path, alt)
}

// parseArg splits the `path#alt text` form of an --attach argument. An existing
// path wins over the delimiter, since `#` is legal in filenames.
func parseArg(arg string) (path, alt string) {
	if _, err := os.Stat(arg); err == nil {
		return arg, ""
	}
	// Scan from the last hash to the first, so the longest path that exists
	// wins. That continues the rule above, where the whole argument is the
	// longest match of all, and it leaves a hash inside the alt text usable.
	for i := strings.LastIndex(arg, "#"); i > 0; i = strings.LastIndex(arg[:i], "#") {
		if _, err := os.Stat(arg[:i]); err == nil {
			return arg[:i], arg[i+1:]
		}
	}
	// No candidate exists, so the argument names a file that is not there. The
	// last hash keeps the error naming the path a reader would expect.
	if idx := strings.LastIndex(arg, "#"); idx > 0 {
		return arg[:idx], arg[idx+1:]
	}
	return arg, ""
}

// checkFlagConflicts rejects the modes an asset cannot be written in. Only
// FromFlagValues calls it, so --attach has been passed by the time it runs.
func checkFlagConflicts(cmd *cobra.Command) error {
	if web, _ := cmd.Flags().GetBool("web"); web {
		return cmdutil.FlagErrorf("`--attach` is not supported when using `--web`")
	}

	if deleteLast, _ := cmd.Flags().GetBool("delete-last"); deleteLast {
		return cmdutil.FlagErrorf("`--attach` is not supported when using `--delete-last`")
	}

	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		return cmdutil.FlagErrorf("`--attach` is not supported when using `--dry-run`")
	}

	return nil
}
