package acme

import (
	"fmt"
	"testing"

	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/test"
)

func TestNewChallengeStrategy(t *testing.T) {
	testCases := []struct {
		Name              string
		InputName         string
		ExpectedError     string
		ExpectedStratType string
	}{
		{
			Name:          "unknown name",
			InputName:     "hyper-quauntum-math-mesh-challenge",
			ExpectedError: `ChallengeStrategy "HYPER-QUAUNTUM-MATH-MESH-CHALLENGE" unknown`,
		},
		{
			Name:              "known name, HTTP-01",
			InputName:         "HTTP-01",
			ExpectedStratType: "*acme.preferredTypeChallengeStrategy",
		},
		{
			Name:              "known name, DNS-01",
			InputName:         "DNS-01",
			ExpectedStratType: "*acme.preferredTypeChallengeStrategy",
		},
		{
			Name:              "known name, TLS-ALPN-01",
			InputName:         "TLS-ALPN-01",
			ExpectedStratType: "*acme.preferredTypeChallengeStrategy",
		},
		{
			Name:              "known name, RANDOM",
			InputName:         "RANDOM",
			ExpectedStratType: "*acme.randomChallengeStrategy",
		},
		{
			Name:              "known name, mixed case",
			InputName:         "rAnDoM",
			ExpectedStratType: "*acme.randomChallengeStrategy",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			strategy, err := NewChallengeStrategy(tc.InputName)
			if err == nil && tc.ExpectedError == "" {
				test.AssertEquals(t, fmt.Sprintf("%T", strategy), tc.ExpectedStratType)
			} else if err == nil && tc.ExpectedError != "" {
				t.Errorf("Expected %q got no error\n", tc.ExpectedError)
			} else if err != nil {
				test.AssertEquals(t, err.Error(), tc.ExpectedError)
			}
		})
	}
}

func TestPickChallenge(t *testing.T) {
	exampleDNSChall := core.Challenge{
		Type: "dns-01",
	}
	exampleAuthz := &core.Authorization{
		ID: "1234",
		Challenges: []core.Challenge{
			{
				Type: "arm-wrestling",
			},
			exampleDNSChall,
			{
				Type: "http-01",
			},
		},
	}

	testCases := []struct {
		Name              string
		StratName         string
		InputAuthz        *core.Authorization
		ExpectedError     string
		ExpectedChallenge *core.Challenge
	}{
		{
			Name:          "Preferred type strategy, nil input authz",
			StratName:     "http-01",
			ExpectedError: ErrPickChallengeNilAuthz.Error(),
		},
		{
			Name:          "Random type strategy, nil input authz",
			StratName:     "random",
			ExpectedError: ErrPickChallengeNilAuthz.Error(),
		},
		{
			Name:          "Preferred type strategy, nil input authz challenges",
			StratName:     "http-01",
			InputAuthz:    &core.Authorization{},
			ExpectedError: ErrPickChallengeAuthzMissingChallenges.Error(),
		},
		{
			Name:          "Random type strategy, nil input authz challenges",
			StratName:     "random",
			InputAuthz:    &core.Authorization{},
			ExpectedError: ErrPickChallengeAuthzMissingChallenges.Error(),
		},
		{
			Name:          "Preferred type strategy, no challenge of type",
			StratName:     "tls-alpn-01",
			InputAuthz:    exampleAuthz,
			ExpectedError: `authorization (ID "1234") had no "tls-alpn-01" type challenge`,
		},
		{
			Name:              "Preferred type strategy, challenge of type present",
			StratName:         "dns-01",
			InputAuthz:        exampleAuthz,
			ExpectedChallenge: &exampleDNSChall,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			strategy, err := NewChallengeStrategy(tc.StratName)
			test.AssertNotError(t, err, "Failed to create challenge strategy")
			chall, err := strategy.PickChallenge(tc.InputAuthz)
			if err == nil && tc.ExpectedError == "" {
				test.AssertDeepEquals(t, chall, tc.ExpectedChallenge)
			} else if err == nil && tc.ExpectedError != "" {
				t.Errorf("Expected %q got no error\n", tc.ExpectedError)
			} else if err != nil {
				test.AssertEquals(t, err.Error(), tc.ExpectedError)
			}
		})
	}
}
