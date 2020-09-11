package list

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)

	Limit      int
	Visibility string // all, secret, public
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your gists",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Limit < 1 {
				return &cmdutil.FlagError{Err: fmt.Errorf("invalid limit: %v", opts.Limit)}
			}

			pub := cmd.Flags().Changed("public")
			priv := cmd.Flags().Changed("private")

			opts.Visibility = "all"
			if pub && !priv {
				opts.Visibility = "public"
			} else if priv && !pub {
				opts.Visibility = "private"
			}

			if runF != nil {
				return runF(opts)
			}

			return listRun(opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 10, "Maximum number of gists to fetch")
	cmd.Flags().Bool("public", false, "Show only public gists")
	cmd.Flags().Bool("private", false, "Show only private gists")

	return cmd
}

func listRun(opts *ListOptions) error {
	client, err := opts.HttpClient()
	if err != nil {
		return err
	}

	gists, err := listGists(client, ghinstance.OverridableDefault(), opts.Limit, opts.Visibility)
	if err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()

	tp := utils.NewTablePrinter(opts.IO)

	for _, gist := range gists {
		// TODO i was getting confusing results with table printer's truncation
		description := gist.Description
		if len(description) > 40 {
			description = description[0:37] + "..."
		}

		tp.AddField(description, nil, cs.Bold)
		tp.AddField(gist.HTMLURL, nil, nil)
		tp.EndRow()
	}

	return tp.Render()
}
