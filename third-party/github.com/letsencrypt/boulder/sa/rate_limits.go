package sa

import (
	"context"
	"strings"
	"time"

	"github.com/letsencrypt/boulder/db"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/weppos/publicsuffix-go/publicsuffix"
)

// baseDomain returns the eTLD+1 of a domain name for the purpose of rate
// limiting. For a domain name that is itself an eTLD, it returns its input.
func baseDomain(name string) string {
	eTLDPlusOne, err := publicsuffix.Domain(name)
	if err != nil {
		// publicsuffix.Domain will return an error if the input name is itself a
		// public suffix. In that case we use the input name as the key for rate
		// limiting. Since all of its subdomains will have separate keys for rate
		// limiting (e.g. "foo.bar.publicsuffix.com" will have
		// "bar.publicsuffix.com", this means that domains exactly equal to a
		// public suffix get their own rate limit bucket. This is important
		// because otherwise they might be perpetually unable to issue, assuming
		// the rate of issuance from their subdomains was high enough.
		return name
	}
	return eTLDPlusOne
}

// addCertificatesPerName adds 1 to the rate limit count for the provided
// domains, in a specific time bucket. It must be executed in a transaction, and
// the input timeToTheHour must be a time rounded to an hour.
func (ssa *SQLStorageAuthority) addCertificatesPerName(ctx context.Context, db db.SelectExecer, names []string, timeToTheHour time.Time) error {
	// De-duplicate the base domains.
	baseDomainsMap := make(map[string]bool)
	var qmarks []string
	var values []interface{}
	for _, name := range names {
		base := baseDomain(name)
		if !baseDomainsMap[base] {
			baseDomainsMap[base] = true
			values = append(values, base, timeToTheHour, 1)
			qmarks = append(qmarks, "(?, ?, ?)")
		}
	}

	_, err := db.ExecContext(ctx, `INSERT INTO certificatesPerName (eTLDPlusOne, time, count) VALUES `+
		strings.Join(qmarks, ", ")+` ON DUPLICATE KEY UPDATE count=count+1;`,
		values...)
	if err != nil {
		return err
	}

	return nil
}

// countCertificates returns the count of certificates issued for a domain's
// eTLD+1 (aka base domain), during a given time range.
func (ssa *SQLStorageAuthorityRO) countCertificates(ctx context.Context, dbMap db.Selector, domain string, timeRange *sapb.Range) (int64, time.Time, error) {
	latest := timeRange.Latest.AsTime()
	var results []struct {
		Count int64
		Time  time.Time
	}
	_, err := dbMap.Select(
		ctx,
		&results,
		`SELECT count, time FROM certificatesPerName
		 WHERE eTLDPlusOne = :baseDomain AND
		 time > :earliest AND
		 time <= :latest`,
		map[string]interface{}{
			"baseDomain": baseDomain(domain),
			"earliest":   timeRange.Earliest.AsTime(),
			"latest":     latest,
		})
	if err != nil {
		if db.IsNoRows(err) {
			return 0, time.Time{}, nil
		}
		return 0, time.Time{}, err
	}
	// Set earliest to the latest possible time, so that we can find the
	// earliest certificate in the results.
	var earliest = latest
	var total int64
	for _, r := range results {
		total += r.Count
		if r.Time.Before(earliest) {
			earliest = r.Time
		}
	}
	if total <= 0 && earliest == latest {
		// If we didn't find any certificates, return a zero time.
		return total, time.Time{}, nil
	}
	return total, earliest, nil
}

// addNewOrdersRateLimit adds 1 to the rate limit count for the provided ID, in
// a specific time bucket. It must be executed in a transaction, and the input
// timeToTheMinute must be a time rounded to a minute.
func addNewOrdersRateLimit(ctx context.Context, dbMap db.SelectExecer, regID int64, timeToTheMinute time.Time) error {
	_, err := dbMap.ExecContext(ctx, `INSERT INTO newOrdersRL
		(regID, time, count)
		VALUES (?, ?, 1)
		ON DUPLICATE KEY UPDATE count=count+1;`,
		regID,
		timeToTheMinute,
	)
	if err != nil {
		return err
	}
	return nil
}

// countNewOrders returns the count of orders created in the given time range
// for the given registration ID.
func countNewOrders(ctx context.Context, dbMap db.Selector, req *sapb.CountOrdersRequest) (*sapb.Count, error) {
	var counts []int64
	_, err := dbMap.Select(
		ctx,
		&counts,
		`SELECT count FROM newOrdersRL
		WHERE regID = :regID AND
		time > :earliest AND
		time <= :latest`,
		map[string]interface{}{
			"regID":    req.AccountID,
			"earliest": req.Range.Earliest.AsTime(),
			"latest":   req.Range.Latest.AsTime(),
		},
	)
	if err != nil {
		if db.IsNoRows(err) {
			return &sapb.Count{Count: 0}, nil
		}
		return nil, err
	}
	var total int64
	for _, count := range counts {
		total += count
	}
	return &sapb.Count{Count: total}, nil
}
