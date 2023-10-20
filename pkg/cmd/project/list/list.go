package list

import (
	"fmt"
	"strconv"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/client"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/format"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type listOpts struct {
	limit  int
	web    bool
	owner  string
	closed bool
	format string
}

type listConfig struct {
	client    *queries.Client
	opts      listOpts
	URLOpener func(string) error
	io        *iostreams.IOStreams
}

func NewCmdList(f *cmdutil.Factory, runF func(config listConfig) error) *cobra.Command {
	opts := listOpts{}
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List the projects for an owner",
		Example: heredoc.Doc(`
			# list the current user's projects
			gh project list

			# list the projects for org github including closed projects
			gh project list --owner github --closed
		`),
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client.New(f)
			if err != nil {
				return err
			}

			URLOpener := func(url string) error {
				return f.Browser.Browse(url)
			}

			config := listConfig{
				client:    client,
				opts:      opts,
				URLOpener: URLOpener,
				io:        f.IOStreams,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runList(config)
		},
	}
	listCmd.Flags().StringVar(&opts.owner, "owner", "", "Login of the owner")
	listCmd.Flags().BoolVarP(&opts.closed, "closed", "", false, "Include closed projects")
	listCmd.Flags().BoolVarP(&opts.web, "web", "w", false, "Open projects list in the browser")
	cmdutil.StringEnumFlag(listCmd, &opts.format, "format", "", "", []string{"json"}, "Output format")
	listCmd.Flags().IntVarP(&opts.limit, "limit", "L", queries.LimitDefault, "Maximum number of projects to fetch")

	return listCmd
}

func runList(config listConfig) error {
	if config.opts.web {
		url, err := buildURL(config)
		if err != nil {
			return err
		}

		if err := config.URLOpener(url); err != nil {
			return err
		}
		return nil
	}

	if config.opts.owner == "" {
		config.opts.owner = "@me"
	}
	canPrompt := config.io.CanPrompt()
	owner, err := config.client.NewOwner(canPrompt, config.opts.owner)
	if err != nil {
		return err
	}

	projects, totalCount, err := config.client.Projects(config.opts.owner, owner.Type, config.opts.limit, false)
	if err != nil {
		return err
	}
	projects = filterProjects(projects, config)

	if config.opts.format == "json" {
		return printJSON(config, projects, totalCount)
	}

	return printResults(config, projects, owner.Login)
}

// TODO: support non-github.com hostnames
func buildURL(config listConfig) (string, error) {
	var url string
	if config.opts.owner == "@me" || config.opts.owner == "" {
		owner, err := config.client.ViewerLoginName()
		if err != nil {
			return "", err
		}
		url = fmt.Sprintf("https://github.com/users/%s/projects", owner)
	} else {
		_, ownerType, err := config.client.OwnerIDAndType(config.opts.owner)
		if err != nil {
			return "", err
		}

		if ownerType == queries.UserOwner {
			url = fmt.Sprintf("https://github.com/users/%s/projects", config.opts.owner)
		} else {
			url = fmt.Sprintf("https://github.com/orgs/%s/projects", config.opts.owner)
		}
	}

	if config.opts.closed {
		return url + "?query=is%3Aclosed", nil
	}
	return url, nil
}

func filterProjects(nodes []queries.Project, config listConfig) []queries.Project {
	projects := make([]queries.Project, 0, len(nodes))
	for _, p := range nodes {
		if !config.opts.closed && p.Closed {
			continue
		}
		projects = append(projects, p)
	}
	return projects
}

func printResults(config listConfig, projects []queries.Project, owner string) error {
	if len(projects) == 0 {
		return cmdutil.NewNoResultsError(fmt.Sprintf("No projects found for %s", owner))
	}

	tp := tableprinter.New(config.io, tableprinter.WithHeader("Number", "Title", "State", "ID"))

	for _, p := range projects {
		tp.AddField(strconv.Itoa(int(p.Number)), tableprinter.WithTruncate(nil))
		tp.AddField(p.Title)
		var state string
		if p.Closed {
			state = "closed"
		} else {
			state = "open"
		}
		tp.AddField(state)
		tp.AddField(p.ID, tableprinter.WithTruncate(nil))
		tp.EndRow()
	}

	return tp.Render()
}

func printJSON(config listConfig, projects []queries.Project, totalCount int) error {
	b, err := format.JSONProjects(projects, totalCount)
	if err != nil {
		return err
	}

	_, err = config.io.Out.Write(b)
	return err
}
