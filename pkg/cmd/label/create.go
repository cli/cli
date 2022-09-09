package label

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

var randomColors = []string{
	"B60205",
	"D93F0B",
	"FBCA04",
	"0E8A16",
	"006B75",
	"1D76DB",
	"0052CC",
	"5319E7",
	"E99695",
	"F9D0C4",
	"FEF2C0",
	"C2E0C6",
	"BFDADC",
	"C5DEF5",
	"BFD4F2",
	"D4C5F9",
}

type createOptions struct {
	BaseRepo   func() (ghrepo.Interface, error)
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Color       string
	Description string
	Name        string
	Force       bool
}

func newCmdCreate(f *cmdutil.Factory, runF func(*createOptions) error) *cobra.Command {
	opts := createOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new label",
		Long: heredoc.Doc(`
			Create a new label on GitHub, or updates an existing one with --force.

			Must specify name for the label. The description and color are optional.
			If a color isn't provided, a random one will be chosen.

			The label color needs to be 6 character hex value.
		`),
		Example: heredoc.Doc(`
			# create new bug label
			$ gh label create bug --description "Something isn't working" --color E99695
		`),
		Args: cmdutil.ExactArgs(1, "cannot create label: name argument required"),
		RunE: func(c *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo
			opts.Name = args[0]
			opts.Color = strings.TrimPrefix(opts.Color, "#")
			if runF != nil {
				return runF(&opts)
			}
			return createRun(&opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "Description of the label")
	cmd.Flags().StringVarP(&opts.Color, "color", "c", "", "Color of the label")
	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "Update the label color and description if label already exists")

	return cmd
}

var errLabelAlreadyExists = errors.New("label already exists")

func createRun(opts *createOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	if opts.Color == "" {
		rand.Seed(time.Now().UnixNano())
		opts.Color = randomColors[rand.Intn(len(randomColors)-1)]
	}

	opts.IO.StartProgressIndicator()
	err = createLabel(httpClient, baseRepo, opts)
	opts.IO.StopProgressIndicator()
	if err != nil {
		if errors.Is(err, errLabelAlreadyExists) {
			return fmt.Errorf("label with name %q already exists; use `--force` to update its color and description", opts.Name)
		}
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		successMsg := fmt.Sprintf("%s Label %q created in %s\n", cs.SuccessIcon(), opts.Name, ghrepo.FullName(baseRepo))
		fmt.Fprint(opts.IO.Out, successMsg)
	}

	return nil
}

func createLabel(client *http.Client, repo ghrepo.Interface, opts *createOptions) error {
	apiClient := api.NewClientFromHTTP(client)
	path := fmt.Sprintf("repos/%s/%s/labels", repo.RepoOwner(), repo.RepoName())
	requestByte, err := json.Marshal(map[string]string{
		"name":        opts.Name,
		"description": opts.Description,
		"color":       opts.Color,
	})
	if err != nil {
		return err
	}
	requestBody := bytes.NewReader(requestByte)
	result := label{}
	err = apiClient.REST(repo.RepoHost(), "POST", path, requestBody, &result)

	if httpError, ok := err.(api.HTTPError); ok && isLabelAlreadyExistsError(httpError) {
		err = errLabelAlreadyExists
	}

	if opts.Force && errors.Is(err, errLabelAlreadyExists) {
		editOpts := editOptions{
			Description: opts.Description,
			Color:       opts.Color,
			Name:        opts.Name,
		}
		return updateLabel(apiClient, repo, &editOpts)
	}

	return err
}

func updateLabel(apiClient *api.Client, repo ghrepo.Interface, opts *editOptions) error {
	path := fmt.Sprintf("repos/%s/%s/labels/%s", repo.RepoOwner(), repo.RepoName(), opts.Name)
	properties := map[string]string{}
	if opts.Description != "" {
		properties["description"] = opts.Description
	}
	if opts.Color != "" {
		properties["color"] = opts.Color
	}
	if opts.NewName != "" {
		properties["new_name"] = opts.NewName
	}
	requestByte, err := json.Marshal(properties)
	if err != nil {
		return err
	}
	requestBody := bytes.NewReader(requestByte)
	result := label{}
	err = apiClient.REST(repo.RepoHost(), "PATCH", path, requestBody, &result)

	if httpError, ok := err.(api.HTTPError); ok && isLabelAlreadyExistsError(httpError) {
		err = errLabelAlreadyExists
	}

	return err
}

func isLabelAlreadyExistsError(err api.HTTPError) bool {
	return err.StatusCode == 422 && len(err.Errors) == 1 && err.Errors[0].Field == "name" && err.Errors[0].Code == "already_exists"
}
