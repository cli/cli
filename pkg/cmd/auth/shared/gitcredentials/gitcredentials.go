package gitcredentials

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghinstance"
)

type HelperConfigurer struct {
	SelfExecutablePath string
	GitClient          *git.Client
}

func (hc *HelperConfigurer) Configure(hostname string) error {
	ctx := context.TODO()

	credHelperKeys := []string{
		gitCredentialHelperKey(hostname),
	}

	gistHost := strings.TrimSuffix(ghinstance.GistHost(hostname), "/")
	if strings.HasPrefix(gistHost, "gist.") {
		credHelperKeys = append(credHelperKeys, gitCredentialHelperKey(gistHost))
	}

	var configErr error

	for _, credHelperKey := range credHelperKeys {
		if configErr != nil {
			break
		}
		// first use a blank value to indicate to git we want to sever the chain of credential helpers
		preConfigureCmd, err := hc.GitClient.Command(ctx, "config", "--global", "--replace-all", credHelperKey, "")
		if err != nil {
			configErr = err
			break
		}
		if _, err = preConfigureCmd.Output(); err != nil {
			configErr = err
			break
		}

		// second configure the actual helper for this host
		configureCmd, err := hc.GitClient.Command(ctx,
			"config", "--global", "--add",
			credHelperKey,
			fmt.Sprintf("!%s auth git-credential", shellQuote(hc.SelfExecutablePath)),
		)
		if err != nil {
			configErr = err
		} else {
			_, configErr = configureCmd.Output()
		}
	}

	return configErr
}

func gitCredentialHelperKey(hostname string) string {
	host := strings.TrimSuffix(ghinstance.HostPrefix(hostname), "/")
	return fmt.Sprintf("credential.%s.helper", host)
}

func shellQuote(s string) string {
	if strings.ContainsAny(s, " $\\") {
		return "'" + s + "'"
	}
	return s
}

type Updater struct {
	GitClient *git.Client
}

func (u *Updater) Update(hostname, username, password string) error {
	ctx := context.TODO()

	// clear previous cached credentials
	rejectCmd, err := u.GitClient.Command(ctx, "credential", "reject")
	if err != nil {
		return err
	}

	rejectCmd.Stdin = bytes.NewBufferString(heredoc.Docf(`
		protocol=https
		host=%s
	`, hostname))

	_, err = rejectCmd.Output()
	if err != nil {
		return err
	}

	approveCmd, err := u.GitClient.Command(ctx, "credential", "approve")
	if err != nil {
		return err
	}

	approveCmd.Stdin = bytes.NewBufferString(heredoc.Docf(`
		protocol=https
		host=%s
		username=%s
		password=%s
	`, hostname, username, password))

	_, err = approveCmd.Output()
	if err != nil {
		return err
	}

	return nil
}
