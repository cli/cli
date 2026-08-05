package list

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/githubrest"
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

func userKeys(ctx context.Context, client *githubrest.Client, userHandle string) ([]gpgKey, error) {
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
	req, err := client.NewRequest(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if _, err := client.Do(req, &keys); err != nil {
		if httpErr, ok := errors.AsType[*githubrest.ErrorResponse](err); ok && httpErr.StatusCode == 404 {
			return nil, errScopes
		}
		return nil, err
	}

	return keys, nil
}
