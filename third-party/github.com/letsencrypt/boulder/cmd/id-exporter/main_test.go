package notmain

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/sa"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/test"
	isa "github.com/letsencrypt/boulder/test/inmem/sa"
	"github.com/letsencrypt/boulder/test/vars"
)

var (
	regA *corepb.Registration
	regB *corepb.Registration
	regC *corepb.Registration
	regD *corepb.Registration
)

const (
	emailARaw = "test@example.com"
	emailBRaw = "example@example.com"
	emailCRaw = "test-example@example.com"
	telNum    = "666-666-7777"
)

func TestFindIDs(t *testing.T) {
	ctx := context.Background()

	testCtx := setup(t)
	defer testCtx.cleanUp()

	// Add some test registrations
	testCtx.addRegistrations(t)

	// Run findIDs - since no certificates have been added corresponding to
	// the above registrations, no IDs should be found.
	results, err := testCtx.c.findIDs(ctx)
	test.AssertNotError(t, err, "findIDs() produced error")
	test.AssertEquals(t, len(results), 0)

	// Now add some certificates
	testCtx.addCertificates(t)

	// Run findIDs - since there are three registrations with unexpired certs
	// we should get exactly three IDs back: RegA, RegC and RegD. RegB should
	// *not* be present since their certificate has already expired. Unlike
	// previous versions of this test RegD is not filtered out for having a `tel:`
	// contact field anymore - this is the duty of the notify-mailer.
	results, err = testCtx.c.findIDs(ctx)
	test.AssertNotError(t, err, "findIDs() produced error")
	test.AssertEquals(t, len(results), 3)
	for _, entry := range results {
		switch entry.ID {
		case regA.Id:
		case regC.Id:
		case regD.Id:
		default:
			t.Errorf("ID: %d not expected", entry.ID)
		}
	}

	// Allow a 1 year grace period
	testCtx.c.grace = 360 * 24 * time.Hour
	results, err = testCtx.c.findIDs(ctx)
	test.AssertNotError(t, err, "findIDs() produced error")
	// Now all four registration should be returned, including RegB since its
	// certificate expired within the grace period
	for _, entry := range results {
		switch entry.ID {
		case regA.Id:
		case regB.Id:
		case regC.Id:
		case regD.Id:
		default:
			t.Errorf("ID: %d not expected", entry.ID)
		}
	}
}

func TestFindIDsWithExampleHostnames(t *testing.T) {
	ctx := context.Background()
	testCtx := setup(t)
	defer testCtx.cleanUp()

	// Add some test registrations
	testCtx.addRegistrations(t)

	// Run findIDsWithExampleHostnames - since no certificates have been
	// added corresponding to the above registrations, no IDs should be
	// found.
	results, err := testCtx.c.findIDsWithExampleHostnames(ctx)
	test.AssertNotError(t, err, "findIDs() produced error")
	test.AssertEquals(t, len(results), 0)

	// Now add some certificates
	testCtx.addCertificates(t)

	// Run findIDsWithExampleHostnames - since there are three
	// registrations with unexpired certs we should get exactly three
	// IDs back: RegA, RegC and RegD. RegB should *not* be present since
	// their certificate has already expired.
	results, err = testCtx.c.findIDsWithExampleHostnames(ctx)
	test.AssertNotError(t, err, "findIDs() produced error")
	test.AssertEquals(t, len(results), 3)
	for _, entry := range results {
		switch entry.ID {
		case regA.Id:
			test.AssertEquals(t, entry.Hostname, "example-a.com")
		case regC.Id:
			test.AssertEquals(t, entry.Hostname, "example-c.com")
		case regD.Id:
			test.AssertEquals(t, entry.Hostname, "example-d.com")
		default:
			t.Errorf("ID: %d not expected", entry.ID)
		}
	}

	// Allow a 1 year grace period
	testCtx.c.grace = 360 * 24 * time.Hour
	results, err = testCtx.c.findIDsWithExampleHostnames(ctx)
	test.AssertNotError(t, err, "findIDs() produced error")

	// Now all four registrations should be returned, including RegB
	// since it expired within the grace period
	test.AssertEquals(t, len(results), 4)
	for _, entry := range results {
		switch entry.ID {
		case regA.Id:
			test.AssertEquals(t, entry.Hostname, "example-a.com")
		case regB.Id:
			test.AssertEquals(t, entry.Hostname, "example-b.com")
		case regC.Id:
			test.AssertEquals(t, entry.Hostname, "example-c.com")
		case regD.Id:
			test.AssertEquals(t, entry.Hostname, "example-d.com")
		default:
			t.Errorf("ID: %d not expected", entry.ID)
		}
	}
}

func TestFindIDsForHostnames(t *testing.T) {
	ctx := context.Background()

	testCtx := setup(t)
	defer testCtx.cleanUp()

	// Add some test registrations
	testCtx.addRegistrations(t)

	// Run findIDsForHostnames - since no certificates have been added corresponding to
	// the above registrations, no IDs should be found.
	results, err := testCtx.c.findIDsForHostnames(ctx, []string{"example-a.com", "example-b.com", "example-c.com", "example-d.com"})
	test.AssertNotError(t, err, "findIDs() produced error")
	test.AssertEquals(t, len(results), 0)

	// Now add some certificates
	testCtx.addCertificates(t)

	results, err = testCtx.c.findIDsForHostnames(ctx, []string{"example-a.com", "example-b.com", "example-c.com", "example-d.com"})
	test.AssertNotError(t, err, "findIDsForHostnames() failed")
	test.AssertEquals(t, len(results), 3)
	for _, entry := range results {
		switch entry.ID {
		case regA.Id:
		case regC.Id:
		case regD.Id:
		default:
			t.Errorf("ID: %d not expected", entry.ID)
		}
	}
}

func TestWriteToFile(t *testing.T) {
	expected := `[{"id":1},{"id":2},{"id":3}]`
	mockResults := idExporterResults{{ID: 1}, {ID: 2}, {ID: 3}}
	dir := os.TempDir()

	f, err := os.CreateTemp(dir, "ids_test")
	test.AssertNotError(t, err, "os.CreateTemp produced an error")

	// Writing the result to an outFile should produce the correct results
	err = mockResults.writeToFile(f.Name())
	test.AssertNotError(t, err, fmt.Sprintf("writeIDs produced an error writing to %s", f.Name()))

	contents, err := os.ReadFile(f.Name())
	test.AssertNotError(t, err, fmt.Sprintf("os.ReadFile produced an error reading from %s", f.Name()))

	test.AssertEquals(t, string(contents), expected+"\n")
}

func Test_unmarshalHostnames(t *testing.T) {
	testDir := os.TempDir()
	testFile, err := os.CreateTemp(testDir, "ids_test")
	test.AssertNotError(t, err, "os.CreateTemp produced an error")

	// Non-existent hostnamesFile
	_, err = unmarshalHostnames("file_does_not_exist")
	test.AssertError(t, err, "expected error for non-existent file")

	// Empty hostnamesFile
	err = os.WriteFile(testFile.Name(), []byte(""), 0644)
	test.AssertNotError(t, err, "os.WriteFile produced an error")
	_, err = unmarshalHostnames(testFile.Name())
	test.AssertError(t, err, "expected error for file containing 0 entries")

	// One hostname present in the hostnamesFile
	err = os.WriteFile(testFile.Name(), []byte("example-a.com"), 0644)
	test.AssertNotError(t, err, "os.WriteFile produced an error")
	results, err := unmarshalHostnames(testFile.Name())
	test.AssertNotError(t, err, "error when unmarshalling hostnamesFile with a single hostname")
	test.AssertEquals(t, len(results), 1)

	// Two hostnames present in the hostnamesFile
	err = os.WriteFile(testFile.Name(), []byte("example-a.com\nexample-b.com"), 0644)
	test.AssertNotError(t, err, "os.WriteFile produced an error")
	results, err = unmarshalHostnames(testFile.Name())
	test.AssertNotError(t, err, "error when unmarshalling hostnamesFile with a two hostnames")
	test.AssertEquals(t, len(results), 2)

	// Three hostnames present in the hostnamesFile but two are separated only by a space
	err = os.WriteFile(testFile.Name(), []byte("example-a.com\nexample-b.com example-c.com"), 0644)
	test.AssertNotError(t, err, "os.WriteFile produced an error")
	_, err = unmarshalHostnames(testFile.Name())
	test.AssertError(t, err, "error when unmarshalling hostnamesFile with three space separated domains")
}

type testCtx struct {
	c       idExporter
	ssa     sapb.StorageAuthorityClient
	cleanUp func()
}

func (tc testCtx) addRegistrations(t *testing.T) {
	emailA := "mailto:" + emailARaw
	emailB := "mailto:" + emailBRaw
	emailC := "mailto:" + emailCRaw
	tel := "tel:" + telNum

	// Every registration needs a unique JOSE key
	jsonKeyA := []byte(`{
  "kty":"RSA",
  "n":"0vx7agoebGcQSuuPiLJXZptN9nndrQmbXEps2aiAFbWhM78LhWx4cbbfAAtVT86zwu1RK7aPFFxuhDR1L6tSoc_BJECPebWKRXjBZCiFV4n3oknjhMstn64tZ_2W-5JsGY4Hc5n9yBXArwl93lqt7_RN5w6Cf0h4QyQ5v-65YGjQR0_FDW2QvzqY368QQMicAtaSqzs8KJZgnYb9c7d0zgdAZHzu6qMQvRL5hajrn1n91CbOpbISD08qNLyrdkt-bFTWhAI4vMQFh6WeZu0fM4lFd2NcRwr3XPksINHaQ-G_xBniIqbw0Ls1jF44-csFCur-kEgU8awapJzKnqDKgw",
  "e":"AQAB"
}`)
	jsonKeyB := []byte(`{
  "kty":"RSA",
  "n":"z8bp-jPtHt4lKBqepeKF28g_QAEOuEsCIou6sZ9ndsQsEjxEOQxQ0xNOQezsKa63eogw8YS3vzjUcPP5BJuVzfPfGd5NVUdT-vSSwxk3wvk_jtNqhrpcoG0elRPQfMVsQWmxCAXCVRz3xbcFI8GTe-syynG3l-g1IzYIIZVNI6jdljCZML1HOMTTW4f7uJJ8mM-08oQCeHbr5ejK7O2yMSSYxW03zY-Tj1iVEebROeMv6IEEJNFSS4yM-hLpNAqVuQxFGetwtwjDMC1Drs1dTWrPuUAAjKGrP151z1_dE74M5evpAhZUmpKv1hY-x85DC6N0hFPgowsanmTNNiV75w",
  "e":"AAEAAQ"
}`)
	jsonKeyC := []byte(`{
  "kty":"RSA",
  "n":"rFH5kUBZrlPj73epjJjyCxzVzZuV--JjKgapoqm9pOuOt20BUTdHqVfC2oDclqM7HFhkkX9OSJMTHgZ7WaVqZv9u1X2yjdx9oVmMLuspX7EytW_ZKDZSzL-sCOFCuQAuYKkLbsdcA3eHBK_lwc4zwdeHFMKIulNvLqckkqYB9s8GpgNXBDIQ8GjR5HuJke_WUNjYHSd8jY1LU9swKWsLQe2YoQUz_ekQvBvBCoaFEtrtRaSJKNLIVDObXFr2TLIiFiM0Em90kK01-eQ7ZiruZTKomll64bRFPoNo4_uwubddg3xTqur2vdF3NyhTrYdvAgTem4uC0PFjEQ1bK_djBQ",
  "e":"AQAB"
}`)
	jsonKeyD := []byte(`{
  "kty":"RSA",
  "n":"rFH5kUBZrlPj73epjJjyCxzVzZuV--JjKgapoqm9pOuOt20BUTdHqVfC2oDclqM7HFhkkX9OSJMTHgZ7WaVqZv9u1X2yjdx9oVmMLuspX7EytW_ZKDZSzL-FCOFCuQAuYKkLbsdcA3eHBK_lwc4zwdeHFMKIulNvLqckkqYB9s8GpgNXBDIQ8GjR5HuJke_WUNjYHSd8jY1LU9swKWsLQe2YoQUz_ekQvBvBCoaFEtrtRaSJKNLIVDObXFr2TLIiFiM0Em90kK01-eQ7ZiruZTKomll64bRFPoNo4_uwubddg3xTqur2vdF3NyhTrYdvAgTem4uC0PFjEQ1bK_djBQ",
  "e":"AQAB"
}`)

	initialIP, err := net.ParseIP("127.0.0.1").MarshalText()
	test.AssertNotError(t, err, "Couldn't create initialIP")

	// Regs A through C have `mailto:` contact ACME URL's
	regA = &corepb.Registration{
		Id:        1,
		Contact:   []string{emailA},
		Key:       jsonKeyA,
		InitialIP: initialIP,
	}
	regB = &corepb.Registration{
		Id:        2,
		Contact:   []string{emailB},
		Key:       jsonKeyB,
		InitialIP: initialIP,
	}
	regC = &corepb.Registration{
		Id:        3,
		Contact:   []string{emailC},
		Key:       jsonKeyC,
		InitialIP: initialIP,
	}
	// Reg D has a `tel:` contact ACME URL
	regD = &corepb.Registration{
		Id:        4,
		Contact:   []string{tel},
		Key:       jsonKeyD,
		InitialIP: initialIP,
	}

	// Add the four test registrations
	ctx := context.Background()
	regA, err = tc.ssa.NewRegistration(ctx, regA)
	test.AssertNotError(t, err, "Couldn't store regA")
	regB, err = tc.ssa.NewRegistration(ctx, regB)
	test.AssertNotError(t, err, "Couldn't store regB")
	regC, err = tc.ssa.NewRegistration(ctx, regC)
	test.AssertNotError(t, err, "Couldn't store regC")
	regD, err = tc.ssa.NewRegistration(ctx, regD)
	test.AssertNotError(t, err, "Couldn't store regD")
}

func (tc testCtx) addCertificates(t *testing.T) {
	ctx := context.Background()
	serial1 := big.NewInt(1336)
	serial1String := core.SerialToString(serial1)
	serial2 := big.NewInt(1337)
	serial2String := core.SerialToString(serial2)
	serial3 := big.NewInt(1338)
	serial3String := core.SerialToString(serial3)
	serial4 := big.NewInt(1339)
	serial4String := core.SerialToString(serial4)
	n := bigIntFromB64("n4EPtAOCc9AlkeQHPzHStgAbgs7bTZLwUBZdR8_KuKPEHLd4rHVTeT-O-XV2jRojdNhxJWTDvNd7nqQ0VEiZQHz_AJmSCpMaJMRBSFKrKb2wqVwGU_NsYOYL-QtiWN2lbzcEe6XC0dApr5ydQLrHqkHHig3RBordaZ6Aj-oBHqFEHYpPe7Tpe-OfVfHd1E6cS6M1FZcD1NNLYD5lFHpPI9bTwJlsde3uhGqC0ZCuEHg8lhzwOHrtIQbS0FVbb9k3-tVTU4fg_3L_vniUFAKwuCLqKnS2BYwdq_mzSnbLY7h_qixoR7jig3__kRhuaxwUkRz5iaiQkqgc5gHdrNP5zw==")
	e := intFromB64("AQAB")
	d := bigIntFromB64("bWUC9B-EFRIo8kpGfh0ZuyGPvMNKvYWNtB_ikiH9k20eT-O1q_I78eiZkpXxXQ0UTEs2LsNRS-8uJbvQ-A1irkwMSMkK1J3XTGgdrhCku9gRldY7sNA_AKZGh-Q661_42rINLRCe8W-nZ34ui_qOfkLnK9QWDDqpaIsA-bMwWWSDFu2MUBYwkHTMEzLYGqOe04noqeq1hExBTHBOBdkMXiuFhUq1BU6l-DqEiWxqg82sXt2h-LMnT3046AOYJoRioz75tSUQfGCshWTBnP5uDjd18kKhyv07lhfSJdrPdM5Plyl21hsFf4L_mHCuoFau7gdsPfHPxxjVOcOpBrQzwQ==")
	p := bigIntFromB64("uKE2dh-cTf6ERF4k4e_jy78GfPYUIaUyoSSJuBzp3Cubk3OCqs6grT8bR_cu0Dm1MZwWmtdqDyI95HrUeq3MP15vMMON8lHTeZu2lmKvwqW7anV5UzhM1iZ7z4yMkuUwFWoBvyY898EXvRD-hdqRxHlSqAZ192zB3pVFJ0s7pFc=")
	q := bigIntFromB64("uKE2dh-cTf6ERF4k4e_jy78GfPYUIaUyoSSJuBzp3Cubk3OCqs6grT8bR_cu0Dm1MZwWmtdqDyI95HrUeq3MP15vMMON8lHTeZu2lmKvwqW7anV5UzhM1iZ7z4yMkuUwFWoBvyY898EXvRD-hdqRxHlSqAZ192zB3pVFJ0s7pFc=")

	testKey := rsa.PrivateKey{
		PublicKey: rsa.PublicKey{N: n, E: e},
		D:         d,
		Primes:    []*big.Int{p, q},
	}

	fc := clock.NewFake()

	// Add one cert for RegA that expires in 30 days
	rawCertA := x509.Certificate{
		Subject: pkix.Name{
			CommonName: "happy A",
		},
		NotAfter:     fc.Now().Add(30 * 24 * time.Hour),
		DNSNames:     []string{"example-a.com"},
		SerialNumber: serial1,
	}
	certDerA, _ := x509.CreateCertificate(rand.Reader, &rawCertA, &rawCertA, &testKey.PublicKey, &testKey)
	certA := &core.Certificate{
		RegistrationID: regA.Id,
		Serial:         serial1String,
		Expires:        rawCertA.NotAfter,
		DER:            certDerA,
	}
	err := tc.c.dbMap.Insert(ctx, certA)
	test.AssertNotError(t, err, "Couldn't add certA")
	_, err = tc.c.dbMap.ExecContext(
		ctx,
		"INSERT INTO issuedNames (reversedName, serial, notBefore) VALUES (?,?,0)",
		"com.example-a",
		serial1String,
	)
	test.AssertNotError(t, err, "Couldn't add issued name for certA")

	// Add one cert for RegB that already expired 30 days ago
	rawCertB := x509.Certificate{
		Subject: pkix.Name{
			CommonName: "happy B",
		},
		NotAfter:     fc.Now().Add(-30 * 24 * time.Hour),
		DNSNames:     []string{"example-b.com"},
		SerialNumber: serial2,
	}
	certDerB, _ := x509.CreateCertificate(rand.Reader, &rawCertB, &rawCertB, &testKey.PublicKey, &testKey)
	certB := &core.Certificate{
		RegistrationID: regB.Id,
		Serial:         serial2String,
		Expires:        rawCertB.NotAfter,
		DER:            certDerB,
	}
	err = tc.c.dbMap.Insert(ctx, certB)
	test.AssertNotError(t, err, "Couldn't add certB")
	_, err = tc.c.dbMap.ExecContext(
		ctx,
		"INSERT INTO issuedNames (reversedName, serial, notBefore) VALUES (?,?,0)",
		"com.example-b",
		serial2String,
	)
	test.AssertNotError(t, err, "Couldn't add issued name for certB")

	// Add one cert for RegC that expires in 30 days
	rawCertC := x509.Certificate{
		Subject: pkix.Name{
			CommonName: "happy C",
		},
		NotAfter:     fc.Now().Add(30 * 24 * time.Hour),
		DNSNames:     []string{"example-c.com"},
		SerialNumber: serial3,
	}
	certDerC, _ := x509.CreateCertificate(rand.Reader, &rawCertC, &rawCertC, &testKey.PublicKey, &testKey)
	certC := &core.Certificate{
		RegistrationID: regC.Id,
		Serial:         serial3String,
		Expires:        rawCertC.NotAfter,
		DER:            certDerC,
	}
	err = tc.c.dbMap.Insert(ctx, certC)
	test.AssertNotError(t, err, "Couldn't add certC")
	_, err = tc.c.dbMap.ExecContext(
		ctx,
		"INSERT INTO issuedNames (reversedName, serial, notBefore) VALUES (?,?,0)",
		"com.example-c",
		serial3String,
	)
	test.AssertNotError(t, err, "Couldn't add issued name for certC")

	// Add one cert for RegD that expires in 30 days
	rawCertD := x509.Certificate{
		Subject: pkix.Name{
			CommonName: "happy D",
		},
		NotAfter:     fc.Now().Add(30 * 24 * time.Hour),
		DNSNames:     []string{"example-d.com"},
		SerialNumber: serial4,
	}
	certDerD, _ := x509.CreateCertificate(rand.Reader, &rawCertD, &rawCertD, &testKey.PublicKey, &testKey)
	certD := &core.Certificate{
		RegistrationID: regD.Id,
		Serial:         serial4String,
		Expires:        rawCertD.NotAfter,
		DER:            certDerD,
	}
	err = tc.c.dbMap.Insert(ctx, certD)
	test.AssertNotError(t, err, "Couldn't add certD")
	_, err = tc.c.dbMap.ExecContext(
		ctx,
		"INSERT INTO issuedNames (reversedName, serial, notBefore) VALUES (?,?,0)",
		"com.example-d",
		serial4String,
	)
	test.AssertNotError(t, err, "Couldn't add issued name for certD")
}

func setup(t *testing.T) testCtx {
	log := blog.UseMock()
	fc := clock.NewFake()

	// Using DBConnSAFullPerms to be able to insert registrations and certificates
	dbMap, err := sa.DBMapForTest(vars.DBConnSAFullPerms)
	if err != nil {
		t.Fatalf("Couldn't connect the database: %s", err)
	}
	cleanUp := test.ResetBoulderTestDatabase(t)

	ssa, err := sa.NewSQLStorageAuthority(dbMap, dbMap, nil, 1, 0, fc, log, metrics.NoopRegisterer)
	if err != nil {
		t.Fatalf("unable to create SQLStorageAuthority: %s", err)
	}

	return testCtx{
		c: idExporter{
			dbMap: dbMap,
			log:   log,
			clk:   fc,
		},
		ssa:     isa.SA{Impl: ssa},
		cleanUp: cleanUp,
	}
}

func bigIntFromB64(b64 string) *big.Int {
	bytes, _ := base64.URLEncoding.DecodeString(b64)
	x := big.NewInt(0)
	x.SetBytes(bytes)
	return x
}

func intFromB64(b64 string) int {
	return int(bigIntFromB64(b64).Int64())
}
