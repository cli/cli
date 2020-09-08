package config

import (
	"fmt"
	"os"

	"github.com/cli/cli/internal/ghinstance"
)

const (
	GITHUB_TOKEN            = "GITHUB_TOKEN"
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
	if (err != nil || !hasDefault) && os.Getenv(GITHUB_TOKEN) != "" {
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
		envName := GITHUB_TOKEN
		if ghinstance.IsEnterprise(hostname) {
			envName = GITHUB_ENTERPRISE_TOKEN
		}

		if value := os.Getenv(envName); value != "" {
			return value, envName, nil
		}
	}

	return c.Config.GetWithSource(hostname, key)
}

func (c *envConfig) CheckWriteable(hostname, key string) error {
	if hostname != "" && key == "oauth_token" {
		envName := GITHUB_TOKEN
		if ghinstance.IsEnterprise(hostname) {
			envName = GITHUB_ENTERPRISE_TOKEN
		}

		if os.Getenv(envName) != "" {
			return fmt.Errorf("read-only token in %s cannot be modified", envName)
		}
	}

	return c.Config.CheckWriteable(hostname, key)
}
