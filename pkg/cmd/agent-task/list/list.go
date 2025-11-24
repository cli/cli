package list

import (
	"context"
	"fmt"
	"time"

	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/agent-task/capi"
	"github.com/cli/cli/v2/pkg/cmd/agent-task/shared"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

const defaultLimit = 30

// ListOptions are the options for the list command
type ListOptions struct {
	IO         *iostreams.IOStreams
	Limit      int
	CapiClient func() (capi.CapiClient, error)
	Web        bool
	Browser    browser.Browser
}

// NewCmdList creates the list command
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		CapiClient: shared.CapiClientFunc(f),
		Limit:      defaultLimit,
		Browser:    f.Browser,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agent tasks (preview)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Limit < 1 {
				return cmdutil.FlagErrorf("invalid limit: %v", opts.Limit)
			}
			if runF != nil {
				return runF(opts)
			}
			return listRun(opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", defaultLimit, "Maximum number of agent tasks to fetch")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open agent tasks in the browser")

	return cmd
}

func listRun(opts *ListOptions) error {
	if opts.Web {
		webURL := capi.AgentsHomeURL
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(webURL))
		}
		return opts.Browser.Browse(webURL)
	}

	if opts.Limit <= 0 {
		opts.Limit = defaultLimit
	}

	capiClient, err := opts.CapiClient()
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicatorWithLabel("Fetching agent tasks...")
	defer opts.IO.StopProgressIndicator()
	var sessions []*capi.Session
	ctx := context.Background()

	sessions, err = capiClient.ListLatestSessionsForViewer(ctx, opts.Limit)
	if err != nil {
		return err
	}

	opts.IO.StopProgressIndicator()

	if len(sessions) == 0 {
		return cmdutil.NewNoResultsError("no agent tasks found")
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "error starting pager: %v\n", err)
	}

	if opts.IO.IsStdoutTTY() {
		count := len(sessions)
		header := fmt.Sprintf("Showing %s", text.Pluralize(count, "session"))
		fmt.Fprintf(opts.IO.Out, "%s\n\n", header)
	}

	cs := opts.IO.ColorScheme()
	tp := tableprinter.New(opts.IO, tableprinter.WithHeader("Session Name", "Pull Request", "Repo", "Session State", "Created"))
	for _, s := range sessions {
		if s.ResourceType != "pull" || s.PullRequest == nil || s.PullRequest.Repository == nil {
			// Skip these sessions in case they happen, for now.
			continue
		}

		pr := fmt.Sprintf("#%d", s.PullRequest.Number)
		repo := s.PullRequest.Repository.NameWithOwner

		// Name
		tp.AddField(s.Name)
		if tp.IsTTY() {
			tp.AddField(pr, tableprinter.WithColor(cs.ColorFromString(prShared.ColorForPRState(*s.PullRequest))))
		} else {
			tp.AddField(pr)
		}

		// Repo
		tp.AddField(repo, tableprinter.WithColor(cs.Muted))

		// State
		if tp.IsTTY() {
			tp.AddField(shared.SessionStateString(s.State), tableprinter.WithColor(shared.ColorFuncForSessionState(*s, cs)))
		} else {
			tp.AddField(shared.SessionStateString(s.State))
		}

		// Created
		if tp.IsTTY() {
			tp.AddTimeField(time.Now(), s.CreatedAt, cs.Muted)
		} else {
			tp.AddField(s.CreatedAt.Format(time.RFC3339))
		}

		tp.EndRow()
	}

	if err := tp.Render(); err != nil {
		return err
	}

	return nil
}
