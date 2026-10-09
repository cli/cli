package edit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

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

// EditOptions holds the flags and dependencies of gh issue artifact edit.
type EditOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Config     func() (gh.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Client     func() (client.ArtifactClient, error)

	IssueNumber    int
	ArtifactNumber int
	// Name is the new name from --name, or nil to keep the current name.
	Name *string
	// Body is the new content from --body, or nil when --body wasn't given.
	Body *string
	// BodyFile is the file --body-file reads the new content from, "-" for
	// standard input, or empty when --body-file wasn't given. Result lines
	// show it as the source.
	BodyFile string
	// RestoreVersion is the version from the edit history to restore with
	// --restore-version, or 0 when it wasn't given. A restore takes its name
	// and body from that version, so no other flag goes with it.
	RestoreVersion int

	AttachFlag  *attachments.Flag
	AttachEvent *attachments.TelemetryEvent
	Assets      []attachments.UserAsset
}

// NewCmdEdit creates the gh issue artifact edit command.
func NewCmdEdit(f *cmdutil.Factory, telemetry ghtelemetry.InvocationRecorder, runF func(*EditOptions) error) *cobra.Command {
	opts := &EditOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "edit {<issue-number> | <issue-url>} <artifact-number>",
		Short: "Edit an issue artifact (preview)",
		Long: heredoc.Docf(`
			Rename an issue artifact or replace its content. The type never changes, so a
			link's new content must be an http(s) URL or a %[1]s.url%[1]s Internet Shortcut file.

			With only %[1]s--attach%[1]s, the attachments are appended to the current content.

			With %[1]s--restore-version%[1]s, an earlier version's name and content are saved as a
			new version. %[1]sgh issue artifact view%[1]s shows the edit history.
		`, "`"),
		Example: heredoc.Doc(`
			# Rename an artifact
			$ gh issue artifact edit 142 2 --name 'OAuth callback plan v2'

			# Replace a document's content with a file
			$ gh issue artifact edit 142 2 --body-file 2-OAuth-callback-plan.md

			# Point a link at a new URL
			$ gh issue artifact edit 142 3 --body https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook-v2

			# Append a screenshot to a document
			$ gh issue artifact edit 142 2 --attach ./signin-flow.png

			# Restore an earlier version
			$ gh issue artifact edit 142 2 --restore-version 5
		`),
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			opts.IssueNumber, opts.BaseRepo, err = shared.ParseIssueArg(args[0], f.BaseRepo)
			if err != nil {
				return err
			}
			opts.ArtifactNumber, err = shared.ParseArtifactNumber(args[1])
			if err != nil {
				return err
			}

			// An empty --body-file names no file, as in gh issue edit.
			editing := opts.Name != nil || opts.Body != nil || opts.BodyFile != "" || opts.AttachFlag.Changed()
			restoring := cmd.Flags().Changed("restore-version")
			if restoring && editing {
				return cmdutil.FlagErrorf("the `--restore-version` flag is not supported with `--name`, `--body`, `--body-file`, or `--attach`")
			}
			if err := cmdutil.MutuallyExclusive("specify only one of `--body` or `--body-file`", opts.Body != nil, opts.BodyFile != ""); err != nil {
				return err
			}
			switch {
			case restoring && opts.RestoreVersion < 1:
				return cmdutil.FlagErrorf("invalid version: %v", opts.RestoreVersion)
			case !restoring && !editing:
				return cmdutil.FlagErrorf("specify at least one of `--name`, `--body`, `--body-file`, `--attach`, or `--restore-version`")
			case opts.Name != nil && *opts.Name == "":
				return cmdutil.FlagErrorf("--name cannot be blank")
			case opts.Body != nil && strings.TrimSpace(*opts.Body) == "":
				return cmdutil.FlagErrorf("--body cannot be blank")
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
			return editRun(opts)
		},
	}

	cmdutil.NilStringFlag(cmd, &opts.Body, "body", "b", "New content; for a link, a URL")
	cmd.Flags().StringVarP(&opts.BodyFile, "body-file", "F", "", "Read new content from `file` (use \"-\" to read from standard input)")
	cmdutil.NilStringFlag(cmd, &opts.Name, "name", "", "New name")
	opts.AttachFlag = attachments.AddFlag(cmd)
	cmd.Flags().IntVar(&opts.RestoreVersion, "restore-version", 0, "Restore the version with this `number` from the edit history")

	return cmd
}

func editRun(opts *EditOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}
	if err := shared.CheckHost(repo.RepoHost()); err != nil {
		return err
	}

	// The new content is read and checked before any request, so a file that
	// can't be used fails without one.
	content, err := readContent(opts)
	if err != nil {
		return err
	}

	c, err := opts.Client()
	if err != nil {
		return err
	}

	var uploader *attachments.Uploader
	if len(opts.Assets) > 0 {
		uploader, err = shared.NewUploader(c, repo, opts.IssueNumber, opts.HttpClient, opts.Config)
		if err != nil {
			return err
		}
	} else if err := shared.CheckIssue(c, repo, opts.IssueNumber); err != nil {
		return err
	}

	action := fmt.Sprintf("update artifact %d", opts.ArtifactNumber)
	if opts.RestoreVersion > 0 {
		action = fmt.Sprintf("restore artifact %d", opts.ArtifactNumber)
	}
	result := shared.Result{Number: opts.ArtifactNumber, Source: opts.BodyFile}
	current, err := c.Get(repo, opts.IssueNumber, opts.ArtifactNumber)
	if err != nil {
		shared.PrintFailure(opts.IO, result, action, err)
		return cmdutil.SilentError
	}
	result.Name = text.RemoveExcessiveWhitespace(current.Name)

	// The type decides what the new content can be, so every check that
	// depends on it runs before anything is uploaded or saved.
	if err := shared.CheckType(current.Artifact); err != nil {
		return err
	}
	if opts.RestoreVersion > 0 {
		return restoreVersion(opts, c, repo, current, result, action)
	}
	body, err := newBody(opts, current.Artifact, content)
	if err != nil {
		return err
	}

	var uploadErr error
	if uploader != nil {
		// A --body-file's references resolve against its own directory first.
		// Other content has no directory of its own, so its references
		// resolve against the working directory.
		var dir string
		if opts.BodyFile != "" && opts.BodyFile != "-" {
			dir = filepath.Dir(opts.BodyFile)
		}
		md, uploadResult, err := uploader.UploadAndAttach(context.Background(), *body, dir, opts.Assets)
		opts.AttachEvent.RecordOperations(uploadResult)
		// Uploads can't be undone, so the artifact is saved if any file
		// uploaded, even when another failed. It isn't saved when none did.
		if err != nil && uploadResult.Uploaded == 0 {
			shared.PrintFailure(opts.IO, result, action, err)
			return cmdutil.SilentError
		}
		body, uploadErr = &md, err
	}

	updated, err := c.Update(repo, opts.IssueNumber, opts.ArtifactNumber, opts.Name, body)
	if err != nil {
		shared.PrintFailure(opts.IO, result, action, err)
		return cmdutil.SilentError
	}

	result.Name = text.RemoveExcessiveWhitespace(updated.Name)
	message := fmt.Sprintf("Updated artifact %d (%s) on %s#%d", opts.ArtifactNumber, result.Name, ghrepo.FullName(repo), opts.IssueNumber)
	if uploadErr != nil {
		shared.PrintPartialFailure(opts.IO, "updated", result, message, uploadErr)
		return cmdutil.SilentError
	}
	shared.PrintSuccess(opts.IO, "updated", result, opts.IO.ColorScheme().SuccessIcon(), message)
	return nil
}

// restoreVersion saves the name and body of version opts.RestoreVersion as a
// new version. They are sent as the API returned them, so the server checks
// them like any other edit. Every save adds a version to the edit history,
// even when nothing changed, so nothing is saved when the artifact already
// matches the version.
func restoreVersion(opts *EditOptions, c client.ArtifactClient, repo ghrepo.Interface, current *client.ArtifactWithVersions, result shared.Result, action string) error {
	v, err := shared.FindVersion(current, opts.RestoreVersion)
	if err != nil {
		return err
	}
	// FindVersion returns nil for the current version, which the artifact
	// matches.
	if v == nil || (v.Name == current.Name && v.Body == current.Body) {
		message := fmt.Sprintf("Artifact %d (%s) on %s#%d already matches version %d", opts.ArtifactNumber, result.Name, ghrepo.FullName(repo), opts.IssueNumber, opts.RestoreVersion)
		shared.PrintUnchanged(opts.IO, result, message, fmt.Sprintf("already matches version %d", opts.RestoreVersion))
		return nil
	}

	updated, err := c.Update(repo, opts.IssueNumber, opts.ArtifactNumber, &v.Name, &v.Body)
	if err != nil {
		shared.PrintFailure(opts.IO, result, action, err)
		return cmdutil.SilentError
	}

	result.Name = text.RemoveExcessiveWhitespace(updated.Name)
	message := fmt.Sprintf("Restored artifact %d (%s) to version %d on %s#%d", opts.ArtifactNumber, result.Name, opts.RestoreVersion, ghrepo.FullName(repo), opts.IssueNumber)
	shared.PrintSuccess(opts.IO, "restored", result, opts.IO.ColorScheme().SuccessIcon(), message)
	return nil
}

// readContent returns the new content from --body or --body-file, or nil
// when neither was given. Standard input is only read with --body-file -, so
// a stdin that never closes can't hang the command.
func readContent(opts *EditOptions) (*string, error) {
	if opts.BodyFile == "" {
		return opts.Body, nil
	}
	content, err := shared.ReadContent(opts.BodyFile, opts.IO.In)
	if err != nil {
		source := opts.BodyFile
		if source == "-" {
			source = "standard input"
		}
		return nil, fmt.Errorf("failed to update artifact %d from %s: %w", opts.ArtifactNumber, source, err)
	}
	return &content, nil
}

// newBody returns the body to save, checked against the artifact's type, or
// nil to keep the current body. A link's new content must be an http(s) URL:
// a .url file is read as a shortcut, and anything else must be a bare URL,
// with its surrounding whitespace trimmed. A link has no Markdown for
// --attach. Any other artifact is a Markdown document, which never takes a
// .url file, and with only --attach, the files are appended to its current
// body.
func newBody(opts *EditOptions, a client.Artifact, content *string) (*string, error) {
	if a.Type != client.TypeLink {
		if shared.IsShortcutFile(opts.BodyFile) {
			return nil, fmt.Errorf("artifact %d is not a link; .url files can only update link artifacts", a.Number)
		}
		if content == nil && len(opts.Assets) > 0 {
			return &a.Body, nil
		}
		return content, nil
	}

	if len(opts.Assets) > 0 {
		return nil, fmt.Errorf("artifact %d is a link; --attach can't be used with link artifacts", a.Number)
	}
	if content == nil {
		return nil, nil
	}
	if shared.IsShortcutFile(opts.BodyFile) {
		u, err := shared.ShortcutURL(*content)
		if errors.Is(err, shared.ErrNoShortcutSection) {
			return nil, fmt.Errorf("%s: %w; pass the URL with `--body` instead", opts.BodyFile, err)
		} else if err != nil {
			return nil, fmt.Errorf("%s: %w", opts.BodyFile, err)
		}
		return &u, nil
	}
	u, ok := shared.HTTPURL(*content)
	if !ok {
		return nil, fmt.Errorf("artifact %d is a link; its new content must be an http(s) URL or a .url file", a.Number)
	}
	return &u, nil
}
