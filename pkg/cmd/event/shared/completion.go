package shared

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// RegisterEventTypesCompletion suggests the known event types for a comma-separated flag. The flag still accepts
// any value, because the server may support event types that this build does not know about.
func RegisterEventTypesCompletion(cmd *cobra.Command, flag string) {
	_ = cmd.RegisterFlagCompletionFunc(flag, func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return CompleteEventTypes(toComplete), cobra.ShellCompDirectiveNoSpace | cobra.ShellCompDirectiveNoFileComp
	})
}

// CompleteEventTypes returns the known event types that complete the last comma-separated item of toComplete, each
// prefixed with the preceding items and followed by its description. Types already listed are left out.
func CompleteEventTypes(toComplete string) []string {
	var prefix string
	var chosen []string
	if idx := strings.LastIndexByte(toComplete, ','); idx >= 0 {
		prefix = toComplete[:idx+1]
		chosen = strings.Split(toComplete[:idx], ",")
		toComplete = toComplete[idx+1:]
	}
	var results []string
	for _, t := range EventTypes {
		if strings.HasPrefix(t.Name, toComplete) && !slices.Contains(chosen, t.Name) {
			results = append(results, prefix+t.Name+"\t"+t.Description)
		}
	}
	return results
}
