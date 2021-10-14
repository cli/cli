package edit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmd/gist/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/cli/cli/v2/pkg/surveyext"
	"github.com/spf13/cobra"
)

type EditOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)

	Edit func(string, string, string, *iostreams.IOStreams) (string, error)

	Selector     string
	EditFilename string
	AddFilename  string
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
		Use:   "edit {<id> | <url>}",
		Short: "Edit one of your gists",
		Args:  cmdutil.ExactArgs(1, "cannot edit: gist argument required"),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Selector = args[0]

			if runF != nil {
				return runF(&opts)
			}

			return editRun(&opts)
		},
	}

	cmd.Flags().StringVarP(&opts.AddFilename, "add", "a", "", "Add a new file to the gist")
	cmd.Flags().StringVarP(&opts.EditFilename, "filename", "f", "", "Select a file to edit")

	return cmd
}

func editRun(opts *EditOptions) error {
	gistID := opts.Selector

	if strings.Contains(gistID, "/") {
		id, err := shared.GistIDFromURL(gistID)
		if err != nil {
			return err
		}
		gistID = id
	}

	client, err := opts.HttpClient()
	if err != nil {
		return err
	}

	apiClient := api.NewClientFromHTTP(client)

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host, err := cfg.DefaultHost()
	if err != nil {
		return err
	}

	gist, err := shared.GetGist(client, host, gistID)
	if err != nil {
		if errors.Is(err, shared.NotFoundErr) {
			return fmt.Errorf("gist not found: %s", gistID)
		}
		return err
	}

	username, err := api.CurrentLoginName(apiClient, host)
	if err != nil {
		return err
	}

	if username != gist.Owner.Login {
		return fmt.Errorf("You do not own this gist.")
	}

	if opts.AddFilename != "" {
		files, err := getFilesToAdd(opts.AddFilename)
		if err != nil {
			return err
		}

		gist.Files = files
		return updateGist(apiClient, host, gist)
	}

	filesToUpdate := map[string]string{}

	for {
		filename := opts.EditFilename
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

		gistFile, found := gist.Files[filename]
		if !found {
			return fmt.Errorf("gist has no file %q", filename)
		}
		if shared.IsBinaryContents([]byte(gistFile.Content)) {
			return fmt.Errorf("editing binary files not supported")
		}

		editorCommand, err := cmdutil.DetermineEditor(opts.Config)
		if err != nil {
			return err
		}
		text, err := opts.Edit(editorCommand, filename, gistFile.Content, opts.IO)

		if err != nil {
			return err
		}

		if text != gistFile.Content {
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
			return cmdutil.CancelError
		}

		if stop {
			break
		}
	}

	if len(filesToUpdate) == 0 {
		return nil
	}

	err = updateGist(apiClient, host, gist)
	if err != nil {
		return err
	}

	return nil
}

func updateGist(apiClient *api.Client, hostname string, gist *shared.Gist) error {
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

	err = apiClient.REST(hostname, "POST", path, requestBody, &result)

	if err != nil {
		return err
	}

	return nil
}

func getFilesToAdd(file string) (map[string]*shared.GistFile, error) {
	isBinary, err := shared.IsBinaryFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", file, err)
	}
	if isBinary {
		return nil, fmt.Errorf("failed to upload %s: binary file not supported", file)
	}

	content, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", file, err)
	}

	if len(content) == 0 {
		return nil, errors.New("file contents cannot be empty")
	}

	filename := filepath.Base(file)
	return map[string]*shared.GistFile{
		filename: {
			Filename: filename,
			Content:  string(content),
		},
	}, nil
}
