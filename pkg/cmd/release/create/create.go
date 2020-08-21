package create

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/release/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type CreateOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	TagName      string
	Target       string
	Name         string
	Body         string
	BodyProvided bool
	Draft        bool
	Prerelease   bool

	Assets []*shared.AssetForUpload

	// maximum number of simultaneous uploads
	Concurrency int
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
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
