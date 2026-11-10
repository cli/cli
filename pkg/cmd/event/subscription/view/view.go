// Package view implements the gh event subscription view command.
package view

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/event/client"
	"github.com/cli/cli/v2/pkg/cmd/event/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	IO          *iostreams.IOStreams
	Config      func() (gh.Config, error)
	EventClient func(host string) (client.EventClient, error)
	Exporter    cmdutil.Exporter
	Now         func() time.Time

	ID string
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:          f.IOStreams,
		Config:      f.Config,
		EventClient: shared.EventClientFunc(f),
		Now:         time.Now,
	}

	cmd := &cobra.Command{
		Use:   "view <id>",
		Short: "View an event subscription (preview)",
		Example: heredoc.Doc(`
			# View a subscription
			$ gh event subscription view sub_123

			# Print when a subscription expires
			$ gh event subscription view sub_123 --json expiresAt --jq .expiresAt
		`),
		Args: cmdutil.ExactArgs(1, "subscription ID required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ID = args[0]

			if runF != nil {
				return runF(opts)
			}
			return viewRun(cmd.Context(), opts)
		},
	}

	cmdutil.AddJSONFlags(cmd, &opts.Exporter, client.SubscriptionFields)
	return cmd
}

func viewRun(ctx context.Context, opts *ViewOptions) error {
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
	sub, err := c.GetSubscription(ctx, opts.ID)
	opts.IO.StopProgressIndicator()
	if err != nil {
		if errors.Is(err, client.ErrSubscriptionNotFound) {
			return shared.SubscriptionNotFoundError(opts.ID)
		}
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, sub)
	}

	out := opts.IO.Out
	if !opts.IO.IsStdoutTTY() {
		fmt.Fprintf(out, "id:\t%s\n", sub.ID)
		fmt.Fprintf(out, "resource:\t%s\n", sub.ResourceURL)
		fmt.Fprintf(out, "events:\t%s\n", strings.Join(sub.EventTypes, ","))
		fmt.Fprintf(out, "created:\t%s\n", sub.CreatedAt.Format(time.RFC3339))
		fmt.Fprintf(out, "expires:\t%s\n", sub.ExpiresAt.Format(time.RFC3339))
		return nil
	}

	cs := opts.IO.ColorScheme()
	now := opts.Now()
	fmt.Fprintln(out, cs.Bold(sub.ID))
	fmt.Fprintln(out, cs.Muted(fmt.Sprintf("Created %s • Expires %s",
		text.FuzzyAgo(now, sub.CreatedAt), text.FuzzyIn(now, sub.ExpiresAt))))
	fmt.Fprintln(out)
	fmt.Fprintln(out, cs.Bold("Resource"))
	fmt.Fprintln(out, sub.ResourceURL)
	fmt.Fprintln(out)
	fmt.Fprintln(out, cs.Bold("Events"))
	width := 0
	for _, name := range sub.EventTypes {
		width = max(width, len(name))
	}
	for _, name := range sub.EventTypes {
		desc := shared.EventTypeDescription(name)
		if desc == "" {
			fmt.Fprintf(out, "• %s\n", cs.Cyan(name))
			continue
		}
		fmt.Fprintf(out, "• %s%s  %s\n", cs.Cyan(name), strings.Repeat(" ", width-len(name)), cs.Muted(desc))
	}
	return nil
}
