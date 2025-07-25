package updater

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"github.com/letsencrypt/boulder/issuance"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/test"
)

func TestRunOnce(t *testing.T) {
	e1, err := issuance.LoadCertificate("../../test/hierarchy/int-e1.cert.pem")
	test.AssertNotError(t, err, "loading test issuer")
	r3, err := issuance.LoadCertificate("../../test/hierarchy/int-r3.cert.pem")
	test.AssertNotError(t, err, "loading test issuer")

	mockLog := blog.NewMock()
	clk := clock.NewFake()
	clk.Set(time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC))
	cu, err := NewUpdater(
		[]*issuance.Certificate{e1, r3},
		2, 18*time.Hour, 24*time.Hour,
		6*time.Hour, time.Minute, 1, 1,
		"stale-if-error=60",
		5*time.Minute,
		nil,
		&fakeSAC{revokedCerts: revokedCertsStream{err: errors.New("db no worky")}, maxNotAfter: clk.Now().Add(90 * 24 * time.Hour)},
		&fakeCA{gcc: generateCRLStream{}},
		&fakeStorer{uploaderStream: &noopUploader{}},
		metrics.NoopRegisterer, mockLog, clk,
	)
	test.AssertNotError(t, err, "building test crlUpdater")

	// An error that affects all issuers should have every issuer reflected in the
	// combined error message.
	err = cu.RunOnce(context.Background())
	test.AssertError(t, err, "database error")
	test.AssertContains(t, err.Error(), "one or more errors")
	test.AssertEquals(t, len(mockLog.GetAllMatching("Generating CRL failed:")), 4)
	cu.tickHistogram.Reset()
}
