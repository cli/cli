package list

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/safeurl"
)

var errScopes = errors.New("insufficient OAuth scopes")

type emails []email

type email struct {
	Email string `json:"email"`
}

func (es emails) String() string {
	s := []string{}
	for _, e := range es {
		s = append(s, e.Email)
	}
	return strings.Join(s, ", ")
}

type gpgKey struct {
	KeyID     string    `json:"key_id"`
	PublicKey string    `json:"public_key"`
	Emails    emails    `json:"emails"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func userKeys(httpClient *http.Client, host, userHandle string) ([]gpgKey, error) {
	u, err := safeurl.JoinPath("user", "gpg_keys")
	if err != nil {
		return nil, err
	}
	if userHandle != "" {
		u, err = safeurl.JoinPath("users", userHandle, "gpg_keys")
		if err != nil {
			return nil, err
		}
	}
	u.SetQuery("per_page", "100")

	var keys []gpgKey
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	err = api.NewClientFromHTTP(httpClient).REST(host, "GET", u.String(), nil, &keys)
	if err != nil {
		if httpErr, ok := errors.AsType[api.HTTPError](err); ok && httpErr.StatusCode == 404 {
			return nil, errScopes
		}
		return nil, err
	}

	return keys, nil
}
