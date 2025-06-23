package sa

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/base64"
	"fmt"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/letsencrypt/boulder/db"
	"github.com/letsencrypt/boulder/features"
	"github.com/letsencrypt/boulder/grpc"
	"github.com/letsencrypt/boulder/probs"
	"github.com/letsencrypt/boulder/test/vars"

	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	"github.com/letsencrypt/boulder/test"
)

func TestRegistrationModelToPb(t *testing.T) {
	badCases := []struct {
		name  string
		input regModel
	}{
		{
			name:  "No ID",
			input: regModel{ID: 0, Key: []byte("foo"), InitialIP: []byte("foo")},
		},
		{
			name:  "No Key",
			input: regModel{ID: 1, Key: nil, InitialIP: []byte("foo")},
		},
		{
			name:  "No IP",
			input: regModel{ID: 1, Key: []byte("foo"), InitialIP: nil},
		},
		{
			name:  "Bad IP",
			input: regModel{ID: 1, Key: []byte("foo"), InitialIP: []byte("foo")},
		},
	}
	for _, tc := range badCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := registrationModelToPb(&tc.input)
			test.AssertError(t, err, "Should fail")
		})
	}

	_, err := registrationModelToPb(&regModel{
		ID: 1, Key: []byte("foo"), InitialIP: net.ParseIP("1.2.3.4"),
	})
	test.AssertNotError(t, err, "Should pass")
}

func TestRegistrationPbToModel(t *testing.T) {}

func TestAuthzModel(t *testing.T) {
	clk := clock.New()
	now := clk.Now()
	expires := now.Add(24 * time.Hour)
	authzPB := &corepb.Authorization{
		Id:             "1",
		Identifier:     "example.com",
		RegistrationID: 1,
		Status:         string(core.StatusValid),
		Expires:        timestamppb.New(expires),
		Challenges: []*corepb.Challenge{
			{
				Type:      string(core.ChallengeTypeHTTP01),
				Status:    string(core.StatusValid),
				Token:     "MTIz",
				Validated: timestamppb.New(now),
				Validationrecords: []*corepb.ValidationRecord{
					{
						AddressUsed:       []byte("1.2.3.4"),
						Url:               "https://example.com",
						Hostname:          "example.com",
						Port:              "443",
						AddressesResolved: [][]byte{{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4}},
						AddressesTried:    [][]byte{{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4}},
					},
				},
			},
		},
	}

	model, err := authzPBToModel(authzPB)
	test.AssertNotError(t, err, "authzPBToModel failed")

	authzPBOut, err := modelToAuthzPB(*model)
	test.AssertNotError(t, err, "modelToAuthzPB failed")
	if authzPB.Challenges[0].Validationrecords[0].Hostname != "" {
		test.Assert(t, false, fmt.Sprintf("dehydrated http-01 validation record expected hostname field to be missing, but found %v", authzPB.Challenges[0].Validationrecords[0].Hostname))
	}
	if authzPB.Challenges[0].Validationrecords[0].Port != "" {
		test.Assert(t, false, fmt.Sprintf("rehydrated http-01 validation record expected port field to be missing, but found %v", authzPB.Challenges[0].Validationrecords[0].Port))
	}
	// Shoving the Hostname and Port backinto the validation record should
	// succeed because authzPB validation record will should match the retrieved
	// model from the database with the rehydrated Hostname and Port.
	authzPB.Challenges[0].Validationrecords[0].Hostname = "example.com"
	authzPB.Challenges[0].Validationrecords[0].Port = "443"
	test.AssertDeepEquals(t, authzPB.Challenges, authzPBOut.Challenges)

	now = clk.Now()
	expires = now.Add(24 * time.Hour)
	authzPB = &corepb.Authorization{
		Id:             "1",
		Identifier:     "example.com",
		RegistrationID: 1,
		Status:         string(core.StatusValid),
		Expires:        timestamppb.New(expires),
		Challenges: []*corepb.Challenge{
			{
				Type:      string(core.ChallengeTypeHTTP01),
				Status:    string(core.StatusValid),
				Token:     "MTIz",
				Validated: timestamppb.New(now),
				Validationrecords: []*corepb.ValidationRecord{
					{
						AddressUsed:       []byte("1.2.3.4"),
						Url:               "https://example.com",
						Hostname:          "example.com",
						Port:              "443",
						AddressesResolved: [][]byte{{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4}},
						AddressesTried:    [][]byte{{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4}},
					},
				},
			},
		},
	}

	validationErr := probs.Connection("weewoo")

	authzPB.Challenges[0].Status = string(core.StatusInvalid)
	authzPB.Challenges[0].Error, err = grpc.ProblemDetailsToPB(validationErr)
	test.AssertNotError(t, err, "grpc.ProblemDetailsToPB failed")
	model, err = authzPBToModel(authzPB)
	test.AssertNotError(t, err, "authzPBToModel failed")

	authzPBOut, err = modelToAuthzPB(*model)
	test.AssertNotError(t, err, "modelToAuthzPB failed")
	if authzPB.Challenges[0].Validationrecords[0].Hostname != "" {
		test.Assert(t, false, fmt.Sprintf("dehydrated http-01 validation record expected hostname field to be missing, but found %v", authzPB.Challenges[0].Validationrecords[0].Hostname))
	}
	if authzPB.Challenges[0].Validationrecords[0].Port != "" {
		test.Assert(t, false, fmt.Sprintf("rehydrated http-01 validation record expected port field to be missing, but found %v", authzPB.Challenges[0].Validationrecords[0].Port))
	}
	// Shoving the Hostname and Port back into the validation record should
	// succeed because authzPB validation record will should match the retrieved
	// model from the database with the rehydrated Hostname and Port.
	authzPB.Challenges[0].Validationrecords[0].Hostname = "example.com"
	authzPB.Challenges[0].Validationrecords[0].Port = "443"
	test.AssertDeepEquals(t, authzPB.Challenges, authzPBOut.Challenges)

	now = clk.Now()
	expires = now.Add(24 * time.Hour)
	authzPB = &corepb.Authorization{
		Id:             "1",
		Identifier:     "example.com",
		RegistrationID: 1,
		Status:         string(core.StatusInvalid),
		Expires:        timestamppb.New(expires),
		Challenges: []*corepb.Challenge{
			{
				Type:   string(core.ChallengeTypeHTTP01),
				Status: string(core.StatusInvalid),
				Token:  "MTIz",
				Validationrecords: []*corepb.ValidationRecord{
					{
						AddressUsed:       []byte("1.2.3.4"),
						Url:               "url",
						AddressesResolved: [][]byte{{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4}},
						AddressesTried:    [][]byte{{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4}},
					},
				},
			},
			{
				Type:   string(core.ChallengeTypeDNS01),
				Status: string(core.StatusInvalid),
				Token:  "MTIz",
				Validationrecords: []*corepb.ValidationRecord{
					{
						AddressUsed:       []byte("1.2.3.4"),
						Url:               "url",
						AddressesResolved: [][]byte{{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4}},
						AddressesTried:    [][]byte{{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4}},
					},
				},
			},
		},
	}
	_, err = authzPBToModel(authzPB)
	test.AssertError(t, err, "authzPBToModel didn't fail with multiple non-pending challenges")

	// Test that the caller Hostname and Port rehydration returns the expected data in the expected fields.
	now = clk.Now()
	expires = now.Add(24 * time.Hour)
	authzPB = &corepb.Authorization{
		Id:             "1",
		Identifier:     "example.com",
		RegistrationID: 1,
		Status:         string(core.StatusValid),
		Expires:        timestamppb.New(expires),
		Challenges: []*corepb.Challenge{
			{
				Type:      string(core.ChallengeTypeHTTP01),
				Status:    string(core.StatusValid),
				Token:     "MTIz",
				Validated: timestamppb.New(now),
				Validationrecords: []*corepb.ValidationRecord{
					{
						AddressUsed:       []byte("1.2.3.4"),
						Url:               "https://example.com",
						AddressesResolved: [][]byte{{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4}},
						AddressesTried:    [][]byte{{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4}},
					},
				},
			},
		},
	}

	model, err = authzPBToModel(authzPB)
	test.AssertNotError(t, err, "authzPBToModel failed")

	authzPBOut, err = modelToAuthzPB(*model)
	test.AssertNotError(t, err, "modelToAuthzPB failed")
	if authzPBOut.Challenges[0].Validationrecords[0].Hostname != "example.com" {
		test.Assert(t, false, fmt.Sprintf("rehydrated http-01 validation record expected hostname example.com but found %v", authzPBOut.Challenges[0].Validationrecords[0].Hostname))
	}
	if authzPBOut.Challenges[0].Validationrecords[0].Port != "443" {
		test.Assert(t, false, fmt.Sprintf("rehydrated http-01 validation record expected port 443 but found %v", authzPBOut.Challenges[0].Validationrecords[0].Port))
	}
}

// TestModelToOrderBADJSON tests that converting an order model with an invalid
// validation error JSON field to an Order produces the expected bad JSON error.
func TestModelToOrderBadJSON(t *testing.T) {
	badJSON := []byte(`{`)
	_, err := modelToOrderv2(&orderModelv2{
		Error: badJSON,
	})
	test.AssertError(t, err, "expected error from modelToOrderv2")
	var badJSONErr errBadJSON
	test.AssertErrorWraps(t, err, &badJSONErr)
	test.AssertEquals(t, string(badJSONErr.json), string(badJSON))
}

func TestOrderModelThereAndBackAgain(t *testing.T) {
	clk := clock.New()
	now := clk.Now()
	order := &corepb.Order{
		Id:                0,
		RegistrationID:    2016,
		Expires:           timestamppb.New(now.Add(24 * time.Hour)),
		Created:           timestamppb.New(now),
		Error:             nil,
		CertificateSerial: "1",
		BeganProcessing:   true,
	}
	model1, err := orderToModelv1(order)
	test.AssertNotError(t, err, "orderToModelv1 should not have errored")
	returnOrder, err := modelToOrderv1(model1)
	test.AssertNotError(t, err, "modelToOrderv1 should not have errored")
	test.AssertDeepEquals(t, order, returnOrder)

	anotherOrder := &corepb.Order{
		Id:                     1,
		RegistrationID:         2024,
		Expires:                timestamppb.New(now.Add(24 * time.Hour)),
		Created:                timestamppb.New(now),
		Error:                  nil,
		CertificateSerial:      "2",
		BeganProcessing:        true,
		CertificateProfileName: "phljny",
	}
	model2, err := orderToModelv2(anotherOrder)
	test.AssertNotError(t, err, "orderToModelv2 should not have errored")
	returnOrder, err = modelToOrderv2(model2)
	test.AssertNotError(t, err, "modelToOrderv2 should not have errored")
	test.AssertDeepEquals(t, anotherOrder, returnOrder)
}

// TestPopulateAttemptedFieldsBadJSON tests that populating a challenge from an
// authz2 model with an invalid validation error or an invalid validation record
// produces the expected bad JSON error.
func TestPopulateAttemptedFieldsBadJSON(t *testing.T) {
	badJSON := []byte(`{`)

	testCases := []struct {
		Name  string
		Model *authzModel
	}{
		{
			Name: "Bad validation error field",
			Model: &authzModel{
				ValidationError: badJSON,
			},
		},
		{
			Name: "Bad validation record field",
			Model: &authzModel{
				ValidationRecord: badJSON,
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			err := populateAttemptedFields(*tc.Model, &corepb.Challenge{})
			test.AssertError(t, err, "expected error from populateAttemptedFields")
			var badJSONErr errBadJSON
			test.AssertErrorWraps(t, err, &badJSONErr)
			test.AssertEquals(t, string(badJSONErr.json), string(badJSON))
		})
	}
}

func TestCertificatesTableContainsDuplicateSerials(t *testing.T) {
	ctx := context.Background()

	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	serialString := core.SerialToString(big.NewInt(1337))

	// Insert a certificate with a serial of `1337`.
	err := insertCertificate(ctx, sa.dbMap, fc, "1337.com", "leet", 1337, 1)
	test.AssertNotError(t, err, "couldn't insert valid certificate")

	// This should return the certificate that we just inserted.
	certA, err := SelectCertificate(ctx, sa.dbMap, serialString)
	test.AssertNotError(t, err, "received an error for a valid query")

	// Insert a certificate with a serial of `1337` but for a different
	// hostname.
	err = insertCertificate(ctx, sa.dbMap, fc, "1337.net", "leet", 1337, 1)
	test.AssertNotError(t, err, "couldn't insert valid certificate")

	// Despite a duplicate being present, this shouldn't error.
	certB, err := SelectCertificate(ctx, sa.dbMap, serialString)
	test.AssertNotError(t, err, "received an error for a valid query")

	// Ensure that `certA` and `certB` are the same.
	test.AssertByteEquals(t, certA.DER, certB.DER)
}

func insertCertificate(ctx context.Context, dbMap *db.WrappedMap, fc clock.FakeClock, hostname, cn string, serial, regID int64) error {
	serialBigInt := big.NewInt(serial)
	serialString := core.SerialToString(serialBigInt)

	template := x509.Certificate{
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotAfter:     fc.Now().Add(30 * 24 * time.Hour),
		DNSNames:     []string{hostname},
		SerialNumber: serialBigInt,
	}

	testKey := makeKey()
	certDer, _ := x509.CreateCertificate(rand.Reader, &template, &template, &testKey.PublicKey, &testKey)
	cert := &core.Certificate{
		RegistrationID: regID,
		Serial:         serialString,
		Expires:        template.NotAfter,
		DER:            certDer,
	}
	err := dbMap.Insert(ctx, cert)
	if err != nil {
		return err
	}
	return nil
}

func bigIntFromB64(b64 string) *big.Int {
	bytes, _ := base64.URLEncoding.DecodeString(b64)
	x := big.NewInt(0)
	x.SetBytes(bytes)
	return x
}

func makeKey() rsa.PrivateKey {
	n := bigIntFromB64("n4EPtAOCc9AlkeQHPzHStgAbgs7bTZLwUBZdR8_KuKPEHLd4rHVTeT-O-XV2jRojdNhxJWTDvNd7nqQ0VEiZQHz_AJmSCpMaJMRBSFKrKb2wqVwGU_NsYOYL-QtiWN2lbzcEe6XC0dApr5ydQLrHqkHHig3RBordaZ6Aj-oBHqFEHYpPe7Tpe-OfVfHd1E6cS6M1FZcD1NNLYD5lFHpPI9bTwJlsde3uhGqC0ZCuEHg8lhzwOHrtIQbS0FVbb9k3-tVTU4fg_3L_vniUFAKwuCLqKnS2BYwdq_mzSnbLY7h_qixoR7jig3__kRhuaxwUkRz5iaiQkqgc5gHdrNP5zw==")
	e := int(bigIntFromB64("AQAB").Int64())
	d := bigIntFromB64("bWUC9B-EFRIo8kpGfh0ZuyGPvMNKvYWNtB_ikiH9k20eT-O1q_I78eiZkpXxXQ0UTEs2LsNRS-8uJbvQ-A1irkwMSMkK1J3XTGgdrhCku9gRldY7sNA_AKZGh-Q661_42rINLRCe8W-nZ34ui_qOfkLnK9QWDDqpaIsA-bMwWWSDFu2MUBYwkHTMEzLYGqOe04noqeq1hExBTHBOBdkMXiuFhUq1BU6l-DqEiWxqg82sXt2h-LMnT3046AOYJoRioz75tSUQfGCshWTBnP5uDjd18kKhyv07lhfSJdrPdM5Plyl21hsFf4L_mHCuoFau7gdsPfHPxxjVOcOpBrQzwQ==")
	p := bigIntFromB64("uKE2dh-cTf6ERF4k4e_jy78GfPYUIaUyoSSJuBzp3Cubk3OCqs6grT8bR_cu0Dm1MZwWmtdqDyI95HrUeq3MP15vMMON8lHTeZu2lmKvwqW7anV5UzhM1iZ7z4yMkuUwFWoBvyY898EXvRD-hdqRxHlSqAZ192zB3pVFJ0s7pFc=")
	q := bigIntFromB64("uKE2dh-cTf6ERF4k4e_jy78GfPYUIaUyoSSJuBzp3Cubk3OCqs6grT8bR_cu0Dm1MZwWmtdqDyI95HrUeq3MP15vMMON8lHTeZu2lmKvwqW7anV5UzhM1iZ7z4yMkuUwFWoBvyY898EXvRD-hdqRxHlSqAZ192zB3pVFJ0s7pFc=")
	return rsa.PrivateKey{PublicKey: rsa.PublicKey{N: n, E: e}, D: d, Primes: []*big.Int{p, q}}
}

func TestIncidentSerialModel(t *testing.T) {
	ctx := context.Background()

	testIncidentsDbMap, err := DBMapForTest(vars.DBConnIncidentsFullPerms)
	test.AssertNotError(t, err, "Couldn't create test dbMap")
	defer test.ResetIncidentsTestDatabase(t)

	// Inserting and retrieving a row with only the serial populated should work.
	_, err = testIncidentsDbMap.ExecContext(ctx,
		"INSERT INTO incident_foo (serial) VALUES (?)",
		"1337",
	)
	test.AssertNotError(t, err, "inserting row with only serial")

	var res1 incidentSerialModel
	err = testIncidentsDbMap.SelectOne(
		ctx,
		&res1,
		"SELECT * FROM incident_foo WHERE serial = ?",
		"1337",
	)
	test.AssertNotError(t, err, "selecting row with only serial")

	test.AssertEquals(t, res1.Serial, "1337")
	test.AssertBoxedNil(t, res1.RegistrationID, "registrationID should be NULL")
	test.AssertBoxedNil(t, res1.OrderID, "orderID should be NULL")
	test.AssertBoxedNil(t, res1.LastNoticeSent, "lastNoticeSent should be NULL")

	// Inserting and retrieving a row with all columns populated should work.
	_, err = testIncidentsDbMap.ExecContext(ctx,
		"INSERT INTO incident_foo (serial, registrationID, orderID, lastNoticeSent) VALUES (?, ?, ?, ?)",
		"1338",
		1,
		2,
		time.Date(2023, 06, 29, 16, 9, 00, 00, time.UTC),
	)
	test.AssertNotError(t, err, "inserting row with only serial")

	var res2 incidentSerialModel
	err = testIncidentsDbMap.SelectOne(
		ctx,
		&res2,
		"SELECT * FROM incident_foo WHERE serial = ?",
		"1338",
	)
	test.AssertNotError(t, err, "selecting row with only serial")

	test.AssertEquals(t, res2.Serial, "1338")
	test.AssertEquals(t, *res2.RegistrationID, int64(1))
	test.AssertEquals(t, *res2.OrderID, int64(2))
	test.AssertEquals(t, *res2.LastNoticeSent, time.Date(2023, 06, 29, 16, 9, 00, 00, time.UTC))
}

func TestAddReplacementOrder(t *testing.T) {
	if os.Getenv("BOULDER_CONFIG_DIR") != "test/config-next" {
		t.Skip("Test requires replacementOrders database table")
	}

	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	features.Set(features.Config{TrackReplacementCertificatesARI: true})
	defer features.Reset()

	oldCertSerial := "1234567890"
	orderId := int64(1337)
	orderExpires := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)

	// Add a replacement order which doesn't exist.
	err := addReplacementOrder(ctx, sa.dbMap, oldCertSerial, orderId, orderExpires)
	test.AssertNotError(t, err, "addReplacementOrder failed")

	// Fetch the replacement order so we can ensure it was added.
	var replacementRow replacementOrderModel
	err = sa.dbReadOnlyMap.SelectOne(
		ctx,
		&replacementRow,
		"SELECT * FROM replacementOrders WHERE serial = ? LIMIT 1",
		oldCertSerial,
	)
	test.AssertNotError(t, err, "SELECT from replacementOrders failed")
	test.AssertEquals(t, oldCertSerial, replacementRow.Serial)
	test.AssertEquals(t, orderId, replacementRow.OrderID)
	test.AssertEquals(t, orderExpires, replacementRow.OrderExpires)

	nextOrderId := int64(1338)
	nextOrderExpires := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)

	// Add a replacement order which already exists.
	err = addReplacementOrder(ctx, sa.dbMap, oldCertSerial, nextOrderId, nextOrderExpires)
	test.AssertNotError(t, err, "addReplacementOrder failed")

	// Fetch the replacement order so we can ensure it was updated.
	err = sa.dbReadOnlyMap.SelectOne(
		ctx,
		&replacementRow,
		"SELECT * FROM replacementOrders WHERE serial = ? LIMIT 1",
		oldCertSerial,
	)
	test.AssertNotError(t, err, "SELECT from replacementOrders failed")
	test.AssertEquals(t, oldCertSerial, replacementRow.Serial)
	test.AssertEquals(t, nextOrderId, replacementRow.OrderID)
	test.AssertEquals(t, nextOrderExpires, replacementRow.OrderExpires)
}

func TestSetReplacementOrderFinalized(t *testing.T) {
	if os.Getenv("BOULDER_CONFIG_DIR") != "test/config-next" {
		t.Skip("Test requires replacementOrders database table")
	}

	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	features.Set(features.Config{TrackReplacementCertificatesARI: true})
	defer features.Reset()

	oldCertSerial := "1234567890"
	orderId := int64(1337)
	orderExpires := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)

	// Mark a non-existent certificate as finalized/replaced.
	err := setReplacementOrderFinalized(ctx, sa.dbMap, orderId)
	test.AssertNotError(t, err, "setReplacementOrderFinalized failed")

	// Ensure no replacement order was added for some reason.
	var replacementRow replacementOrderModel
	err = sa.dbReadOnlyMap.SelectOne(
		ctx,
		&replacementRow,
		"SELECT * FROM replacementOrders WHERE serial = ? LIMIT 1",
		oldCertSerial,
	)
	test.AssertErrorIs(t, err, sql.ErrNoRows)

	// Add a replacement order.
	err = addReplacementOrder(ctx, sa.dbMap, oldCertSerial, orderId, orderExpires)
	test.AssertNotError(t, err, "addReplacementOrder failed")

	// Mark the certificate as finalized/replaced.
	err = setReplacementOrderFinalized(ctx, sa.dbMap, orderId)
	test.AssertNotError(t, err, "setReplacementOrderFinalized failed")

	// Fetch the replacement order so we can ensure it was finalized.
	err = sa.dbReadOnlyMap.SelectOne(
		ctx,
		&replacementRow,
		"SELECT * FROM replacementOrders WHERE serial = ? LIMIT 1",
		oldCertSerial,
	)
	test.AssertNotError(t, err, "SELECT from replacementOrders failed")
	test.Assert(t, replacementRow.Replaced, "replacement order should be marked as finalized")
}
