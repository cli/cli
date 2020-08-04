package command

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

func init() {
	prCmd.PersistentFlags().StringP("repo", "R", "", "Select another repository using the `OWNER/REPO` format")

	RootCmd.AddCommand(prCmd)
	prCmd.AddCommand(prCloseCmd)
	prCmd.AddCommand(prReopenCmd)
	prCmd.AddCommand(prReadyCmd)
}

var prCmd = &cobra.Command{
	Use:   "pr <command>",
	Short: "Create, view, and checkout pull requests",
	Long:  `Work with GitHub pull requests`,
	Example: heredoc.Doc(`
	$ gh pr checkout 353
	$ gh pr create --fill
	$ gh pr view --web
	`),
	Annotations: map[string]string{
		"IsCore": "true",
		"help:arguments": `A pull request can be supplied as argument in any of the following formats:
- by number, e.g. "123";
- by URL, e.g. "https://github.com/OWNER/REPO/pull/123"; or
- by the name of its head branch, e.g. "patch-1" or "OWNER:patch-1".`},
}
var prCloseCmd = &cobra.Command{
	Use:   "close {<number> | <url> | <branch>}",
	Short: "Close a pull request",
	Args:  cobra.ExactArgs(1),
	RunE:  prClose,
}
var prReopenCmd = &cobra.Command{
	Use:   "reopen {<number> | <url> | <branch>}",
	Short: "Reopen a pull request",
	Args:  cobra.ExactArgs(1),
	RunE:  prReopen,
}
var prReadyCmd = &cobra.Command{
	Use:   "ready [<number> | <url> | <branch>]",
	Short: "Mark a pull request as ready for review",
	Args:  cobra.MaximumNArgs(1),
	RunE:  prReady,
}

func prClose(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	pr, baseRepo, err := prFromArgs(ctx, apiClient, cmd, args)
	if err != nil {
		return err
	}

	if pr.State == "MERGED" {
		err := fmt.Errorf("%s Pull request #%d (%s) can't be closed because it was already merged", utils.Red("!"), pr.Number, pr.Title)
		return err
	} else if pr.Closed {
		fmt.Fprintf(colorableErr(cmd), "%s Pull request #%d (%s) is already closed\n", utils.Yellow("!"), pr.Number, pr.Title)
		return nil
	}

	err = api.PullRequestClose(apiClient, baseRepo, pr)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}

	fmt.Fprintf(colorableErr(cmd), "%s Closed pull request #%d (%s)\n", utils.Red("✔"), pr.Number, pr.Title)

	return nil
}

func prReopen(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	pr, baseRepo, err := prFromArgs(ctx, apiClient, cmd, args)
	if err != nil {
		return err
	}

	if pr.State == "MERGED" {
		err := fmt.Errorf("%s Pull request #%d (%s) can't be reopened because it was already merged", utils.Red("!"), pr.Number, pr.Title)
		return err
	}

	if !pr.Closed {
		fmt.Fprintf(colorableErr(cmd), "%s Pull request #%d (%s) is already open\n", utils.Yellow("!"), pr.Number, pr.Title)
		return nil
	}

	err = api.PullRequestReopen(apiClient, baseRepo, pr)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}

	fmt.Fprintf(colorableErr(cmd), "%s Reopened pull request #%d (%s)\n", utils.Green("✔"), pr.Number, pr.Title)

	return nil
}

func prReady(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	pr, baseRepo, err := prFromArgs(ctx, apiClient, cmd, args)
	if err != nil {
		return err
	}

	if pr.Closed {
		err := fmt.Errorf("%s Pull request #%d is closed. Only draft pull requests can be marked as \"ready for review\"", utils.Red("!"), pr.Number)
		return err
	} else if !pr.IsDraft {
		fmt.Fprintf(colorableErr(cmd), "%s Pull request #%d is already \"ready for review\"\n", utils.Yellow("!"), pr.Number)
		return nil
	}

	err = api.PullRequestReady(apiClient, baseRepo, pr)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}

	fmt.Fprintf(colorableErr(cmd), "%s Pull request #%d is marked as \"ready for review\"\n", utils.Green("✔"), pr.Number)

	return nil
}
