package identity

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/pkg/cmdutil"
)

func resolveEmail(f *cmdutil.Factory, auth gh.AuthConfig, id identity.Identity) (string, error) {
	selector := id.GitUserEmail
	if !strings.HasPrefix(selector, "@") {
		return selector, nil
	}
	if selector != "@public" && selector != "@noreply" {
		return "", cmdutil.FlagErrorf("unknown Git email source %q; use @public, @noreply, or a literal email address", selector)
	}
	if id.GitUserName == "" {
		return "", cmdutil.FlagErrorf("%s requires --git-user-name or a Git name already stored in the identity", selector)
	}
	if selector == "@noreply" && id.Host != "github.com" {
		return "", cmdutil.FlagErrorf("@noreply is supported only for github.com in this prototype; supply the noreply address from your host's email settings")
	}
	credential, err := identity.CredentialForAccount(auth, id.Host, id.Account)
	if err != nil {
		return "", err
	}
	httpClient, err := f.HttpClient()
	if err != nil {
		return "", err
	}
	client := api.NewClientFromHTTP(httpClient)
	resp, err := client.Request(id.Host, http.MethodGet, "users/"+url.PathEscape(id.Account), nil,
		api.WithHeader("Authorization", "token "+credential.Token), api.WithoutFollowingRedirects())
	if err != nil {
		return "", fmt.Errorf("resolve %s email for %s on %s: %w", selector, id.Account, id.Host, err)
	}
	defer resp.Body.Close()
	var profile struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return "", fmt.Errorf("read account profile for %s: %w", selector, err)
	}
	if !strings.EqualFold(profile.Login, id.Account) {
		return "", fmt.Errorf("profile lookup returned login %q instead of %q; identity was not saved", profile.Login, id.Account)
	}
	if selector == "@public" {
		if strings.TrimSpace(profile.Email) == "" {
			return "", fmt.Errorf("account %q on %s has no public profile email; choose --git-user-email @noreply on github.com or supply a literal address", id.Account, id.Host)
		}
		return profile.Email, nil
	}
	if profile.ID <= 0 {
		return "", fmt.Errorf("profile for %q has no valid account ID; cannot construct its noreply address", id.Account)
	}
	return fmt.Sprintf("%d+%s@users.noreply.github.com", profile.ID, profile.Login), nil
}
