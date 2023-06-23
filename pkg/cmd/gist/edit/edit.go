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

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/gist/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/surveyext"
	"github.com/spf13/cobra"
)

var editNextOptions = []string{"Edit another file", "Submit", "Cancel"}

type EditOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	Prompter   prompter.Prompter

	Edit func(string, string, string, *iostreams.IOStreams) (string, error)

	Selector       string
	EditFilename   string
	AddFilename    string
	RemoveFilename string
	SourceFile     string
	Description    string
}

func NewCmdEdit(f *cmdutil.Factory, runF func(*EditOptions) error) *cobra.Command {
	opts := EditOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Prompter:   f.Prompter,
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
			if len(args) > 2 {
				return cmdutil.FlagErrorf("too many arguments")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Selector = args[0]
			}
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
	cmd.Flags().StringVarP(&opts.RemoveFilename, "remove", "r", "", "Remove a file from the gist")

	cmd.MarkFlagsMutuallyExclusive("add", "remove")
	cmd.MarkFlagsMutuallyExclusive("remove", "filename")

	return cmd
}

func editRun(opts *EditOptions) error {
	client, err := opts.HttpClient()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host, _ := cfg.Authentication().DefaultHost()

	gistID := opts.Selector
	if gistID == "" {
		cs := opts.IO.ColorScheme()
		if gistID == "" {
			gistID, err = shared.PromptGists(opts.Prompter, client, host, cs)
			if err != nil {
				return err
			}

			if gistID == "" {
				fmt.Fprintln(opts.IO.Out, "No gists found.")
				return nil
			}
		}
	}

	if strings.Contains(gistID, "/") {
		id, err := shared.GistIDFromURL(gistID)
		if err != nil {
			return err
		}
		gistID = id
	}

	apiClient := api.NewClientFromHTTP(client)

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

	// Remove a file from the gist
	if opts.RemoveFilename != "" {
		err := removeFile(gist, opts.RemoveFilename)
		if err != nil {
			return err
		}

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
				result, err := opts.Prompter.Select("Edit which file?", "", candidates)
				if err != nil {
					return fmt.Errorf("could not prompt: %w", err)
				}
				filename = candidates[result]
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
		result, err := opts.Prompter.Select("What next?", "", editNextOptions)
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}
		choice = editNextOptions[result]

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

func removeFile(gist *shared.Gist, filename string) error {
	if _, found := gist.Files[filename]; !found {
		return fmt.Errorf("gist has no file %q", filename)
	}

	gist.Files[filename] = nil

	return nil
}
