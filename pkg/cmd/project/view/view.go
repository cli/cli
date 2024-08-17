package view

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/client"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/markdown"
	"github.com/spf13/cobra"
)

type viewOpts struct {
	web      bool
	owner    string
	number   int32
	exporter cmdutil.Exporter
}

type viewConfig struct {
	client    *queries.Client
	opts      viewOpts
	io        *iostreams.IOStreams
	URLOpener func(string) error
}

func NewCmdView(f *cmdutil.Factory, runF func(config viewConfig) error) *cobra.Command {
	opts := viewOpts{}
	viewCmd := &cobra.Command{
		Short: "View a project",
		Use:   "view [<number>]",
		Example: heredoc.Doc(`
			# view the current user's project "1"
			gh project view 1

			# open user monalisa's project "1" in the browser
			gh project view 1 --owner monalisa --web
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client.New(f)
			if err != nil {
				return err
			}

			URLOpener := func(url string) error {
				return f.Browser.Browse(url)
			}

			if len(args) == 1 {
				num, err := strconv.ParseInt(args[0], 10, 32)
				if err != nil {
					return cmdutil.FlagErrorf("invalid number: %v", args[0])
				}
				opts.number = int32(num)
			}

			config := viewConfig{
				client:    client,
				opts:      opts,
				io:        f.IOStreams,
				URLOpener: URLOpener,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runView(config)
		},
	}

	viewCmd.Flags().StringVar(&opts.owner, "owner", "", "Login of the owner. Use \"@me\" for the current user.")
	viewCmd.Flags().BoolVarP(&opts.web, "web", "w", false, "Open a project in the browser")
	cmdutil.AddFormatFlags(viewCmd, &opts.exporter)

	return viewCmd
}

func runView(config viewConfig) error {
	canPrompt := config.io.CanPrompt()
	owner, err := config.client.NewOwner(canPrompt, config.opts.owner)
	if err != nil {
		return err
	}

	project, err := config.client.NewProject(canPrompt, owner, config.opts.number, true)
	if err != nil {
		return err
	}

	if config.opts.web {
		return config.URLOpener(project.URL)
	}

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, *project)
	}

	return printResults(config, project)
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

	out, err := markdown.Render(sb.String(),
		markdown.WithTheme(config.io.TerminalTheme()),
		markdown.WithWrap(config.io.TerminalWidth()))

	if err != nil {
		return err
	}
	_, err = fmt.Fprint(config.io.Out, out)
	return err
}
