package view

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/cli/cli/v2/pkg/cmd/project/shared/format"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"

	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/spf13/cobra"
)

type viewOpts struct {
	web       bool
	userOwner string
	orgOwner  string
	number    int
	format    string
}

type viewConfig struct {
	tp        *tableprinter.TablePrinter
	client    *api.GraphQLClient
	opts      viewOpts
	URLOpener func(string) error
}

func NewCmdView(f *cmdutil.Factory, runF func(config viewConfig) error) *cobra.Command {
	opts := viewOpts{}
	viewCmd := &cobra.Command{
		Short: "View a project",
		Use:   "view [<number>]",
		Example: `
# view the current user's project 1
gh project view 1

# open user monalisa's project 1 in the browser
gh project view 1 --user monalisa --web

# view org github's project 1 including closed projects
gh project view 1 --org github --closed

# add --format=json to output in JSON format
`,
		Args: cobra.MaximumNArgs(1),
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

			if len(args) == 1 {
				opts.number, err = strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("invalid number: %v", args[0])
				}
			}

			t := tableprinter.New(f.IOStreams)
			config := viewConfig{
				tp:        t,
				client:    client,
				opts:      opts,
				URLOpener: URLOpener,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runView(config)
		},
	}

	viewCmd.Flags().StringVar(&opts.userOwner, "user", "", "Login of the user owner. Use \"@me\" for the current user.")
	viewCmd.Flags().StringVar(&opts.orgOwner, "org", "", "Login of the organization owner.")
	viewCmd.Flags().BoolVarP(&opts.web, "web", "w", false, "Open project in the browser.")
	viewCmd.Flags().StringVar(&opts.format, "format", "", "Output format, must be 'json'.")

	return viewCmd
}

func runView(config viewConfig) error {
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

	if config.opts.format != "" && config.opts.format != "json" {
		return fmt.Errorf("format must be 'json'")
	}

	owner, err := queries.NewOwner(config.client, config.opts.userOwner, config.opts.orgOwner)
	if err != nil {
		return err
	}

	project, err := queries.NewProject(config.client, owner, config.opts.number, true)
	if err != nil {
		return err
	}

	if config.opts.format == "json" {
		return printJSON(config, *project)
	}

	return printResults(config, project)
}

func buildURL(config viewConfig) (string, error) {
	var url string
	if config.opts.userOwner == "@me" {
		login, err := queries.ViewerLoginName(config.client)
		if err != nil {
			return "", err
		}
		url = fmt.Sprintf("https://github.com/users/%s/projects/%d", login, config.opts.number)
	} else if config.opts.userOwner != "" {
		url = fmt.Sprintf("https://github.com/users/%s/projects/%d", config.opts.userOwner, config.opts.number)
	} else if config.opts.orgOwner != "" {
		url = fmt.Sprintf("https://github.com/orgs/%s/projects/%d", config.opts.orgOwner, config.opts.number)
	}

	return url, nil
}

func printResults(config viewConfig, project *queries.Project) error {

	var sb strings.Builder
	sb.WriteString("# Title\n")
	sb.WriteString(project.Title)
	sb.WriteString("\n")

	sb.WriteString("## Description\n")
	if project.ShortDescription == "" {
		sb.WriteString(" -- ")
	} else {
		sb.WriteString(project.ShortDescription)
	}
	sb.WriteString("\n")

	sb.WriteString("## Visibility\n")
	if project.Public {
		sb.WriteString("Public")
	} else {
		sb.WriteString("Private")
	}
	sb.WriteString("\n")

	sb.WriteString("## URL\n")
	sb.WriteString(project.URL)
	sb.WriteString("\n")

	sb.WriteString("## Item count\n")
	sb.WriteString(fmt.Sprintf("%d", project.Items.TotalCount))
	sb.WriteString("\n")

	sb.WriteString("## Readme\n")
	if project.Readme == "" {
		sb.WriteString(" -- ")
	} else {
		sb.WriteString(project.Readme)
	}
	sb.WriteString("\n")

	sb.WriteString("## Field Name (Field Type)\n")
	for _, f := range project.Fields.Nodes {
		sb.WriteString(fmt.Sprintf("%s (%s)\n\n", f.Name(), f.Type()))
	}

	// TODO: respect the glamour env var if set
	out, err := glamour.Render(sb.String(), "dark")
	if err != nil {
		return err
	}
	fmt.Println(out)
	config.tp.AddField(out)
	return config.tp.Render()
}

func printJSON(config viewConfig, project queries.Project) error {
	b, err := format.JSONProject(project)
	if err != nil {
		return err
	}
	config.tp.AddField(string(b))
	return config.tp.Render()
}
