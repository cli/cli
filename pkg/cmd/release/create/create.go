package create

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/surveyext"
	"github.com/spf13/cobra"
)

type iprompter interface {
	Select(string, string, []string) (int, error)
	Input(string, string) (string, error)
	Confirm(string, bool) (bool, error)
}

type CreateOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	BaseRepo   func() (ghrepo.Interface, error)
	Edit       func(string, string, string, io.Reader, io.Writer, io.Writer) (string, error)
	Prompter   iprompter

	TagName      string
	Target       string
	Name         string
	Body         string
	BodyProvided bool
	Draft        bool
	Prerelease   bool
	IsLatest     *bool

	Assets []*shared.AssetForUpload

	// for interactive flow
	SubmitAction string
	// for interactive flow
	ReleaseNotesAction string
	// the value from the --repo flag
	RepoOverride string
	// maximum number of simultaneous uploads
	Concurrency        int
	DiscussionCategory string
	GenerateNotes      bool
	NotesStartTag      string
	VerifyTag          bool
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Config:     f.Config,
		Prompter:   f.Prompter,
		Edit:       surveyext.Edit,
	}

	var notesFile string

	cmd := &cobra.Command{
		DisableFlagsInUseLine: true,

		Use:   "create [<tag>] [<files>...]",
		Short: "Create a new release",
		Long: heredoc.Docf(`
			Create a new GitHub Release for a repository.

			A list of asset files may be given to upload to the new release. To define a
			display label for an asset, append text starting with %[1]s#%[1]s after the file name.

			If a matching git tag does not yet exist, one will automatically get created
			from the latest state of the default branch.
			Use %[1]s--target%[1]s to point to a different branch or commit for the automatic tag creation.
			Use %[1]s--verify-tag%[1]s to abort the release if the tag doesn't already exist.
			To fetch the new tag locally after the release, do %[1]sgit fetch --tags origin%[1]s.

			To create a release from an annotated git tag, first create one locally with
			git, push the tag to GitHub, then run this command.

			When using automatically generated release notes, a release title will also be automatically
			generated unless a title was explicitly passed. Additional release notes can be prepended to
			automatically generated notes by using the notes parameter.
		`, "`"),
		Example: heredoc.Doc(`
			Interactively create a release
			$ gh release create

			Interactively create a release from specific tag
			$ gh release create v1.2.3

			Non-interactively create a release
			$ gh release create v1.2.3 --notes "bugfix release"

			Use automatically generated release notes
			$ gh release create v1.2.3 --generate-notes

			Use release notes from a file
			$ gh release create v1.2.3 -F changelog.md

			Upload all tarballs in a directory as release assets
			$ gh release create v1.2.3 ./dist/*.tgz

			Upload a release asset with a display label
			$ gh release create v1.2.3 '/path/to/asset.zip#My display label'

			Create a release and start a discussion
			$ gh release create v1.2.3 --discussion-category "General"
		`),
		Aliases: []string{"new"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("discussion-category") && opts.Draft {
				return errors.New("discussions for draft releases not supported")
			}

			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo
			opts.RepoOverride, _ = cmd.Flags().GetString("repo")

			var err error

			if len(args) > 0 {
				opts.TagName = args[0]
				opts.Assets, err = shared.AssetsFromArgs(args[1:])
				if err != nil {
					return err
				}
			}

			if opts.TagName == "" && !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("tag required when not running interactively")
			}

			opts.Concurrency = 5

			opts.BodyProvided = cmd.Flags().Changed("notes") || opts.GenerateNotes
			if notesFile != "" {
				b, err := cmdutil.ReadFile(notesFile, opts.IO.In)
				if err != nil {
					return err
				}
				opts.Body = string(b)
				opts.BodyProvided = true
			}

			if runF != nil {
				return runF(opts)
			}
			return createRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Draft, "draft", "d", false, "Save the release as a draft instead of publishing it")
	cmd.Flags().BoolVarP(&opts.Prerelease, "prerelease", "p", false, "Mark the release as a prerelease")
	cmd.Flags().StringVar(&opts.Target, "target", "", "Target `branch` or full commit SHA (default: main branch)")
	cmd.Flags().StringVarP(&opts.Name, "title", "t", "", "Release title")
	cmd.Flags().StringVarP(&opts.Body, "notes", "n", "", "Release notes")
	cmd.Flags().StringVarP(&notesFile, "notes-file", "F", "", "Read release notes from `file` (use \"-\" to read from standard input)")
	cmd.Flags().StringVarP(&opts.DiscussionCategory, "discussion-category", "", "", "Start a discussion in the specified category")
	cmd.Flags().BoolVarP(&opts.GenerateNotes, "generate-notes", "", false, "Automatically generate title and notes for the release")
	cmd.Flags().StringVar(&opts.NotesStartTag, "notes-start-tag", "", "Tag to use as the starting point for generating release notes")
	cmdutil.NilBoolFlag(cmd, &opts.IsLatest, "latest", "", "Mark this release as \"Latest\" (default: automatic based on date and version)")
	cmd.Flags().BoolVarP(&opts.VerifyTag, "verify-tag", "", false, "Abort in case the git tag doesn't already exist in the remote repository")

	_ = cmdutil.RegisterBranchCompletionFlags(f.GitClient, cmd, "target")

	return cmd
}

func createRun(opts *CreateOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	var existingTag bool
	if opts.TagName == "" {
		tags, err := getTags(httpClient, baseRepo, 5)
		if err != nil {
			return err
		}

		if len(tags) != 0 {
			options := make([]string, len(tags))
			for i, tag := range tags {
				options[i] = tag.Name
			}
			createNewTagOption := "Create a new tag"
			options = append(options, createNewTagOption)
			selected, err := opts.Prompter.Select("Choose a tag", options[0], options)
			if err != nil {
				return fmt.Errorf("could not prompt: %w", err)
			}
			tag := options[selected]
			if tag != createNewTagOption {
				existingTag = true
				opts.TagName = tag
			}
		}

		if opts.TagName == "" {
			opts.TagName, err = opts.Prompter.Input("Tag name", "")
			if err != nil {
				return fmt.Errorf("could not prompt: %w", err)
			}
		}
	}

	if opts.VerifyTag && !existingTag {
		remoteTagPresent, err := remoteTagExists(httpClient, baseRepo, opts.TagName)
		if err != nil {
			return err
		}
		if !remoteTagPresent {
			return fmt.Errorf("tag %s doesn't exist in the repo %s, aborting due to --verify-tag flag",
				opts.TagName, ghrepo.FullName(baseRepo))
		}
	}

	var tagDescription string
	if opts.RepoOverride == "" {
		tagDescription, _ = gitTagInfo(opts.GitClient, opts.TagName)
		// If there is a local tag with the same name as specified
		// the user may not want to create a new tag on the remote
		// as the local one might be annotated or signed.
		// If the user specifies the target take that as explicit instruction
		// to create the tag on the remote pointing to the target regardless
		// of local tag status.
		// If a remote tag with the same name as specified exists already
		// then a new tag will not be created so ignore local tag status.
		if tagDescription != "" && !existingTag && opts.Target == "" && !opts.VerifyTag {
			remoteExists, err := remoteTagExists(httpClient, baseRepo, opts.TagName)
			if err != nil {
				return err
			}
			if !remoteExists {
				return fmt.Errorf("tag %s exists locally but has not been pushed to %s, please push it before continuing or specify the `--target` flag to create a new tag",
					opts.TagName, ghrepo.FullName(baseRepo))
			}
		}
	}

	if !opts.BodyProvided && opts.IO.CanPrompt() {
		editorCommand, err := cmdutil.DetermineEditor(opts.Config)
		if err != nil {
			return err
		}

		var generatedNotes *releaseNotes
		var generatedChangelog string

		generatedNotes, err = generateReleaseNotes(httpClient, baseRepo, opts.TagName, opts.Target, opts.NotesStartTag)
		if err != nil && !errors.Is(err, notImplementedError) {
			return err
		}

		if opts.RepoOverride == "" {
			headRef := opts.TagName
			if tagDescription == "" {
				if opts.Target != "" {
					// TODO: use the remote-tracking version of the branch ref
					headRef = opts.Target
				} else {
					headRef = "HEAD"
				}
			}
			if generatedNotes == nil {
				if opts.NotesStartTag != "" {
					commits, _ := changelogForRange(opts.GitClient, fmt.Sprintf("%s..%s", opts.NotesStartTag, headRef))
					generatedChangelog = generateChangelog(commits)
				} else if prevTag, err := detectPreviousTag(opts.GitClient, headRef); err == nil {
					commits, _ := changelogForRange(opts.GitClient, fmt.Sprintf("%s..%s", prevTag, headRef))
					generatedChangelog = generateChangelog(commits)
				}
			}
		}

		editorOptions := []string{"Write my own"}
		if generatedNotes != nil {
			editorOptions = append(editorOptions, "Write using generated notes as template")
		}
		if generatedChangelog != "" {
			editorOptions = append(editorOptions, "Write using commit log as template")
		}
		if tagDescription != "" {
			editorOptions = append(editorOptions, "Write using git tag message as template")
		}
		editorOptions = append(editorOptions, "Leave blank")

		defaultName := opts.Name
		if defaultName == "" && generatedNotes != nil {
			defaultName = generatedNotes.Name
		}

		opts.Name, err = opts.Prompter.Input("Title (optional)", defaultName)
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}

		selected, err := opts.Prompter.Select("Release notes", "", editorOptions)
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}
		opts.ReleaseNotesAction = editorOptions[selected]

		var openEditor bool
		var editorContents string

		switch opts.ReleaseNotesAction {
		case "Write my own":
			openEditor = true
		case "Write using generated notes as template":
			openEditor = true
			editorContents = generatedNotes.Body
		case "Write using commit log as template":
			openEditor = true
			editorContents = generatedChangelog
		case "Write using git tag message as template":
			openEditor = true
			editorContents = tagDescription
		case "Leave blank":
			openEditor = false
		default:
			return fmt.Errorf("invalid action: %v", opts.ReleaseNotesAction)
		}

		if openEditor {
			text, err := opts.Edit(editorCommand, "*.md", editorContents,
				opts.IO.In, opts.IO.Out, opts.IO.ErrOut)
			if err != nil {
				return err
			}
			opts.Body = text
		}

		saveAsDraft := "Save as draft"
		publishRelease := "Publish release"
		defaultSubmit := publishRelease
		if opts.Draft {
			defaultSubmit = saveAsDraft
		}

		opts.Prerelease, err = opts.Prompter.Confirm("Is this a prerelease?", opts.Prerelease)
		if err != nil {
			return err
		}

		options := []string{publishRelease, saveAsDraft, "Cancel"}
		selected, err = opts.Prompter.Select("Submit?", defaultSubmit, options)
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}

		opts.SubmitAction = options[selected]

		switch opts.SubmitAction {
		case "Publish release":
			opts.Draft = false
		case "Save as draft":
			opts.Draft = true
		case "Cancel":
			return cmdutil.CancelError
		default:
			return fmt.Errorf("invalid action: %v", opts.SubmitAction)
		}
	}

	params := map[string]interface{}{
		"tag_name":   opts.TagName,
		"draft":      opts.Draft,
		"prerelease": opts.Prerelease,
	}
	if opts.Name != "" {
		params["name"] = opts.Name
	}
	if opts.Body != "" {
		params["body"] = opts.Body
	}
	if opts.Target != "" {
		params["target_commitish"] = opts.Target
	}
	if opts.IsLatest != nil {
		// valid values: true/false/legacy
		params["make_latest"] = fmt.Sprintf("%v", *opts.IsLatest)
	}
	if opts.DiscussionCategory != "" {
		params["discussion_category_name"] = opts.DiscussionCategory
	}
	if opts.GenerateNotes {
		if opts.NotesStartTag != "" {
			generatedNotes, err := generateReleaseNotes(httpClient, baseRepo, opts.TagName, opts.Target, opts.NotesStartTag)
			if err != nil && !errors.Is(err, notImplementedError) {
				return err
			}
			if generatedNotes != nil {
				if opts.Body == "" {
					params["body"] = generatedNotes.Body
				} else {
					params["body"] = fmt.Sprintf("%s\n%s", opts.Body, generatedNotes.Body)
				}
				if opts.Name == "" {
					params["name"] = generatedNotes.Name
				}
			}
		} else {
			params["generate_release_notes"] = true
		}
	}

	hasAssets := len(opts.Assets) > 0
	draftWhileUploading := false

	if hasAssets && !opts.Draft {
		// Check for an existing release
		if opts.TagName != "" {
			if ok, err := publishedReleaseExists(httpClient, baseRepo, opts.TagName); err != nil {
				return fmt.Errorf("error checking for existing release: %w", err)
			} else if ok {
				return fmt.Errorf("a release with the same tag name already exists: %s", opts.TagName)
			}
		}
		// Save the release initially as draft and publish it after all assets have finished uploading
		draftWhileUploading = true
		params["draft"] = true
	}

	newRelease, err := createRelease(httpClient, baseRepo, params)
	if err != nil {
		return err
	}

	cleanupDraftRelease := func(err error) error {
		if !draftWhileUploading {
			return err
		}
		if cleanupErr := deleteRelease(httpClient, newRelease); cleanupErr != nil {
			return fmt.Errorf("%w\ncleaning up draft failed: %v", err, cleanupErr)
		}
		return err
	}

	if hasAssets {
		uploadURL := newRelease.UploadURL
		if idx := strings.IndexRune(uploadURL, '{'); idx > 0 {
			uploadURL = uploadURL[:idx]
		}

		opts.IO.StartProgressIndicator()
		err = shared.ConcurrentUpload(httpClient, uploadURL, opts.Concurrency, opts.Assets)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return cleanupDraftRelease(err)
		}

		if draftWhileUploading {
			rel, err := publishRelease(httpClient, newRelease.APIURL, opts.DiscussionCategory)
			if err != nil {
				return cleanupDraftRelease(err)
			}
			newRelease = rel
		}
	}

	fmt.Fprintf(opts.IO.Out, "%s\n", newRelease.URL)

	return nil
}

func gitTagInfo(client *git.Client, tagName string) (string, error) {
	cmd, err := client.Command(context.Background(), "tag", "--list", tagName, "--format=%(contents:subject)%0a%0a%(contents:body)")
	if err != nil {
		return "", err
	}
	b, err := cmd.Output()
	return string(b), err
}

func detectPreviousTag(client *git.Client, headRef string) (string, error) {
	cmd, err := client.Command(context.Background(), "describe", "--tags", "--abbrev=0", fmt.Sprintf("%s^", headRef))
	if err != nil {
		return "", err
	}
	b, err := cmd.Output()
	return strings.TrimSpace(string(b)), err
}

type logEntry struct {
	Subject string
	Body    string
}

func changelogForRange(client *git.Client, refRange string) ([]logEntry, error) {
	cmd, err := client.Command(context.Background(), "-c", "log.ShowSignature=false", "log", "--first-parent", "--reverse", "--pretty=format:%B%x00", refRange)
	if err != nil {
		return nil, err
	}
	b, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var entries []logEntry
	for _, cb := range bytes.Split(b, []byte{'\000'}) {
		c := strings.ReplaceAll(string(cb), "\r\n", "\n")
		c = strings.TrimPrefix(c, "\n")
		if len(c) == 0 {
			continue
		}
		parts := strings.SplitN(c, "\n\n", 2)
		var body string
		subject := strings.ReplaceAll(parts[0], "\n", " ")
		if len(parts) > 1 {
			body = parts[1]
		}
		entries = append(entries, logEntry{
			Subject: subject,
			Body:    body,
		})
	}

	return entries, nil
}

func generateChangelog(commits []logEntry) string {
	var parts []string
	for _, c := range commits {
		// TODO: consider rendering "Merge pull request #123 from owner/branch" differently
		parts = append(parts, fmt.Sprintf("* %s", c.Subject))
		if c.Body != "" {
			parts = append(parts, text.Indent(c.Body, "  "))
		}
	}
	return strings.Join(parts, "\n\n")
}
