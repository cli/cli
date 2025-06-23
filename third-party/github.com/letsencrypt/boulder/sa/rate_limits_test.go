package sa

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/test"
)

func TestCertsPerNameRateLimitTable(t *testing.T) {
	ctx := context.Background()

	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	aprilFirst, err := time.Parse(time.RFC3339, "2019-04-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}

	type inputCase struct {
		time  time.Time
		names []string
	}
	inputs := []inputCase{
		{aprilFirst, []string{"example.com"}},
		{aprilFirst, []string{"example.com", "www.example.com"}},
		{aprilFirst, []string{"example.com", "other.example.com"}},
		{aprilFirst, []string{"dyndns.org"}},
		{aprilFirst, []string{"mydomain.dyndns.org"}},
		{aprilFirst, []string{"mydomain.dyndns.org"}},
		{aprilFirst, []string{"otherdomain.dyndns.org"}},
	}

	// For each hour in a week, add an entry for a certificate that has
	// progressively more names.
	var manyNames []string
	for i := range 7 * 24 {
		manyNames = append(manyNames, fmt.Sprintf("%d.manynames.example.net", i))
		inputs = append(inputs, inputCase{aprilFirst.Add(time.Duration(i) * time.Hour), manyNames})
	}

	for _, input := range inputs {
		tx, err := sa.dbMap.BeginTx(ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = sa.addCertificatesPerName(ctx, tx, input.names, input.time)
		if err != nil {
			t.Fatal(err)
		}
		err = tx.Commit()
		if err != nil {
			t.Fatal(err)
		}
	}

	const aWeek = time.Duration(7*24) * time.Hour

	testCases := []struct {
		caseName   string
		domainName string
		expected   int64
	}{
		{"name doesn't exist", "non.example.org", 0},
		{"base name gets dinged for all certs including it", "example.com", 3},
		{"subdomain gets dinged for neighbors", "www.example.com", 3},
		{"other subdomain", "other.example.com", 3},
		{"many subdomains", "1.manynames.example.net", 168},
		{"public suffix gets its own bucket", "dyndns.org", 1},
		{"subdomain of public suffix gets its own bucket", "mydomain.dyndns.org", 2},
		{"subdomain of public suffix gets its own bucket 2", "otherdomain.dyndns.org", 1},
	}

	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			timeRange := &sapb.Range{
				Earliest: timestamppb.New(aprilFirst.Add(-1 * time.Second)),
				Latest:   timestamppb.New(aprilFirst.Add(aWeek)),
			}
			count, earliest, err := sa.countCertificatesByName(ctx, sa.dbMap, tc.domainName, timeRange)
			if err != nil {
				t.Fatal(err)
			}
			if count != tc.expected {
				t.Errorf("Expected count of %d for %q, got %d", tc.expected, tc.domainName, count)
			}
			if earliest.IsZero() {
				// The count should always be zero if earliest is nil.
				test.AssertEquals(t, count, int64(0))
			} else {
				test.AssertEquals(t, earliest, aprilFirst)
			}
		})
	}
}

func TestNewOrdersRateLimitTable(t *testing.T) {
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	manyCountRegID := int64(2)
	start := time.Now().Truncate(time.Minute)
	req := &sapb.CountOrdersRequest{
		AccountID: 1,
		Range: &sapb.Range{
			Earliest: timestamppb.New(start),
			Latest:   timestamppb.New(start.Add(time.Minute * 10)),
		},
	}

	for i := 0; i <= 10; i++ {
		tx, err := sa.dbMap.BeginTx(ctx)
		test.AssertNotError(t, err, "failed to open tx")
		for j := 0; j < i+1; j++ {
			err = addNewOrdersRateLimit(ctx, tx, manyCountRegID, start.Add(time.Minute*time.Duration(i)))
		}
		test.AssertNotError(t, err, "addNewOrdersRateLimit failed")
		test.AssertNotError(t, tx.Commit(), "failed to commit tx")
	}

	count, err := countNewOrders(ctx, sa.dbMap, req)
	test.AssertNotError(t, err, "countNewOrders failed")
	test.AssertEquals(t, count.Count, int64(0))

	req.AccountID = manyCountRegID
	count, err = countNewOrders(ctx, sa.dbMap, req)
	test.AssertNotError(t, err, "countNewOrders failed")
	test.AssertEquals(t, count.Count, int64(65))

	req.Range.Earliest = timestamppb.New(start.Add(time.Minute * 5))
	req.Range.Latest = timestamppb.New(start.Add(time.Minute * 10))
	count, err = countNewOrders(ctx, sa.dbMap, req)
	test.AssertNotError(t, err, "countNewOrders failed")
	test.AssertEquals(t, count.Count, int64(45))
}
