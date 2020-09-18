package list

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/utils"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// ListOptions struct for list command
type ListOptions struct {
	HTTPClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)

	ListMsg []string
}

// NewCmdList creates a command for list all SSH Keys
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		HTTPClient: f.HttpClient,
		IO:         f.IOStreams,
		Config:     f.Config,

		ListMsg: []string{},
	}

	cmd := &cobra.Command{
		Use:   "list",
		Args:  cobra.ExactArgs(0),
		Short: "Lists currently added ssh keys",
		Long: heredoc.Doc(`Lists currently added ssh keys.

			This interactive command lists all SSH keys associated with your account
		`),
		Example: heredoc.Doc(`
			$ gh ssh-key list
			# => lists all ssh keys associated with your account
		`),
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
	apiClient, err := opts.getAPIClient()
	if err != nil {
		opts.printTerminal()
		return err
	}

	err = opts.hasMinimumScopes(apiClient)
	if err != nil {
		opts.printTerminal()
		return err
	}

	type keys struct {
		Title string
		Key   string
	}

	type result []keys

	rs := result{}
	body := bytes.NewBufferString("")

	err = apiClient.REST(ghinstance.Default(), "GET", "user/keys", body, &rs)
	if err != nil {
		opts.ListMsg = append(opts.ListMsg, fmt.Sprintf("%s: Got %s", utils.RedX(), err))
		opts.printTerminal()
		return err
	}

	for _, r := range rs {
		opts.ListMsg = append(opts.ListMsg, fmt.Sprintf("%s %s: %s \n    %s: %s", utils.Cyan("✹"), utils.Bold("Name"), r.Title, utils.Bold("SSH-KEY"), r.Key))
	}

	opts.printTerminal()

	return nil
}

func (opts *ListOptions) getAPIClient() (*api.Client, error) {
	httpClient, err := opts.HTTPClient()
	if err != nil {
		opts.ListMsg = append(opts.ListMsg, fmt.Sprintf("%s: %s", utils.RedX(), err))
		return nil, err
	}
	return api.NewClientFromHTTP(httpClient), nil
}

func (opts *ListOptions) hasMinimumScopes(apiClient *api.Client) error {
	cfg, err := opts.Config()
	if err != nil {
		opts.ListMsg = append(opts.ListMsg, fmt.Sprintf("%s: %s", utils.RedX(), err))
		return err
	}

	hostname := ghinstance.Default()

	_, tokenSource, _ := cfg.GetWithSource(hostname, "oauth_token")

	// TODO: Implement tests for this case when CheckWriteable function checks filesystem permissions
	tokenIsWriteable := cfg.CheckWriteable(hostname, "oauth_token") == nil

	err = apiClient.HasMinimumScopes(hostname)

	if err != nil {
		var missingScopes *api.MissingScopesError
		if errors.As(err, &missingScopes) {
			opts.ListMsg = append(opts.ListMsg, fmt.Sprintf("%s: %s", utils.RedX(), err))
			if tokenIsWriteable {
				opts.ListMsg = append(opts.ListMsg, fmt.Sprintf("- To request missing scopes, run: %s %s", utils.Bold("gh auth refresh -h"), hostname))
			}
		} else {
			opts.ListMsg = append(opts.ListMsg, fmt.Sprintf("%s: authentication failed", utils.RedX()))
			opts.ListMsg = append(opts.ListMsg, fmt.Sprintf("- The %s token in %s is no longer valid.", utils.Bold(hostname), utils.Bold(tokenSource)))
			if tokenIsWriteable {
				opts.ListMsg = append(opts.ListMsg, fmt.Sprintf("- To re-authenticate, run: %s %s", utils.Bold("gh auth login -h"), utils.Bold(hostname)))
				opts.ListMsg = append(opts.ListMsg, fmt.Sprintf("- To forget about this host, run: %s %s", utils.Bold("gh auth logout -h"), utils.Bold(hostname)))
			}
		}
		return err
	}

	return nil
}

func (opts *ListOptions) printTerminal() {
	stderr := opts.IO.ErrOut
	for _, line := range opts.ListMsg {
		fmt.Fprintf(stderr, "  %s\n", line)
	}
}
