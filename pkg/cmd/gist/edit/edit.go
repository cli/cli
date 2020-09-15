package edit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/pkg/cmd/gist/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/pkg/surveyext"
	"github.com/spf13/cobra"
)

type EditOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)

	Edit func(string, string, string, *iostreams.IOStreams) (string, error)

	Selector string
	Filename string
}

func NewCmdEdit(f *cmdutil.Factory, runF func(*EditOptions) error) *cobra.Command {
	opts := EditOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Edit: func(editorCmd, filename, defaultContent string, io *iostreams.IOStreams) (string, error) {
			return surveyext.Edit(
				editorCmd,
				"*."+filename,
				defaultContent,
				io.In, io.Out, io.ErrOut, nil)
		},
	}

	cmd := &cobra.Command{
		Use:   "edit {<gist ID> | <gist URL>}",
		Short: "Edit one of your gists",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Selector = args[0]

			if runF != nil {
				return runF(&opts)
			}

			return editRun(&opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Filename, "filename", "f", "", "a specific file to edit")

	return cmd
}

func editRun(opts *EditOptions) error {
	gistID := opts.Selector

	u, err := url.Parse(opts.Selector)
	if err == nil {
		if strings.HasPrefix(u.Path, "/") {
			gistID = u.Path[1:]
		}
	}

	client, err := opts.HttpClient()
	if err != nil {
		return err
	}

	gist, err := shared.GetGist(client, ghinstance.OverridableDefault(), gistID)
	if err != nil {
		return err
	}

	filesToUpdate := map[string]string{}

	for {
		filename := opts.Filename
		candidates := []string{}
		for filename := range gist.Files {
			candidates = append(candidates, filename)
		}

		sort.Strings(candidates)

		if filename == "" {
			if len(candidates) == 1 {
				filename = candidates[0]
			} else {
				if !opts.IO.CanPrompt() {
					return errors.New("unsure what file to edit; either specify --filename or run interactively")
				}
				err = prompt.SurveyAskOne(&survey.Select{
					Message: "Edit which file?",
					Options: candidates,
				}, &filename)

				if err != nil {
					return fmt.Errorf("could not prompt: %w", err)
				}
			}
		}

		if _, ok := gist.Files[filename]; !ok {
			return fmt.Errorf("gist has no file %q", filename)
		}

		editorCommand, err := cmdutil.DetermineEditor(opts.Config)
		if err != nil {
			return err
		}
		text, err := opts.Edit(editorCommand, filename, gist.Files[filename].Content, opts.IO)

		if err != nil {
			return err
		}

		if text != gist.Files[filename].Content {
			gistFile := gist.Files[filename]
			gistFile.Content = text // so it appears if they re-edit
			filesToUpdate[filename] = text
		}

		if !opts.IO.CanPrompt() {
			break
		}

		if len(candidates) == 1 {
			break
		}

		choice := ""

		err = prompt.SurveyAskOne(&survey.Select{
			Message: "What next?",
			Options: []string{
				"Edit another file",
				"Submit",
				"Cancel",
			},
		}, &choice)

		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}

		stop := false

		switch choice {
		case "Edit another file":
			continue
		case "Submit":
			stop = true
		case "Cancel":
			return cmdutil.SilentError
		}

		if stop {
			break
		}
	}

	err = updateGist(client, ghinstance.OverridableDefault(), gist)
	if err != nil {
		return err
	}

	return nil
}

func updateGist(client *http.Client, hostname string, gist *shared.Gist) error {
	body := shared.Gist{
		Description: gist.Description,
		Files:       gist.Files,
	}

	path := "gists/" + gist.ID

	requestByte, err := json.Marshal(body)
	if err != nil {
		return err
	}

	requestBody := bytes.NewReader(requestByte)

	result := shared.Gist{}

	apiClient := api.NewClientFromHTTP(client)
	err = apiClient.REST(hostname, "POST", path, requestBody, &result)

	if err != nil {
		return err
	}

	return nil
}
