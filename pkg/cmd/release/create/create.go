package create

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/cli/cli/v2/pkg/surveyext"
	"github.com/cli/cli/v2/pkg/text"
	"github.com/spf13/cobra"
)

type CreateOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Edit       func(string, string, string, io.Reader, io.Writer, io.Writer) (string, error)

	TagName      string
	Target       string
	Name         string
	Body         string
	BodyProvided bool
	Draft        bool
	Prerelease   bool

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
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
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
			from the latest state of the default branch. Use %[1]s--target%[1]s to override this.
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
	cmd.Flags().StringVarP(&opts.DiscussionCategory, "discussion-category", "", "", "Start a discussion of the specified category")
	cmd.Flags().BoolVarP(&opts.GenerateNotes, "generate-notes", "", false, "Automatically generate title and notes for the release")

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
			var tag string
			q := &survey.Select{
				Message: "Choose a tag",
				Options: options,
				Default: options[0],
			}
			err := prompt.SurveyAskOne(q, &tag)
			if err != nil {
				return fmt.Errorf("could not prompt: %w", err)
			}
			if tag != createNewTagOption {
				opts.TagName = tag
			}
		}

		if opts.TagName == "" {
			q := &survey.Input{
				Message: "Tag name",
			}
			err := prompt.SurveyAskOne(q, &opts.TagName)
			if err != nil {
				return fmt.Errorf("could not prompt: %w", err)
			}
		}
	}

	if !opts.BodyProvided && opts.IO.CanPrompt() {
		editorCommand, err := cmdutil.DetermineEditor(opts.Config)
		if err != nil {
			return err
		}

		var generatedNotes *releaseNotes
		var tagDescription string
		var generatedChangelog string

		params := map[string]interface{}{
			"tag_name": opts.TagName,
		}
		if opts.Target != "" {
			params["target_commitish"] = opts.Target
		}
		generatedNotes, err = generateReleaseNotes(httpClient, baseRepo, params)
		if err != nil && !errors.Is(err, notImplementedError) {
			return err
		}

		if opts.RepoOverride == "" && generatedNotes == nil {
			headRef := opts.TagName
			tagDescription, _ = gitTagInfo(opts.TagName)
			if tagDescription == "" {
				if opts.Target != "" {
					// TODO: use the remote-tracking version of the branch ref
					headRef = opts.Target
				} else {
					headRef = "HEAD"
				}
			}
			if prevTag, err := detectPreviousTag(headRef); err == nil {
				commits, _ := changelogForRange(fmt.Sprintf("%s..%s", prevTag, headRef))
				generatedChangelog = generateChangelog(commits)
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
		qs := []*survey.Question{
			{
				Name: "name",
				Prompt: &survey.Input{
					Message: "Title (optional)",
					Default: defaultName,
				},
			},
			{
				Name: "releaseNotesAction",
				Prompt: &survey.Select{
					Message: "Release notes",
					Options: editorOptions,
				},
			},
		}
		err = prompt.SurveyAsk(qs, opts)
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}

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

		qs = []*survey.Question{
			{
				Name: "prerelease",
				Prompt: &survey.Confirm{
					Message: "Is this a prerelease?",
					Default: opts.Prerelease,
				},
			},
			{
				Name: "submitAction",
				Prompt: &survey.Select{
					Message: "Submit?",
					Options: []string{
						publishRelease,
						saveAsDraft,
						"Cancel",
					},
					Default: defaultSubmit,
				},
			},
		}

		err = prompt.SurveyAsk(qs, opts)
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}

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

	if opts.Draft && len(opts.DiscussionCategory) > 0 {
		return fmt.Errorf(
			"%s Discussions not supported with draft releases",
			opts.IO.ColorScheme().FailureIcon(),
		)
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
	if opts.DiscussionCategory != "" {
		params["discussion_category_name"] = opts.DiscussionCategory
	}
	if opts.GenerateNotes {
		params["generate_release_notes"] = true
	}

	hasAssets := len(opts.Assets) > 0

	// Avoid publishing the release until all assets have finished uploading
	if hasAssets {
		params["draft"] = true
	}

	newRelease, err := createRelease(httpClient, baseRepo, params)
	if err != nil {
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
			return err
		}

		if !opts.Draft {
			rel, err := publishRelease(httpClient, newRelease.APIURL)
			if err != nil {
				return err
			}
			newRelease = rel
		}
	}

	fmt.Fprintf(opts.IO.Out, "%s\n", newRelease.URL)

	return nil
}

func gitTagInfo(tagName string) (string, error) {
	cmd, err := git.GitCommand("tag", "--list", tagName, "--format=%(contents:subject)%0a%0a%(contents:body)")
	if err != nil {
		return "", err
	}
	b, err := run.PrepareCmd(cmd).Output()
	return string(b), err
}

func detectPreviousTag(headRef string) (string, error) {
	cmd, err := git.GitCommand("describe", "--tags", "--abbrev=0", fmt.Sprintf("%s^", headRef))
	if err != nil {
		return "", err
	}
	b, err := run.PrepareCmd(cmd).Output()
	return strings.TrimSpace(string(b)), err
}

type logEntry struct {
	Subject string
	Body    string
}

func changelogForRange(refRange string) ([]logEntry, error) {
	cmd, err := git.GitCommand("-c", "log.ShowSignature=false", "log", "--first-parent", "--reverse", "--pretty=format:%B%x00", refRange)
	if err != nil {
		return nil, err
	}
	b, err := run.PrepareCmd(cmd).Output()
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
