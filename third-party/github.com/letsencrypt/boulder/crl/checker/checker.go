package checker

import (
	"bytes"
	"crypto/x509"
	"fmt"
	"math/big"
	"sort"
	"time"

	zlint_x509 "github.com/zmap/zcrypto/x509"
	"github.com/zmap/zlint/v3"

	"github.com/letsencrypt/boulder/linter"
)

// Validate runs the given CRL through our set of lints, ensures its signature
// validates (if supplied with a non-nil issuer), and checks that the CRL is
// less than ageLimit old. It returns an error if any of these conditions are
// not met.
func Validate(crl *x509.RevocationList, issuer *x509.Certificate, ageLimit time.Duration) error {
	zcrl, err := zlint_x509.ParseRevocationList(crl.Raw)
	if err != nil {
		return fmt.Errorf("parsing CRL: %w", err)
	}

	err = linter.ProcessResultSet(zlint.LintRevocationList(zcrl))
	if err != nil {
		return fmt.Errorf("linting CRL: %w", err)
	}

	if issuer != nil {
		err = crl.CheckSignatureFrom(issuer)
		if err != nil {
			return fmt.Errorf("checking CRL signature: %w", err)
		}
	}

	if time.Since(crl.ThisUpdate) >= ageLimit {
		return fmt.Errorf("thisUpdate more than %s in the past: %v", ageLimit, crl.ThisUpdate)
	}

	return nil
}

type diffResult struct {
	Added   []*big.Int
	Removed []*big.Int
	// TODO: consider adding a "changed" field, for entries whose revocation time
	// or revocation reason changes.
}

// Diff returns the sets of serials that were added and removed between two
// CRLs. In order to be comparable, the CRLs must come from the same issuer, and
// be given in the correct order (the "old" CRL's Number and ThisUpdate must
// both precede the "new" CRL's).
func Diff(old, new *x509.RevocationList) (*diffResult, error) {
	if !bytes.Equal(old.AuthorityKeyId, new.AuthorityKeyId) {
		return nil, fmt.Errorf("CRLs were not issued by same issuer")
	}

	if !old.ThisUpdate.Before(new.ThisUpdate) {
		return nil, fmt.Errorf("old CRL does not precede new CRL")
	}

	if old.Number.Cmp(new.Number) >= 0 {
		return nil, fmt.Errorf("old CRL does not precede new CRL")
	}

	// Sort both sets of serials so we can march through them in order.
	oldSerials := make([]*big.Int, len(old.RevokedCertificateEntries))
	for i, rc := range old.RevokedCertificateEntries {
		oldSerials[i] = rc.SerialNumber
	}
	sort.Slice(oldSerials, func(i, j int) bool {
		return oldSerials[i].Cmp(oldSerials[j]) < 0
	})

	newSerials := make([]*big.Int, len(new.RevokedCertificateEntries))
	for j, rc := range new.RevokedCertificateEntries {
		newSerials[j] = rc.SerialNumber
	}
	sort.Slice(newSerials, func(i, j int) bool {
		return newSerials[i].Cmp(newSerials[j]) < 0
	})

	// Work our way through both lists of sorted serials. If the old list skips
	// past a serial seen in the new list, then that serial was added. If the new
	// list skips past a serial seen in the old list, then it was removed.
	i, j := 0, 0
	added := make([]*big.Int, 0)
	removed := make([]*big.Int, 0)
	for {
		if i >= len(oldSerials) {
			added = append(added, newSerials[j:]...)
			break
		}
		if j >= len(newSerials) {
			removed = append(removed, oldSerials[i:]...)
			break
		}
		cmp := oldSerials[i].Cmp(newSerials[j])
		if cmp < 0 {
			removed = append(removed, oldSerials[i])
			i++
		} else if cmp > 0 {
			added = append(added, newSerials[j])
			j++
		} else {
			i++
			j++
		}
	}

	return &diffResult{added, removed}, nil
}
