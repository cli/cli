package list

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/issue/artifact/client"
	"github.com/cli/cli/v2/pkg/cmd/issue/artifact/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

// ListOptions holds the flags and dependencies of gh issue artifact list.
type ListOptions struct {
	IO       *iostreams.IOStreams
	BaseRepo func() (ghrepo.Interface, error)
	Client   func() (client.ArtifactClient, error)
	Exporter cmdutil.Exporter
	Now      func() time.Time

	IssueNumber int
	// Limit is the most artifacts to list. 0 lists every artifact.
	Limit int
	Type  string
}

// NewCmdList creates the gh issue artifact list command.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:  f.IOStreams,
		Now: time.Now,
	}

	cmd := &cobra.Command{
		Use:   "list {<issue-number> | <issue-url>}",
		Short: "List artifacts on an issue (preview)",
		Long: heredoc.Docf(`
			List the artifacts on an issue, in number order. Every artifact is listed
			unless you set %[1]s--limit%[1]s.
		`, "`"),
		Example: heredoc.Doc(`
			# List the artifacts on issue 142
			$ gh issue artifact list 142

			# List only links
			$ gh issue artifact list 142 --type link

			# Print artifact numbers and names as JSON
			$ gh issue artifact list 142 --json number,name
		`),
		Aliases: []string{"ls"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("limit") && opts.Limit < 1 {
				return cmdutil.FlagErrorf("invalid limit: %v", opts.Limit)
			}

			var err error
			opts.IssueNumber, opts.BaseRepo, err = shared.ParseIssueArg(args[0], f.BaseRepo)
			if err != nil {
				return cmdutil.FlagErrorWrap(err)
			}
			opts.Client = shared.ClientFunc(f)

			if runF != nil {
				return runF(opts)
			}
			return listRun(opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 0, "Maximum number of artifacts to fetch")
	cmdutil.StringEnumFlag(cmd, &opts.Type, "type", "", "", []string{client.TypeGeneric, client.TypePlan, client.TypeLink}, "Filter by type")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, client.ArtifactFields)

	return cmd
}

func listRun(opts *ListOptions) error {
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

	artifacts, err := listArtifacts(c, repo, opts.IssueNumber, opts.Type, opts.Limit)
	if err != nil {
		return err
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, artifacts)
	}

	issueRef := fmt.Sprintf("%s#%d", ghrepo.FullName(repo), opts.IssueNumber)
	if len(artifacts) == 0 {
		if opts.Type != "" {
			return cmdutil.NewNoResultsError(fmt.Sprintf("no artifacts match your search on %s", issueRef))
		}
		return cmdutil.NewNoResultsError(fmt.Sprintf("no artifacts found on %s", issueRef))
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "\n%s\n\n", listHeader(issueRef, len(artifacts), opts.Type != ""))
	}
	return printArtifacts(opts.IO, opts.Now(), artifacts)
}

// listArtifacts lists the artifacts of one type, or of every type when
// artifactType is empty. gh treats generic and plan artifacts alike as
// documents, so either type lists both: each type is listed up to the limit,
// then the two are merged in number order and cut to the limit.
func listArtifacts(c client.ArtifactClient, repo ghrepo.Interface, issueNumber int, artifactType string, limit int) ([]client.Artifact, error) {
	if artifactType != client.TypeGeneric && artifactType != client.TypePlan {
		return c.List(repo, issueNumber, artifactType, limit)
	}

	var artifacts []client.Artifact
	for _, documentType := range []string{client.TypeGeneric, client.TypePlan} {
		documents, err := c.List(repo, issueNumber, documentType, limit)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, documents...)
	}

	slices.SortStableFunc(artifacts, func(a, b client.Artifact) int {
		return cmp.Compare(a.Number, b.Number)
	})
	if limit > 0 && len(artifacts) > limit {
		artifacts = artifacts[:limit]
	}
	return artifacts, nil
}

// listHeader counts the artifacts shown. Without a total count from the API,
// it can't say how many there are in all.
func listHeader(issueRef string, count int, filtered bool) string {
	header := fmt.Sprintf("Showing %s on %s", text.Pluralize(count, "artifact"), issueRef)
	if !filtered {
		return header
	}
	if count == 1 {
		return header + " that matches your search"
	}
	return header + " that match your search"
}

func printArtifacts(ios *iostreams.IOStreams, now time.Time, artifacts []client.Artifact) error {
	cs := ios.ColorScheme()
	tp := tableprinter.New(ios, tableprinter.WithHeader("Number", "Name", "Type", "Updated"))
	for _, a := range artifacts {
		tp.AddField(strconv.Itoa(a.Number))
		tp.AddField(text.RemoveExcessiveWhitespace(a.Name))
		tp.AddField(a.Type)
		if a.UpdatedAt != nil {
			tp.AddTimeField(now, *a.UpdatedAt, cs.Muted)
		} else {
			tp.AddField("")
		}
		tp.EndRow()
	}
	return tp.Render()
}
