package notmain

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"log"
	"math/big"
	mrand "math/rand/v2"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	"github.com/letsencrypt/boulder/ctpolicy/loglist"
	"github.com/letsencrypt/boulder/goodkey"
	"github.com/letsencrypt/boulder/goodkey/sagoodkey"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/linter"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/policy"
	"github.com/letsencrypt/boulder/sa"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/sa/satest"
	"github.com/letsencrypt/boulder/test"
	isa "github.com/letsencrypt/boulder/test/inmem/sa"
	"github.com/letsencrypt/boulder/test/vars"
)

var (
	testValidityDuration  = 24 * 90 * time.Hour
	testValidityDurations = map[time.Duration]bool{testValidityDuration: true}
	pa                    *policy.AuthorityImpl
	kp                    goodkey.KeyPolicy
)

func init() {
	var err error
	pa, err = policy.New(
		map[identifier.IdentifierType]bool{identifier.TypeDNS: true, identifier.TypeIP: true},
		map[core.AcmeChallenge]bool{},
		blog.NewMock())
	if err != nil {
		log.Fatal(err)
	}
	err = pa.LoadHostnamePolicyFile("../../test/hostname-policy.yaml")
	if err != nil {
		log.Fatal(err)
	}
	kp, err = sagoodkey.NewPolicy(nil, nil)
	if err != nil {
		log.Fatal(err)
	}
}

func BenchmarkCheckCert(b *testing.B) {
	checker := newChecker(nil, clock.New(), pa, kp, time.Hour, testValidityDurations, nil, blog.NewMock())
	testKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	expiry := time.Now().AddDate(0, 0, 1)
	serial := big.NewInt(1337)
	rawCert := x509.Certificate{
		Subject: pkix.Name{
			CommonName: "example.com",
		},
		NotAfter:     expiry,
		DNSNames:     []string{"example-a.com"},
		SerialNumber: serial,
	}
	certDer, _ := x509.CreateCertificate(rand.Reader, &rawCert, &rawCert, &testKey.PublicKey, testKey)
	cert := &corepb.Certificate{
		Serial:  core.SerialToString(serial),
		Digest:  core.Fingerprint256(certDer),
		Der:     certDer,
		Issued:  timestamppb.New(time.Now()),
		Expires: timestamppb.New(expiry),
	}
	b.ResetTimer()
	for range b.N {
		checker.checkCert(context.Background(), cert)
	}
}

func TestCheckWildcardCert(t *testing.T) {
	saDbMap, err := sa.DBMapForTest(vars.DBConnSA)
	test.AssertNotError(t, err, "Couldn't connect to database")
	saCleanup := test.ResetBoulderTestDatabase(t)
	defer func() {
		saCleanup()
	}()

	testKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	fc := clock.NewFake()
	checker := newChecker(saDbMap, fc, pa, kp, time.Hour, testValidityDurations, nil, blog.NewMock())
	issued := checker.clock.Now().Add(-time.Minute)
	goodExpiry := issued.Add(testValidityDuration - time.Second)
	serial := big.NewInt(1337)

	wildcardCert := x509.Certificate{
		Subject: pkix.Name{
			CommonName: "*.example.com",
		},
		NotBefore:             issued,
		NotAfter:              goodExpiry,
		DNSNames:              []string{"*.example.com"},
		SerialNumber:          serial,
		BasicConstraintsValid: true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature,
		OCSPServer:            []string{"http://example.com/ocsp"},
		IssuingCertificateURL: []string{"http://example.com/cert"},
	}
	wildcardCertDer, err := x509.CreateCertificate(rand.Reader, &wildcardCert, &wildcardCert, &testKey.PublicKey, testKey)
	test.AssertNotError(t, err, "Couldn't create certificate")
	parsed, err := x509.ParseCertificate(wildcardCertDer)
	test.AssertNotError(t, err, "Couldn't parse created certificate")
	cert := &corepb.Certificate{
		Serial:  core.SerialToString(serial),
		Digest:  core.Fingerprint256(wildcardCertDer),
		Expires: timestamppb.New(parsed.NotAfter),
		Issued:  timestamppb.New(parsed.NotBefore),
		Der:     wildcardCertDer,
	}
	_, problems := checker.checkCert(context.Background(), cert)
	for _, p := range problems {
		t.Error(p)
	}
}

func TestCheckCertReturnsSANs(t *testing.T) {
	saDbMap, err := sa.DBMapForTest(vars.DBConnSA)
	test.AssertNotError(t, err, "Couldn't connect to database")
	saCleanup := test.ResetBoulderTestDatabase(t)
	defer func() {
		saCleanup()
	}()
	checker := newChecker(saDbMap, clock.NewFake(), pa, kp, time.Hour, testValidityDurations, nil, blog.NewMock())

	certPEM, err := os.ReadFile("testdata/quite_invalid.pem")
	if err != nil {
		t.Fatal(err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("failed to parse cert PEM")
	}

	cert := &corepb.Certificate{
		Serial:  "00000000000",
		Digest:  core.Fingerprint256(block.Bytes),
		Expires: timestamppb.New(time.Now().Add(time.Hour)),
		Issued:  timestamppb.New(time.Now()),
		Der:     block.Bytes,
	}

	names, problems := checker.checkCert(context.Background(), cert)
	if !slices.Equal(names, []string{"quite_invalid.com", "al--so--wr--ong.com", "127.0.0.1"}) {
		t.Errorf("didn't get expected DNS names. other problems: %s", strings.Join(problems, "\n"))
	}
}

type keyGen interface {
	genKey() (crypto.Signer, error)
}

type ecP256Generator struct{}

func (*ecP256Generator) genKey() (crypto.Signer, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

type rsa2048Generator struct{}

func (*rsa2048Generator) genKey() (crypto.Signer, error) {
	return rsa.GenerateKey(rand.Reader, 2048)
}

func TestCheckCert(t *testing.T) {
	saDbMap, err := sa.DBMapForTest(vars.DBConnSA)
	test.AssertNotError(t, err, "Couldn't connect to database")
	saCleanup := test.ResetBoulderTestDatabase(t)
	defer func() {
		saCleanup()
	}()

	testCases := []struct {
		name string
		key  keyGen
	}{
		{
			name: "RSA 2048 key",
			key:  &rsa2048Generator{},
		},
		{
			name: "ECDSA P256 key",
			key:  &ecP256Generator{},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testKey, _ := tc.key.genKey()

			checker := newChecker(saDbMap, clock.NewFake(), pa, kp, time.Hour, testValidityDurations, nil, blog.NewMock())

			// Create a RFC 7633 OCSP Must Staple Extension.
			// OID 1.3.6.1.5.5.7.1.24
			ocspMustStaple := pkix.Extension{
				Id:       asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 24},
				Critical: false,
				Value:    []uint8{0x30, 0x3, 0x2, 0x1, 0x5},
			}

			// Create a made up PKIX extension
			imaginaryExtension := pkix.Extension{
				Id:       asn1.ObjectIdentifier{1, 3, 3, 7},
				Critical: false,
				Value:    []uint8{0xC0, 0xFF, 0xEE},
			}

			issued := checker.clock.Now().Add(-time.Minute)
			goodExpiry := issued.Add(testValidityDuration - time.Second)
			serial := big.NewInt(1337)
			longName := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeexample.com"
			rawCert := x509.Certificate{
				Subject: pkix.Name{
					CommonName: longName,
				},
				NotBefore: issued,
				NotAfter:  goodExpiry.AddDate(0, 0, 1), // Period too long
				DNSNames: []string{
					"example-a.com",
					"foodnotbombs.mil",
					// `dev-myqnapcloud.com` is included because it is an exact private
					// entry on the public suffix list
					"dev-myqnapcloud.com",
					// don't include longName in the SANs, so the unique CN gets flagged
				},
				SerialNumber:          serial,
				BasicConstraintsValid: false,
				ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
				KeyUsage:              x509.KeyUsageDigitalSignature,
				OCSPServer:            []string{"http://example.com/ocsp"},
				IssuingCertificateURL: []string{"http://example.com/cert"},
				ExtraExtensions:       []pkix.Extension{ocspMustStaple, imaginaryExtension},
			}
			brokenCertDer, err := x509.CreateCertificate(rand.Reader, &rawCert, &rawCert, testKey.Public(), testKey)
			test.AssertNotError(t, err, "Couldn't create certificate")
			// Problems
			//   Digest doesn't match
			//   Serial doesn't match
			//   Expiry doesn't match
			//   Issued doesn't match
			cert := &corepb.Certificate{
				Serial:  "8485f2687eba29ad455ae4e31c8679206fec",
				Der:     brokenCertDer,
				Issued:  timestamppb.New(issued.Add(12 * time.Hour)),
				Expires: timestamppb.New(goodExpiry.AddDate(0, 0, 2)), // Expiration doesn't match
			}

			_, problems := checker.checkCert(context.Background(), cert)

			problemsMap := map[string]int{
				"Stored digest doesn't match certificate digest":                            1,
				"Stored serial doesn't match certificate serial":                            1,
				"Stored expiration doesn't match certificate NotAfter":                      1,
				"Certificate doesn't have basic constraints set":                            1,
				"Certificate has unacceptable validity period":                              1,
				"Stored issuance date is outside of 6 hour window of certificate NotBefore": 1,
				"Certificate has incorrect key usage extensions":                            1,
				"Certificate has common name >64 characters long (65)":                      1,
				"Certificate contains an unexpected extension: 1.3.3.7":                     1,
				"Certificate Common Name does not appear in Subject Alternative Names: \"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeexample.com\" !< [example-a.com foodnotbombs.mil dev-myqnapcloud.com]": 1,
			}
			for _, p := range problems {
				_, ok := problemsMap[p]
				if !ok {
					t.Errorf("Found unexpected problem '%s'.", p)
				}
				delete(problemsMap, p)
			}
			for k := range problemsMap {
				t.Errorf("Expected problem but didn't find '%s' in problems: %q.", k, problems)
			}

			// Same settings as above, but the stored serial number in the DB is invalid.
			cert.Serial = "not valid"
			_, problems = checker.checkCert(context.Background(), cert)
			foundInvalidSerialProblem := false
			for _, p := range problems {
				if p == "Stored serial is invalid" {
					foundInvalidSerialProblem = true
				}
			}
			test.Assert(t, foundInvalidSerialProblem, "Invalid certificate serial number in DB did not trigger problem.")

			// Fix the problems
			rawCert.Subject.CommonName = "example-a.com"
			rawCert.DNSNames = []string{"example-a.com"}
			rawCert.NotAfter = goodExpiry
			rawCert.BasicConstraintsValid = true
			rawCert.ExtraExtensions = []pkix.Extension{ocspMustStaple}
			rawCert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
			goodCertDer, err := x509.CreateCertificate(rand.Reader, &rawCert, &rawCert, testKey.Public(), testKey)
			test.AssertNotError(t, err, "Couldn't create certificate")
			parsed, err := x509.ParseCertificate(goodCertDer)
			test.AssertNotError(t, err, "Couldn't parse created certificate")
			cert.Serial = core.SerialToString(serial)
			cert.Digest = core.Fingerprint256(goodCertDer)
			cert.Der = goodCertDer
			cert.Expires = timestamppb.New(parsed.NotAfter)
			cert.Issued = timestamppb.New(parsed.NotBefore)
			_, problems = checker.checkCert(context.Background(), cert)
			test.AssertEquals(t, len(problems), 0)
		})
	}
}

func TestGetAndProcessCerts(t *testing.T) {
	saDbMap, err := sa.DBMapForTest(vars.DBConnSA)
	test.AssertNotError(t, err, "Couldn't connect to database")
	fc := clock.NewFake()
	fc.Set(fc.Now().Add(time.Hour))

	checker := newChecker(saDbMap, fc, pa, kp, time.Hour, testValidityDurations, nil, blog.NewMock())
	sa, err := sa.NewSQLStorageAuthority(saDbMap, saDbMap, nil, 1, 0, fc, blog.NewMock(), metrics.NoopRegisterer)
	test.AssertNotError(t, err, "Couldn't create SA to insert certificates")
	saCleanUp := test.ResetBoulderTestDatabase(t)
	defer func() {
		saCleanUp()
	}()

	testKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	// Problems
	//   Expiry period is too long
	rawCert := x509.Certificate{
		Subject: pkix.Name{
			CommonName: "not-blacklisted.com",
		},
		BasicConstraintsValid: true,
		DNSNames:              []string{"not-blacklisted.com"},
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	reg := satest.CreateWorkingRegistration(t, isa.SA{Impl: sa})
	test.AssertNotError(t, err, "Couldn't create registration")
	for range 5 {
		rawCert.SerialNumber = big.NewInt(mrand.Int64())
		certDER, err := x509.CreateCertificate(rand.Reader, &rawCert, &rawCert, &testKey.PublicKey, testKey)
		test.AssertNotError(t, err, "Couldn't create certificate")
		_, err = sa.AddCertificate(context.Background(), &sapb.AddCertificateRequest{
			Der:    certDER,
			RegID:  reg.Id,
			Issued: timestamppb.New(fc.Now()),
		})
		test.AssertNotError(t, err, "Couldn't add certificate")
	}

	batchSize = 2
	err = checker.getCerts(context.Background())
	test.AssertNotError(t, err, "Failed to retrieve certificates")
	test.AssertEquals(t, len(checker.certs), 5)
	wg := new(sync.WaitGroup)
	wg.Add(1)
	checker.processCerts(context.Background(), wg, false)
	test.AssertEquals(t, checker.issuedReport.BadCerts, int64(5))
	test.AssertEquals(t, len(checker.issuedReport.Entries), 5)
}

// mismatchedCountDB is a certDB implementation for `getCerts` that returns one
// high value when asked how many rows there are, and then returns nothing when
// asked for the actual rows.
type mismatchedCountDB struct{}

// `getCerts` calls `SelectInt` first to determine how many rows there are
// matching the `getCertsCountQuery` criteria. For this mock we return
// a non-zero number
func (db mismatchedCountDB) SelectNullInt(_ context.Context, _ string, _ ...interface{}) (sql.NullInt64, error) {
	return sql.NullInt64{
			Int64: 99999,
			Valid: true,
		},
		nil
}

// `getCerts` then calls `Select` to retrieve the Certificate rows. We pull
// a dastardly switch-a-roo here and return an empty set
func (db mismatchedCountDB) Select(_ context.Context, output interface{}, _ string, _ ...interface{}) ([]interface{}, error) {
	return nil, nil
}

func (db mismatchedCountDB) SelectOne(_ context.Context, _ interface{}, _ string, _ ...interface{}) error {
	return errors.New("unimplemented")
}

/*
 * In Boulder #2004[0] we identified that there is a race in `getCerts`
 * between the first call to `SelectOne` to identify how many rows there are,
 * and the subsequent call to `Select` to get the actual rows in batches. This
 * manifests in an index out of range panic where the cert checker thinks there
 * are more rows than there are and indexes into an empty set of certificates to
 * update the lastSerial field of the query `args`. This has been fixed by
 * adding a len() check in the inner `getCerts` loop that processes the certs
 * one batch at a time.
 *
 * TestGetCertsEmptyResults tests the fix remains in place by using a mock that
 * exploits this corner case deliberately. The `mismatchedCountDB` mock (defined
 * above) will return a high count for the `SelectOne` call, but an empty slice
 * for the `Select` call. Without the fix in place this reliably produced the
 * "index out of range" panic from #2004. With the fix in place the test passes.
 *
 * 0: https://github.com/letsencrypt/boulder/issues/2004
 */
func TestGetCertsEmptyResults(t *testing.T) {
	saDbMap, err := sa.DBMapForTest(vars.DBConnSA)
	test.AssertNotError(t, err, "Couldn't connect to database")
	checker := newChecker(saDbMap, clock.NewFake(), pa, kp, time.Hour, testValidityDurations, nil, blog.NewMock())
	checker.dbMap = mismatchedCountDB{}

	batchSize = 3
	err = checker.getCerts(context.Background())
	test.AssertNotError(t, err, "Failed to retrieve certificates")
}

// emptyDB is a certDB object with methods used for testing that 'null'
// responses received from the database are handled properly.
type emptyDB struct {
	certDB
}

// SelectNullInt is a method that returns a false sql.NullInt64 struct to
// mock a null DB response
func (db emptyDB) SelectNullInt(_ context.Context, _ string, _ ...interface{}) (sql.NullInt64, error) {
	return sql.NullInt64{Valid: false},
		nil
}

// TestGetCertsNullResults tests that a null response from the database will
// be handled properly. It uses the emptyDB above to mock the response
// expected if the DB finds no certificates to match the SELECT query and
// should return an error.
func TestGetCertsNullResults(t *testing.T) {
	checker := newChecker(emptyDB{}, clock.NewFake(), pa, kp, time.Hour, testValidityDurations, nil, blog.NewMock())

	err := checker.getCerts(context.Background())
	test.AssertError(t, err, "Should have gotten error from empty DB")
	if !strings.Contains(err.Error(), "no rows found for certificates issued between") {
		t.Errorf("expected error to contain 'no rows found for certificates issued between', got '%s'", err.Error())
	}
}

// lateDB is a certDB object that helps with TestGetCertsLate.
// It pretends to contain a single cert issued at the given time.
type lateDB struct {
	issuedTime    time.Time
	selectedACert bool
}

// SelectNullInt is a method that returns a false sql.NullInt64 struct to
// mock a null DB response
func (db *lateDB) SelectNullInt(_ context.Context, _ string, args ...interface{}) (sql.NullInt64, error) {
	args2 := args[0].(map[string]interface{})
	begin := args2["begin"].(time.Time)
	end := args2["end"].(time.Time)
	if begin.Compare(db.issuedTime) < 0 && end.Compare(db.issuedTime) > 0 {
		return sql.NullInt64{Int64: 23, Valid: true}, nil
	}
	return sql.NullInt64{Valid: false}, nil
}

func (db *lateDB) Select(_ context.Context, output interface{}, _ string, args ...interface{}) ([]interface{}, error) {
	db.selectedACert = true
	// For expediency we respond with an empty list of certificates; the checker will treat this as if it's
	// reached the end of the list of certificates to process.
	return nil, nil
}

func (db *lateDB) SelectOne(_ context.Context, _ interface{}, _ string, _ ...interface{}) error {
	return nil
}

// TestGetCertsLate checks for correct behavior when certificates exist only late in the provided window.
func TestGetCertsLate(t *testing.T) {
	clk := clock.NewFake()
	db := &lateDB{issuedTime: clk.Now().Add(-time.Hour)}
	checkPeriod := 24 * time.Hour
	checker := newChecker(db, clk, pa, kp, checkPeriod, testValidityDurations, nil, blog.NewMock())

	err := checker.getCerts(context.Background())
	test.AssertNotError(t, err, "getting certs")

	if !db.selectedACert {
		t.Errorf("checker never selected a certificate after getting a MIN(id)")
	}
}

func TestSaveReport(t *testing.T) {
	r := report{
		begin:     time.Time{},
		end:       time.Time{},
		GoodCerts: 2,
		BadCerts:  1,
		Entries: map[string]reportEntry{
			"020000000000004b475da49b91da5c17": {
				Valid: true,
			},
			"020000000000004d1613e581432cba7e": {
				Valid: true,
			},
			"020000000000004e402bc21035c6634a": {
				Valid:    false,
				Problems: []string{"None really..."},
			},
		},
	}

	err := r.dump()
	test.AssertNotError(t, err, "Failed to dump results")
}

func TestIsForbiddenDomain(t *testing.T) {
	// Note: These testcases are not an exhaustive representation of domains
	// Boulder won't issue for, but are instead testing the defense-in-depth
	// `isForbiddenDomain` function called *after* the PA has vetted the name
	// against the complex hostname policy file.
	testcases := []struct {
		Name     string
		Expected bool
	}{
		/* Expected to be forbidden test cases */
		// Whitespace only
		{Name: "", Expected: true},
		{Name: "   ", Expected: true},
		// Anything .local
		{Name: "yokel.local", Expected: true},
		{Name: "off.on.remote.local", Expected: true},
		{Name: ".local", Expected: true},
		// Localhost is verboten
		{Name: "localhost", Expected: true},
		// Anything .localhost
		{Name: ".localhost", Expected: true},
		{Name: "local.localhost", Expected: true},
		{Name: "extremely.local.localhost", Expected: true},

		/* Expected to be allowed test cases */
		{Name: "ok.computer.com", Expected: false},
		{Name: "ok.millionaires", Expected: false},
		{Name: "ok.milly", Expected: false},
		{Name: "ok", Expected: false},
		{Name: "nearby.locals", Expected: false},
		{Name: "yocalhost", Expected: false},
		{Name: "jokes.yocalhost", Expected: false},
	}

	for _, tc := range testcases {
		result, _ := isForbiddenDomain(tc.Name)
		test.AssertEquals(t, result, tc.Expected)
	}
}

func TestIgnoredLint(t *testing.T) {
	saDbMap, err := sa.DBMapForTest(vars.DBConnSA)
	test.AssertNotError(t, err, "Couldn't connect to database")
	saCleanup := test.ResetBoulderTestDatabase(t)
	defer func() {
		saCleanup()
	}()

	err = loglist.InitLintList("../../test/ct-test-srv/log_list.json")
	test.AssertNotError(t, err, "failed to load ct log list")
	testKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	checker := newChecker(saDbMap, clock.NewFake(), pa, kp, time.Hour, testValidityDurations, nil, blog.NewMock())
	serial := big.NewInt(1337)

	x509OID, err := x509.OIDFromInts([]uint64{1, 2, 3})
	test.AssertNotError(t, err, "failed to create x509.OID")

	template := &x509.Certificate{
		Subject: pkix.Name{
			CommonName: "CPU's Cool CA",
		},
		SerialNumber:          serial,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(testValidityDuration - time.Second),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		Policies:              []x509.OID{x509OID},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IssuingCertificateURL: []string{"http://aia.example.org"},
		SubjectKeyId:          []byte("foobar"),
	}

	// Create a self-signed issuer certificate to use
	issuerDer, err := x509.CreateCertificate(rand.Reader, template, template, testKey.Public(), testKey)
	test.AssertNotError(t, err, "failed to create self-signed issuer cert")
	issuerCert, err := x509.ParseCertificate(issuerDer)
	test.AssertNotError(t, err, "failed to parse self-signed issuer cert")

	// Reconfigure the template for an EE cert with a Subj. CN
	serial = big.NewInt(1338)
	template.SerialNumber = serial
	template.Subject.CommonName = "zombo.com"
	template.DNSNames = []string{"zombo.com"}
	template.KeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	template.IsCA = false

	subjectCertDer, err := x509.CreateCertificate(rand.Reader, template, issuerCert, testKey.Public(), testKey)
	test.AssertNotError(t, err, "failed to create EE cert")
	subjectCert, err := x509.ParseCertificate(subjectCertDer)
	test.AssertNotError(t, err, "failed to parse EE cert")

	cert := &corepb.Certificate{
		Serial:  core.SerialToString(serial),
		Der:     subjectCertDer,
		Digest:  core.Fingerprint256(subjectCertDer),
		Issued:  timestamppb.New(subjectCert.NotBefore),
		Expires: timestamppb.New(subjectCert.NotAfter),
	}

	// Without any ignored lints we expect several errors and warnings about SCTs,
	// the common name, and the subject key identifier extension.
	expectedProblems := []string{
		"zlint warn: w_subject_common_name_included",
		"zlint warn: w_ext_subject_key_identifier_not_recommended_subscriber",
		"zlint info: w_ct_sct_policy_count_unsatisfied Certificate had 0 embedded SCTs. Browser policy may require 2 for this certificate.",
		"zlint error: e_scts_from_same_operator Certificate had too few embedded SCTs; browser policy requires 2.",
	}
	slices.Sort(expectedProblems)

	// Check the certificate with a nil ignore map. This should return the
	// expected zlint problems.
	_, problems := checker.checkCert(context.Background(), cert)
	slices.Sort(problems)
	test.AssertDeepEquals(t, problems, expectedProblems)

	// Check the certificate again with an ignore map that excludes the affected
	// lints. This should return no problems.
	lints, err := linter.NewRegistry([]string{
		"w_subject_common_name_included",
		"w_ext_subject_key_identifier_not_recommended_subscriber",
		"w_ct_sct_policy_count_unsatisfied",
		"e_scts_from_same_operator",
	})
	test.AssertNotError(t, err, "creating test lint registry")
	checker.lints = lints
	_, problems = checker.checkCert(context.Background(), cert)
	test.AssertEquals(t, len(problems), 0)
}

func TestPrecertCorrespond(t *testing.T) {
	checker := newChecker(nil, clock.New(), pa, kp, time.Hour, testValidityDurations, nil, blog.NewMock())
	checker.getPrecert = func(_ context.Context, _ string) ([]byte, error) {
		return []byte("hello"), nil
	}
	testKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	expiry := time.Now().AddDate(0, 0, 1)
	serial := big.NewInt(1337)
	rawCert := x509.Certificate{
		Subject: pkix.Name{
			CommonName: "example.com",
		},
		NotAfter:     expiry,
		DNSNames:     []string{"example-a.com"},
		SerialNumber: serial,
	}
	certDer, _ := x509.CreateCertificate(rand.Reader, &rawCert, &rawCert, &testKey.PublicKey, testKey)
	cert := &corepb.Certificate{
		Serial:  core.SerialToString(serial),
		Digest:  core.Fingerprint256(certDer),
		Der:     certDer,
		Issued:  timestamppb.New(time.Now()),
		Expires: timestamppb.New(expiry),
	}
	_, problems := checker.checkCert(context.Background(), cert)
	if len(problems) == 0 {
		t.Errorf("expected precert correspondence problem")
	}
	// Ensure that at least one of the problems was related to checking correspondence
	for _, p := range problems {
		if strings.Contains(p, "does not correspond to precert") {
			return
		}
	}
	t.Fatalf("expected precert correspondence problem, but got: %v", problems)
}
