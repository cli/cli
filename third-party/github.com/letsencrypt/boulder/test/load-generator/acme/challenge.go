package acme

import (
	"errors"
	"fmt"
	mrand "math/rand"
	"strings"

	"github.com/letsencrypt/boulder/core"
)

// ChallengeStrategy is an interface describing a strategy for picking
// a challenge from a given authorization.
type ChallengeStrategy interface {
	PickChallenge(*core.Authorization) (*core.Challenge, error)
}

const (
	// RandomChallengeStrategy is the name for a random challenge selection
	// strategy that will choose one of the authorization's challenges at random.
	RandomChallengeStrategy = "RANDOM"
	// The following challenge strategies will always pick the named challenge
	// type or return an error if there isn't a challenge of that type to pick.
	HTTP01ChallengeStrategy    = "HTTP-01"
	DNS01ChallengeStrategy     = "DNS-01"
	TLSALPN01ChallengeStrategy = "TLS-ALPN-01"
)

// NewChallengeStrategy returns the ChallengeStrategy for the given
// ChallengeStrategyName, or an error if it is unknown.
func NewChallengeStrategy(rawName string) (ChallengeStrategy, error) {
	var preferredType core.AcmeChallenge
	switch name := strings.ToUpper(rawName); name {
	case RandomChallengeStrategy:
		return &randomChallengeStrategy{}, nil
	case HTTP01ChallengeStrategy:
		preferredType = core.ChallengeTypeHTTP01
	case DNS01ChallengeStrategy:
		preferredType = core.ChallengeTypeDNS01
	case TLSALPN01ChallengeStrategy:
		preferredType = core.ChallengeTypeTLSALPN01
	default:
		return nil, fmt.Errorf("ChallengeStrategy %q unknown", name)
	}

	return &preferredTypeChallengeStrategy{
		preferredType: preferredType,
	}, nil
}

var (
	ErrPickChallengeNilAuthz               = errors.New("PickChallenge: provided authorization can not be nil")
	ErrPickChallengeAuthzMissingChallenges = errors.New("PickChallenge: provided authorization had no challenges")
)

// randomChallengeStrategy is a ChallengeStrategy implementation that always
// returns a random challenge from the given authorization.
type randomChallengeStrategy struct {
}

// PickChallenge for a randomChallengeStrategy returns a random challenge from
// the authorization.
func (strategy randomChallengeStrategy) PickChallenge(authz *core.Authorization) (*core.Challenge, error) {
	if authz == nil {
		return nil, ErrPickChallengeNilAuthz
	}
	if len(authz.Challenges) == 0 {
		return nil, ErrPickChallengeAuthzMissingChallenges
	}
	return &authz.Challenges[mrand.Intn(len(authz.Challenges))], nil
}

// preferredTypeChallengeStrategy is a ChallengeStrategy implementation that
// always returns the authorization's challenge with type matching the
// preferredType.
type preferredTypeChallengeStrategy struct {
	preferredType core.AcmeChallenge
}

// PickChallenge for a preferredTypeChallengeStrategy returns the authorization
// challenge that has Type equal the preferredType. An error is returned if the
// challenge doesn't have an authorization matching the preferredType.
func (strategy preferredTypeChallengeStrategy) PickChallenge(authz *core.Authorization) (*core.Challenge, error) {
	if authz == nil {
		return nil, ErrPickChallengeNilAuthz
	}
	if len(authz.Challenges) == 0 {
		return nil, ErrPickChallengeAuthzMissingChallenges
	}
	for _, chall := range authz.Challenges {
		if chall.Type == strategy.preferredType {
			return &chall, nil
		}
	}
	return nil, fmt.Errorf("authorization (ID %q) had no %q type challenge",
		authz.ID,
		strategy.preferredType)
}
