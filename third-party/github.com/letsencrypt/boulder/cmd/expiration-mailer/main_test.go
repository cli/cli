package notmain

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"

	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	"github.com/letsencrypt/boulder/db"
	berrors "github.com/letsencrypt/boulder/errors"
	blog "github.com/letsencrypt/boulder/log"
	bmail "github.com/letsencrypt/boulder/mail"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/mocks"
	"github.com/letsencrypt/boulder/sa"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/sa/satest"
	"github.com/letsencrypt/boulder/test"
	isa "github.com/letsencrypt/boulder/test/inmem/sa"
	"github.com/letsencrypt/boulder/test/vars"
)

type fakeRegStore struct {
	RegByID map[int64]*corepb.Registration
}

func (f fakeRegStore) GetRegistration(ctx context.Context, req *sapb.RegistrationID, _ ...grpc.CallOption) (*corepb.Registration, error) {
	r, ok := f.RegByID[req.Id]
	if !ok {
		return r, berrors.NotFoundError("no registration found for %q", req.Id)
	}
	return r, nil
}

func newFakeRegStore() fakeRegStore {
	return fakeRegStore{RegByID: make(map[int64]*corepb.Registration)}
}

const testTmpl = `hi, cert for DNS names {{.DNSNames}} is going to expire in {{.DaysToExpiration}} days ({{.ExpirationDate}})`
const testEmailSubject = `email subject for test`
const emailARaw = "rolandshoemaker@gmail.com"
const emailBRaw = "test@gmail.com"

var (
	emailA   = "mailto:" + emailARaw
	emailB   = "mailto:" + emailBRaw
	jsonKeyA = []byte(`{
  "kty":"RSA",
  "n":"0vx7agoebGcQSuuPiLJXZptN9nndrQmbXEps2aiAFbWhM78LhWx4cbbfAAtVT86zwu1RK7aPFFxuhDR1L6tSoc_BJECPebWKRXjBZCiFV4n3oknjhMstn64tZ_2W-5JsGY4Hc5n9yBXArwl93lqt7_RN5w6Cf0h4QyQ5v-65YGjQR0_FDW2QvzqY368QQMicAtaSqzs8KJZgnYb9c7d0zgdAZHzu6qMQvRL5hajrn1n91CbOpbISD08qNLyrdkt-bFTWhAI4vMQFh6WeZu0fM4lFd2NcRwr3XPksINHaQ-G_xBniIqbw0Ls1jF44-csFCur-kEgU8awapJzKnqDKgw",
  "e":"AQAB"
}`)
	jsonKeyB = []byte(`{
  "kty":"RSA",
  "n":"z8bp-jPtHt4lKBqepeKF28g_QAEOuEsCIou6sZ9ndsQsEjxEOQxQ0xNOQezsKa63eogw8YS3vzjUcPP5BJuVzfPfGd5NVUdT-vSSwxk3wvk_jtNqhrpcoG0elRPQfMVsQWmxCAXCVRz3xbcFI8GTe-syynG3l-g1IzYIIZVNI6jdljCZML1HOMTTW4f7uJJ8mM-08oQCeHbr5ejK7O2yMSSYxW03zY-Tj1iVEebROeMv6IEEJNFSS4yM-hLpNAqVuQxFGetwtwjDMC1Drs1dTWrPuUAAjKGrP151z1_dE74M5evpAhZUmpKv1hY-x85DC6N0hFPgowsanmTNNiV75w",
  "e":"AAEAAQ"
}`)
	jsonKeyC = []byte(`{
  "kty":"RSA",
  "n":"rFH5kUBZrlPj73epjJjyCxzVzZuV--JjKgapoqm9pOuOt20BUTdHqVfC2oDclqM7HFhkkX9OSJMTHgZ7WaVqZv9u1X2yjdx9oVmMLuspX7EytW_ZKDZSzL-sCOFCuQAuYKkLbsdcA3eHBK_lwc4zwdeHFMKIulNvLqckkqYB9s8GpgNXBDIQ8GjR5HuJke_WUNjYHSd8jY1LU9swKWsLQe2YoQUz_ekQvBvBCoaFEtrtRaSJKNLIVDObXFr2TLIiFiM0Em90kK01-eQ7ZiruZTKomll64bRFPoNo4_uwubddg3xTqur2vdF3NyhTrYdvAgTem4uC0PFjEQ1bK_djBQ",
  "e":"AQAB"
}`)
	tmpl     = template.Must(template.New("expiry-email").Parse(testTmpl))
	subjTmpl = template.Must(template.New("expiry-email-subject").Parse("Testing: " + defaultExpirationSubject))
)

func TestSendNagsManyCerts(t *testing.T) {
	mc := mocks.Mailer{}
	rs := newFakeRegStore()
	fc := clock.NewFake()

	staticTmpl := template.Must(template.New("expiry-email-subject-static").Parse(testEmailSubject))
	tmpl := template.Must(template.New("expiry-email").Parse(
		`cert for DNS names {{.TruncatedDNSNames}} is going to expire in {{.DaysToExpiration}} days ({{.ExpirationDate}})`))

	m := mailer{
		log:            blog.NewMock(),
		mailer:         &mc,
		emailTemplate:  tmpl,
		addressLimiter: &limiter{clk: fc, limit: 4},
		// Explicitly override the default subject to use testEmailSubject
		subjectTemplate: staticTmpl,
		rs:              rs,
		clk:             fc,
		stats:           initStats(metrics.NoopRegisterer),
	}

	var certs []*x509.Certificate
	for i := range 101 {
		certs = append(certs, &x509.Certificate{
			SerialNumber: big.NewInt(0x0304),
			NotAfter:     fc.Now().AddDate(0, 0, 2),
			DNSNames:     []string{fmt.Sprintf("example-%d.com", i)},
		})
	}

	conn, err := m.mailer.Connect()
	test.AssertNotError(t, err, "connecting SMTP")
	err = m.sendNags(conn, []string{emailA}, certs)
	test.AssertNotError(t, err, "sending mail")

	test.AssertEquals(t, len(mc.Messages), 1)
	if len(strings.Split(mc.Messages[0].Body, "\n")) > 100 {
		t.Errorf("Expected mailed message to truncate after 100 domains, got: %q", mc.Messages[0].Body)
	}
}

func TestSendNags(t *testing.T) {
	mc := mocks.Mailer{}
	rs := newFakeRegStore()
	fc := clock.NewFake()

	staticTmpl := template.Must(template.New("expiry-email-subject-static").Parse(testEmailSubject))

	log := blog.NewMock()
	m := mailer{
		log:            log,
		mailer:         &mc,
		emailTemplate:  tmpl,
		addressLimiter: &limiter{clk: fc, limit: 4},
		// Explicitly override the default subject to use testEmailSubject
		subjectTemplate: staticTmpl,
		rs:              rs,
		clk:             fc,
		stats:           initStats(metrics.NoopRegisterer),
	}

	cert := &x509.Certificate{
		SerialNumber: big.NewInt(0x0304),
		NotAfter:     fc.Now().AddDate(0, 0, 2),
		DNSNames:     []string{"example.com"},
	}

	conn, err := m.mailer.Connect()
	test.AssertNotError(t, err, "connecting SMTP")
	err = m.sendNags(conn, []string{emailA}, []*x509.Certificate{cert})
	test.AssertNotError(t, err, "Failed to send warning messages")
	test.AssertEquals(t, len(mc.Messages), 1)
	test.AssertEquals(t, mc.Messages[0], mocks.MailerMessage{
		To:      emailARaw,
		Subject: testEmailSubject,
		Body:    fmt.Sprintf(`hi, cert for DNS names example.com is going to expire in 2 days (%s)`, cert.NotAfter.Format(time.DateOnly)),
	})

	mc.Clear()
	conn, err = m.mailer.Connect()
	test.AssertNotError(t, err, "connecting SMTP")
	err = m.sendNags(conn, []string{emailA, emailB}, []*x509.Certificate{cert})
	test.AssertNotError(t, err, "Failed to send warning messages")
	test.AssertEquals(t, len(mc.Messages), 2)
	test.AssertEquals(t, mc.Messages[0], mocks.MailerMessage{
		To:      emailARaw,
		Subject: testEmailSubject,
		Body:    fmt.Sprintf(`hi, cert for DNS names example.com is going to expire in 2 days (%s)`, cert.NotAfter.Format(time.DateOnly)),
	})
	test.AssertEquals(t, mc.Messages[1], mocks.MailerMessage{
		To:      emailBRaw,
		Subject: testEmailSubject,
		Body:    fmt.Sprintf(`hi, cert for DNS names example.com is going to expire in 2 days (%s)`, cert.NotAfter.Format(time.DateOnly)),
	})

	mc.Clear()
	conn, err = m.mailer.Connect()
	test.AssertNotError(t, err, "connecting SMTP")
	err = m.sendNags(conn, []string{}, []*x509.Certificate{cert})
	test.AssertErrorIs(t, err, errNoValidEmail)
	test.AssertEquals(t, len(mc.Messages), 0)

	sendLogs := log.GetAllMatching("INFO: attempting send JSON=.*")
	if len(sendLogs) != 2 {
		t.Errorf("expected 2 'attempting send' log line, got %d: %s", len(sendLogs), strings.Join(sendLogs, "\n"))
	}
	if !strings.Contains(sendLogs[0], `"Rcpt":["rolandshoemaker@gmail.com"]`) {
		t.Errorf("expected first 'attempting send' log line to have one address, got %q", sendLogs[0])
	}
	if !strings.Contains(sendLogs[0], `"TruncatedSerials":["000000000000000000000000000000000304"]`) {
		t.Errorf("expected first 'attempting send' log line to have one serial, got %q", sendLogs[0])
	}
	if !strings.Contains(sendLogs[0], `"DaysToExpiration":2`) {
		t.Errorf("expected first 'attempting send' log line to have 2 days to expiration, got %q", sendLogs[0])
	}
	if !strings.Contains(sendLogs[0], `"TruncatedDNSNames":["example.com"]`) {
		t.Errorf("expected first 'attempting send' log line to have 1 domain, 'example.com', got %q", sendLogs[0])
	}
}

func TestSendNagsAddressLimited(t *testing.T) {
	mc := mocks.Mailer{}
	rs := newFakeRegStore()
	fc := clock.NewFake()

	staticTmpl := template.Must(template.New("expiry-email-subject-static").Parse(testEmailSubject))

	log := blog.NewMock()
	m := mailer{
		log:            log,
		mailer:         &mc,
		emailTemplate:  tmpl,
		addressLimiter: &limiter{clk: fc, limit: 1},
		// Explicitly override the default subject to use testEmailSubject
		subjectTemplate: staticTmpl,
		rs:              rs,
		clk:             fc,
		stats:           initStats(metrics.NoopRegisterer),
	}

	m.addressLimiter.inc(emailARaw)

	cert := &x509.Certificate{
		SerialNumber: big.NewInt(0x0304),
		NotAfter:     fc.Now().AddDate(0, 0, 2),
		DNSNames:     []string{"example.com"},
	}

	conn, err := m.mailer.Connect()
	test.AssertNotError(t, err, "connecting SMTP")

	// Try sending a message to an over-the-limit address
	err = m.sendNags(conn, []string{emailA}, []*x509.Certificate{cert})
	test.AssertErrorIs(t, err, errNoValidEmail)
	// Expect that no messages were sent because this address was over the limit
	test.AssertEquals(t, len(mc.Messages), 0)

	// Try sending a message to an over-the-limit address and an under-the-limit
	// one. It should only go to the under-the-limit one.
	err = m.sendNags(conn, []string{emailA, emailB}, []*x509.Certificate{cert})
	test.AssertNotError(t, err, "sending warning messages to two addresses")
	test.AssertEquals(t, len(mc.Messages), 1)
	test.AssertEquals(t, mc.Messages[0], mocks.MailerMessage{
		To:      emailBRaw,
		Subject: testEmailSubject,
		Body:    fmt.Sprintf(`hi, cert for DNS names example.com is going to expire in 2 days (%s)`, cert.NotAfter.Format(time.DateOnly)),
	})
}

var serial1 = big.NewInt(0x1336)
var serial2 = big.NewInt(0x1337)
var serial3 = big.NewInt(0x1338)
var serial4 = big.NewInt(0x1339)
var serial4String = core.SerialToString(serial4)
var serial5 = big.NewInt(0x1340)
var serial5String = core.SerialToString(serial5)
var serial6 = big.NewInt(0x1341)
var serial7 = big.NewInt(0x1342)
var serial8 = big.NewInt(0x1343)
var serial9 = big.NewInt(0x1344)

var testKey *ecdsa.PrivateKey

func init() {
	var err error
	testKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
}

func TestProcessCerts(t *testing.T) {
	expiresIn := time.Hour * 24 * 7
	testCtx := setup(t, []time.Duration{expiresIn})

	certs := addExpiringCerts(t, testCtx)
	err := testCtx.m.processCerts(context.Background(), certs, expiresIn)
	test.AssertNotError(t, err, "processing certs")
	// Test that the lastExpirationNagSent was updated for the certificate
	// corresponding to serial4, which is set up as "already renewed" by
	// addExpiringCerts.
	if len(testCtx.log.GetAllMatching("UPDATE certificateStatus.*000000000000000000000000000000001339")) != 1 {
		t.Errorf("Expected an update to certificateStatus, got these log lines:\n%s",
			strings.Join(testCtx.log.GetAll(), "\n"))
	}
}

// There's an account with an expiring certificate but no email address. We shouldn't examine
// that certificate repeatedly; we should mark it as if it had an email sent already.
func TestNoContactCertIsNotRenewed(t *testing.T) {
	expiresIn := time.Hour * 24 * 7
	testCtx := setup(t, []time.Duration{expiresIn})

	reg, err := makeRegistration(testCtx.ssa, 1, jsonKeyA, nil)
	test.AssertNotError(t, err, "Couldn't store regA")

	cert, err := makeCertificate(
		reg.Id,
		serial1,
		[]string{"example-a.com"},
		23*time.Hour,
		testCtx.fc)
	test.AssertNotError(t, err, "creating cert A")

	err = insertCertificate(cert, time.Time{})
	test.AssertNotError(t, err, "inserting certificate")

	err = testCtx.m.findExpiringCertificates(context.Background())
	test.AssertNotError(t, err, "finding expired certificates")

	// We should have sent no mail, because there was no contact address
	test.AssertEquals(t, len(testCtx.mc.Messages), 0)

	// We should have examined exactly one certificate
	certsExamined := testCtx.m.stats.certificatesExamined
	test.AssertMetricWithLabelsEquals(t, certsExamined, prometheus.Labels{}, 1.0)

	certsAlreadyRenewed := testCtx.m.stats.certificatesAlreadyRenewed
	test.AssertMetricWithLabelsEquals(t, certsAlreadyRenewed, prometheus.Labels{}, 0.0)

	// Run findExpiringCertificates again. The count of examined certificates
	// should not increase again.
	err = testCtx.m.findExpiringCertificates(context.Background())
	test.AssertNotError(t, err, "finding expired certificates")
	test.AssertMetricWithLabelsEquals(t, certsExamined, prometheus.Labels{}, 1.0)
	test.AssertMetricWithLabelsEquals(t, certsAlreadyRenewed, prometheus.Labels{}, 0.0)
}

// An account with no contact info has a certificate that is expiring but has been renewed.
// We should only examine that certificate once.
func TestNoContactCertIsRenewed(t *testing.T) {
	ctx := context.Background()

	testCtx := setup(t, []time.Duration{time.Hour * 24 * 7})

	reg, err := makeRegistration(testCtx.ssa, 1, jsonKeyA, []string{})
	test.AssertNotError(t, err, "Couldn't store regA")

	names := []string{"example-a.com"}
	cert, err := makeCertificate(
		reg.Id,
		serial1,
		names,
		23*time.Hour,
		testCtx.fc)
	test.AssertNotError(t, err, "creating cert A")

	expires := testCtx.fc.Now().Add(23 * time.Hour)

	err = insertCertificate(cert, time.Time{})
	test.AssertNotError(t, err, "inserting certificate")

	setupDBMap, err := sa.DBMapForTest(vars.DBConnSAFullPerms)
	test.AssertNotError(t, err, "setting up DB")
	err = setupDBMap.Insert(ctx, &core.FQDNSet{
		SetHash: core.HashNames(names),
		Serial:  core.SerialToString(serial2),
		Issued:  testCtx.fc.Now().Add(time.Hour),
		Expires: expires.Add(time.Hour),
	})
	test.AssertNotError(t, err, "inserting FQDNSet for renewal")

	err = testCtx.m.findExpiringCertificates(ctx)
	test.AssertNotError(t, err, "finding expired certificates")

	// We should have examined exactly one certificate
	certsExamined := testCtx.m.stats.certificatesExamined
	test.AssertMetricWithLabelsEquals(t, certsExamined, prometheus.Labels{}, 1.0)

	certsAlreadyRenewed := testCtx.m.stats.certificatesAlreadyRenewed
	test.AssertMetricWithLabelsEquals(t, certsAlreadyRenewed, prometheus.Labels{}, 1.0)

	// Run findExpiringCertificates again. The count of examined certificates
	// should not increase again.
	err = testCtx.m.findExpiringCertificates(ctx)
	test.AssertNotError(t, err, "finding expired certificates")
	test.AssertMetricWithLabelsEquals(t, certsExamined, prometheus.Labels{}, 1.0)
	test.AssertMetricWithLabelsEquals(t, certsAlreadyRenewed, prometheus.Labels{}, 1.0)
}

func TestProcessCertsParallel(t *testing.T) {
	expiresIn := time.Hour * 24 * 7
	testCtx := setup(t, []time.Duration{expiresIn})

	testCtx.m.parallelSends = 2
	certs := addExpiringCerts(t, testCtx)
	err := testCtx.m.processCerts(context.Background(), certs, expiresIn)
	test.AssertNotError(t, err, "processing certs")
	// Test that the lastExpirationNagSent was updated for the certificate
	// corresponding to serial4, which is set up as "already renewed" by
	// addExpiringCerts.
	if len(testCtx.log.GetAllMatching("UPDATE certificateStatus.*000000000000000000000000000000001339")) != 1 {
		t.Errorf("Expected an update to certificateStatus, got these log lines:\n%s",
			strings.Join(testCtx.log.GetAll(), "\n"))
	}
}

type erroringMailClient struct{}

func (e erroringMailClient) Connect() (bmail.Conn, error) {
	return nil, errors.New("whoopsie-doo")
}

func TestProcessCertsConnectError(t *testing.T) {
	expiresIn := time.Hour * 24 * 7
	testCtx := setup(t, []time.Duration{expiresIn})

	testCtx.m.mailer = erroringMailClient{}
	certs := addExpiringCerts(t, testCtx)
	// Checking that this terminates rather than deadlocks
	err := testCtx.m.processCerts(context.Background(), certs, expiresIn)
	test.AssertError(t, err, "processing certs")
}

func TestFindExpiringCertificates(t *testing.T) {
	testCtx := setup(t, []time.Duration{time.Hour * 24, time.Hour * 24 * 4, time.Hour * 24 * 7})

	addExpiringCerts(t, testCtx)

	err := testCtx.m.findExpiringCertificates(context.Background())
	test.AssertNotError(t, err, "Failed on no certificates")
	test.AssertEquals(t, len(testCtx.log.GetAllMatching("Searching for certificates that expire between.*")), 3)

	err = testCtx.m.findExpiringCertificates(context.Background())
	test.AssertNotError(t, err, "Failed to find expiring certs")
	// Should get 001 and 003
	if len(testCtx.mc.Messages) != 2 {
		builder := new(strings.Builder)
		for _, m := range testCtx.mc.Messages {
			fmt.Fprintf(builder, "%s\n", m)
		}
		t.Fatalf("Expected two messages when finding expiring certificates, got:\n%s",
			builder.String())
	}

	test.AssertEquals(t, testCtx.mc.Messages[0], mocks.MailerMessage{
		To: emailARaw,
		// A certificate with only one domain should have only one domain listed in
		// the subject
		Subject: "Testing: Let's Encrypt certificate expiration notice for domain \"example-a.com\"",
		Body:    "hi, cert for DNS names example-a.com is going to expire in 0 days (1970-01-01)",
	})
	test.AssertEquals(t, testCtx.mc.Messages[1], mocks.MailerMessage{
		To: emailBRaw,
		// A certificate with two domains should have only one domain listed and an
		// additional count included
		Subject: "Testing: Let's Encrypt certificate expiration notice for domain \"another.example-c.com\" (and 1 more)",
		Body:    "hi, cert for DNS names another.example-c.com\nexample-c.com is going to expire in 7 days (1970-01-08)",
	})

	// Check that regC's only certificate being renewed does not cause a log
	test.AssertEquals(t, len(testCtx.log.GetAllMatching("no certs given to send nags for")), 0)

	// A consecutive run shouldn't find anything
	testCtx.mc.Clear()
	err = testCtx.m.findExpiringCertificates(context.Background())
	test.AssertNotError(t, err, "Failed to find expiring certs")
	test.AssertEquals(t, len(testCtx.mc.Messages), 0)
	test.AssertMetricWithLabelsEquals(t, testCtx.m.stats.sendDelay, prometheus.Labels{"nag_group": "48h0m0s"}, 90000)
	test.AssertMetricWithLabelsEquals(t, testCtx.m.stats.sendDelay, prometheus.Labels{"nag_group": "192h0m0s"}, 82800)
}

func makeRegistration(sac sapb.StorageAuthorityClient, id int64, jsonKey []byte, contacts []string) (*corepb.Registration, error) {
	var ip [4]byte
	_, err := rand.Reader.Read(ip[:])
	if err != nil {
		return nil, err
	}
	ipText, err := net.IP(ip[:]).MarshalText()
	if err != nil {
		return nil, fmt.Errorf("formatting IP address: %s", err)
	}
	reg, err := sac.NewRegistration(context.Background(), &corepb.Registration{
		Id:        id,
		Contact:   contacts,
		Key:       jsonKey,
		InitialIP: ipText,
	})
	if err != nil {
		return nil, fmt.Errorf("storing registration: %s", err)
	}
	return reg, nil
}

func makeCertificate(regID int64, serial *big.Int, dnsNames []string, expires time.Duration, fc clock.FakeClock) (certDERWithRegID, error) {
	// Expires in <1d, last nag was the 4d nag
	template := &x509.Certificate{
		NotAfter:     fc.Now().Add(expires),
		DNSNames:     dnsNames,
		SerialNumber: serial,
	}
	certDer, err := x509.CreateCertificate(rand.Reader, template, template, &testKey.PublicKey, testKey)
	if err != nil {
		return certDERWithRegID{}, err
	}
	return certDERWithRegID{
		RegID: regID,
		DER:   certDer,
	}, nil
}

func insertCertificate(cert certDERWithRegID, lastNagSent time.Time) error {
	ctx := context.Background()

	parsedCert, err := x509.ParseCertificate(cert.DER)
	if err != nil {
		return err
	}

	setupDBMap, err := sa.DBMapForTest(vars.DBConnSAFullPerms)
	if err != nil {
		return err
	}
	err = setupDBMap.Insert(ctx, &core.Certificate{
		RegistrationID: cert.RegID,
		Serial:         core.SerialToString(parsedCert.SerialNumber),
		Issued:         parsedCert.NotBefore,
		Expires:        parsedCert.NotAfter,
		DER:            cert.DER,
	})
	if err != nil {
		return fmt.Errorf("inserting certificate: %w", err)
	}

	return setupDBMap.Insert(ctx, &core.CertificateStatus{
		Serial:                core.SerialToString(parsedCert.SerialNumber),
		LastExpirationNagSent: lastNagSent,
		Status:                core.OCSPStatusGood,
		NotAfter:              parsedCert.NotAfter,
		OCSPLastUpdated:       time.Time{},
		RevokedDate:           time.Time{},
		RevokedReason:         0,
	})
}

func addExpiringCerts(t *testing.T, ctx *testCtx) []certDERWithRegID {
	// Add some expiring certificates and registrations
	regA, err := makeRegistration(ctx.ssa, 1, jsonKeyA, []string{emailA})
	test.AssertNotError(t, err, "Couldn't store regA")
	regB, err := makeRegistration(ctx.ssa, 2, jsonKeyB, []string{emailB})
	test.AssertNotError(t, err, "Couldn't store regB")
	regC, err := makeRegistration(ctx.ssa, 3, jsonKeyC, []string{emailB})
	test.AssertNotError(t, err, "Couldn't store regC")

	// Expires in <1d, last nag was the 4d nag
	certA, err := makeCertificate(
		regA.Id,
		serial1,
		[]string{"example-a.com"},
		23*time.Hour,
		ctx.fc)
	test.AssertNotError(t, err, "creating cert A")

	// Expires in 3d, already sent 4d nag at 4.5d
	certB, err := makeCertificate(
		regA.Id,
		serial2,
		[]string{"example-b.com"},
		72*time.Hour,
		ctx.fc)
	test.AssertNotError(t, err, "creating cert B")

	// Expires in 7d and change, no nag sent at all yet
	certC, err := makeCertificate(
		regB.Id,
		serial3,
		[]string{"example-c.com", "another.example-c.com"},
		(7*24+1)*time.Hour,
		ctx.fc)
	test.AssertNotError(t, err, "creating cert C")

	// Expires in 3d, renewed
	certDNames := []string{"example-d.com"}
	certD, err := makeCertificate(
		regC.Id,
		serial4,
		certDNames,
		72*time.Hour,
		ctx.fc)
	test.AssertNotError(t, err, "creating cert D")

	fqdnStatusD := &core.FQDNSet{
		SetHash: core.HashNames(certDNames),
		Serial:  serial4String,
		Issued:  ctx.fc.Now().AddDate(0, 0, -87),
		Expires: ctx.fc.Now().AddDate(0, 0, 3),
	}
	fqdnStatusDRenewed := &core.FQDNSet{
		SetHash: core.HashNames(certDNames),
		Serial:  serial5String,
		Issued:  ctx.fc.Now().AddDate(0, 0, -3),
		Expires: ctx.fc.Now().AddDate(0, 0, 87),
	}

	err = insertCertificate(certA, ctx.fc.Now().Add(-72*time.Hour))
	test.AssertNotError(t, err, "inserting certA")
	err = insertCertificate(certB, ctx.fc.Now().Add(-36*time.Hour))
	test.AssertNotError(t, err, "inserting certB")
	err = insertCertificate(certC, ctx.fc.Now().Add(-36*time.Hour))
	test.AssertNotError(t, err, "inserting certC")
	err = insertCertificate(certD, ctx.fc.Now().Add(-36*time.Hour))
	test.AssertNotError(t, err, "inserting certD")

	setupDBMap, err := sa.DBMapForTest(vars.DBConnSAFullPerms)
	test.AssertNotError(t, err, "setting up DB")
	err = setupDBMap.Insert(context.Background(), fqdnStatusD)
	test.AssertNotError(t, err, "Couldn't add fqdnStatusD")
	err = setupDBMap.Insert(context.Background(), fqdnStatusDRenewed)
	test.AssertNotError(t, err, "Couldn't add fqdnStatusDRenewed")
	return []certDERWithRegID{certA, certB, certC, certD}
}

func countGroupsAtCapacity(group string, counter *prometheus.GaugeVec) int {
	ch := make(chan prometheus.Metric, 10)
	counter.With(prometheus.Labels{"nag_group": group}).Collect(ch)
	m := <-ch
	var iom io_prometheus_client.Metric
	_ = m.Write(&iom)
	return int(iom.Gauge.GetValue())
}

func TestFindCertsAtCapacity(t *testing.T) {
	testCtx := setup(t, []time.Duration{time.Hour * 24})

	addExpiringCerts(t, testCtx)

	// Set the limit to 1 so we are "at capacity" with one result
	testCtx.m.certificatesPerTick = 1

	err := testCtx.m.findExpiringCertificates(context.Background())
	test.AssertNotError(t, err, "Failed to find expiring certs")
	test.AssertEquals(t, len(testCtx.mc.Messages), 1)

	// The "48h0m0s" nag group should have its prometheus stat incremented once.
	// Note: this is not the 24h0m0s nag as you would expect sending time.Hour
	// * 24 to setup() for the nag duration. This is because all of the nags are
	// offset by 24 hours in this test file's setup() function, to mimic a 24h
	// setting for the "Frequency" field in the JSON config.
	test.AssertEquals(t, countGroupsAtCapacity("48h0m0s", testCtx.m.stats.nagsAtCapacity), 1)

	// A consecutive run shouldn't find anything
	testCtx.mc.Clear()
	err = testCtx.m.findExpiringCertificates(context.Background())
	test.AssertNotError(t, err, "Failed to find expiring certs")
	test.AssertEquals(t, len(testCtx.mc.Messages), 0)

	// The "48h0m0s" nag group should now be reporting that it isn't at capacity
	test.AssertEquals(t, countGroupsAtCapacity("48h0m0s", testCtx.m.stats.nagsAtCapacity), 0)
}

func TestCertIsRenewed(t *testing.T) {
	testCtx := setup(t, []time.Duration{time.Hour * 24, time.Hour * 24 * 4, time.Hour * 24 * 7})

	reg := satest.CreateWorkingRegistration(t, testCtx.ssa)

	testCerts := []*struct {
		Serial       *big.Int
		stringSerial string
		DNS          []string
		NotBefore    time.Time
		NotAfter     time.Time
		// this field is the test assertion
		IsRenewed bool
	}{
		{
			Serial:    serial1,
			DNS:       []string{"a.example.com", "a2.example.com"},
			NotBefore: testCtx.fc.Now().Add((-1 * 24) * time.Hour),
			NotAfter:  testCtx.fc.Now().Add((89 * 24) * time.Hour),
			IsRenewed: true,
		},
		{
			Serial:    serial2,
			DNS:       []string{"a.example.com", "a2.example.com"},
			NotBefore: testCtx.fc.Now().Add((0 * 24) * time.Hour),
			NotAfter:  testCtx.fc.Now().Add((90 * 24) * time.Hour),
			IsRenewed: false,
		},
		{
			Serial:    serial3,
			DNS:       []string{"b.example.net"},
			NotBefore: testCtx.fc.Now().Add((0 * 24) * time.Hour),
			NotAfter:  testCtx.fc.Now().Add((90 * 24) * time.Hour),
			IsRenewed: false,
		},
		{
			Serial:    serial4,
			DNS:       []string{"c.example.org"},
			NotBefore: testCtx.fc.Now().Add((-100 * 24) * time.Hour),
			NotAfter:  testCtx.fc.Now().Add((-10 * 24) * time.Hour),
			IsRenewed: true,
		},
		{
			Serial:    serial5,
			DNS:       []string{"c.example.org"},
			NotBefore: testCtx.fc.Now().Add((-80 * 24) * time.Hour),
			NotAfter:  testCtx.fc.Now().Add((10 * 24) * time.Hour),
			IsRenewed: true,
		},
		{
			Serial:    serial6,
			DNS:       []string{"c.example.org"},
			NotBefore: testCtx.fc.Now().Add((-75 * 24) * time.Hour),
			NotAfter:  testCtx.fc.Now().Add((15 * 24) * time.Hour),
			IsRenewed: true,
		},
		{
			Serial:    serial7,
			DNS:       []string{"c.example.org"},
			NotBefore: testCtx.fc.Now().Add((-1 * 24) * time.Hour),
			NotAfter:  testCtx.fc.Now().Add((89 * 24) * time.Hour),
			IsRenewed: false,
		},
		{
			Serial:    serial8,
			DNS:       []string{"d.example.com", "d2.example.com"},
			NotBefore: testCtx.fc.Now().Add((-1 * 24) * time.Hour),
			NotAfter:  testCtx.fc.Now().Add((89 * 24) * time.Hour),
			IsRenewed: false,
		},
		{
			Serial:    serial9,
			DNS:       []string{"d.example.com", "d2.example.com", "d3.example.com"},
			NotBefore: testCtx.fc.Now().Add((0 * 24) * time.Hour),
			NotAfter:  testCtx.fc.Now().Add((90 * 24) * time.Hour),
			IsRenewed: false,
		},
	}

	setupDBMap, err := sa.DBMapForTest(vars.DBConnSAFullPerms)
	if err != nil {
		t.Fatal(err)
	}

	for _, testData := range testCerts {
		testData.stringSerial = core.SerialToString(testData.Serial)

		rawCert := x509.Certificate{
			NotBefore:    testData.NotBefore,
			NotAfter:     testData.NotAfter,
			DNSNames:     testData.DNS,
			SerialNumber: testData.Serial,
		}
		// Can't use makeCertificate here because we also care about NotBefore
		certDer, err := x509.CreateCertificate(rand.Reader, &rawCert, &rawCert, &testKey.PublicKey, testKey)
		if err != nil {
			t.Fatal(err)
		}
		fqdnStatus := &core.FQDNSet{
			SetHash: core.HashNames(testData.DNS),
			Serial:  testData.stringSerial,
			Issued:  testData.NotBefore,
			Expires: testData.NotAfter,
		}

		err = insertCertificate(certDERWithRegID{DER: certDer, RegID: reg.Id}, time.Time{})
		test.AssertNotError(t, err, fmt.Sprintf("Couldn't add cert %s", testData.stringSerial))

		err = setupDBMap.Insert(context.Background(), fqdnStatus)
		test.AssertNotError(t, err, fmt.Sprintf("Couldn't add fqdnStatus %s", testData.stringSerial))
	}

	for _, testData := range testCerts {
		renewed, err := testCtx.m.certIsRenewed(context.Background(), testData.DNS, testData.NotBefore)
		if err != nil {
			t.Errorf("error checking renewal state for %s: %v", testData.stringSerial, err)
			continue
		}
		if renewed != testData.IsRenewed {
			t.Errorf("for %s: got %v, expected %v", testData.stringSerial, renewed, testData.IsRenewed)
		}
	}
}

func TestLifetimeOfACert(t *testing.T) {
	testCtx := setup(t, []time.Duration{time.Hour * 24, time.Hour * 24 * 4, time.Hour * 24 * 7})
	defer testCtx.cleanUp()

	regA, err := makeRegistration(testCtx.ssa, 1, jsonKeyA, []string{emailA})
	test.AssertNotError(t, err, "Couldn't store regA")

	certA, err := makeCertificate(
		regA.Id,
		serial1,
		[]string{"example-a.com"},
		0,
		testCtx.fc)
	test.AssertNotError(t, err, "making certificate")

	err = insertCertificate(certA, time.Time{})
	test.AssertNotError(t, err, "unable to insert Certificate")

	type lifeTest struct {
		timeLeft time.Duration
		numMsgs  int
		context  string
	}
	tests := []lifeTest{
		{
			timeLeft: 9 * 24 * time.Hour, // 9 days before expiration

			numMsgs: 0,
			context: "Expected no emails sent because we are more than 7 days out.",
		},
		{
			(7*24 + 12) * time.Hour, // 7.5 days before
			1,
			"Sent 1 for 7 day notice.",
		},
		{
			7 * 24 * time.Hour,
			1,
			"The 7 day email was already sent.",
		},
		{
			(4*24 - 1) * time.Hour, // <4 days before, the mailer did not run yesterday
			2,
			"Sent 1 for the 7 day notice, and 1 for the 4 day notice.",
		},
		{
			36 * time.Hour, // within 1day + nagMargin
			3,
			"Sent 1 for the 7 day notice, 1 for the 4 day notice, and 1 for the 1 day notice.",
		},
		{
			12 * time.Hour,
			3,
			"The 1 day before email was already sent.",
		},
		{
			-2 * 24 * time.Hour, // 2 days after expiration
			3,
			"No expiration warning emails are sent after expiration",
		},
	}

	for _, tt := range tests {
		testCtx.fc.Add(-tt.timeLeft)
		err = testCtx.m.findExpiringCertificates(context.Background())
		test.AssertNotError(t, err, "error calling findExpiringCertificates")
		if len(testCtx.mc.Messages) != tt.numMsgs {
			t.Errorf(tt.context+" number of messages: expected %d, got %d", tt.numMsgs, len(testCtx.mc.Messages))
		}
		testCtx.fc.Add(tt.timeLeft)
	}
}

func TestDontFindRevokedCert(t *testing.T) {
	expiresIn := 24 * time.Hour
	testCtx := setup(t, []time.Duration{expiresIn})

	regA, err := makeRegistration(testCtx.ssa, 1, jsonKeyA, []string{"mailto:one@mail.com"})
	test.AssertNotError(t, err, "Couldn't store regA")
	certA, err := makeCertificate(
		regA.Id,
		serial1,
		[]string{"example-a.com"},
		expiresIn,
		testCtx.fc)
	test.AssertNotError(t, err, "making certificate")

	err = insertCertificate(certA, time.Time{})
	test.AssertNotError(t, err, "inserting certificate")

	ctx := context.Background()

	setupDBMap, err := sa.DBMapForTest(vars.DBConnSAFullPerms)
	test.AssertNotError(t, err, "sa.NewDbMap failed")
	_, err = setupDBMap.ExecContext(ctx, "UPDATE certificateStatus SET status = ? WHERE serial = ?",
		string(core.OCSPStatusRevoked), core.SerialToString(serial1))
	test.AssertNotError(t, err, "revoking certificate")

	err = testCtx.m.findExpiringCertificates(ctx)
	test.AssertNotError(t, err, "err from findExpiringCertificates")

	if len(testCtx.mc.Messages) != 0 {
		t.Errorf("no emails should have been sent, but sent %d", len(testCtx.mc.Messages))
	}
}

func TestDedupOnRegistration(t *testing.T) {
	expiresIn := 96 * time.Hour
	testCtx := setup(t, []time.Duration{expiresIn})

	regA, err := makeRegistration(testCtx.ssa, 1, jsonKeyA, []string{emailA})
	test.AssertNotError(t, err, "Couldn't store regA")
	certA, err := makeCertificate(
		regA.Id,
		serial1,
		[]string{"example-a.com", "shared-example.com"},
		72*time.Hour,
		testCtx.fc)
	test.AssertNotError(t, err, "making certificate")
	err = insertCertificate(certA, time.Time{})
	test.AssertNotError(t, err, "inserting certificate")

	certB, err := makeCertificate(
		regA.Id,
		serial2,
		[]string{"example-b.com", "shared-example.com"},
		48*time.Hour,
		testCtx.fc)
	test.AssertNotError(t, err, "making certificate")
	err = insertCertificate(certB, time.Time{})
	test.AssertNotError(t, err, "inserting certificate")

	expires := testCtx.fc.Now().Add(48 * time.Hour)

	err = testCtx.m.findExpiringCertificates(context.Background())
	test.AssertNotError(t, err, "error calling findExpiringCertificates")
	if len(testCtx.mc.Messages) > 1 {
		t.Errorf("num of messages, want %d, got %d", 1, len(testCtx.mc.Messages))
	}
	if len(testCtx.mc.Messages) == 0 {
		t.Fatalf("no messages sent")
	}
	domains := "example-a.com\nexample-b.com\nshared-example.com"
	test.AssertEquals(t, testCtx.mc.Messages[0], mocks.MailerMessage{
		To: emailARaw,
		// A certificate with three domain names should have one in the subject and
		// a count of '2 more' at the end
		Subject: "Testing: Let's Encrypt certificate expiration notice for domain \"example-a.com\" (and 2 more)",
		Body: fmt.Sprintf(`hi, cert for DNS names %s is going to expire in 2 days (%s)`,
			domains,
			expires.Format(time.DateOnly)),
	})
}

type testCtx struct {
	dbMap   *db.WrappedMap
	ssa     sapb.StorageAuthorityClient
	mc      *mocks.Mailer
	fc      clock.FakeClock
	m       *mailer
	log     *blog.Mock
	cleanUp func()
}

func setup(t *testing.T, nagTimes []time.Duration) *testCtx {
	log := blog.NewMock()

	// We use the test_setup user (which has full permissions to everything)
	// because the SA we return is used for inserting data to set up the test.
	dbMap, err := sa.DBMapForTestWithLog(vars.DBConnSAFullPerms, log)
	if err != nil {
		t.Fatalf("Couldn't connect the database: %s", err)
	}

	fc := clock.NewFake()
	ssa, err := sa.NewSQLStorageAuthority(dbMap, dbMap, nil, 1, 0, fc, log, metrics.NoopRegisterer)
	if err != nil {
		t.Fatalf("unable to create SQLStorageAuthority: %s", err)
	}
	cleanUp := test.ResetBoulderTestDatabase(t)

	mc := &mocks.Mailer{}

	offsetNags := make([]time.Duration, len(nagTimes))
	for i, t := range nagTimes {
		offsetNags[i] = t + 24*time.Hour
	}

	m := &mailer{
		log:                 log,
		mailer:              mc,
		emailTemplate:       tmpl,
		subjectTemplate:     subjTmpl,
		dbMap:               dbMap,
		rs:                  isa.SA{Impl: ssa},
		nagTimes:            offsetNags,
		addressLimiter:      &limiter{clk: fc, limit: 4},
		certificatesPerTick: 100,
		clk:                 fc,
		stats:               initStats(metrics.NoopRegisterer),
	}
	return &testCtx{
		dbMap:   dbMap,
		ssa:     isa.SA{Impl: ssa},
		mc:      mc,
		fc:      fc,
		m:       m,
		log:     log,
		cleanUp: cleanUp,
	}
}

func TestLimiter(t *testing.T) {
	clk := clock.NewFake()
	lim := &limiter{clk: clk, limit: 4}
	fooAtExample := "foo@example.com"
	lim.inc(fooAtExample)
	test.AssertNotError(t, lim.check(fooAtExample), "expected no error")
	lim.inc(fooAtExample)
	test.AssertNotError(t, lim.check(fooAtExample), "expected no error")
	lim.inc(fooAtExample)
	test.AssertNotError(t, lim.check(fooAtExample), "expected no error")
	lim.inc(fooAtExample)
	test.AssertError(t, lim.check(fooAtExample), "expected an error")

	clk.Sleep(time.Hour)
	test.AssertError(t, lim.check(fooAtExample), "expected an error")

	// Sleep long enough to reset the limit
	clk.Sleep(24 * time.Hour)
	test.AssertNotError(t, lim.check(fooAtExample), "expected no error")
}
