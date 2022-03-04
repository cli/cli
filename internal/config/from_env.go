package config

import (
	"fmt"
	"os"
	"sort"
	"strconv"

	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/set"
)

const (
	GH_HOST                 = "GH_HOST"
	GH_TOKEN                = "GH_TOKEN"
	GITHUB_TOKEN            = "GITHUB_TOKEN"
	GH_ENTERPRISE_TOKEN     = "GH_ENTERPRISE_TOKEN"
	GITHUB_ENTERPRISE_TOKEN = "GITHUB_ENTERPRISE_TOKEN"
	CODESPACES              = "CODESPACES"
)

type ReadOnlyEnvError struct {
	Variable string
}

func (e *ReadOnlyEnvError) Error() string {
	return fmt.Sprintf("read-only value in %s", e.Variable)
}

func InheritEnv(c Config) Config {
	return &envConfig{Config: c}
}

type envConfig struct {
	Config
}

func (c *envConfig) Hosts() ([]string, error) {
	hosts, err := c.Config.Hosts()
	if err != nil {
		return nil, err
	}

	hostSet := set.NewStringSet()
	hostSet.AddValues(hosts)

	// If GH_HOST is set then add it to list.
	if host := os.Getenv(GH_HOST); host != "" {
		hostSet.Add(host)
	}

	// If there is a valid environment variable token for the
	// default host then add default host to list.
	if token, _ := AuthTokenFromEnv(ghinstance.Default()); token != "" {
		hostSet.Add(ghinstance.Default())
	}

	s := hostSet.ToSlice()
	// If default host is in list then move it to the front.
	sort.SliceStable(s, func(i, j int) bool { return s[i] == ghinstance.Default() })
	return s, nil
}

func (c *envConfig) DefaultHost() (string, error) {
	val, _, err := c.DefaultHostWithSource()
	return val, err
}

func (c *envConfig) DefaultHostWithSource() (string, string, error) {
	if host := os.Getenv(GH_HOST); host != "" {
		return host, GH_HOST, nil
	}
	return c.Config.DefaultHostWithSource()
}

func (c *envConfig) Get(hostname, key string) (string, error) {
	val, _, err := c.GetWithSource(hostname, key)
	return val, err
}

func (c *envConfig) GetWithSource(hostname, key string) (string, string, error) {
	if hostname != "" && key == "oauth_token" {
		if token, env := AuthTokenFromEnv(hostname); token != "" {
			return token, env, nil
		}
	}

	return c.Config.GetWithSource(hostname, key)
}

func (c *envConfig) GetOrDefault(hostname, key string) (val string, err error) {
	val, _, err = c.GetOrDefaultWithSource(hostname, key)
	return
}

func (c *envConfig) GetOrDefaultWithSource(hostname, key string) (val string, src string, err error) {
	val, src, err = c.GetWithSource(hostname, key)
	if err == nil && val == "" {
		val = c.Default(key)
	}

	return
}

func (c *envConfig) Default(key string) string {
	return c.Config.Default(key)
}

func (c *envConfig) CheckWriteable(hostname, key string) error {
	if hostname != "" && key == "oauth_token" {
		if token, env := AuthTokenFromEnv(hostname); token != "" {
			return &ReadOnlyEnvError{Variable: env}
		}
	}

	return c.Config.CheckWriteable(hostname, key)
}

func AuthTokenFromEnv(hostname string) (string, string) {
	if ghinstance.IsEnterprise(hostname) {
		if token := os.Getenv(GH_ENTERPRISE_TOKEN); token != "" {
			return token, GH_ENTERPRISE_TOKEN
		}

		if token := os.Getenv(GITHUB_ENTERPRISE_TOKEN); token != "" {
			return token, GITHUB_ENTERPRISE_TOKEN
		}

		if isCodespaces, _ := strconv.ParseBool(os.Getenv(CODESPACES)); isCodespaces {
			return os.Getenv(GITHUB_TOKEN), GITHUB_TOKEN
		}

		return "", ""
	}

	if token := os.Getenv(GH_TOKEN); token != "" {
		return token, GH_TOKEN
	}

	return os.Getenv(GITHUB_TOKEN), GITHUB_TOKEN
}

func AuthTokenProvidedFromEnv() bool {
	return os.Getenv(GH_ENTERPRISE_TOKEN) != "" ||
		os.Getenv(GITHUB_ENTERPRISE_TOKEN) != "" ||
		os.Getenv(GH_TOKEN) != "" ||
		os.Getenv(GITHUB_TOKEN) != ""
}

func IsHostEnv(src string) bool {
	return src == GH_HOST
}

func IsEnterpriseEnv(src string) bool {
	return src == GH_ENTERPRISE_TOKEN || src == GITHUB_ENTERPRISE_TOKEN
}
