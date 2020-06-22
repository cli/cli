package command

import (
	"fmt"
	"os"
	"strings"

	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

var prDiffCmd = &cobra.Command{
	Use:   "diff {<number> | <url>}",
	Short: "View a pull request's changes.",
	RunE:  prDiff,
}

func init() {
	prDiffCmd.Flags().StringP("color", "c", "auto", "Whether or not to output color: {always|never|auto}")

	prCmd.AddCommand(prDiffCmd)
	includeRepoFlag(prDiffCmd)
}

func prDiff(cmd *cobra.Command, args []string) error {
	color, err := cmd.Flags().GetString("color")
	if err != nil {
		return err
	}
	if !validColorFlag(color) {
		return fmt.Errorf("did not understand color: %q. Expected one of always, never, or auto", color)
	}

	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	pr, baseRepo, err := prFromArgs(ctx, apiClient, cmd, args)
	if err != nil {
		return fmt.Errorf("could not find pull request: %w", err)
	}

	diff, err := apiClient.PullRequestDiff(baseRepo, pr.Number)
	if err != nil {
		return fmt.Errorf("could not find pull request diff: %w", err)
	}

	out := cmd.OutOrStdout()
	if color == "auto" {
		color = "never"
		isTTY := false
		if outFile, isFile := out.(*os.File); isFile {
			isTTY = utils.IsTerminal(outFile)
			if isTTY {
				color = "always"
			}
		}
	}

	if color == "never" {
		fmt.Fprint(out, diff)
		return nil
	}

	out = colorableOut(cmd)
	for _, diffLine := range strings.Split(diff, "\n") {
		output := diffLine
		switch {
		case isHeaderLine(diffLine):
			output = utils.Bold(diffLine)
		case isAdditionLine(diffLine):
			output = utils.Green(diffLine)
		case isRemovalLine(diffLine):
			output = utils.Red(diffLine)
		}

		fmt.Fprintln(out, output)
	}

	return nil
}

func isHeaderLine(dl string) bool {
	prefixes := []string{"+++", "---", "diff", "index"}
	for _, p := range prefixes {
		if strings.HasPrefix(dl, p) {
			return true
		}
	}
	return false
}

func isAdditionLine(dl string) bool {
	return strings.HasPrefix(dl, "+")
}

func isRemovalLine(dl string) bool {
	return strings.HasPrefix(dl, "-")
}

func validColorFlag(c string) bool {
	return c == "auto" || c == "always" || c == "never"
}
