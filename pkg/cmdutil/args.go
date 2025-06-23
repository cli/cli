package cmdutil

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func MinimumArgs(n int, msg string) cobra.PositionalArgs {
	if msg == "" {
		return cobra.MinimumNArgs(1)
	}

	return func(cmd *cobra.Command, args []string) error {
		if len(args) < n {
			return FlagErrorf("%s", msg)
		}
		return nil
	}
}

func ExactArgs(n int, msg string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > n {
			return FlagErrorf("too many arguments")
		}

		if len(args) < n {
			return FlagErrorf("%s", msg)
		}

		return nil
	}
}

func NoArgsQuoteReminder(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return nil
	}

	errMsg := fmt.Sprintf("unknown argument %q", args[0])
	if len(args) > 1 {
		errMsg = fmt.Sprintf("unknown arguments %q", args)
	}

	hasValueFlag := false
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if f.Value.Type() != "bool" {
			hasValueFlag = true
		}
	})

	if hasValueFlag {
		errMsg += "; please quote all values that have spaces"
	}

	return FlagErrorf("%s", errMsg)
}

// Partition takes a slice of any type T and separates it into two slices
// of the same type based on the provided predicate function. Any item
// that returns true for the predicate will be included in the first slice
// returned, and any item that returns false for the predicate will be
// included in the second slice returned.
func Partition[T any](slice []T, predicate func(T) bool) ([]T, []T) {
	var matching, nonMatching []T
	for _, item := range slice {
		if predicate(item) {
			matching = append(matching, item)
		} else {
			nonMatching = append(nonMatching, item)
		}
	}
	return matching, nonMatching
}

// GlobPaths expands a list of file patterns into a list of file paths.
// If no files match a pattern, that pattern will return an error.
// If no pattern is passed, this returns an empty list and no error.
//
// For information on supported glob patterns, see
// https://pkg.go.dev/path/filepath#Match
func GlobPaths(patterns []string) ([]string, error) {
	expansions := []string{}

	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", pattern, err)
		}
		if len(matches) == 0 {
			return []string{}, fmt.Errorf("no matches found for `%s`", pattern)
		}
		expansions = append(expansions, matches...)
	}

	return expansions, nil
}
