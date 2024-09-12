package view

import (
	"fmt"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/iostreams"
)

type RawIssuePrinter struct {
	IO       *iostreams.IOStreams
	Comments bool
}

// Print outputs the issue to the terminal for non-TTY use cases.
func (p *RawIssuePrinter) Print(pi PresentationIssue, repo ghrepo.Interface) error {
	if p.Comments {
		fmt.Fprint(p.IO.Out, prShared.RawCommentList(pi.Comments, api.PullRequestReviews{}))
		return nil
	}

	// Print empty strings for empty values so the number of metadata lines is consistent when
	// processing many issues with head and grep.
	fmt.Fprintf(p.IO.Out, "title:\t%s\n", pi.Title)
	fmt.Fprintf(p.IO.Out, "state:\t%s\n", pi.State)
	fmt.Fprintf(p.IO.Out, "author:\t%s\n", pi.Author)
	fmt.Fprintf(p.IO.Out, "labels:\t%s\n", pi.LabelsList)
	fmt.Fprintf(p.IO.Out, "comments:\t%d\n", pi.Comments.TotalCount)
	fmt.Fprintf(p.IO.Out, "assignees:\t%s\n", pi.AssigneesList)
	fmt.Fprintf(p.IO.Out, "projects:\t%s\n", pi.ProjectsList)
	fmt.Fprintf(p.IO.Out, "milestone:\t%s\n", pi.MilestoneTitle)
	fmt.Fprintf(p.IO.Out, "number:\t%d\n", pi.Number)
	fmt.Fprintln(p.IO.Out, "--")
	fmt.Fprintln(p.IO.Out, pi.Body)

	return nil
}
