package list

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
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

	//nolint:staticcheck // SA1019: utils.NewTablePrinter is deprecated: use internal/tableprinter
	t := utils.NewTablePrinter(opts.IO)
	cs := opts.IO.ColorScheme()
	now := time.Now()

	if t.IsTTY() {
		t.AddField("EMAIL", nil, nil)
		t.AddField("KEY ID", nil, nil)
		t.AddField("PUBLIC KEY", nil, nil)
		t.AddField("ADDED", nil, nil)
		t.AddField("EXPIRES", nil, nil)
		t.EndRow()
	}

	for _, gpgKey := range gpgKeys {
		t.AddField(gpgKey.Emails.String(), nil, nil)
		t.AddField(gpgKey.KeyID, nil, nil)
		t.AddField(gpgKey.PublicKey, truncateMiddle, nil)

		createdAt := gpgKey.CreatedAt.Format(time.RFC3339)
		if t.IsTTY() {
			createdAt = text.FuzzyAgoAbbr(now, gpgKey.CreatedAt)
		}
		t.AddField(createdAt, nil, cs.Gray)

		expiresAt := gpgKey.ExpiresAt.Format(time.RFC3339)
		if t.IsTTY() {
			if gpgKey.ExpiresAt.IsZero() {
				expiresAt = "Never"
			} else {
				expiresAt = gpgKey.ExpiresAt.Format("2006-01-02")
			}
		}
		t.AddField(expiresAt, nil, cs.Gray)

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
