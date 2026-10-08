package delete

import (
	"fmt"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/issue/artifact/client"
	"github.com/cli/cli/v2/pkg/cmd/issue/artifact/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

// DeleteOptions holds the flags and dependencies of gh issue artifact delete.
type DeleteOptions struct {
	IO       *iostreams.IOStreams
	BaseRepo func() (ghrepo.Interface, error)
	Client   func() (client.ArtifactClient, error)
	Prompter iprompter

	IssueNumber     int
	ArtifactNumbers []int
	// Confirmed skips the confirmation prompt. It is required when gh can't
	// prompt.
	Confirmed bool
}

type iprompter interface {
	Confirm(string, bool) (bool, error)
}

// NewCmdDelete creates the gh issue artifact delete command.
func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := &DeleteOptions{
		IO:       f.IOStreams,
		Prompter: f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "delete {<issue-number> | <issue-url>} <artifact-number>...",
		Short: "Delete issue artifacts (preview)",
		Long: heredoc.Docf(`
			Delete artifacts from an issue. A deleted artifact can't be restored.

			In a terminal, gh asks for confirmation first. %[1]s--yes%[1]s skips the prompt, and it
			is required when gh can't prompt.
		`, "`"),
		Example: heredoc.Doc(`
			# Delete artifact 2 from issue 142
			$ gh issue artifact delete 142 2

			# Delete two artifacts without a prompt
			$ gh issue artifact delete 142 2 5 --yes
		`),
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			opts.IssueNumber, opts.BaseRepo, err = shared.ParseIssueArg(args[0], f.BaseRepo)
			if err != nil {
				return cmdutil.FlagErrorWrap(err)
			}
			opts.ArtifactNumbers = make([]int, 0, len(args)-1)
			for _, arg := range args[1:] {
				number, err := shared.ParseArtifactNumber(arg)
				if err != nil {
					return cmdutil.FlagErrorWrap(err)
				}
				if slices.Contains(opts.ArtifactNumbers, number) {
					return cmdutil.FlagErrorf("duplicate artifact number: %d", number)
				}
				opts.ArtifactNumbers = append(opts.ArtifactNumbers, number)
			}

			if !opts.IO.CanPrompt() && !opts.Confirmed {
				return cmdutil.FlagErrorf("--yes required when not running interactively")
			}
			opts.Client = shared.ClientFunc(f)

			if runF != nil {
				return runF(opts)
			}
			return deleteRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Confirmed, "yes", false, "Confirm deletion without prompting")

	return cmd
}

// lookup is what gh learned about one artifact number before deleting it.
type lookup struct {
	number int
	// name is the artifact's name, with its whitespace cleaned up for
	// printing.
	name string
	err  error
}

func deleteRun(opts *DeleteOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}
	if err := shared.CheckHost(repo.RepoHost()); err != nil {
		return err
	}

	c, err := opts.Client()
	if err != nil {
		return err
	}
	if err := shared.CheckIssue(c, repo, opts.IssueNumber); err != nil {
		return err
	}

	// Every number is looked up before anything is deleted: the prompt and the
	// result lines name each artifact, and without --yes one failed lookup
	// stops the command.
	lookups := make([]lookup, 0, len(opts.ArtifactNumbers))
	for _, number := range opts.ArtifactNumbers {
		l := lookup{number: number}
		a, err := c.Get(repo, opts.IssueNumber, number)
		if err != nil {
			l.err = err
		} else {
			l.name = text.RemoveExcessiveWhitespace(a.Name)
		}
		lookups = append(lookups, l)
	}

	issueRef := fmt.Sprintf("%s#%d", ghrepo.FullName(repo), opts.IssueNumber)
	cs := opts.IO.ColorScheme()

	if !opts.Confirmed {
		if slices.ContainsFunc(lookups, func(l lookup) bool { return l.err != nil }) {
			for _, l := range lookups {
				if l.err != nil {
					printFailure(opts.IO, l.number, l.err)
				}
			}
			fmt.Fprintln(opts.IO.ErrOut, "No artifacts were deleted.")
			return cmdutil.SilentError
		}

		fmt.Fprintf(opts.IO.Out, "%s Deleted artifacts cannot be recovered.\n", cs.WarningIcon())
		confirmed, err := opts.Prompter.Confirm(confirmationPrompt(lookups, issueRef), false)
		if err != nil {
			return err
		}
		if !confirmed {
			return cmdutil.CancelError
		}
	}

	failed := false
	for _, l := range lookups {
		if l.err != nil {
			printFailure(opts.IO, l.number, l.err)
			failed = true
			continue
		}
		if err := c.Delete(repo, opts.IssueNumber, l.number); err != nil {
			printFailure(opts.IO, l.number, err)
			failed = true
			continue
		}
		// Like gh issue delete, a deleted artifact is reported on stderr, and
		// only in a terminal: a script already knows the numbers it passed.
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "%s Deleted artifact %d (%s) from %s\n", cs.SuccessIconWithColor(cs.Red), l.number, l.name, issueRef)
		}
	}

	if failed {
		return cmdutil.SilentError
	}
	return nil
}

// printFailure reports an artifact gh couldn't delete, on stderr. In a
// terminal, it's the failure line the other artifact commands print. Piped,
// it's "<number>: <reason>", like gh codespace delete, with the same reason.
func printFailure(ios *iostreams.IOStreams, number int, err error) {
	if !ios.IsStdoutTTY() {
		fmt.Fprintf(ios.ErrOut, "%d: %s\n", number, shared.FailureReason(err))
		return
	}
	shared.PrintFailure(ios, shared.Result{Number: number}, fmt.Sprintf("delete artifact %d", number), err)
}

// confirmationPrompt asks once for every artifact, naming each one, such as
// "Delete artifacts 2 (OAuth callback plan) and 5 (Barista feedback notes)
// from monalisa/monas-cafe#142?".
func confirmationPrompt(lookups []lookup, issueRef string) string {
	artifacts := make([]string, 0, len(lookups))
	for _, l := range lookups {
		artifacts = append(artifacts, fmt.Sprintf("%d (%s)", l.number, l.name))
	}
	noun := "artifact"
	if len(artifacts) > 1 {
		noun = "artifacts"
	}
	return fmt.Sprintf("Delete %s %s from %s?", noun, joinList(artifacts), issueRef)
}

// joinList joins items the way the prompt reads them: "a", "a and b", or
// "a, b, and c".
func joinList(items []string) string {
	switch len(items) {
	case 0, 1:
		return strings.Join(items, "")
	case 2:
		return items[0] + " and " + items[1]
	}
	return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
}
