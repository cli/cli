package grpc

import (
	"encoding/json"
	"net/netip"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/probs"
	"github.com/letsencrypt/boulder/test"
)

const JWK1JSON = `{"kty":"RSA","n":"vuc785P8lBj3fUxyZchF_uZw6WtbxcorqgTyq-qapF5lrO1U82Tp93rpXlmctj6fyFHBVVB5aXnUHJ7LZeVPod7Wnfl8p5OyhlHQHC8BnzdzCqCMKmWZNX5DtETDId0qzU7dPzh0LP0idt5buU7L9QNaabChw3nnaL47iu_1Di5Wp264p2TwACeedv2hfRDjDlJmaQXuS8Rtv9GnRWyC9JBu7XmGvGDziumnJH7Hyzh3VNu-kSPQD3vuAFgMZS6uUzOztCkT0fpOalZI6hqxtWLvXUMj-crXrn-Maavz8qRhpAyp5kcYk3jiHGgQIi7QSK2JIdRJ8APyX9HlmTN5AQ","e":"AQAB"}`

func TestProblemDetails(t *testing.T) {
	pb, err := ProblemDetailsToPB(nil)
	test.AssertNotEquals(t, err, "problemDetailToPB failed")
	test.Assert(t, pb == nil, "Returned corepb.ProblemDetails is not nil")

	prob := &probs.ProblemDetails{Type: probs.TLSProblem, Detail: "asd", HTTPStatus: 200}
	pb, err = ProblemDetailsToPB(prob)
	test.AssertNotError(t, err, "problemDetailToPB failed")
	test.Assert(t, pb != nil, "return corepb.ProblemDetails is nill")
	test.AssertDeepEquals(t, pb.ProblemType, string(prob.Type))
	test.AssertEquals(t, pb.Detail, prob.Detail)
	test.AssertEquals(t, int(pb.HttpStatus), prob.HTTPStatus)

	recon, err := PBToProblemDetails(pb)
	test.AssertNotError(t, err, "PBToProblemDetails failed")
	test.AssertDeepEquals(t, recon, prob)

	recon, err = PBToProblemDetails(nil)
	test.AssertNotError(t, err, "PBToProblemDetails failed")
	test.Assert(t, recon == nil, "Returned core.PRoblemDetails is not nil")
	_, err = PBToProblemDetails(&corepb.ProblemDetails{})
	test.AssertError(t, err, "PBToProblemDetails did not fail")
	test.AssertEquals(t, err, ErrMissingParameters)
	_, err = PBToProblemDetails(&corepb.ProblemDetails{ProblemType: ""})
	test.AssertError(t, err, "PBToProblemDetails did not fail")
	test.AssertEquals(t, err, ErrMissingParameters)
	_, err = PBToProblemDetails(&corepb.ProblemDetails{Detail: ""})
	test.AssertError(t, err, "PBToProblemDetails did not fail")
	test.AssertEquals(t, err, ErrMissingParameters)
}

func TestChallenge(t *testing.T) {
	var jwk jose.JSONWebKey
	err := json.Unmarshal([]byte(JWK1JSON), &jwk)
	test.AssertNotError(t, err, "Failed to unmarshal test key")
	validated := time.Now().Round(0).UTC()
	chall := core.Challenge{
		Type:      core.ChallengeTypeDNS01,
		Status:    core.StatusValid,
		Token:     "asd",
		Validated: &validated,
	}

	pb, err := ChallengeToPB(chall)
	test.AssertNotError(t, err, "ChallengeToPB failed")
	test.Assert(t, pb != nil, "Returned corepb.Challenge is nil")

	recon, err := PBToChallenge(pb)
	test.AssertNotError(t, err, "PBToChallenge failed")
	test.AssertDeepEquals(t, recon, chall)

	ip := netip.MustParseAddr("1.1.1.1")
	chall.ValidationRecord = []core.ValidationRecord{
		{
			Hostname:          "example.com",
			Port:              "2020",
			AddressesResolved: []netip.Addr{ip},
			AddressUsed:       ip,
			URL:               "https://example.com:2020",
			AddressesTried:    []netip.Addr{ip},
		},
	}
	chall.Error = &probs.ProblemDetails{Type: probs.TLSProblem, Detail: "asd", HTTPStatus: 200}
	pb, err = ChallengeToPB(chall)
	test.AssertNotError(t, err, "ChallengeToPB failed")
	test.Assert(t, pb != nil, "Returned corepb.Challenge is nil")

	recon, err = PBToChallenge(pb)
	test.AssertNotError(t, err, "PBToChallenge failed")
	test.AssertDeepEquals(t, recon, chall)

	_, err = PBToChallenge(nil)
	test.AssertError(t, err, "PBToChallenge did not fail")
	test.AssertEquals(t, err, ErrMissingParameters)
	_, err = PBToChallenge(&corepb.Challenge{})
	test.AssertError(t, err, "PBToChallenge did not fail")
	test.AssertEquals(t, err, ErrMissingParameters)

	challNilValidation := core.Challenge{
		Type:      core.ChallengeTypeDNS01,
		Status:    core.StatusValid,
		Token:     "asd",
		Validated: nil,
	}
	pb, err = ChallengeToPB(challNilValidation)
	test.AssertNotError(t, err, "ChallengeToPB failed")
	test.Assert(t, pb != nil, "Returned corepb.Challenge is nil")
	recon, err = PBToChallenge(pb)
	test.AssertNotError(t, err, "PBToChallenge failed")
	test.AssertDeepEquals(t, recon, challNilValidation)
}

func TestValidationRecord(t *testing.T) {
	ip := netip.MustParseAddr("1.1.1.1")
	vr := core.ValidationRecord{
		Hostname:          "exampleA.com",
		Port:              "80",
		AddressesResolved: []netip.Addr{ip},
		AddressUsed:       ip,
		URL:               "http://exampleA.com",
		AddressesTried:    []netip.Addr{ip},
		ResolverAddrs:     []string{"resolver:5353"},
	}

	pb, err := ValidationRecordToPB(vr)
	test.AssertNotError(t, err, "ValidationRecordToPB failed")
	test.Assert(t, pb != nil, "Return core.ValidationRecord is nil")

	recon, err := PBToValidationRecord(pb)
	test.AssertNotError(t, err, "PBToValidationRecord failed")
	test.AssertDeepEquals(t, recon, vr)
}

func TestValidationResult(t *testing.T) {
	ip := netip.MustParseAddr("1.1.1.1")
	vrA := core.ValidationRecord{
		Hostname:          "exampleA.com",
		Port:              "443",
		AddressesResolved: []netip.Addr{ip},
		AddressUsed:       ip,
		URL:               "https://exampleA.com",
		AddressesTried:    []netip.Addr{ip},
		ResolverAddrs:     []string{"resolver:5353"},
	}
	vrB := core.ValidationRecord{
		Hostname:          "exampleB.com",
		Port:              "443",
		AddressesResolved: []netip.Addr{ip},
		AddressUsed:       ip,
		URL:               "https://exampleB.com",
		AddressesTried:    []netip.Addr{ip},
		ResolverAddrs:     []string{"resolver:5353"},
	}
	result := []core.ValidationRecord{vrA, vrB}
	prob := &probs.ProblemDetails{Type: probs.TLSProblem, Detail: "asd", HTTPStatus: 200}

	pb, err := ValidationResultToPB(result, prob, "surreal", "ARIN")
	test.AssertNotError(t, err, "ValidationResultToPB failed")
	test.Assert(t, pb != nil, "Returned vapb.ValidationResult is nil")
	test.AssertEquals(t, pb.Perspective, "surreal")
	test.AssertEquals(t, pb.Rir, "ARIN")

	reconResult, reconProb, err := pbToValidationResult(pb)
	test.AssertNotError(t, err, "pbToValidationResult failed")
	test.AssertDeepEquals(t, reconResult, result)
	test.AssertDeepEquals(t, reconProb, prob)
}

func TestRegistration(t *testing.T) {
	contacts := []string{"email"}
	var key jose.JSONWebKey
	err := json.Unmarshal([]byte(`
		{
			"e": "AQAB",
			"kty": "RSA",
			"n": "tSwgy3ORGvc7YJI9B2qqkelZRUC6F1S5NwXFvM4w5-M0TsxbFsH5UH6adigV0jzsDJ5imAechcSoOhAh9POceCbPN1sTNwLpNbOLiQQ7RD5mY_pSUHWXNmS9R4NZ3t2fQAzPeW7jOfF0LKuJRGkekx6tXP1uSnNibgpJULNc4208dgBaCHo3mvaE2HV2GmVl1yxwWX5QZZkGQGjNDZYnjFfa2DKVvFs0QbAk21ROm594kAxlRlMMrvqlf24Eq4ERO0ptzpZgm_3j_e4hGRD39gJS7kAzK-j2cacFQ5Qi2Y6wZI2p-FCq_wiYsfEAIkATPBiLKl_6d_Jfcvs_impcXQ"
		}
	`), &key)
	test.AssertNotError(t, err, "Could not unmarshal testing key")
	createdAt := time.Now().Round(0).UTC()
	inReg := core.Registration{
		ID:        1,
		Key:       &key,
		Contact:   &contacts,
		Agreement: "yup",
		CreatedAt: &createdAt,
		Status:    core.StatusValid,
	}
	pbReg, err := RegistrationToPB(inReg)
	test.AssertNotError(t, err, "registrationToPB failed")
	outReg, err := PbToRegistration(pbReg)
	test.AssertNotError(t, err, "PbToRegistration failed")
	test.AssertDeepEquals(t, inReg, outReg)

	inReg.Contact = nil
	pbReg, err = RegistrationToPB(inReg)
	test.AssertNotError(t, err, "registrationToPB failed")
	pbReg.Contact = []string{}
	outReg, err = PbToRegistration(pbReg)
	test.AssertNotError(t, err, "PbToRegistration failed")
	test.AssertDeepEquals(t, inReg, outReg)

	var empty []string
	inReg.Contact = &empty
	pbReg, err = RegistrationToPB(inReg)
	test.AssertNotError(t, err, "registrationToPB failed")
	outReg, err = PbToRegistration(pbReg)
	test.AssertNotError(t, err, "PbToRegistration failed")
	if outReg.Contact != nil {
		t.Errorf("Empty contacts should be a nil slice")
	}

	inRegNilCreatedAt := core.Registration{
		ID:        1,
		Key:       &key,
		Contact:   &contacts,
		Agreement: "yup",
		CreatedAt: nil,
		Status:    core.StatusValid,
	}
	pbReg, err = RegistrationToPB(inRegNilCreatedAt)
	test.AssertNotError(t, err, "registrationToPB failed")
	outReg, err = PbToRegistration(pbReg)
	test.AssertNotError(t, err, "PbToRegistration failed")
	test.AssertDeepEquals(t, inRegNilCreatedAt, outReg)
}

func TestAuthz(t *testing.T) {
	exp := time.Now().AddDate(0, 0, 1).UTC()
	ident := identifier.NewDNS("example.com")
	challA := core.Challenge{
		Type:   core.ChallengeTypeDNS01,
		Status: core.StatusPending,
		Token:  "asd",
	}
	challB := core.Challenge{
		Type:   core.ChallengeTypeDNS01,
		Status: core.StatusPending,
		Token:  "asd2",
	}
	inAuthz := core.Authorization{
		ID:             "1",
		Identifier:     ident,
		RegistrationID: 5,
		Status:         core.StatusPending,
		Expires:        &exp,
		Challenges:     []core.Challenge{challA, challB},
	}
	pbAuthz, err := AuthzToPB(inAuthz)
	test.AssertNotError(t, err, "AuthzToPB failed")
	outAuthz, err := PBToAuthz(pbAuthz)
	test.AssertNotError(t, err, "PBToAuthz failed")
	test.AssertDeepEquals(t, inAuthz, outAuthz)

	inAuthzNilExpires := core.Authorization{
		ID:             "1",
		Identifier:     ident,
		RegistrationID: 5,
		Status:         core.StatusPending,
		Expires:        nil,
		Challenges:     []core.Challenge{challA, challB},
	}
	pbAuthz2, err := AuthzToPB(inAuthzNilExpires)
	test.AssertNotError(t, err, "AuthzToPB failed")
	outAuthz2, err := PBToAuthz(pbAuthz2)
	test.AssertNotError(t, err, "PBToAuthz failed")
	test.AssertDeepEquals(t, inAuthzNilExpires, outAuthz2)
}

func TestOrderValid(t *testing.T) {
	created := time.Now()
	expires := created.Add(1 * time.Hour)
	testCases := []struct {
		Name          string
		Order         *corepb.Order
		ExpectedValid bool
	}{
		{
			Name: "All valid",
			Order: &corepb.Order{
				Id:                1,
				RegistrationID:    1,
				Expires:           timestamppb.New(expires),
				CertificateSerial: "",
				V2Authorizations:  []int64{},
				Identifiers:       []*corepb.Identifier{identifier.NewDNS("example.com").ToProto()},
				BeganProcessing:   false,
				Created:           timestamppb.New(created),
			},
			ExpectedValid: true,
		},
		{
			Name: "Serial empty",
			Order: &corepb.Order{
				Id:               1,
				RegistrationID:   1,
				Expires:          timestamppb.New(expires),
				V2Authorizations: []int64{},
				Identifiers:      []*corepb.Identifier{identifier.NewDNS("example.com").ToProto()},
				BeganProcessing:  false,
				Created:          timestamppb.New(created),
			},
			ExpectedValid: true,
		},
		{
			Name:  "All zero",
			Order: &corepb.Order{},
		},
		{
			Name: "ID 0",
			Order: &corepb.Order{
				Id:                0,
				RegistrationID:    1,
				Expires:           timestamppb.New(expires),
				CertificateSerial: "",
				V2Authorizations:  []int64{},
				Identifiers:       []*corepb.Identifier{identifier.NewDNS("example.com").ToProto()},
				BeganProcessing:   false,
			},
		},
		{
			Name: "Reg ID zero",
			Order: &corepb.Order{
				Id:                1,
				RegistrationID:    0,
				Expires:           timestamppb.New(expires),
				CertificateSerial: "",
				V2Authorizations:  []int64{},
				Identifiers:       []*corepb.Identifier{identifier.NewDNS("example.com").ToProto()},
				BeganProcessing:   false,
			},
		},
		{
			Name: "Expires 0",
			Order: &corepb.Order{
				Id:                1,
				RegistrationID:    1,
				Expires:           nil,
				CertificateSerial: "",
				V2Authorizations:  []int64{},
				Identifiers:       []*corepb.Identifier{identifier.NewDNS("example.com").ToProto()},
				BeganProcessing:   false,
			},
		},
		{
			Name: "Names empty",
			Order: &corepb.Order{
				Id:                1,
				RegistrationID:    1,
				Expires:           timestamppb.New(expires),
				CertificateSerial: "",
				V2Authorizations:  []int64{},
				Identifiers:       []*corepb.Identifier{},
				BeganProcessing:   false,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := orderValid(tc.Order)
			test.AssertEquals(t, result, tc.ExpectedValid)
		})
	}
}
