package unpause

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/jmhodges/clock"

	"github.com/letsencrypt/boulder/cmd"
)

const (
	// API

	// Changing this value will invalidate all existing JWTs.
	APIVersion = "v1"
	APIPrefix  = "/sfe/" + APIVersion
	GetForm    = APIPrefix + "/unpause"

	// BatchSize is the maximum number of identifiers that the SA will unpause
	// in a single batch.
	BatchSize = 10000

	// MaxBatches is the maximum number of batches that the SA will unpause in a
	// single request.
	MaxBatches = 5

	// RequestLimit is the maximum number of identifiers that the SA will
	// unpause in a single request. This is used by the SFE to infer whether
	// there are more identifiers to unpause.
	RequestLimit = BatchSize * MaxBatches

	// JWT
	defaultIssuer   = "WFE"
	defaultAudience = "SFE Unpause"
)

// JWTSigner is a type alias for jose.Signer. To create a JWTSigner instance,
// use the NewJWTSigner function provided in this package.
type JWTSigner = jose.Signer

// NewJWTSigner loads the HMAC key from the provided configuration and returns a
// new JWT signer.
func NewJWTSigner(hmacKey cmd.HMACKeyConfig) (JWTSigner, error) {
	key, err := hmacKey.Load()
	if err != nil {
		return nil, err
	}
	return jose.NewSigner(jose.SigningKey{Algorithm: jose.HS256, Key: key}, nil)
}

// JWTClaims represents the claims of a JWT token issued by the WFE for
// redemption by the SFE. The following claims required for unpausing:
//   - Subject: the account ID of the Subscriber
//   - V: the API version this JWT was created for
//   - I: a set of ACME identifier values. Identifier types are omitted
//     since DNS and IP string representations do not overlap.
type JWTClaims struct {
	jwt.Claims

	// V is the API version this JWT was created for.
	V string `json:"version"`

	// I is set of comma separated ACME identifiers.
	I string `json:"identifiers"`
}

// GenerateJWT generates a serialized unpause JWT with the provided claims.
func GenerateJWT(signer JWTSigner, regID int64, idents []string, lifetime time.Duration, clk clock.Clock) (string, error) {
	claims := JWTClaims{
		Claims: jwt.Claims{
			Issuer:   defaultIssuer,
			Subject:  fmt.Sprintf("%d", regID),
			Audience: jwt.Audience{defaultAudience},
			// IssuedAt is necessary for metrics.
			IssuedAt: jwt.NewNumericDate(clk.Now()),
			Expiry:   jwt.NewNumericDate(clk.Now().Add(lifetime)),
		},
		V: APIVersion,
		I: strings.Join(idents, ","),
	}

	serialized, err := jwt.Signed(signer).Claims(&claims).Serialize()
	if err != nil {
		return "", fmt.Errorf("serializing JWT: %s", err)
	}

	return serialized, nil
}

// ErrMalformedJWT is returned when the JWT is malformed.
var ErrMalformedJWT = errors.New("malformed JWT")

// RedeemJWT deserializes an unpause JWT and returns the validated claims. The
// key is used to validate the signature of the JWT. The version is the expected
// API version of the JWT. This function validates that the JWT is:
//   - well-formed,
//   - valid for the current time (+/- 1 minute leeway),
//   - issued by the WFE,
//   - intended for the SFE,
//   - contains an Account ID as the 'Subject',
//   - subject can be parsed as a 64-bit integer,
//   - contains a set of paused identifiers as 'Identifiers', and
//   - contains the API the expected version as 'Version'.
//
// If the JWT is malformed or invalid in any way, ErrMalformedJWT is returned.
func RedeemJWT(token string, key []byte, version string, clk clock.Clock) (JWTClaims, error) {
	parsedToken, err := jwt.ParseSigned(token, []jose.SignatureAlgorithm{jose.HS256})
	if err != nil {
		return JWTClaims{}, errors.Join(ErrMalformedJWT, err)
	}

	claims := JWTClaims{}
	err = parsedToken.Claims(key, &claims)
	if err != nil {
		return JWTClaims{}, errors.Join(ErrMalformedJWT, err)
	}

	err = claims.Validate(jwt.Expected{
		Issuer:      defaultIssuer,
		AnyAudience: jwt.Audience{defaultAudience},

		// By default, the go-jose library validates the NotBefore and Expiry
		// fields with a default leeway of 1 minute.
		Time: clk.Now(),
	})
	if err != nil {
		return JWTClaims{}, fmt.Errorf("validating JWT: %w", err)
	}

	if len(claims.Subject) == 0 {
		return JWTClaims{}, errors.New("no account ID specified in the JWT")
	}
	account, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return JWTClaims{}, errors.New("invalid account ID specified in the JWT")
	}
	if account == 0 {
		return JWTClaims{}, errors.New("no account ID specified in the JWT")
	}

	if claims.V == "" {
		return JWTClaims{}, errors.New("no API version specified in the JWT")
	}

	if claims.V != version {
		return JWTClaims{}, fmt.Errorf("unexpected API version in the JWT: %s", claims.V)
	}

	if claims.I == "" {
		return JWTClaims{}, errors.New("no identifiers specified in the JWT")
	}

	return claims, nil
}
