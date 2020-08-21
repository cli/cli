package create

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/release/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/pkg/surveyext"
	"github.com/spf13/cobra"
)

type CreateOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)

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

	// maximum number of simultaneous uploads
	Concurrency int
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	var notesFile string

	cmd := &cobra.Command{
		Use:   "create <tag> [<files>...]",
		Short: "Create a new release",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			opts.TagName = args[0]

			var err error
			opts.Assets, err = shared.AssetsFromArgs(args[1:])
			if err != nil {
				return err
			}

			opts.Concurrency = 5

			opts.BodyProvided = cmd.Flags().Changed("notes")
			if notesFile != "" {
				var b []byte
				if notesFile == "-" {
					b, err = ioutil.ReadAll(opts.IO.In)
				} else {
					b, err = ioutil.ReadFile(notesFile)
				}
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
	cmd.Flags().StringVar(&opts.Target, "target", "", "Target `branch` or commit SHA (default: main branch)")
	cmd.Flags().StringVarP(&opts.Name, "title", "t", "", "Release title")
	cmd.Flags().StringVarP(&opts.Body, "notes", "n", "", "Release notes")
	cmd.Flags().StringVarP(&notesFile, "notes-file", "F", "", "Read release notes from `file`")

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

	if !opts.BodyProvided && opts.IO.CanPrompt() {
		editorCommand, err := cmdutil.DetermineEditor(opts.Config)
		if err != nil {
			return err
		}

		qs := []*survey.Question{
			{
				Name: "name",
				Prompt: &survey.Input{
					Message: "Title",
					Default: opts.Name,
				},
			},
			{
				Name: "body",
				Prompt: &surveyext.GhEditor{
					BlankAllowed:  true,
					EditorCommand: editorCommand,
					Editor: &survey.Editor{
						Message:     "Release notes",
						FileName:    "*.md",
						Default:     opts.Body,
						HideDefault: true,
					},
				},
			},
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
						"Publish release",
						"Save as draft",
						"Cancel",
					},
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
			return cmdutil.SilentError
		}
	}

	params := map[string]interface{}{
		"tag_name":   opts.TagName,
		"draft":      opts.Draft,
		"prerelease": opts.Prerelease,
		"name":       opts.Name,
		"body":       opts.Body,
	}
	if opts.Target != "" {
		params["target_commitish"] = opts.Target
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
			err := publishRelease(httpClient, newRelease.URL)
			if err != nil {
				return err
			}
		}
	}

	fmt.Fprintf(opts.IO.Out, "%s\n", newRelease.HTMLURL)

	return nil
}
