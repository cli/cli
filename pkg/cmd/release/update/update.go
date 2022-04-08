package update

import (
	"errors"
	"fmt"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"net/http"
)

type UpdateOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)

	ReleaseId    string
	TagName      string
	Target       string
	Name         string
	Body         string
	BodyProvided bool
	Draft        *bool
	Prerelease   *bool

	// the value from the --repo flag
	RepoOverride string
}

func NewCmdUpdate(f *cmdutil.Factory, runF func(*UpdateOptions) error) *cobra.Command {
	opts := &UpdateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	var notesFile string

	cmd := &cobra.Command{
		DisableFlagsInUseLine: true,

		Use:   "update <release_id>",
		Short: "Edit a release",
		Example: heredoc.Doc(`
			Publish a release that was previously a draft
			$ gh release update 63899317 --draft=false

			Mark a release as a prerelease
			$ gh release update 63899317 --prerelease

			Mark a release as a full release
			$ gh release update 63899317 --prerelease=false

			Update the target commitish of a release
			$ gh release update 63899317 --target 97ea5e77b4d61d5d80ed08f7512847dee3ec9af5

			Update the name of the tag
			$ gh release update 63899317 --tag v9.8.7

			Update the name/title of a release
			$ gh release update 63899317 --title 'Some new title'

			Update the release notes
			$ gh release update 63899317 --notes 'Some short release notes'

			Update the release notes from the content of a file
			$ gh release update 63899317 --notes-file /path/to/release_notes.md
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ReleaseId = args[0]

			opts.BaseRepo = f.BaseRepo
			opts.RepoOverride, _ = cmd.Flags().GetString("repo")

			opts.BodyProvided = cmd.Flags().Changed("notes")
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
			return updateRun(opts)
		},
	}

	cmdutil.NilBoolFlag(cmd, &opts.Draft, "draft", "", "Save the release as a draft instead of publishing it")
	cmdutil.NilBoolFlag(cmd, &opts.Prerelease, "prerelease", "", "Mark the release as a prerelease")
	cmd.Flags().StringVar(&opts.Target, "target", "", "Target `branch` or full commit SHA (default: main branch)")
	cmd.Flags().StringVar(&opts.TagName, "tag", "", "The name of the tag")
	cmd.Flags().StringVarP(&opts.Name, "title", "t", "", "Release title")
	cmd.Flags().StringVarP(&opts.Body, "notes", "n", "", "Release notes")
	cmd.Flags().StringVarP(&notesFile, "notes-file", "F", "", "Read release notes from `file` (use \"-\" to read from standard input)")

	return cmd
}

func updateRun(opts *UpdateOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	params := getParams(opts)

	if len(params) == 0 {
		return errors.New("nothing to update")
	}

	updatedRelease, err := updateRelease(httpClient, baseRepo, opts.ReleaseId, params)
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.Out, "%s\n", updatedRelease.URL)

	return nil
}

func getParams(opts *UpdateOptions) map[string]interface{} {
	params := map[string]interface{}{}

	if opts.Body != "" {
		params["body"] = opts.Body
	}

	if opts.Draft != nil {
		params["draft"] = *opts.Draft
	}

	if opts.Name != "" {
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
