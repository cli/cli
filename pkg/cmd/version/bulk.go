package command

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(bulkCmd)
	bulkCmd.Flags().StringSliceP("label", "l", nil, "Add label")
}

var bulkCmd = &cobra.Command{
	Use:   "bulk {add|open|close|browse}",
	Args:  cobra.ExactArgs(1),
	Short: "Perform operation on a set of objects",
	Long: `Perform bulk operation on a set of objects passed in via standard input.

The first word of every line of input is interpreted like so:
- number, e.g. "123": an issue number in the current repository
- repo with number, e.g. "owner/repo#123": an issue in a specific repository
- repo, e.g. "owner/repo": a specific repository

Valid operations are:
- "add": add labels
- "close": set issue state to closed
- "open": set issue state to open
- "browse": open items in the web browser

Examples:

	$ gh issue list -L 10 | gh bulk add --label "triage"`,
	RunE: bulk,
}

func bulk(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)

	baseRepo, err := ctx.BaseRepo()
	if err != nil {
		return err
	}

	client, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	input := bufio.NewScanner(os.Stdin)
	var inputItems []api.BulkInput

	repoWithIssueRE := regexp.MustCompile(`^([^/]+/[^/]+)#(\d+)$`)
	repoNameRE := regexp.MustCompile(`^([^/]+/[^/]+)$`)

	for input.Scan() {
		line := input.Text()
		tokens := strings.Fields(line)
		if len(tokens) < 1 {
			continue
		}

		if issueNum, err := strconv.Atoi(tokens[0]); err == nil {
			inputItems = append(inputItems, api.BulkInput{
				Number:     issueNum,
				Repository: baseRepo,
			})
			continue
		}

		if m := repoWithIssueRE.FindAllStringSubmatch(tokens[0], 1); len(m) > 0 {
			repo := ghrepo.FromFullName(m[0][1])
			issueNum, _ := strconv.Atoi(m[0][2])
			inputItems = append(inputItems, api.BulkInput{
				Number:     issueNum,
				Repository: repo,
			})
			continue
		}

		if m := repoNameRE.FindAllStringSubmatch(tokens[0], 1); len(m) > 0 {
			repo := ghrepo.FromFullName(m[0][1])
			inputItems = append(inputItems, api.BulkInput{
				Repository: repo,
			})
			continue
		}

		return fmt.Errorf("unrecognized input format: %q", tokens[0])
	}

	labels, err := cmd.Flags().GetStringSlice("label")
	if err != nil {
		return err
	}

	operation := args[0]
	switch operation {
	case "add":
		return api.BulkAddLabels(client, inputItems, labels)
	case "open", "close":
		return api.BulkChangeState(client, inputItems, operation)
	case "browse":
		for _, item := range inputItems {
			url := fmt.Sprintf("https://github.com/%s/%s", item.Repository.RepoOwner(), item.Repository.RepoName())
			if item.Number > 0 {
				url += fmt.Sprintf("/issues/%d", item.Number)
			}
			_ = utils.OpenInBrowser(url)
		}
		return nil
	default:
		return fmt.Errorf("unrecognized operation: %q", operation)
	}
}
