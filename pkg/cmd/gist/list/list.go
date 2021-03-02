package list

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/pkg/cmd/gist/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/text"
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

	var flagPublic bool
	var flagSecret bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your gists",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Limit < 1 {
				return &cmdutil.FlagError{Err: fmt.Errorf("invalid limit: %v", opts.Limit)}
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

	gists, err := shared.ListGists(client, ghinstance.OverridableDefault(), opts.Limit, opts.Visibility)
	if err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()

	tp := utils.NewTablePrinter(opts.IO)

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

		gistTime := gist.UpdatedAt.Format(time.RFC3339)
		if tp.IsTTY() {
			gistTime = utils.FuzzyAgo(time.Since(gist.UpdatedAt))
		}

		tp.AddField(gist.ID, nil, nil)
		tp.AddField(text.ReplaceExcessiveWhitespace(description), nil, cs.Bold)
		tp.AddField(utils.Pluralize(fileCount, "file"), nil, nil)
		tp.AddField(visibility, nil, visColor)
		tp.AddField(gistTime, nil, cs.Gray)
		tp.EndRow()
	}

	return tp.Render()
}
