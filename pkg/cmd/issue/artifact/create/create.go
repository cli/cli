package create

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/attachments"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/issue/artifact/client"
	"github.com/cli/cli/v2/pkg/cmd/issue/artifact/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

// CreateOptions holds the flags and dependencies of gh issue artifact create.
type CreateOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Config     func() (gh.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Client     func() (client.ArtifactClient, error)

	IssueNumber int
	// Sources are the artifacts to create, in argument order, which is the
	// order they are created in.
	Sources []source
	// Type is the type of documents, from --type. Empty creates generic
	// documents.
	Type string

	AttachFlag  *attachments.Flag
	AttachEvent *attachments.TelemetryEvent
	Assets      []attachments.UserAsset
}

// sourceKind is where an artifact's content comes from.
type sourceKind int

const (
	// fromFile is a file argument, or --body-file with a path.
	fromFile sourceKind = iota
	// fromURL is a URL argument.
	fromURL
	// fromStdin is --body-file -.
	fromStdin
	// fromBody is --body.
	fromBody
)

// source is one artifact to create, as the command line gave it.
type source struct {
	kind sourceKind
	// arg is the file path, URL or "-" as typed, which result lines show as
	// the source. It is empty for --body.
	arg string
	// name is the name typed with #<name> or --name, or empty for the name gh
	// gives it.
	name string
	// body is --body's text.
	body string
	// bodyFile reports whether a file came from --body-file, which takes its
	// path as typed, so its artifact can't be named with #<name>.
	bodyFile bool
}

// isLink reports whether the source creates a link: a URL, or a .url file.
func (s source) isLink() bool {
	return s.kind == fromURL || (s.kind == fromFile && shared.IsShortcutFile(s.arg))
}

// NewCmdCreate creates the gh issue artifact create command.
func NewCmdCreate(f *cmdutil.Factory, telemetry ghtelemetry.InvocationRecorder, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	var body, bodyFile, name string

	cmd := &cobra.Command{
		Use:   "create {<issue-number> | <issue-url>} [<file>[#<name>] | <url>]...",
		Short: "Create issue artifacts (preview)",
		Long: heredoc.Docf(`
			Create one artifact for each file or URL. A text file becomes a document
			named after the file; add %[1]s#<name>%[1]s to choose a different name. A URL, or a
			%[1]s.url%[1]s Internet Shortcut file, becomes a link; a URL is named after its host.

			Artifact names can't contain some characters, such as parentheses. When a
			file's name has them, gh drops them: %[1]splan (1).md%[1]s is named %[1]splan 1.md%[1]s.

			Use %[1]s--body%[1]s to create a single document from text, or %[1]s--body-file%[1]s to create
			one artifact from a file (%[1]s-%[1]s reads standard input). %[1]s--name%[1]s sets its name.

			Documents are saved as %[1]sgeneric%[1]s unless %[1]s--type plan%[1]s is given. A plan
			currently behaves the same as a generic document.
		`, "`"),
		Example: heredoc.Doc(`
			# Create a document from a file
			$ gh issue artifact create 142 signin-plan.md

			# Create documents from two files, naming the first
			$ gh issue artifact create 142 'docs/signin-plan.md#Sign-in plan' oauth-research.md

			# Create a link
			$ gh issue artifact create 142 https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook --name 'Staging OAuth runbook'

			# Create a document from standard input
			$ cat order-sync.log | gh issue artifact create 142 --body-file - --name 'Order sync log'

			# Upload a screenshot that the document references
			$ gh issue artifact create 142 signin-plan.md --attach ./signin-flow.png
		`),
		Aliases: []string{"add", "new"},
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			opts.IssueNumber, opts.BaseRepo, err = shared.ParseIssueArg(args[0], f.BaseRepo)
			if err != nil {
				return err
			}

			// An empty --body-file names no file, as in gh issue create.
			bodySet := cmd.Flags().Changed("body")
			bodyFileSet := bodyFile != ""
			nameSet := cmd.Flags().Changed("name")
			if err := cmdutil.MutuallyExclusive("specify only one of `--body` or `--body-file`", bodySet, bodyFileSet); err != nil {
				return err
			}

			fileArgs := args[1:]
			switch {
			case len(fileArgs) == 0 && !bodySet && !bodyFileSet:
				return cmdutil.FlagErrorf("specify a file or URL, or use `--body` or `--body-file`")
			case len(fileArgs) > 0 && (bodySet || bodyFileSet):
				return cmdutil.FlagErrorf("specify files or one of --body and --body-file, not both")
			case nameSet && len(fileArgs) > 1:
				return cmdutil.FlagErrorf("--name can only be used when creating one artifact")
			case nameSet && name == "":
				return cmdutil.FlagErrorf("--name cannot be blank")
			}

			// Standard input is only read with --body-file -, so a stdin that
			// never closes can't hang the command.
			switch {
			case bodySet || bodyFile == "-":
				if !nameSet {
					return cmdutil.FlagErrorf("--name is required when the content has no file name")
				}
				if !bodySet {
					opts.Sources = []source{{kind: fromStdin, arg: "-", name: name}}
					break
				}
				if strings.TrimSpace(body) == "" {
					return cmdutil.FlagErrorf("--body cannot be blank")
				}
				opts.Sources = []source{{kind: fromBody, name: name, body: body}}
			case bodyFileSet:
				opts.Sources = []source{{kind: fromFile, arg: bodyFile, name: name, bodyFile: true}}
			default:
				for _, arg := range fileArgs {
					s, err := parseSource(arg)
					if err != nil {
						return err
					}
					opts.Sources = append(opts.Sources, s)
				}
				if nameSet {
					if opts.Sources[0].name != "" {
						return cmdutil.FlagErrorf("specify the name with `#<name>` or `--name`, not both")
					}
					opts.Sources[0].name = name
				}
			}

			if cmd.Flags().Changed("type") && slices.ContainsFunc(opts.Sources, source.isLink) {
				return cmdutil.FlagErrorf("--type only applies to documents; links are created from URLs and .url files")
			}
			// A link has no Markdown to attach files to.
			if opts.AttachFlag.Changed() && !slices.ContainsFunc(opts.Sources, func(s source) bool { return !s.isLink() }) {
				return cmdutil.FlagErrorf("--attach can't be used with link artifacts")
			}

			opts.AttachEvent = attachments.BeginTelemetry(telemetry, cmd.CommandPath(), opts.AttachFlag.Count())
			opts.Assets, err = opts.AttachFlag.UserAssets()
			if err != nil {
				return err
			}
			opts.Client = shared.ClientFunc(f)

			if runF != nil {
				return runF(opts)
			}
			return createRun(opts)
		},
	}

	cmd.Flags().StringVarP(&body, "body", "b", "", "Content for a single document")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read content for a single artifact from `file` (use \"-\" to read from standard input)")
	cmd.Flags().StringVar(&name, "name", "", "Name for a single artifact")
	cmdutil.StringEnumFlag(cmd, &opts.Type, "type", "", "", []string{client.TypeGeneric, client.TypePlan}, "Type for documents")
	opts.AttachFlag = attachments.AddFlag(cmd)

	return cmd
}

// parseSource reads one file or URL argument. An http:// or https:// argument
// is always a link, and any `#` stays part of the URL. Like every link, a URL
// is read with its surrounding whitespace trimmed, which also keeps it on one
// result line. Any other argument is a file, and a name can follow it after
// `#`, split the way --attach splits its alt text.
func parseSource(arg string) (source, error) {
	trimmed := strings.TrimSpace(arg)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return source{kind: fromURL, arg: trimmed}, nil
	}

	path, name := attachments.SplitFileArg(arg)
	// Only --body-file reads standard input, and "-" is how a result line
	// names it.
	if path == "-" {
		return source{}, cmdutil.FlagErrorf("cannot create an artifact from `-`; use `--body-file -` to read standard input")
	}
	if path != arg && name == "" {
		return source{}, fmt.Errorf("failed to create artifact from %s: the name after `#` cannot be blank", path)
	}
	return source{kind: fromFile, arg: path, name: name}, nil
}

// artifact is one artifact ready to create.
type artifact struct {
	source       source
	artifactType string
	name         string
	body         string
	// dir is the directory of the file body was read from, which --attach
	// resolves the body's Markdown references against first, or "" for
	// content from no file.
	dir string
	// attachErr is why some of the document's --attach files didn't upload.
	// skip is set when none of them did, so the document isn't saved.
	attachErr error
	skip      bool
}

func createRun(opts *CreateOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}
	if err := shared.CheckHost(repo.RepoHost()); err != nil {
		return err
	}

	// Every artifact is read and checked before any request, so a file that
	// can't be used fails the whole command and nothing is created.
	documentType := opts.Type
	if documentType == "" {
		documentType = client.TypeGeneric
	}
	artifacts := make([]artifact, 0, len(opts.Sources))
	for _, s := range opts.Sources {
		a, err := readSource(s, opts.IO.In, documentType)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, a)
	}

	c, err := opts.Client()
	if err != nil {
		return err
	}

	if len(opts.Assets) > 0 {
		uploader, err := newUploader(opts, c, repo)
		if err != nil {
			return err
		}
		if err := attach(opts, uploader, artifacts); err != nil {
			return err
		}
	} else if err := shared.CheckIssue(c, repo, opts.IssueNumber); err != nil {
		return err
	}

	issueRef := fmt.Sprintf("%s#%d", ghrepo.FullName(repo), opts.IssueNumber)
	cs := opts.IO.ColorScheme()
	failed := false
	for _, a := range artifacts {
		result := shared.Result{Name: text.RemoveExcessiveWhitespace(a.name), Source: a.source.arg}
		if a.skip {
			shared.PrintFailure(opts.IO, result, createAction(a), a.attachErr)
			failed = true
			continue
		}

		created, err := c.Create(repo, opts.IssueNumber, a.artifactType, a.name, a.body)
		if err != nil {
			shared.PrintFailure(opts.IO, result, createAction(a), err)
			failed = true
			continue
		}

		result.Number = created.Number
		result.Name = text.RemoveExcessiveWhitespace(created.Name)
		message := fmt.Sprintf("Created artifact %d (%s) on %s", created.Number, result.Name, issueRef)
		if a.attachErr != nil {
			shared.PrintPartialFailure(opts.IO, "created", result, message, a.attachErr)
			failed = true
			continue
		}
		shared.PrintSuccess(opts.IO, "created", result, cs.SuccessIcon(), message)
	}

	if failed {
		return cmdutil.SilentError
	}
	return nil
}

// readSource reads an artifact's content and works out its type and name.
func readSource(s source, stdin io.ReadCloser, documentType string) (artifact, error) {
	a := artifact{source: s, artifactType: documentType, name: s.name}
	switch s.kind {
	case fromBody:
		a.body = s.body
	case fromStdin:
		body, err := shared.ReadContent(s.arg, stdin)
		if err != nil {
			return a, fmt.Errorf("failed to create artifact from standard input: %w", err)
		}
		a.body = body
	case fromURL:
		u, ok := shared.HTTPURL(s.arg)
		if !ok {
			return a, fmt.Errorf("failed to create artifact from %s: links must be http(s) URLs with a host, in printable ASCII with no spaces", s.arg)
		}
		a.artifactType, a.body = client.TypeLink, u
		if a.name == "" {
			// The server rejects the ":" and "/" of a whole URL in a name.
			parsed, err := url.Parse(u)
			if err != nil {
				return a, err
			}
			a.name = parsed.Hostname()
		}
	case fromFile:
		content, err := shared.ReadContent(s.arg, stdin)
		if err != nil {
			return a, fmt.Errorf("failed to create artifact from %s: %w", s.arg, err)
		}
		if shared.IsShortcutFile(s.arg) {
			u, err := shared.ShortcutURL(content)
			if errors.Is(err, shared.ErrNoShortcutSection) {
				return a, fmt.Errorf("%s: %w; pass the URL as an argument instead", s.arg, err)
			} else if err != nil {
				return a, fmt.Errorf("%s: %w", s.arg, err)
			}
			a.artifactType, a.body = client.TypeLink, u
		} else {
			a.body, a.dir = content, filepath.Dir(s.arg)
		}
		if a.name == "" {
			a.name = nameFromFile(s.arg)
			if a.name == "" {
				hint := "name it with `#<name>` or `--name`"
				if s.bodyFile {
					hint = "name it with `--name`"
				}
				return a, fmt.Errorf("failed to create artifact from %s: the file name has no letters or numbers; %s", s.arg, hint)
			}
		}
	}
	return a, nil
}

// nameFromFile returns the name gh gives an artifact from a file: its base
// name, as typed rather than a symlink's target, cleaned up to fit the
// server's rule for names, since the user didn't type it. Names may contain
// letters, numbers, accent marks, spaces, ".", "-" and "_", and must start
// and end with a letter or number. Other characters are removed, then the
// ends are trimmed, so "plan (1).md" becomes "plan 1.md" and "_draft.md"
// becomes "draft.md". The result is empty when the file name has no letters
// or numbers.
func nameFromFile(path string) string {
	name := strings.Map(func(r rune) rune {
		if isLetterOrNumber(r) || unicode.IsMark(r) || strings.ContainsRune(" .-_", r) {
			return r
		}
		return -1
	}, filepath.Base(path))
	return strings.TrimFunc(name, func(r rune) bool { return !isLetterOrNumber(r) })
}

func isLetterOrNumber(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}

// newUploader returns the uploader for --attach. It checks the issue like
// CheckIssue does, with one lookup that also returns what an upload needs.
func newUploader(opts *CreateOptions, c client.ArtifactClient, repo ghrepo.Interface) (*attachments.Uploader, error) {
	target, err := shared.CheckUploadTarget(c, repo, opts.IssueNumber)
	if err != nil {
		return nil, err
	}
	httpClient, err := opts.HttpClient()
	if err != nil {
		return nil, err
	}
	cfg, err := opts.Config()
	if err != nil {
		return nil, err
	}
	host := repo.RepoHost()
	return attachments.NewUploader(httpClient, cfg.Authentication().ActiveTokenType(host), host, target.RepositoryID, target.ViewerPermission)
}

// attach uploads the --attach files, each once, and points every document at
// the files it references. Links are left out, since they have no Markdown.
// With one document, the files it doesn't reference are appended to it.
//
// Uploads can't be undone, so a document is saved if any of its files
// uploaded, even when another failed, and that failure is the reason on its
// result line. A document none of whose files uploaded isn't saved.
func attach(opts *CreateOptions, uploader *attachments.Uploader, artifacts []artifact) error {
	var docs []attachments.Document
	var indexes []int
	for i, a := range artifacts {
		if a.artifactType == client.TypeLink {
			continue
		}
		docs = append(docs, attachments.Document{Markdown: a.body, Dir: a.dir})
		indexes = append(indexes, i)
	}

	results, err := uploader.UploadAndAttachDocuments(context.Background(), docs, opts.Assets)
	var operations attachments.UploadResult
	for _, r := range results {
		operations.AppendOperations += r.AppendOperations
		operations.ReplaceOperations += r.ReplaceOperations
	}
	opts.AttachEvent.RecordOperations(operations)

	// Without results, the files were refused before anything uploaded.
	if len(results) == 0 {
		if unreferenced, ok := errors.AsType[*attachments.UnreferencedError](err); ok {
			return unreferencedError(unreferenced.Paths)
		}
		return err
	}

	for j, r := range results {
		a := &artifacts[indexes[j]]
		a.body = r.Markdown
		a.attachErr = r.Err
		a.skip = r.Err != nil && len(r.Uploaded) == 0
	}
	return nil
}

// unreferencedError explains attached files that no document references,
// when there are several documents and none to append them to.
func unreferencedError(paths []string) error {
	if len(paths) == 1 {
		return fmt.Errorf("no file references %s; reference it in a file, or attach it when creating a single artifact", paths[0])
	}
	list := paths[0] + " or " + paths[1]
	if len(paths) > 2 {
		list = strings.Join(paths[:len(paths)-1], ", ") + ", or " + paths[len(paths)-1]
	}
	return fmt.Errorf("no file references %s; reference them in a file, or attach them when creating a single artifact", list)
}

// createAction names an artifact in its failure line by what it was created
// from, or by its name when it came from --body.
func createAction(a artifact) string {
	switch a.source.kind {
	case fromStdin:
		return "create artifact from standard input"
	case fromBody:
		return fmt.Sprintf("create artifact (%s)", text.RemoveExcessiveWhitespace(a.name))
	}
	return "create artifact from " + a.source.arg
}
