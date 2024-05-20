package gitcredentials_test

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/auth/shared"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/contract"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/gitcredentials"
)

func TestFakeHelperConfigContract(t *testing.T) {
	// Note that this being mutated by `NewHelperConfig` makes these tests not parallelizable
	var fhc *gitcredentials.FakeHelperConfig

	contract.HelperConfig{
		NewHelperConfig: func(t *testing.T) shared.HelperConfig {
			// Mutate the closed over fhc so that ConfigureHelper is able to configure helpers
			// for tests. An alternative would be to provide the Helper as an argument back to ConfigureHelper
			// but then we'd have to type assert it back to *FakeHelperConfig, which is probably more trouble than
			// it's worth to parallelize these tests, sinced it's not even possible to parallelize the real Helperconfig
			// ones due to them using t.Setenv
			fhc = &gitcredentials.FakeHelperConfig{
				SelfExecutablePath: "/path/to/gh",
				Helpers:            map[string]gitcredentials.Helper{},
			}
			return fhc
		},
		ConfigureHelper: func(t *testing.T, hostname string) {
			fhc.Helpers[hostname] = gitcredentials.Helper{Cmd: "test-helper"}
		},
	}.Test(t)
}
