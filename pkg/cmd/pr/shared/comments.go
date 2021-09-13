package shared

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/markdown"
	"github.com/cli/cli/v2/utils"
)

type Comment interface {
	AuthorLogin() string
	Association() string
	Content() string
	Created() time.Time
	HiddenReason() string
	IsEdited() bool
	IsHidden() bool
	Link() string
	Reactions() api.ReactionGroups
	Status() string
}

func RawCommentList(comments api.Comments, reviews api.PullRequestReviews) string {
	sortedComments := sortComments(comments, reviews)
	var b strings.Builder
	for _, comment := range sortedComments {
		fmt.Fprint(&b, formatRawComment(comment))
	}
	return b.String()
}

func formatRawComment(comment Comment) string {
	if comment.IsHidden() {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "author:\t%s\n", comment.AuthorLogin())
	fmt.Fprintf(&b, "association:\t%s\n", strings.ToLower(comment.Association()))
	fmt.Fprintf(&b, "edited:\t%t\n", comment.IsEdited())
	fmt.Fprintf(&b, "status:\t%s\n", formatRawCommentStatus(comment.Status()))
	fmt.Fprintln(&b, "--")
	fmt.Fprintln(&b, comment.Content())
	fmt.Fprintln(&b, "--")
	return b.String()
}

func CommentList(io *iostreams.IOStreams, comments api.Comments, reviews api.PullRequestReviews, preview bool) (string, error) {
	sortedComments := sortComments(comments, reviews)
	if preview && len(sortedComments) > 0 {
		sortedComments = sortedComments[len(sortedComments)-1:]
	}
	var b strings.Builder
	cs := io.ColorScheme()
	totalCount := comments.TotalCount + reviews.TotalCount
	retrievedCount := len(sortedComments)
	hiddenCount := totalCount - retrievedCount

	if preview && hiddenCount > 0 {
		fmt.Fprint(&b, cs.Gray(fmt.Sprintf("———————— Not showing %s ————————", utils.Pluralize(hiddenCount, "comment"))))
		fmt.Fprintf(&b, "\n\n\n")
	}

	for i, comment := range sortedComments {
		last := i+1 == retrievedCount
		cmt, err := formatComment(io, comment, last)
		if err != nil {
			return "", err
		}
		fmt.Fprint(&b, cmt)
		if last {
			fmt.Fprintln(&b)
		}
	}

	if preview && hiddenCount > 0 {
		fmt.Fprint(&b, cs.Gray("Use --comments to view the full conversation"))
		fmt.Fprintln(&b)
	}

	return b.String(), nil
}

func formatComment(io *iostreams.IOStreams, comment Comment, newest bool) (string, error) {
	var b strings.Builder
	cs := io.ColorScheme()

	if comment.IsHidden() {
		return cs.Bold(formatHiddenComment(comment)), nil
	}

	// Header
	fmt.Fprint(&b, cs.Bold(comment.AuthorLogin()))
	if comment.Status() != "" {
		fmt.Fprint(&b, formatCommentStatus(cs, comment.Status()))
	}
	if comment.Association() != "NONE" {
		fmt.Fprint(&b, cs.Boldf(" (%s)", strings.Title(strings.ToLower(comment.Association()))))
	}
	fmt.Fprint(&b, cs.Boldf(" • %s", utils.FuzzyAgoAbbr(time.Now(), comment.Created())))
	if comment.IsEdited() {
		fmt.Fprint(&b, cs.Bold(" • Edited"))
	}
	if newest {
		fmt.Fprint(&b, cs.Bold(" • "))
		fmt.Fprint(&b, cs.CyanBold("Newest comment"))
	}
	fmt.Fprintln(&b)

	// Reactions
	if reactions := ReactionGroupList(comment.Reactions()); reactions != "" {
		fmt.Fprint(&b, reactions)
		fmt.Fprintln(&b)
	}

	// Body
	var md string
	var err error
	if comment.Content() == "" {
		md = fmt.Sprintf("\n  %s\n\n", cs.Gray("No body provided"))
	} else {
		style := markdown.GetStyle(io.TerminalTheme())
		md, err = markdown.Render(comment.Content(), style)
		if err != nil {
			return "", err
		}
	}
	fmt.Fprint(&b, md)

	// Footer
	if comment.Link() != "" {
		fmt.Fprintf(&b, cs.Gray("View the full review: %s\n\n"), comment.Link())
	}

	return b.String(), nil
}

func sortComments(cs api.Comments, rs api.PullRequestReviews) []Comment {
	comments := cs.Nodes
	reviews := rs.Nodes
	var sorted []Comment = make([]Comment, len(comments)+len(reviews))

	var i int
	for _, c := range comments {
		sorted[i] = c
		i++
	}
	for _, r := range reviews {
		sorted[i] = r
		i++
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Created().Before(sorted[j].Created())
	})

	return sorted
}

const (
	approvedStatus         = "APPROVED"
	changesRequestedStatus = "CHANGES_REQUESTED"
	commentedStatus        = "COMMENTED"
	dismissedStatus        = "DISMISSED"
)

func formatCommentStatus(cs *iostreams.ColorScheme, status string) string {
	switch status {
	case approvedStatus:
		return fmt.Sprintf(" %s", cs.Green("approved"))
	case changesRequestedStatus:
		return fmt.Sprintf(" %s", cs.Red("requested changes"))
	case commentedStatus, dismissedStatus:
		return fmt.Sprintf(" %s", strings.ToLower(status))
	}

	return ""
}

func formatRawCommentStatus(status string) string {
	if status == approvedStatus ||
		status == changesRequestedStatus ||
		status == commentedStatus ||
		status == dismissedStatus {
		return strings.ReplaceAll(strings.ToLower(status), "_", " ")
	}

	return "none"
}

func formatHiddenComment(comment Comment) string {
	var b strings.Builder
	fmt.Fprint(&b, comment.AuthorLogin())
	if comment.Association() != "NONE" {
		fmt.Fprintf(&b, " (%s)", strings.Title(strings.ToLower(comment.Association())))
	}
	fmt.Fprintf(&b, " • This comment has been marked as %s\n\n", comment.HiddenReason())
	return b.String()
}
