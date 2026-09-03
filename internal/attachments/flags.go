package attachments

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	flagName       = "attach"
	maxAttachments = 50
)

// errEmptyPath is reported for a --attach that named no file, whether the flag
// held nothing else or the empty value sat beside a real one.
var errEmptyPath = errors.New("cannot attach an empty path; --attach needs a file path")

// Flag holds the repeatable attachment flag and the values it parsed.
type Flag struct {
	flag   *pflag.Flag
	values []string
}

// AddFlag registers the repeatable --attach flag on cmd.
func AddFlag(cmd *cobra.Command) *Flag {
	f := &Flag{}
	// A string slice would split on commas, which are legal in filenames.
	cmd.Flags().StringArrayVar(&f.values, flagName, nil, "Attach an image or video `file`, in '<file>#<image alt text>' format")
	f.flag = cmd.Flags().Lookup(flagName)
	return f
}

// Changed reports whether the attachment flag was passed.
func (f *Flag) Changed() bool {
	return f.flag.Changed
}

// RecordTelemetry records how many attachment flags were provided.
func (f *Flag) RecordTelemetry(command string, recorder ghtelemetry.CommandRecorder) {
	if recorder == nil || !f.Changed() {
		return
	}

	recorder.SetSampleRate(ghtelemetry.SAMPLE_ALL)
	recorder.Record(ghtelemetry.Event{
		Type: "attachment_invocation",
		Dimensions: ghtelemetry.Dimensions{
			"command": command,
		},
		Measures: ghtelemetry.Measures{
			"attach_count": int64(len(f.values)),
		},
	})
}

// UserAssets validates the files named by the attachment flag, keeping them in
// the order they were written. It returns nothing when the flag was not passed.
func (f *Flag) UserAssets() ([]UserAsset, error) {
	if !f.Changed() {
		return nil, nil
	}
	if len(f.values) > maxAttachments {
		return nil, fmt.Errorf("`--attach` accepts at most %d values per command", maxAttachments)
	}
	return userAssetsFromArgs(f.values)
}

func userAssetsFromArgs(args []string) ([]UserAsset, error) {
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
