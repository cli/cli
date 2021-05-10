package shared

import (
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
)

type Visibility string

const (
	All      = "all"
	Private  = "private"
	Selected = "selected"
)

type PubKey struct {
	Raw [32]byte
	ID  string `json:"key_id"`
	Key string
}

func getPubKey(client *api.Client, host, path string) (*PubKey, error) {
	pk := PubKey{}
	err := client.REST(host, "GET", path, nil, &pk)
	if err != nil {
		return nil, err
	}

	if pk.Key == "" {
		return nil, fmt.Errorf("failed to find public key at %s/%s", host, path)
	}

	decoded, err := base64.StdEncoding.DecodeString(pk.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to decode public key: %w", err)
	}

	copy(pk.Raw[:], decoded[0:32])
	return &pk, nil
}

func GetOrgPublicKey(client *api.Client, host, orgName string) (*PubKey, error) {
	return getPubKey(client, host, fmt.Sprintf("orgs/%s/actions/secrets/public-key", orgName))
}

func GetRepoPubKey(client *api.Client, repo ghrepo.Interface) (*PubKey, error) {
	return getPubKey(client, repo.RepoHost(), fmt.Sprintf("repos/%s/actions/secrets/public-key",
		ghrepo.FullName(repo)))
}

func ValidSecretName(name string) error {
	if name == "" {
		return errors.New("secret name cannot be blank")
	}

	if strings.HasPrefix(name, "GITHUB_") {
		return errors.New("secret name cannot begin with GITHUB_")
	}

	leadingNumber := regexp.MustCompile(`^[0-9]`)
	if leadingNumber.MatchString(name) {
		return errors.New("secret name cannot start with a number")
	}

	validChars := regexp.MustCompile(`^([0-9]|[a-z]|[A-Z]|_)+$`)
	if !validChars.MatchString(name) {
		return errors.New("secret name can only contain letters, numbers, and _")
	}

	return nil
}
