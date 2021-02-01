package action

import (
	"strings"

	"github.com/rsteube/carapace"
	"github.com/spf13/cobra"
)

func ActionSecrets(cmd *cobra.Command, org string) carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		repo := ""
		if cmd.Flag("repo") != nil {
			repo = cmd.Flag("repo").Value.String()
		}
		return carapace.ActionExecCommand(ghExecutable(), "secret", "--repo", repo, "list", "--org", org)(func(output []byte) carapace.Action {
			lines := strings.Split(string(output), "\n")
			vals := make([]string, 0, len(lines)-1)
			for _, line := range lines[:len(lines)-1] {
				vals = append(vals, strings.SplitN(line, "\t", 2)...)
			}
			return carapace.ActionValuesDescribed(vals...)
		})
	})
}
