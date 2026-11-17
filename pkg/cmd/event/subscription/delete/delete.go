// Package delete implements the gh event subscription delete command.
package delete

import (
	"context"
	"errors"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmd/event/client"
	"github.com/cli/cli/v2/pkg/cmd/event/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DeleteOptions struct {
	IO          *iostreams.IOStreams
	Config      func() (gh.Config, error)
	EventClient func(host string) (client.EventClient, error)

	ID string
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := &DeleteOptions{
		IO:          f.IOStreams,
		Config:      f.Config,
		EventClient: shared.EventClientFunc(f),
	}

	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete an event subscription (preview)",
		Long: heredoc.Doc(`
			Delete an event subscription before it expires, which frees a place under the limit of
			subscriptions of your account.

			Event streams that are already open on the subscription are not guaranteed to close.
		`),
		Example: heredoc.Doc(`
			# Delete a subscription
			$ gh event subscription delete sub_123
		`),
		Args: cmdutil.ExactArgs(1, "subscription ID required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ID = args[0]

			if runF != nil {
				return runF(opts)
			}
			return deleteRun(cmd.Context(), opts)
		},
	}

	return cmd
}

func deleteRun(ctx context.Context, opts *DeleteOptions) error {
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
	err = c.DeleteSubscription(ctx, opts.ID)
	opts.IO.StopProgressIndicator()
	if err != nil {
		if errors.Is(err, client.ErrSubscriptionNotFound) {
			return shared.SubscriptionNotFoundError(opts.ID)
		}
		return fmt.Errorf("failed to delete subscription: %w", err)
	}

	if opts.IO.IsStderrTTY() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.ErrOut, "%s Deleted subscription %s\n", cs.SuccessIconWithColor(cs.Red), opts.ID)
	}
	return nil
}
