package shared

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/authflow"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/cmd/ssh-key/add"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
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
	Hostname    string
	Interactive bool
	Web         bool
	Scopes      []string
	Executable  string
	GitProtocol string

	sshContext ssh.Context
}

func Login(opts *LoginOptions) error {
	cfg := opts.Config
	hostname := opts.Hostname
	httpClient := opts.HTTPClient
	cs := opts.IO.ColorScheme()

	gitProtocol := strings.ToLower(opts.GitProtocol)
	if opts.Interactive && gitProtocol == "" {
		var proto string
		err := prompt.SurveyAskOne(&survey.Select{
			Message: "What is your preferred protocol for Git operations?",
			Options: []string{
				"HTTPS",
				"SSH",
			},
		}, &proto)
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}
		gitProtocol = strings.ToLower(proto)
	}

	var additionalScopes []string

	credentialFlow := &GitCredentialFlow{Executable: opts.Executable}
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
			var keyChoice int
			err := prompt.SurveyAskOne(&survey.Select{
				Message: "Upload your SSH public key to your GitHub account?",
				Options: append(pubKeys, "Skip"),
			}, &keyChoice)
			if err != nil {
				return fmt.Errorf("could not prompt: %w", err)
			}
			if keyChoice < len(pubKeys) {
				keyToUpload = pubKeys[keyChoice]
			}
		} else if opts.sshContext.HasKeygen() {
			var sshChoice bool
			err := prompt.SurveyAskOne(&survey.Confirm{
				Message: "Generate a new SSH key to add to your GitHub account?",
				Default: true,
			}, &sshChoice)

			if err != nil {
				return fmt.Errorf("could not prompt: %w", err)
			}

			if sshChoice {
				passphrase, err := promptForSshKeyPassphrase()
				if err != nil {
					return fmt.Errorf("could not prompt for key passphrase: %w", err)
				}

				keyPair, err := opts.sshContext.GenerateSSHKey("id_ed25519", passphrase)
				if err != nil {
					return err
				}
				keyToUpload = keyPair.PublicKeyPath
			}
		}

		if keyToUpload != "" {
			err := prompt.SurveyAskOne(&survey.Input{
				Message: "Title for your SSH key:",
				Default: defaultSSHKeyTitle,
			}, &keyTitle)
			if err != nil {
				return fmt.Errorf("could not prompt: %w", err)
			}

			additionalScopes = append(additionalScopes, "admin:public_key")
		}
	}

	var authMode int
	if opts.Web {
		authMode = 0
	} else if opts.Interactive {
		err := prompt.SurveyAskOne(&survey.Select{
			Message: "How would you like to authenticate GitHub CLI?",
			Options: []string{
				"Login with a web browser",
				"Paste an authentication token",
			},
		}, &authMode)
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
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

		err := prompt.SurveyAskOne(&survey.Password{
			Message: "Paste your authentication token:",
		}, &authToken, survey.WithValidator(survey.Required))
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
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

func promptForSshKeyPassphrase() (string, error) {
	var sshPassphrase string
	err := prompt.SurveyAskOne(&survey.Password{
		Message: "Enter a passphrase for your new SSH key (Optional)",
	}, &sshPassphrase)

	if err != nil {
		return "", err
	}

	return sshPassphrase, nil
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
