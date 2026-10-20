package config

import (
	"testing"

	"github.com/cli/cli/v2/internal/keyring"
	ghConfig "github.com/cli/go-gh/v2/pkg/config"
)

func BenchmarkActiveTokenFromKeyring(b *testing.B) {
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"} {
		b.Setenv(name, "")
	}

	host := "github.com"
	username := "benchmark-user"
	token := "benchmark-token"
	configData := ghConfig.ReadFromString("")
	configData.Set([]string{hostsKey, host, userKey}, username)
	keyring.MockInit()
	if err := keyring.Set(keyringServiceName(host), username, token); err != nil {
		b.Fatal(err)
	}

	previousRead := ghConfig.Read
	ghConfig.Read = func(_ *ghConfig.Config) (*ghConfig.Config, error) {
		return configData, nil
	}
	defer func() {
		ghConfig.Read = previousRead
	}()

	appConfig, err := NewConfig()
	if err != nil {
		b.Fatal(err)
	}
	authConfig := appConfig.Authentication()
	if actualToken, source := authConfig.ActiveToken(host); actualToken != token || source != "keyring" {
		b.Fatalf("unexpected setup token %q from %q", actualToken, source)
	}

	b.ResetTimer()
	for b.Loop() {
		if actualToken, source := authConfig.ActiveToken(host); actualToken != token || source != "keyring" {
			b.Fatalf("unexpected token %q from %q", actualToken, source)
		}
	}
}
