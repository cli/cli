package list

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
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
		Use:   "list",
		Short: "Lists GPG keys in your GitHub account",
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

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host, err := cfg.DefaultHost()
	if err != nil {
		return err
	}

	gpgKeys, err := userKeys(apiClient, host, "")
	if err != nil {
		if errors.Is(err, scopesError) {
			cs := opts.IO.ColorScheme()
			fmt.Fprint(opts.IO.ErrOut, "Error: insufficient OAuth scopes to list GPG keys\n")
			fmt.Fprintf(opts.IO.ErrOut, "Run the following to grant scopes: %s\n", cs.Bold("gh auth refresh -s read:gpg_key"))
			return cmdutil.SilentError
		}
		return err
	}

	if len(gpgKeys) == 0 {
		fmt.Fprintln(opts.IO.ErrOut, "No GPG keys present in GitHub account.")
		return cmdutil.SilentError
	}

	t := utils.NewTablePrinter(opts.IO)
	cs := opts.IO.ColorScheme()
	now := time.Now()

	for _, gpgKey := range gpgKeys {
		t.AddField(gpgKey.Emails.String(), nil, nil)

		t.AddField(gpgKey.KeyId, nil, nil)

		createdAt := gpgKey.CreatedAt.Format(time.RFC3339)
		if t.IsTTY() {
			createdAt = "Created " + utils.FuzzyAgoAbbr(now, gpgKey.CreatedAt)
		}
		t.AddField(createdAt, nil, cs.Gray)

		expiresAt := gpgKey.ExpiresAt.Format(time.RFC3339)
		if t.IsTTY() {
			if gpgKey.ExpiresAt.IsZero() {
				expiresAt = "Never expires"
			} else {
				expiresAt = "Expires " + gpgKey.ExpiresAt.Format("2006-01-02")
			}
		}
		t.AddField(expiresAt, nil, cs.Gray)

		if !t.IsTTY() {
			t.AddField(gpgKey.PublicKey, nil, nil)
		}
		t.EndRow()
	}

	return t.Render()
}
