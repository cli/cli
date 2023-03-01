package edit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
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
	SourceFile   string
	Description  string
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
				io.In, io.Out, io.ErrOut)
		},
	}

	cmd := &cobra.Command{
		Use:   "edit {<id> | <url>} [<filename>]",
		Short: "Edit one of your gists",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return cmdutil.FlagErrorf("cannot edit: gist argument required")
			}
			if len(args) > 2 {
				return cmdutil.FlagErrorf("too many arguments")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			opts.Selector = args[0]
			if len(args) > 1 {
				opts.SourceFile = args[1]
			}

			if runF != nil {
				return runF(&opts)
			}

			return editRun(&opts)
		},
	}

	cmd.Flags().StringVarP(&opts.AddFilename, "add", "a", "", "Add a new file to the gist")
	cmd.Flags().StringVarP(&opts.Description, "desc", "d", "", "New description for the gist")
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

	host, _ := cfg.Authentication().DefaultHost()

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
		return errors.New("you do not own this gist")
	}

	shouldUpdate := false
	if opts.Description != "" {
		shouldUpdate = true
		gist.Description = opts.Description
	}

	if opts.AddFilename != "" {
		var input io.Reader
		switch src := opts.SourceFile; {
		case src == "-":
			input = opts.IO.In
		case src != "":
			f, err := os.Open(src)
			if err != nil {
				return err
			}
			defer func() {
				_ = f.Close()
			}()
			input = f
		default:
			f, err := os.Open(opts.AddFilename)
			if err != nil {
				return err
			}
			defer func() {
				_ = f.Close()
			}()
			input = f
		}

		content, err := io.ReadAll(input)
		if err != nil {
			return fmt.Errorf("read content: %w", err)
		}

		files, err := getFilesToAdd(opts.AddFilename, content)
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
				//nolint:staticcheck // SA1019: prompt.SurveyAskOne is deprecated: use Prompter
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

		var text string
		if src := opts.SourceFile; src != "" {
			if src == "-" {
				data, err := io.ReadAll(opts.IO.In)
				if err != nil {
					return fmt.Errorf("read from stdin: %w", err)
				}
				text = string(data)
			} else {
				data, err := os.ReadFile(src)
				if err != nil {
					return fmt.Errorf("read %s: %w", src, err)
				}
				text = string(data)
			}
		} else {
			editorCommand, err := cmdutil.DetermineEditor(opts.Config)
			if err != nil {
				return err
			}

			data, err := opts.Edit(editorCommand, filename, gistFile.Content, opts.IO)
			if err != nil {
				return err
			}
			text = data
		}

		if text != gistFile.Content {
			gistFile.Content = text // so it appears if they re-edit
			filesToUpdate[filename] = text
		}

		if !opts.IO.CanPrompt() ||
			len(candidates) == 1 ||
			opts.EditFilename != "" {
			break
		}

		choice := ""

		//nolint:staticcheck // SA1019: prompt.SurveyAskOne is deprecated: use Prompter
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

	if len(filesToUpdate) > 0 {
		shouldUpdate = true
	}

	if !shouldUpdate {
		return nil
	}

	return updateGist(apiClient, host, gist)
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

func getFilesToAdd(file string, content []byte) (map[string]*shared.GistFile, error) {
	if shared.IsBinaryContents(content) {
		return nil, fmt.Errorf("failed to upload %s: binary file not supported", file)
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
