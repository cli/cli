package shared

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/authflow"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/cmd/ssh-key/add"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/ssh"
)

const defaultSSHKeyTitle = "GitHub CLI"

type iconfig interface {
	Get(string, string) (string, error)
	Set(string, string, string)
	Write() error
}

type LoginOptions struct {
	IO          *iostreams.IOStreams
	Config      iconfig
	HTTPClient  *http.Client
	GitClient   *git.Client
	Hostname    string
	Interactive bool
	Web         bool
	Scopes      []string
	Executable  string
	GitProtocol string
	Prompter    Prompt

	sshContext ssh.Context
}

func Login(opts *LoginOptions) error {
	cfg := opts.Config
	hostname := opts.Hostname
	httpClient := opts.HTTPClient
	cs := opts.IO.ColorScheme()

	gitProtocol := strings.ToLower(opts.GitProtocol)
	if opts.Interactive && gitProtocol == "" {
		options := []string{
			"HTTPS",
			"SSH",
		}
		result, err := opts.Prompter.Select(
			"What is your preferred protocol for Git operations?",
			options[0],
			options)
		if err != nil {
			return err
		}
		proto := options[result]
		gitProtocol = strings.ToLower(proto)
	}

	var additionalScopes []string

	credentialFlow := &GitCredentialFlow{
		Executable: opts.Executable,
		Prompter:   opts.Prompter,
		GitClient:  opts.GitClient,
	}
	if opts.Interactive && gitProtocol == "https" {
		if err := credentialFlow.Prompt(hostname); err != nil {
			return err
		}
		additionalScopes = append(additionalScopes, credentialFlow.Scopes()...)
	}

	var keyToUpload string
	keyTitle := defaultSSHKeyTitle
	if opts.Interactive && gitProtocol == "ssh" {
		pubKeys, err := opts.sshContext.LocalPublicKeys()
		if err != nil {
			return err
		}

		if len(pubKeys) > 0 {
			options := append(pubKeys, "Skip")
			keyChoice, err := opts.Prompter.Select(
				"Upload your SSH public key to your GitHub account?",
				options[0],
				options)
			if err != nil {
				return err
			}
			if keyChoice < len(pubKeys) {
				keyToUpload = pubKeys[keyChoice]
			}
		} else if opts.sshContext.HasKeygen() {
			sshChoice, err := opts.Prompter.Confirm("Generate a new SSH key to add to your GitHub account?", true)
			if err != nil {
				return err
			}

			if sshChoice {
				passphrase, err := opts.Prompter.Password(
					"Enter a passphrase for your new SSH key (Optional)")
				if err != nil {
					return err
				}
				keyPair, err := opts.sshContext.GenerateSSHKey("id_ed25519", passphrase)
				if err != nil {
					return err
				}
				keyToUpload = keyPair.PublicKeyPath
			}
		}

		if keyToUpload != "" {
			var err error
			keyTitle, err = opts.Prompter.Input(
				"Title for your SSH key:", defaultSSHKeyTitle)
			if err != nil {
				return err
			}

			additionalScopes = append(additionalScopes, "admin:public_key")
		}
	}

	var authMode int
	if opts.Web {
		authMode = 0
	} else if opts.Interactive {
		options := []string{"Login with a web browser", "Paste an authentication token"}
		var err error
		authMode, err = opts.Prompter.Select(
			"How would you like to authenticate GitHub CLI?",
			options[0],
			options)
		if err != nil {
			return err
		}
	}

	var authToken string
	userValidated := false

	if authMode == 0 {
		var err error
		authToken, err = authflow.AuthFlowWithConfig(cfg, opts.IO, hostname, "", append(opts.Scopes, additionalScopes...), opts.Interactive)
		if err != nil {
			return fmt.Errorf("failed to authenticate via web browser: %w", err)
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s Authentication complete.\n", cs.SuccessIcon())
		userValidated = true
	} else {
		minimumScopes := append([]string{"repo", "read:org"}, additionalScopes...)
		fmt.Fprint(opts.IO.ErrOut, heredoc.Docf(`
			Tip: you can generate a Personal Access Token here https://%s/settings/tokens
			The minimum required scopes are %s.
		`, hostname, scopesSentence(minimumScopes, ghinstance.IsEnterprise(hostname))))

		authToken, err := opts.Prompter.AuthToken()
		if err != nil {
			return err
		}

		if err := HasMinimumScopes(httpClient, hostname, authToken); err != nil {
			return fmt.Errorf("error validating token: %w", err)
		}

		cfg.Set(hostname, "oauth_token", authToken)
	}

	var username string
	if userValidated {
		username, _ = cfg.Get(hostname, "user")
	} else {
		apiClient := api.NewClientFromHTTP(httpClient)
		var err error
		username, err = api.CurrentLoginName(apiClient, hostname)
		if err != nil {
			return fmt.Errorf("error using api: %w", err)
		}

		cfg.Set(hostname, "user", username)
	}

	if gitProtocol != "" {
		fmt.Fprintf(opts.IO.ErrOut, "- gh config set -h %s git_protocol %s\n", hostname, gitProtocol)
		cfg.Set(hostname, "git_protocol", gitProtocol)
		fmt.Fprintf(opts.IO.ErrOut, "%s Configured git protocol\n", cs.SuccessIcon())
	}

	err := cfg.Write()
	if err != nil {
		return err
	}

	if credentialFlow.ShouldSetup() {
		err := credentialFlow.Setup(hostname, username, authToken)
		if err != nil {
			return err
		}
	}

	if keyToUpload != "" {
		err := sshKeyUpload(httpClient, hostname, keyToUpload, keyTitle)
		if err != nil {
			return err
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s Uploaded the SSH key to your GitHub account: %s\n", cs.SuccessIcon(), cs.Bold(keyToUpload))
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Logged in as %s\n", cs.SuccessIcon(), cs.Bold(username))
	return nil
}

func scopesSentence(scopes []string, isEnterprise bool) string {
	quoted := make([]string, len(scopes))
	for i, s := range scopes {
		quoted[i] = fmt.Sprintf("'%s'", s)
		if s == "workflow" && isEnterprise {
			// remove when GHE 2.x reaches EOL
			quoted[i] += " (GHE 3.0+)"
		}
	}
	return strings.Join(quoted, ", ")
}

func sshKeyUpload(httpClient *http.Client, hostname, keyFile string, title string) error {
	f, err := os.Open(keyFile)
	if err != nil {
		return err
	}
	defer f.Close()

	return add.SSHKeyUpload(httpClient, hostname, f, title)
}
