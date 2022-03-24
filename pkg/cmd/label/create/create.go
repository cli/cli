package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/label/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

var randomColor = []string{
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

type CreateOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	Name        string
	Description string
	Color       string
	Interactive bool
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new label",
		Example: heredoc.Doc(`
		$ gh label create --name bug
		$ gh label create --name bug --description "Something isn't working"
		$ gh label create --name bug --description "Something isn't working" --color "E99695"
	`),
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			nameProvided := c.Flags().Changed("name")

			if !nameProvided || strings.TrimSpace(opts.Name) == "" {
				return cmdutil.FlagErrorf("must specify name for label.")
			}

			if runF != nil {
				return runF(&opts)
			}
			return createRun(&opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Name, "name", "n", "", "Name of the label")
	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "Description of the label")
	cmd.Flags().StringVarP(&opts.Color, "color", "c", "", "Color of the label, if not specified will be random")

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

	if opts.Color == "" {
		rand.Seed(time.Now().UnixNano())
		opts.Color = randomColor[rand.Intn(len(randomColor)-1)]
	}

	client := api.NewClientFromHTTP(httpClient)

	opts.IO.StartProgressIndicator()

	err = createLabel(opts, client, baseRepo)

	opts.IO.StopProgressIndicator()

	if err != nil {
		return err
	}

	isTerminal := opts.IO.IsStdoutTTY()

	if isTerminal {
		cs := opts.IO.ColorScheme()
		successMsg := fmt.Sprintf("\n%s Label created.\n", cs.SuccessIcon())

		fmt.Fprintf(opts.IO.ErrOut, successMsg)
	}

	return nil
}

type CreateLabelRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
}

func createLabel(opts *CreateOptions, client *api.Client, repo ghrepo.Interface) error {
	path := fmt.Sprintf("repos/%s/%s/labels", repo.RepoOwner(), repo.RepoName())

	body := CreateLabelRequest{
		Name:        opts.Name,
		Description: opts.Description,
		Color:       opts.Color,
	}

	requestByte, err := json.Marshal(body)
	if err != nil {
		return err
	}
	requestBody := bytes.NewReader(requestByte)

	result := shared.Label{}

	err = client.REST(repo.RepoHost(), "POST", path, requestBody, &result)
	if err != nil {
		return err
	}
	return nil
}
