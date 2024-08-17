package list

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/ssh-key/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (gh.Config, error)
	HTTPClient func() (*http.Client, error)
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HTTPClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "Lists SSH keys in your GitHub account",
		Aliases: []string{"ls"},
		Args:    cobra.ExactArgs(0),
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

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host, _ := cfg.Authentication().DefaultHost()
	sshAuthKeys, authKeyErr := shared.UserKeys(apiClient, host, "")
	if authKeyErr != nil {
		printError(opts.IO.ErrOut, authKeyErr)
	}

	sshSigningKeys, signKeyErr := shared.UserSigningKeys(apiClient, host, "")
	if signKeyErr != nil {
		printError(opts.IO.ErrOut, signKeyErr)
	}

	if authKeyErr != nil && signKeyErr != nil {
		return cmdutil.SilentError
	}

	sshKeys := append(sshAuthKeys, sshSigningKeys...)

	if len(sshKeys) == 0 {
		return cmdutil.NewNoResultsError("no SSH keys present in the GitHub account")
	}

	t := tableprinter.New(opts.IO, tableprinter.WithHeader("TITLE", "ID", "KEY", "TYPE", "ADDED"))
	cs := opts.IO.ColorScheme()
	now := time.Now()

	for _, sshKey := range sshKeys {
		id := strconv.Itoa(sshKey.ID)
		if t.IsTTY() {
			t.AddField(sshKey.Title)
			t.AddField(id)
			t.AddField(sshKey.Key, tableprinter.WithTruncate(truncateMiddle))
			t.AddField(sshKey.Type)
			t.AddTimeField(now, sshKey.CreatedAt, cs.Gray)
		} else {
			t.AddField(sshKey.Title)
			t.AddField(sshKey.Key)
			t.AddTimeField(now, sshKey.CreatedAt, cs.Gray)
			t.AddField(id)
			t.AddField(sshKey.Type)
		}
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
	remainder := (maxWidth - len(ellipsis)) % 2
	return t[0:halfWidth+remainder] + ellipsis + t[len(t)-halfWidth:]
}

func printError(w io.Writer, err error) {
	fmt.Fprintln(w, "warning: ", err)
	var httpErr api.HTTPError
	if errors.As(err, &httpErr) {
		if msg := httpErr.ScopesSuggestion(); msg != "" {
			fmt.Fprintln(w, msg)
		}
	}
}
