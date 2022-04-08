package edit

import (
	"errors"
	"fmt"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"net/http"
)

type EditOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)

	TagName    string
	Target     string
	Name       *string
	Body       *string
	Draft      *bool
	Prerelease *bool
}

func NewCmdEdit(f *cmdutil.Factory, runF func(*EditOptions) error) *cobra.Command {
	opts := &EditOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	var notesFile string

	cmd := &cobra.Command{
		DisableFlagsInUseLine: true,

		Use:   "edit <tag>",
		Short: "Edit a release",
		Example: heredoc.Doc(`
			Publish a release that was previously a draft
			$ gh release edit v1.0 --draft=false

			Update the release notes from the content of a file
			$ gh release edit v1.0 --notes-file /path/to/release_notes.md
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo

			if notesFile != "" {
				b, err := cmdutil.ReadFile(notesFile, opts.IO.In)
				if err != nil {
					return err
				}
				body := string(b)
				opts.Body = &body
			}

			if runF != nil {
				return runF(opts)
			}
			return editRun(args[0], opts)
		},
	}

	cmdutil.NilBoolFlag(cmd, &opts.Draft, "draft", "", "Save the release as a draft instead of publishing it")
	cmdutil.NilBoolFlag(cmd, &opts.Prerelease, "prerelease", "", "Mark the release as a prerelease")
	cmdutil.NilStringFlag(cmd, &opts.Body, "notes", "n", "Release notes")
	cmdutil.NilStringFlag(cmd, &opts.Name, "title", "t", "Release title")
	cmd.Flags().StringVar(&opts.Target, "target", "", "Target `branch` or full commit SHA (default: main branch)")
	cmd.Flags().StringVar(&opts.TagName, "tag", "", "The name of the tag")
	cmd.Flags().StringVarP(&notesFile, "notes-file", "F", "", "Read release notes from `file` (use \"-\" to read from standard input)")

	return cmd
}

func editRun(tag string, opts *EditOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	release, err := shared.FetchRelease(httpClient, baseRepo, tag)
	if err != nil {
		return err
	}

	params := getParams(opts)

	if len(params) == 0 {
		return errors.New("nothing to edit")
	}

	editedRelease, err := editRelease(httpClient, baseRepo, release.ReleaseID, params)
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.Out, "%s\n", editedRelease.URL)

	return nil
}

func getParams(opts *EditOptions) map[string]interface{} {
	params := map[string]interface{}{}

	if opts.Body != nil {
		params["body"] = opts.Body
	}

	if opts.Draft != nil {
		params["draft"] = *opts.Draft
	}

	if opts.Name != nil {
		params["name"] = opts.Name
	}

	if opts.Prerelease != nil {
		params["prerelease"] = *opts.Prerelease
	}

	if opts.TagName != "" {
		params["tag_name"] = opts.TagName
	}

	if opts.Target != "" {
		params["target_commitish"] = opts.Target
	}

	return params
}
