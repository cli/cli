package list

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
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
	u, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(host), "user", "gpg_keys")
	if err != nil {
		return nil, err
	}
	if userHandle != "" {
		u, err = safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(host), "users", userHandle, "gpg_keys")
		if err != nil {
			return nil, err
		}
	}
	u.SetQuery("per_page", "100")
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, errScopes
	} else if resp.StatusCode > 299 {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var keys []gpgKey
	err = json.Unmarshal(b, &keys)
	if err != nil {
		return nil, err
	}

	return keys, nil
}
