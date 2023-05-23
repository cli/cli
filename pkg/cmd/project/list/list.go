package list

import (
	"fmt"

	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/format"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/spf13/cobra"
)

type listOpts struct {
	limit     int
	web       bool
	userOwner string
	orgOwner  string
	closed    bool
	format    string
}

type listConfig struct {
	tp        *tableprinter.TablePrinter
	client    *api.GraphQLClient
	opts      listOpts
	URLOpener func(string) error
}

func NewCmdList(f *cmdutil.Factory, runF func(config listConfig) error) *cobra.Command {
	opts := listOpts{}
	listCmd := &cobra.Command{
		Short: "List the projects for an owner",
		Use:   "list",
		Example: `
# list the current user's projects
gh project list

# open projects for user monalisa in the browser
gh project list --user monalisa --web

# list the projects for org github including closed projects
gh project list --org github --closed
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmdutil.MutuallyExclusive(
				"only one of `--user` or `--org` may be used",
				opts.userOwner != "",
				opts.orgOwner != "",
			); err != nil {
				return err
			}

			client, err := queries.NewClient()
			if err != nil {
				return err
			}

			URLOpener := func(url string) error {
				return f.Browser.Browse(url)
			}
			t := tableprinter.New(f.IOStreams)
			config := listConfig{
				tp:        t,
				client:    client,
				opts:      opts,
				URLOpener: URLOpener,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runList(config)
		},
	}

	listCmd.Flags().StringVar(&opts.userOwner, "user", "", "Login of the user owner")
	listCmd.Flags().StringVar(&opts.orgOwner, "org", "", "Login of the organization owner")
	listCmd.Flags().BoolVarP(&opts.closed, "closed", "", false, "Include closed projects")
	listCmd.Flags().BoolVarP(&opts.web, "web", "w", false, "Open projects list in the browser")
	cmdutil.StringEnumFlag(listCmd, &opts.format, "format", "", "", []string{"json"}, "Output format")
	listCmd.Flags().IntVarP(&opts.limit, "limit", "L", 30, "Maximum number of projects to fetch")

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

	var login string
	var ownerType queries.OwnerType
	if config.opts.userOwner != "" {
		if config.opts.userOwner == "@me" {
			login = "me"
			ownerType = queries.ViewerOwner
		} else {
			login = config.opts.userOwner
			ownerType = queries.UserOwner
		}
	} else if config.opts.orgOwner != "" {
		login = config.opts.orgOwner
		ownerType = queries.OrgOwner
	} else {
		login = "me"
		ownerType = queries.ViewerOwner
	}

	projects, totalCount, err := queries.Projects(config.client, login, ownerType, config.opts.limit, false)
	if err != nil {
		return err
	}
	projects = filterProjects(projects, config)

	if config.opts.format == "json" {
		return printJSON(config, projects, totalCount)
	}

	return printResults(config, projects, login)
}

func buildURL(config listConfig) (string, error) {
	var url string
	if config.opts.userOwner != "" {
		url = fmt.Sprintf("https://github.com/users/%s/projects", config.opts.userOwner)
	} else if config.opts.orgOwner != "" {
		url = fmt.Sprintf("https://github.com/orgs/%s/projects", config.opts.orgOwner)
	} else {
		login, err := queries.ViewerLoginName(config.client)
		if err != nil {
			return "", err
		}
		url = fmt.Sprintf("https://github.com/users/%s/projects", login)
	}

	if config.opts.closed {
		url = fmt.Sprintf("%s?query=is%%3Aclosed", url)
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

func printResults(config listConfig, projects []queries.Project, login string) error {
	if len(projects) == 0 {
		return cmdutil.NewNoResultsError(fmt.Sprintf("No projects found for %s", login))
	}

	config.tp.AddField("Title")
	config.tp.AddField("Description")
	config.tp.AddField("URL")
	if config.opts.closed {
		config.tp.AddField("State")
	}
	config.tp.AddField("ID")
	config.tp.EndRow()

	for _, p := range projects {
		config.tp.AddField(p.Title)
		if p.ShortDescription == "" {
			config.tp.AddField(" - ")
		} else {
			config.tp.AddField(p.ShortDescription)
		}
		config.tp.AddField(p.URL)
		if config.opts.closed {
			var state string
			if p.Closed {
				state = "closed"
			} else {
				state = "open"
			}
			config.tp.AddField(state)
		}
		config.tp.AddField(p.ID)
		config.tp.EndRow()
	}

	return config.tp.Render()
}

func printJSON(config listConfig, projects []queries.Project, totalCount int) error {
	b, err := format.JSONProjects(projects, totalCount)
	if err != nil {
		return err
	}
	config.tp.AddField(string(b))
	return config.tp.Render()
}
