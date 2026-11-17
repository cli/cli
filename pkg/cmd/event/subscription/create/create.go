// Package create implements the gh event subscription create command.
package create

import (
	"context"
	"errors"
	"fmt"
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

type CreateOptions struct {
	IO          *iostreams.IOStreams
	Config      func() (gh.Config, error)
	EventClient func(host string) (client.EventClient, error)
	Exporter    cmdutil.Exporter
	Now         func() time.Time

	Resource string
	Events   []string
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:          f.IOStreams,
		Config:      f.Config,
		EventClient: shared.EventClientFunc(f),
		Now:         time.Now,
	}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an event subscription (preview)",
		Long: heredoc.Docf(`
			Create a subscription to events of the given types on a resource, without listening to it.

			Pass the resource URL with %[1]s--resource%[1]s and the event types with %[1]s--event%[1]s, repeating the flag
			or separating types with commas:

			%[2]s

			Creating a subscription is idempotent: if an active subscription for the same resource and
			event types exists, it is returned instead.

			Once successful, the command prints the subscription ID to stdout. Use
			%[1]sgh event listen --subscription%[1]s to receive its events. To create a subscription and listen
			to it in one step, use %[1]sgh event listen --resource%[1]s instead.
		`, "`", shared.EventTypesHelp()),
		Example: heredoc.Doc(`
			# Subscribe to reviews and state changes of a pull request
			$ gh event subscription create --resource https://github.com/OWNER/REPO/pull/123 --event com.github.pull_request.review.submitted.v1,com.github.pull_request.state.changed.v1
		`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Resource == "" {
				return cmdutil.FlagErrorf("`--resource` is required")
			}
			if len(opts.Events) == 0 {
				return cmdutil.FlagErrorf("`--event` is required")
			}

			if runF != nil {
				return runF(opts)
			}
			return createRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.Resource, "resource", "", "URL of the resource to subscribe to")
	cmd.Flags().StringSliceVar(&opts.Events, "event", nil, "Event `type` to subscribe to; repeat or separate with commas for more than one")
	shared.RegisterEventTypesCompletion(cmd, "event")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, client.SubscriptionFields)
	return cmd
}

func createRun(ctx context.Context, opts *CreateOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	host, _ := cfg.Authentication().DefaultHost()

	c, err := opts.EventClient(shared.ResourceHost(opts.Resource, host))
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicator()
	sub, created, err := c.CreateSubscription(ctx, opts.Resource, shared.NormalizeEventTypes(opts.Events))
	opts.IO.StopProgressIndicator()
	if err != nil {
		if errors.Is(err, client.ErrResourceNotFound) {
			return fmt.Errorf("could not subscribe to %s; the resource was not found, or you cannot access it: %w",
				opts.Resource, err)
		}
		return shared.SubscriptionLimitError(fmt.Errorf("failed to create subscription: %w", err))
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, sub)
	}

	if _, err := fmt.Fprintln(opts.IO.Out, sub.ID); err != nil {
		return err
	}

	if !opts.IO.IsStderrTTY() {
		return nil
	}
	action := "Created subscription"
	if !created {
		action = "Reused existing subscription"
	}
	cs := opts.IO.ColorScheme()
	_, err = fmt.Fprintf(opts.IO.ErrOut, "%s %s %s, which expires %s\n",
		cs.SuccessIcon(), action, cs.Bold(sub.ID), text.FuzzyIn(opts.Now(), sub.ExpiresAt))
	return err
}
