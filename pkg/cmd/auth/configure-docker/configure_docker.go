package docker

import (
	"fmt"
	"io"
	"net/http"
	"os/exec"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ConfigureDockerOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)

	Hostname string
}

func NewCmdConfigureDocker(f *cmdutil.Factory, runF func(*ConfigureDockerOptions) error) *cobra.Command {
	opts := &ConfigureDockerOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "configure-docker",
		Short: "Implements docker credential helper protocol",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return configureDockerRun(opts)
		},
	}

	return cmd
}

func configureDockerRun(opts *ConfigureDockerOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	stderr := opts.IO.ErrOut
	cs := opts.IO.ColorScheme()
	hostnames, err := cfg.Hosts()
	if err != nil {
		return fmt.Errorf("Unable to get hostnames: %w\n", err)
	}

	if len(hostnames) == 0 {
		fmt.Fprintf(stderr,
			"You are not logged into any GitHub hosts. Run %s to authenticate.\n", cs.Bold("gh auth login"))
		return cmdutil.SilentError
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("Unable to create httpClient: %w\n", err)
	}

	var username, token string
	for _, hostname := range hostnames {
		if opts.Hostname != "" && opts.Hostname != hostname {
			continue
		}
		token, _, err = cfg.GetWithSource(hostname, "oauth_token")
		if err != nil {
			return fmt.Errorf("Unable to get oauth_token: %w\n", err)
		}

		apiClient := api.NewClientFromHTTP(httpClient)
		username, err = api.CurrentLoginName(apiClient, hostname)
		if err != nil {
			return fmt.Errorf("Unable to get username: %w\n", err)
		}
	}

	cmd := exec.Command("docker", "login", "ghcr.io", "--username", username, "--password-stdin")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("Unable to create oauth_token writer: %w\n", err)
	}

	io.WriteString(stdin, token)
	err = stdin.Close()
	if err != nil {
		return fmt.Errorf("Unable to close oauth_token writer: %w\n", err)
	}

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("Unable to login ghcr.io: %w\n", err)
	}

	fmt.Fprintf(opts.IO.Out, "%s %s\n", cs.Bold("gh auth configure-docker"), out)
	return nil
}
