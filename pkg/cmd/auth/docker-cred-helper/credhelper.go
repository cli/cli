package credhelper

import (
	"errors"
	"fmt"

	"github.com/cli/cli/v2/internal/config"
	"github.com/docker/docker-credential-helpers/credentials"
)

var errNotImplemented = errors.New("not implemented")

type helper struct{}

func CredHelper() credentials.Helper { return helper{} }

func (helper) Add(*credentials.Credentials) error { return errNotImplemented }
func (helper) Delete(string) error                { return errNotImplemented }
func (helper) List() (map[string]string, error)   { return nil, errNotImplemented }

func (h helper) Get(serverURL string) (string, string, error) {
	cfg, err := config.NewConfig() // TODO
	if err != nil {
		return "", "", err
	}
	authCfg := cfg.Authentication()

	hostname, _ := authCfg.DefaultHost()
	val, _ := authCfg.TokenFromKeyring(hostname)
	if val == "" {
		return "", "", fmt.Errorf("no oauth token")
	}
	return "_token", val, nil
}
