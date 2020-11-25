package project

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ProjectOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	Selector string
}

func NewCmdProject(f *cmdutil.Factory, runF func(*ProjectOptions) error) *cobra.Command {
	opts := ProjectOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:    "project [<project id>]",
		Short:  "Interact with a GitHub project",
		Long:   "Enter an interactive UI for viewing and modifying a GitHub project",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if runF != nil {
				return runF(&opts)
			}
			return projectRun(&opts)
		},
	}

	return cmd
}

func projectRun(opts *ProjectOptions) error {
	// TODO interactively ask which project they want since these IDs are not easy to get
	projectID := "3514315"

	c, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(c)

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	project, err := getProject(client, baseRepo, projectID)
	if err != nil {
		return err
	}

	fmt.Printf("DBG %#v\n", project)

	return nil
}

type Project struct {
	Name string
}

func getProject(client *api.Client, baseRepo ghrepo.Interface, projectID string) (*Project, error) {
	data, err := client.GetProject(baseRepo, projectID)
	if err != nil {
		return nil, err
	}

	project := &Project{}

	err = json.Unmarshal(data, project)
	if err != nil {
		return nil, err
	}

	return project, nil
}
