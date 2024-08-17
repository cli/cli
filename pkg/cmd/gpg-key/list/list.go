package list

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/tableprinter"
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
		Short:   "Lists GPG keys in your GitHub account",
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

	gpgKeys, err := userKeys(apiClient, host, "")
	if err != nil {
		if errors.Is(err, errScopes) {
			cs := opts.IO.ColorScheme()
			fmt.Fprint(opts.IO.ErrOut, "Error: insufficient OAuth scopes to list GPG keys\n")
			fmt.Fprintf(opts.IO.ErrOut, "Run the following to grant scopes: %s\n", cs.Bold("gh auth refresh -s read:gpg_key"))
			return cmdutil.SilentError
		}
		return err
	}

	if len(gpgKeys) == 0 {
		return cmdutil.NewNoResultsError("no GPG keys present in the GitHub account")
	}

	t := tableprinter.New(opts.IO, tableprinter.WithHeader("EMAIL", "KEY ID", "PUBLIC KEY", "ADDED", "EXPIRES"))
	cs := opts.IO.ColorScheme()
	now := time.Now()

	for _, gpgKey := range gpgKeys {
		t.AddField(gpgKey.Emails.String())
		t.AddField(gpgKey.KeyID)
		t.AddField(gpgKey.PublicKey, tableprinter.WithTruncate(truncateMiddle))
		t.AddTimeField(now, gpgKey.CreatedAt, cs.Gray)
		expiresAt := gpgKey.ExpiresAt.Format(time.RFC3339)
		if t.IsTTY() {
			if gpgKey.ExpiresAt.IsZero() {
				expiresAt = "Never"
			} else {
				expiresAt = gpgKey.ExpiresAt.Format("2006-01-02")
			}
		}
		t.AddField(expiresAt, tableprinter.WithColor(cs.Gray))
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
