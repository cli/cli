package list

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/gist/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (gh.Config, error)
	HttpClient func() (*http.Client, error)

	Limit      int
	Visibility string // all, secret, public
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
	}

	var flagPublic bool
	var flagSecret bool

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List your gists",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Limit < 1 {
				return cmdutil.FlagErrorf("invalid limit: %v", opts.Limit)
			}

			opts.Visibility = "all"
			if flagSecret {
				opts.Visibility = "secret"
			} else if flagPublic {
				opts.Visibility = "public"
			}

			if runF != nil {
				return runF(opts)
			}
			return listRun(opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 10, "Maximum number of gists to fetch")
	cmd.Flags().BoolVar(&flagPublic, "public", false, "Show only public gists")
	cmd.Flags().BoolVar(&flagSecret, "secret", false, "Show only secret gists")

	return cmd
}

func listRun(opts *ListOptions) error {
	client, err := opts.HttpClient()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host, _ := cfg.Authentication().DefaultHost()

	gists, err := shared.ListGists(client, host, opts.Limit, opts.Visibility)
	if err != nil {
		return err
	}

	if len(gists) == 0 {
		return cmdutil.NewNoResultsError("no gists found")
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	cs := opts.IO.ColorScheme()
	tp := tableprinter.New(opts.IO, tableprinter.WithHeader("ID", "DESCRIPTION", "FILES", "VISIBILITY", "UPDATED"))

	for _, gist := range gists {
		fileCount := len(gist.Files)

		visibility := "public"
		visColor := cs.Green
		if !gist.Public {
			visibility = "secret"
			visColor = cs.Red
		}

		description := gist.Description
		if description == "" {
			for filename := range gist.Files {
				if !strings.HasPrefix(filename, "gistfile") {
					description = filename
					break
				}
			}
		}

		tp.AddField(gist.ID)
		tp.AddField(
			text.RemoveExcessiveWhitespace(description),
			tableprinter.WithColor(cs.Bold),
		)
		tp.AddField(text.Pluralize(fileCount, "file"))
		tp.AddField(visibility, tableprinter.WithColor(visColor))
		tp.AddTimeField(time.Now(), gist.UpdatedAt, cs.Gray)
		tp.EndRow()
	}

	return tp.Render()
}
