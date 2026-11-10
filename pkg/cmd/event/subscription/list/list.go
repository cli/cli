// Package list implements the gh event subscription list command.
package list

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/event/client"
	"github.com/cli/cli/v2/pkg/cmd/event/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	IO          *iostreams.IOStreams
	Config      func() (gh.Config, error)
	EventClient func(host string) (client.EventClient, error)
	Exporter    cmdutil.Exporter
	Now         func() time.Time

	Limit int
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:          f.IOStreams,
		Config:      f.Config,
		EventClient: shared.EventClientFunc(f),
		Now:         time.Now,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List event subscriptions (preview)",
		Example: heredoc.Doc(`
			# List your subscriptions
			$ gh event subscription list

			# Print the ID and resource of each subscription
			$ gh event subscription list --json id,resourceUrl --jq '.[] | [.id, .resourceUrl] | @tsv'
		`),
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Limit < 1 {
				return cmdutil.FlagErrorf("invalid limit: %v", opts.Limit)
			}

			if runF != nil {
				return runF(opts)
			}
			return listRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of subscriptions to list")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, client.SubscriptionFields)
	return cmd
}

func listRun(ctx context.Context, opts *ListOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	host, _ := cfg.Authentication().DefaultHost()

	c, err := opts.EventClient(host)
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicator()
	subs, err := c.ListSubscriptions(ctx, opts.Limit)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("failed to list subscriptions: %w", err)
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, subs)
	}

	if len(subs) == 0 {
		return cmdutil.NewNoResultsError("no event subscriptions")
	}

	isTTY := opts.IO.IsStdoutTTY()
	if isTTY {
		fmt.Fprintf(opts.IO.Out, "\n%s\n\n", listHeader(len(subs), opts.Limit))
	}
	cs := opts.IO.ColorScheme()
	now := opts.Now()
	tp := tableprinter.New(opts.IO, tableprinter.WithHeader("ID", "RESOURCE", "EVENTS", "EXPIRES"))
	for _, sub := range subs {
		tp.AddField(sub.ID, tableprinter.WithColor(cs.Cyan))
		tp.AddField(sub.ResourceURL)
		if isTTY {
			tp.AddField(strings.Join(sub.EventTypes, ", "))
			tp.AddField(text.FuzzyIn(now, sub.ExpiresAt), tableprinter.WithColor(cs.Muted))
		} else {
			tp.AddField(strings.Join(sub.EventTypes, ","))
			tp.AddField(sub.ExpiresAt.Format(time.RFC3339))
		}
		tp.EndRow()
	}
	return tp.Render()
}

// listHeader describes the listed subscriptions. The API reports no total, so a full page may not be all of them.
func listHeader(count, limit int) string {
	if count < limit {
		return fmt.Sprintf("Showing %s", text.Pluralize(count, "event subscription"))
	}
	return fmt.Sprintf("Showing the %s", text.Pluralize(count, "most recent event subscription"))
}
