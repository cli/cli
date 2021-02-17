package list

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*http.Client, error)
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		HTTPClient: f.HttpClient,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lists SSH keys in your GitHub account",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return listRun(opts)
		},
	}

	return cmd
}

func listRun(opts *ListOptions) error {
	apiClient, err := opts.HTTPClient()
	if err != nil {
		return err
	}

	sshKeys, err := userKeys(apiClient, "")
	if err != nil {
		if errors.Is(err, scopesError) {
			cs := opts.IO.ColorScheme()
			fmt.Fprint(opts.IO.ErrOut, "Error: insufficient OAuth scopes to list SSH keys\n")
			fmt.Fprintf(opts.IO.ErrOut, "Run the following to grant scopes: %s\n", cs.Bold("gh auth refresh -s read:public_key"))
			return cmdutil.SilentError
		}
		return err
	}

	if len(sshKeys) == 0 {
		fmt.Fprintln(opts.IO.ErrOut, "No SSH keys present in GitHub account.")
		return cmdutil.SilentError
	}

	t := utils.NewTablePrinter(opts.IO)
	cs := opts.IO.ColorScheme()
	now := time.Now()

	for _, sshKey := range sshKeys {
		t.AddField(sshKey.Title, nil, nil)
		t.AddField(sshKey.Key, truncateMiddle, nil)

		createdAt := sshKey.CreatedAt.Format(time.RFC3339)
		if t.IsTTY() {
			createdAt = utils.FuzzyAgoAbbr(now, sshKey.CreatedAt)
		}
		t.AddField(createdAt, nil, cs.Gray)
		t.EndRow()
	}

	return t.Render()
}

func truncateMiddle(maxWidth int, t string) string {
	if len(t) <= maxWidth {
		return t
	}

	ellipsis := "..."
	if maxWidth < len(ellipsis)+2 {
		return t[0:maxWidth]
	}

	halfWidth := (maxWidth - len(ellipsis)) / 2
	return t[0:halfWidth] + ellipsis + t[len(t)-halfWidth:]
}
