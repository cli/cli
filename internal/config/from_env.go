package config

import (
	"fmt"
	"os"

	"github.com/cli/cli/internal/ghinstance"
)

const (
	GH_TOKEN                = "GH_TOKEN"
	GITHUB_TOKEN            = "GITHUB_TOKEN"
	GH_ENTERPRISE_TOKEN     = "GH_ENTERPRISE_TOKEN"
	GITHUB_ENTERPRISE_TOKEN = "GITHUB_ENTERPRISE_TOKEN"
)

func InheritEnv(c Config) Config {
	return &envConfig{Config: c}
}

type envConfig struct {
	Config
}

func (c *envConfig) Hosts() ([]string, error) {
	hasDefault := false
	hosts, err := c.Config.Hosts()
	for _, h := range hosts {
		if h == ghinstance.Default() {
			hasDefault = true
		}
	}
	_, _, found := AuthTokenFromEnv(ghinstance.Default())
	if (err != nil || !hasDefault) && found {
		hosts = append([]string{ghinstance.Default()}, hosts...)
		return hosts, nil
	}
	return hosts, err
}

func (c *envConfig) Get(hostname, key string) (string, error) {
	val, _, err := c.GetWithSource(hostname, key)
	return val, err
}

func (c *envConfig) GetWithSource(hostname, key string) (string, string, error) {
	if hostname != "" && key == "oauth_token" {
		if token, env, found := AuthTokenFromEnv(hostname); found {
			return token, env, nil
		}
	}

	return c.Config.GetWithSource(hostname, key)
}

func (c *envConfig) CheckWriteable(hostname, key string) error {
	if hostname != "" && key == "oauth_token" {
		if _, env, found := AuthTokenFromEnv(hostname); found {
			return fmt.Errorf("read-only token in %s cannot be modified", env)
		}
	}

	return c.Config.CheckWriteable(hostname, key)
}

func AuthTokenFromEnv(hostname string) (string, string, bool) {
	if ghinstance.IsEnterprise(hostname) {
		if token, found := os.LookupEnv(GH_ENTERPRISE_TOKEN); found {
			return token, GH_ENTERPRISE_TOKEN, found
		}

		token, found := os.LookupEnv(GITHUB_ENTERPRISE_TOKEN)
		return token, GITHUB_ENTERPRISE_TOKEN, found
	}

	if token, found := os.LookupEnv(GH_TOKEN); found {
		return token, GH_TOKEN, found
	}

	token, found := os.LookupEnv(GITHUB_TOKEN)
	return token, GITHUB_TOKEN, found
}
