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
	limit    int
	web      bool
	owner    string
	closed   bool
	exporter cmdutil.Exporter
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
	cmdutil.AddFormatFlags(listCmd, &opts.exporter)
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

	projects, err := config.client.Projects(config.opts.owner, owner.Type, config.opts.limit, false)
	if err != nil {
		return err
	}
	projects = filterProjects(projects, config)

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, projects)
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

func filterProjects(nodes queries.Projects, config listConfig) queries.Projects {
	filtered := queries.Projects{
		Nodes:      make([]queries.Project, 0, len(nodes.Nodes)),
		TotalCount: nodes.TotalCount,
	}
	for _, project := range nodes.Nodes {
		if !config.opts.closed && project.Closed {
			continue
		}
		filtered.Nodes = append(filtered.Nodes, project)
	}
	return filtered
}

func printResults(config listConfig, projects queries.Projects, owner string) error {
	if len(projects.Nodes) == 0 {
		return cmdutil.NewNoResultsError(fmt.Sprintf("No projects found for %s", owner))
	}

	tp := tableprinter.New(config.io, tableprinter.WithHeader("Number", "Title", "State", "ID"))

	cs := config.io.ColorScheme()
	for _, p := range projects.Nodes {
		tp.AddField(
			strconv.Itoa(int(p.Number)),
			tableprinter.WithTruncate(nil),
		)
		tp.AddField(p.Title)
		tp.AddField(
			format.ProjectState(p),
			tableprinter.WithColor(cs.ColorFromString(format.ColorForProjectState(p))),
		)
		tp.AddField(p.ID, tableprinter.WithTruncate(nil))
		tp.EndRow()
	}

	return tp.Render()
}
