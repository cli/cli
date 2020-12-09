package shared

import (
	"fmt"
	"strings"
	"time"

	"github.com/cli/cli/api"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/markdown"
	"github.com/cli/cli/utils"
)

func RawCommentList(comments api.Comments) string {
	var b strings.Builder
	for _, comment := range comments.Nodes {
		fmt.Fprint(&b, formatRawComment(comment))
	}
	return b.String()
}

func formatRawComment(comment api.Comment) string {
	var b strings.Builder
	fmt.Fprintf(&b, "author:\t%s\n", comment.Author.Login)
	fmt.Fprintf(&b, "association:\t%s\n", strings.ToLower(comment.AuthorAssociation))
	fmt.Fprintf(&b, "edited:\t%t\n", comment.IncludesCreatedEdit)
	fmt.Fprintln(&b, "--")
	fmt.Fprintln(&b, comment.Body)
	fmt.Fprintln(&b, "--")
	return b.String()
}

func CommentList(io *iostreams.IOStreams, comments api.Comments) (string, error) {
	var b strings.Builder
	cs := io.ColorScheme()
	retrievedCount := len(comments.Nodes)
	hiddenCount := comments.TotalCount - retrievedCount

	if hiddenCount > 0 {
		fmt.Fprint(&b, cs.Gray(fmt.Sprintf("———————— Not showing %s ————————", utils.Pluralize(hiddenCount, "comment"))))
		fmt.Fprintf(&b, "\n\n\n")
	}

	for i, comment := range comments.Nodes {
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

	if hiddenCount > 0 {
		fmt.Fprint(&b, cs.Gray("Use --comments to view the full conversation"))
		fmt.Fprintln(&b)
	}

	return b.String(), nil
}

func formatComment(io *iostreams.IOStreams, comment api.Comment, newest bool) (string, error) {
	var b strings.Builder
	cs := io.ColorScheme()

	// Header
	fmt.Fprint(&b, cs.Bold(comment.Author.Login))
	if comment.AuthorAssociation != "NONE" {
		fmt.Fprint(&b, cs.Bold(fmt.Sprintf(" (%s)", strings.ToLower(comment.AuthorAssociation))))
	}
	fmt.Fprint(&b, cs.Bold(fmt.Sprintf(" • %s", utils.FuzzyAgoAbbr(time.Now(), comment.CreatedAt))))
	if comment.IncludesCreatedEdit {
		fmt.Fprint(&b, cs.Bold(" • edited"))
	}
	if newest {
		fmt.Fprint(&b, cs.Bold(" • "))
		fmt.Fprint(&b, cs.CyanBold("Newest comment"))
	}
	fmt.Fprintln(&b)

	// Reactions
	if reactions := ReactionGroupList(comment.ReactionGroups); reactions != "" {
		fmt.Fprint(&b, reactions)
		fmt.Fprintln(&b)
	}

	// Body
	if comment.Body != "" {
		style := markdown.GetStyle(io.TerminalTheme())
		md, err := markdown.Render(comment.Body, style, "")
		if err != nil {
			return "", err
		}
		fmt.Fprint(&b, md)
	}

	return b.String(), nil
}
