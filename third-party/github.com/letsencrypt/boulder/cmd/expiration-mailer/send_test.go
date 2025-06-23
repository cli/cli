package notmain

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/letsencrypt/boulder/mocks"
	"github.com/letsencrypt/boulder/test"
)

var (
	email1 = "mailto:one@shared-example.com"
	email2 = "mailto:two@shared-example.com"
)

func TestSendEarliestCertInfo(t *testing.T) {
	expiresIn := 24 * time.Hour
	ctx := setup(t, []time.Duration{expiresIn})
	defer ctx.cleanUp()

	rawCertA := newX509Cert("happy A",
		ctx.fc.Now().AddDate(0, 0, 5),
		[]string{"example-A.com", "SHARED-example.com"},
		serial1,
	)
	rawCertB := newX509Cert("happy B",
		ctx.fc.Now().AddDate(0, 0, 2),
		[]string{"shared-example.com", "example-b.com"},
		serial2,
	)

	conn, err := ctx.m.mailer.Connect()
	test.AssertNotError(t, err, "connecting SMTP")
	err = ctx.m.sendNags(conn, []string{email1, email2}, []*x509.Certificate{rawCertA, rawCertB})
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.mc.Messages) != 2 {
		t.Errorf("num of messages, want %d, got %d", 2, len(ctx.mc.Messages))
	}
	if len(ctx.mc.Messages) == 0 {
		t.Fatalf("no message sent")
	}
	domains := "example-a.com\nexample-b.com\nshared-example.com"
	expected := mocks.MailerMessage{
		Subject: "Testing: Let's Encrypt certificate expiration notice for domain \"example-a.com\" (and 2 more)",
		Body: fmt.Sprintf(`hi, cert for DNS names %s is going to expire in 2 days (%s)`,
			domains,
			rawCertB.NotAfter.Format(time.DateOnly)),
	}
	expected.To = "one@shared-example.com"
	test.AssertEquals(t, expected, ctx.mc.Messages[0])
	expected.To = "two@shared-example.com"
	test.AssertEquals(t, expected, ctx.mc.Messages[1])
}

func newX509Cert(commonName string, notAfter time.Time, dnsNames []string, serial *big.Int) *x509.Certificate {
	return &x509.Certificate{
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotAfter:     notAfter,
		DNSNames:     dnsNames,
		SerialNumber: serial,
	}

}
