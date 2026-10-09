package view

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/issue/artifact/client"
	"github.com/cli/cli/v2/pkg/cmd/issue/artifact/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/markdown"
	"github.com/spf13/cobra"
)

// ViewOptions holds the flags and dependencies of gh issue artifact view.
type ViewOptions struct {
	IO       *iostreams.IOStreams
	BaseRepo func() (ghrepo.Interface, error)
	Client   func() (client.ArtifactClient, error)
	Browser  browser.Browser
	Exporter cmdutil.Exporter
	Now      func() time.Time

	IssueNumber    int
	ArtifactNumber int
	// Version is the version to show from the edit history. 0 shows the
	// current version.
	Version int
	WebMode bool
}

// NewCmdView creates the gh issue artifact view command.
func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:      f.IOStreams,
		Browser: f.Browser,
		Now:     time.Now,
	}

	cmd := &cobra.Command{
		Use:   "view {<issue-number> | <issue-url>} <artifact-number>",
		Short: "View an issue artifact (preview)",
		Long: heredoc.Docf(`
			Display an issue artifact's name, type, authors, content, and edit history. A
			link shows its URL.

			By default, the current version is shown. Use %[1]s--version%[1]s to show an earlier
			version from the edit history.

			With %[1]s--web%[1]s, open a link artifact's URL in the browser. Artifacts have no
			web page of their own, so documents can't be opened with %[1]s--web%[1]s.
		`, "`"),
		Example: heredoc.Doc(`
			# View artifact 2 on issue 142
			$ gh issue artifact view 142 2

			# View an earlier version
			$ gh issue artifact view 142 2 --version 5

			# Open a link artifact in the browser
			$ gh issue artifact view 142 3 --web

			# Print an artifact's body
			$ gh issue artifact view 142 2 --json body --jq .body
		`),
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			versionSet := cmd.Flags().Changed("version")
			if err := cmdutil.MutuallyExclusive("specify only one of `--version` or `--json`", versionSet, opts.Exporter != nil); err != nil {
				return err
			}
			if err := cmdutil.MutuallyExclusive("specify only one of `--version` or `--web`", versionSet, opts.WebMode); err != nil {
				return err
			}
			if versionSet && opts.Version < 1 {
				return cmdutil.FlagErrorf("invalid version: %v", opts.Version)
			}

			var err error
			opts.IssueNumber, opts.BaseRepo, err = shared.ParseIssueArg(args[0], f.BaseRepo)
			if err != nil {
				return cmdutil.FlagErrorWrap(err)
			}
			opts.ArtifactNumber, err = shared.ParseArtifactNumber(args[1])
			if err != nil {
				return cmdutil.FlagErrorWrap(err)
			}
			opts.Client = shared.ClientFunc(f)

			if runF != nil {
				return runF(opts)
			}
			return viewRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open a link artifact's URL in the browser")
	cmd.Flags().IntVar(&opts.Version, "version", 0, "Show the version with this `number` from the edit history")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, client.ArtifactWithVersionsFields)

	return cmd
}

func viewRun(opts *ViewOptions) error {
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

	artifact, err := c.Get(repo, opts.IssueNumber, opts.ArtifactNumber)
	if err != nil {
		return err
	}
	if err := shared.CheckType(artifact.Artifact); err != nil {
		return err
	}
	viewing, err := shared.FindVersion(artifact, opts.Version)
	if err != nil {
		return err
	}

	if opts.WebMode {
		return openLink(opts, artifact.Artifact)
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, artifact)
	}

	if !opts.IO.IsStdoutTTY() {
		printRawArtifact(opts.IO.Out, artifact, viewing)
		return nil
	}

	opts.IO.DetectTerminalTheme()
	if err := opts.IO.StartPager(); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "error starting pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	return printHumanArtifact(opts, repo, artifact, viewing)
}

// openLink opens a link artifact's URL. Artifacts have no web page of their
// own, so there is nothing to open for a document.
func openLink(opts *ViewOptions, a client.Artifact) error {
	if a.Type != client.TypeLink {
		return fmt.Errorf("artifact %d is not a link; --web only opens link artifacts", a.Number)
	}
	u, ok := shared.HTTPURL(a.Body)
	if !ok {
		return fmt.Errorf("artifact %d doesn't link to an http(s) URL; --web only opens http(s) URLs", a.Number)
	}

	// A link can point anywhere, so the message shows the whole URL, where
	// text.DisplayURL would hide its port, query and fragment.
	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", u)
	}
	return opts.Browser.Browse(u)
}

// printRawArtifact prints key lines in a fixed order, then the body after a
// "--" line, like gh issue view. Null values print as empty fields, so every
// artifact has the same lines. An earlier version replaces the name, version,
// editor and body, while the creator still describes the artifact.
func printRawArtifact(out io.Writer, a *client.ArtifactWithVersions, viewing *client.Version) {
	name, body := a.Name, a.Body
	updatedBy, updatedAt := a.UpdatedByActor, a.UpdatedAt
	var version string
	if len(a.Versions) > 0 {
		version = strconv.Itoa(a.Versions[0].Version)
	}
	if viewing != nil {
		name, body = viewing.Name, viewing.Body
		updatedBy, updatedAt = viewing.Actor, viewing.CreatedAt
		version = strconv.Itoa(viewing.Version)
	}

	fmt.Fprintf(out, "name:\t%s\n", text.RemoveExcessiveWhitespace(name))
	fmt.Fprintf(out, "number:\t%d\n", a.Number)
	fmt.Fprintf(out, "version:\t%s\n", version)
	fmt.Fprintf(out, "type:\t%s\n", a.Type)
	fmt.Fprintf(out, "creator:\t%s\n", login(a.Creator))
	fmt.Fprintf(out, "created:\t%s\n", timestamp(a.CreatedAt))
	fmt.Fprintf(out, "updated-by:\t%s\n", login(updatedBy))
	fmt.Fprintf(out, "updated:\t%s\n", timestamp(updatedAt))
	fmt.Fprintln(out, "--")
	fmt.Fprintln(out, body)
}

func printHumanArtifact(opts *ViewOptions, repo ghrepo.Interface, a *client.ArtifactWithVersions, viewing *client.Version) error {
	out := opts.IO.Out
	cs := opts.IO.ColorScheme()
	now := opts.Now()

	name, body := a.Name, a.Body
	if viewing != nil {
		name, body = viewing.Name, viewing.Body
	}

	fmt.Fprintf(out, "%s %s#%d\n", cs.Bold(text.RemoveExcessiveWhitespace(name)), ghrepo.FullName(repo), opts.IssueNumber)
	fmt.Fprintln(out, cs.Muted(metadataLine(a.Artifact, viewing, now)))

	if a.Type == client.TypeLink {
		fmt.Fprintf(out, "\n  %s\n\n", strings.TrimSpace(body))
	} else {
		md, err := markdown.Render(body,
			markdown.WithTheme(opts.IO.TerminalTheme()),
			markdown.WithWrap(opts.IO.TerminalWidth()))
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "\n%s\n", md)
	}

	if len(a.Versions) > 0 {
		fmt.Fprintln(out, cs.Bold("Edit history"))
		if err := printEditHistory(opts.IO, now, a.Versions, viewing); err != nil {
			return err
		}
		fmt.Fprintln(out)
	}

	fmt.Fprintln(out, cs.Muted("View this issue on GitHub: "+ghrepo.GenerateRepoURL(repo, "issues/%d", opts.IssueNumber)))
	return nil
}

// metadataLine describes the artifact under its name: its number, type,
// creator and last editor, or for an earlier version, that version's number
// and who saved it. A part whose time is null is left out, and so is the
// update of an artifact that was never edited.
func metadataLine(a client.Artifact, viewing *client.Version, now time.Time) string {
	parts := []string{fmt.Sprintf("Artifact %d", a.Number), a.Type}

	if viewing != nil {
		parts = append(parts, fmt.Sprintf("Version %d", viewing.Version))
		if viewing.CreatedAt != nil {
			action := "updated"
			if viewing.Version == 1 {
				action = "created"
			}
			parts = append(parts, fmt.Sprintf("%s %s %s", displayName(viewing.Actor), action, text.FuzzyAgo(now, *viewing.CreatedAt)))
		}
		return strings.Join(parts, " • ")
	}

	if a.CreatedAt != nil {
		parts = append(parts, fmt.Sprintf("%s created %s", displayName(a.Creator), text.FuzzyAgo(now, *a.CreatedAt)))
	}
	if a.UpdatedAt != nil && (a.CreatedAt == nil || !a.UpdatedAt.Equal(*a.CreatedAt)) {
		parts = append(parts, fmt.Sprintf("%s updated %s", displayName(a.UpdatedByActor), text.FuzzyAgo(now, *a.UpdatedAt)))
	}
	return strings.Join(parts, " • ")
}

// printEditHistory lists every version the API returned, newest first, with
// who saved it and when. "Current" marks the newest version, and "Viewing"
// marks the earlier version shown.
func printEditHistory(ios *iostreams.IOStreams, now time.Time, versions []client.Version, viewing *client.Version) error {
	cs := ios.ColorScheme()
	// The "Edit history" heading stands in for column headers.
	tp := tableprinter.New(ios, tableprinter.NoHeader)
	for i, v := range versions {
		tp.AddField(strconv.Itoa(v.Version))
		if v.CreatedAt != nil {
			tp.AddField(displayName(v.Actor))
			tp.AddTimeField(now, *v.CreatedAt, cs.Muted)
		} else {
			tp.AddField(login(v.Actor))
			tp.AddField("")
		}
		switch {
		case i == 0:
			tp.AddField("Current", tableprinter.WithColor(cs.CyanBold))
		case viewing != nil && v.Version == viewing.Version:
			tp.AddField("Viewing", tableprinter.WithColor(cs.CyanBold))
		}
		tp.EndRow()
	}
	return tp.Render()
}

// displayName returns a user's login for the terminal. A null user shows as
// ghost, like a deleted user in gh pr view, so callers use it only when the
// user's action has a time.
func displayName(u *client.Actor) string {
	if u == nil {
		return "ghost"
	}
	return u.Login
}

// login returns a user's login, or an empty string for a null user.
func login(u *client.Actor) string {
	if u == nil {
		return ""
	}
	return u.Login
}

// timestamp returns t in RFC 3339 format, or an empty string for a null time.
func timestamp(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
