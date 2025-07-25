package unpause

import (
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/jmhodges/clock"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/test"
)

func TestUnpauseJWT(t *testing.T) {
	fc := clock.NewFake()

	signer, err := NewJWTSigner(cmd.HMACKeyConfig{KeyFile: "../test/secrets/sfe_unpause_key"})
	test.AssertNotError(t, err, "unexpected error from NewJWTSigner()")

	config := cmd.HMACKeyConfig{KeyFile: "../test/secrets/sfe_unpause_key"}
	hmacKey, err := config.Load()
	test.AssertNotError(t, err, "unexpected error from Load()")

	type args struct {
		key      []byte
		version  string
		account  int64
		idents   []string
		lifetime time.Duration
		clk      clock.Clock
	}

	tests := []struct {
		name               string
		args               args
		want               JWTClaims
		wantGenerateJWTErr bool
		wantRedeemJWTErr   bool
	}{
		{
			name: "valid one identifier",
			args: args{
				key:      hmacKey,
				version:  APIVersion,
				account:  1234567890,
				idents:   []string{"example.com"},
				lifetime: time.Hour,
				clk:      fc,
			},
			want: JWTClaims{
				Claims: jwt.Claims{
					Issuer:   defaultIssuer,
					Subject:  "1234567890",
					Audience: jwt.Audience{defaultAudience},
					Expiry:   jwt.NewNumericDate(fc.Now().Add(time.Hour)),
				},
				V: APIVersion,
				I: "example.com",
			},
			wantGenerateJWTErr: false,
			wantRedeemJWTErr:   false,
		},
		{
			name: "valid multiple identifiers",
			args: args{
				key:      hmacKey,
				version:  APIVersion,
				account:  1234567890,
				idents:   []string{"example.com", "example.org", "example.net"},
				lifetime: time.Hour,
				clk:      fc,
			},
			want: JWTClaims{
				Claims: jwt.Claims{
					Issuer:   defaultIssuer,
					Subject:  "1234567890",
					Audience: jwt.Audience{defaultAudience},
					Expiry:   jwt.NewNumericDate(fc.Now().Add(time.Hour)),
				},
				V: APIVersion,
				I: "example.com,example.org,example.net",
			},
			wantGenerateJWTErr: false,
			wantRedeemJWTErr:   false,
		},
		{
			name: "invalid no account",
			args: args{
				key:      hmacKey,
				version:  APIVersion,
				account:  0,
				idents:   []string{"example.com"},
				lifetime: time.Hour,
				clk:      fc,
			},
			want:               JWTClaims{},
			wantGenerateJWTErr: false,
			wantRedeemJWTErr:   true,
		},
		{
			// This test is only testing the "key too small" case for RedeemJWT
			// because the "key too small" case for GenerateJWT is handled when
			// the key is loaded to initialize a signer.
			name: "invalid key too small",
			args: args{
				key:      []byte("key"),
				version:  APIVersion,
				account:  1234567890,
				idents:   []string{"example.com"},
				lifetime: time.Hour,
				clk:      fc,
			},
			want:               JWTClaims{},
			wantGenerateJWTErr: false,
			wantRedeemJWTErr:   true,
		},
		{
			name: "invalid no identifiers",
			args: args{
				key:      hmacKey,
				version:  APIVersion,
				account:  1234567890,
				idents:   nil,
				lifetime: time.Hour,
				clk:      fc,
			},
			want:               JWTClaims{},
			wantGenerateJWTErr: false,
			wantRedeemJWTErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			token, err := GenerateJWT(signer, tt.args.account, tt.args.idents, tt.args.lifetime, tt.args.clk)
			if tt.wantGenerateJWTErr {
				test.AssertError(t, err, "expected error from GenerateJWT()")
				return
			}
			test.AssertNotError(t, err, "unexpected error from GenerateJWT()")

			got, err := RedeemJWT(token, tt.args.key, tt.args.version, tt.args.clk)
			if tt.wantRedeemJWTErr {
				test.AssertError(t, err, "expected error from RedeemJWT()")
				return
			}
			test.AssertNotError(t, err, "unexpected error from RedeemJWT()")
			test.AssertEquals(t, got.Issuer, tt.want.Issuer)
			test.AssertEquals(t, got.Subject, tt.want.Subject)
			test.AssertDeepEquals(t, got.Audience, tt.want.Audience)
			test.Assert(t, got.Expiry.Time().Equal(tt.want.Expiry.Time()), "expected Expiry time to be equal")
			test.AssertEquals(t, got.V, tt.want.V)
			test.AssertEquals(t, got.I, tt.want.I)
		})
	}
}
