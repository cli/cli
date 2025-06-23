package va

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/letsencrypt/boulder/bdns"
	"github.com/letsencrypt/boulder/canceled"
	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/features"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/probs"
	vapb "github.com/letsencrypt/boulder/va/proto"
)

type caaParams struct {
	accountURIID     int64
	validationMethod core.AcmeChallenge
}

// IsCAAValid checks requested CAA records from a VA, and recursively any RVAs
// configured in the VA. It returns a response or an error.
func (va *ValidationAuthorityImpl) IsCAAValid(ctx context.Context, req *vapb.IsCAAValidRequest) (*vapb.IsCAAValidResponse, error) {
	if core.IsAnyNilOrZero(req.Domain, req.ValidationMethod, req.AccountURIID) {
		return nil, berrors.InternalServerError("incomplete IsCAAValid request")
	}
	logEvent := verificationRequestEvent{
		// TODO(#7061) Plumb req.Authz.Id as "ID:" through from the RA to
		// correlate which authz triggered this request.
		Requester: req.AccountURIID,
		Hostname:  req.Domain,
	}
	checkStartTime := va.clk.Now()

	validationMethod := core.AcmeChallenge(req.ValidationMethod)
	if !validationMethod.IsValid() {
		return nil, berrors.InternalServerError("unrecognized validation method %q", req.ValidationMethod)
	}

	acmeID := identifier.ACMEIdentifier{
		Type:  identifier.DNS,
		Value: req.Domain,
	}
	params := &caaParams{
		accountURIID:     req.AccountURIID,
		validationMethod: validationMethod,
	}

	var remoteCAAResults chan *remoteVAResult
	if features.Get().EnforceMultiCAA {
		if remoteVACount := len(va.remoteVAs); remoteVACount > 0 {
			remoteCAAResults = make(chan *remoteVAResult, remoteVACount)
			go va.performRemoteCAACheck(ctx, req, remoteCAAResults)
		}
	}

	checkResult := "success"
	err := va.checkCAA(ctx, acmeID, params)
	localCheckLatency := time.Since(checkStartTime)
	var prob *probs.ProblemDetails
	if err != nil {
		prob = detailedError(err)
		logEvent.Error = prob.Error()
		logEvent.InternalError = err.Error()
		prob.Detail = fmt.Sprintf("While processing CAA for %s: %s", req.Domain, prob.Detail)
		checkResult = "failure"
	} else if remoteCAAResults != nil {
		if !features.Get().EnforceMultiCAA && features.Get().MultiCAAFullResults {
			// If we're not going to enforce multi CAA but we are logging the
			// differentials then collect and log the remote results in a separate go
			// routine to avoid blocking the primary VA.
			go func() {
				_ = va.processRemoteCAAResults(
					req.Domain,
					req.AccountURIID,
					string(validationMethod),
					remoteCAAResults)
			}()
		} else if features.Get().EnforceMultiCAA {
			remoteProb := va.processRemoteCAAResults(
				req.Domain,
				req.AccountURIID,
				string(validationMethod),
				remoteCAAResults)

			// If the remote result was a non-nil problem then fail the CAA check
			if remoteProb != nil {
				prob = remoteProb
				// We only set .Error here, not InternalError, because the remote VA doesn't send
				// us the internal error. But that's okay, because it got logged at the remote VA.
				logEvent.Error = remoteProb.Error()
				checkResult = "failure"
				va.log.Infof("CAA check failed due to remote failures: identifier=%v err=%s",
					req.Domain, remoteProb)
				va.metrics.remoteCAACheckFailures.Inc()
			}
		}
	}
	checkLatency := time.Since(checkStartTime)
	logEvent.ValidationLatency = checkLatency.Round(time.Millisecond).Seconds()

	va.metrics.localCAACheckTime.With(prometheus.Labels{
		"result": checkResult,
	}).Observe(localCheckLatency.Seconds())
	va.metrics.caaCheckTime.With(prometheus.Labels{
		"result": checkResult,
	}).Observe(checkLatency.Seconds())

	va.log.AuditObject("CAA check result", logEvent)

	if prob != nil {
		// The ProblemDetails will be serialized through gRPC, which requires UTF-8.
		// It will also later be serialized in JSON, which defaults to UTF-8. Make
		// sure it is UTF-8 clean now.
		prob = filterProblemDetails(prob)
		return &vapb.IsCAAValidResponse{Problem: &corepb.ProblemDetails{
			ProblemType: string(prob.Type),
			Detail:      replaceInvalidUTF8([]byte(prob.Detail)),
		}}, nil
	} else {
		return &vapb.IsCAAValidResponse{}, nil
	}
}

// processRemoteCAAResults evaluates a primary VA result, and a channel of
// remote VA problems to produce a single overall validation result based on
// configured feature flags. The overall result is calculated based on the VA's
// configured `maxRemoteFailures` value.
//
// If the `MultiCAAFullResults` feature is enabled then
// `processRemoteCAAResults` will expect to read a result from the
// `remoteResultsChan` channel for each VA and will not produce an overall
// result until all remote VAs have responded. In this case
// `logRemoteDifferentials` will also be called to describe the differential
// between the primary and all of the remote VAs.
//
// If the `MultiCAAFullResults` feature flag is not enabled then
// `processRemoteCAAResults` will potentially return before all remote VAs have
// had a chance to respond. This happens if the success or failure threshold is
// met. This doesn't allow for logging the differential between the primary and
// remote VAs but is more performant.
func (va *ValidationAuthorityImpl) processRemoteCAAResults(
	domain string,
	acctID int64,
	challengeType string,
	remoteResultsChan <-chan *remoteVAResult) *probs.ProblemDetails {

	state := "failure"
	start := va.clk.Now()

	defer func() {
		va.metrics.remoteCAACheckTime.With(prometheus.Labels{
			"result": state,
		}).Observe(va.clk.Since(start).Seconds())
	}()

	required := len(va.remoteVAs) - va.maxRemoteFailures
	good := 0
	bad := 0

	var remoteResults []*remoteVAResult
	var firstProb *probs.ProblemDetails
	// Due to channel behavior this could block indefinitely and we rely on gRPC
	// honoring the context deadline used in client calls to prevent that from
	// happening.
	for result := range remoteResultsChan {
		// Add the result to the slice
		remoteResults = append(remoteResults, result)
		if result.Problem == nil {
			good++
		} else {
			bad++
			// Store the first non-nil problem to return later (if `MultiCAAFullResults`
			// is enabled).
			if firstProb == nil {
				firstProb = result.Problem
			}
		}

		// If MultiCAAFullResults isn't enabled then return early whenever the
		// success or failure threshold is met.
		if !features.Get().MultiCAAFullResults {
			if good >= required {
				state = "success"
				return nil
			} else if bad > va.maxRemoteFailures {
				modifiedProblem := *result.Problem
				modifiedProblem.Detail = "During secondary CAA checking: " + firstProb.Detail
				return &modifiedProblem
			}
		}

		// If we haven't returned early because of MultiCAAFullResults being
		// enabled we need to break the loop once all of the VAs have returned a
		// result.
		if len(remoteResults) == len(va.remoteVAs) {
			break
		}
	}
	// If we are using `features.MultiCAAFullResults` then we haven't returned
	// early and can now log the differential between what the primary VA saw and
	// what all of the remote VAs saw.
	va.logRemoteResults(
		domain,
		acctID,
		challengeType,
		remoteResults)

	// Based on the threshold of good/bad return nil or a problem.
	if good >= required {
		state = "success"
		return nil
	} else if bad > va.maxRemoteFailures {
		modifiedProblem := *firstProb
		modifiedProblem.Detail = "During secondary CAA checking: " + firstProb.Detail
		va.metrics.prospectiveRemoteCAACheckFailures.Inc()
		return &modifiedProblem
	}

	// This condition should not occur - it indicates the good/bad counts didn't
	// meet either the required threshold or the maxRemoteFailures threshold.
	return probs.ServerInternal("Too few remote IsCAAValid RPC results")
}

// performRemoteCAACheck calls `isCAAValid` for each of the configured remoteVAs
// in a random order. The provided `results` chan should have an equal size to
// the number of remote VAs. The CAA checks will be performed in separate
// go-routines. If the result `error` from a remote `isCAAValid` RPC is nil or a
// nil `ProblemDetails` instance it is written directly to the `results` chan.
// If the err is a cancelled error it is treated as a nil error. Otherwise the
// error/problem is written to the results channel as-is.
func (va *ValidationAuthorityImpl) performRemoteCAACheck(
	ctx context.Context,
	req *vapb.IsCAAValidRequest,
	results chan<- *remoteVAResult) {
	for _, i := range rand.Perm(len(va.remoteVAs)) {
		remoteVA := va.remoteVAs[i]
		go func(rva RemoteVA) {
			result := &remoteVAResult{
				VAHostname: rva.Address,
			}
			res, err := rva.IsCAAValid(ctx, req)
			if err != nil {
				if canceled.Is(err) {
					// Handle the cancellation error.
					result.Problem = probs.ServerInternal("Remote VA IsCAAValid RPC cancelled")
				} else {
					// Handle validation error.
					va.log.Errf("Remote VA %q.IsCAAValid failed: %s", rva.Address, err)
					result.Problem = probs.ServerInternal("Remote VA IsCAAValid RPC failed")
				}
			} else if res.Problem != nil {
				prob, err := bgrpc.PBToProblemDetails(res.Problem)
				if err != nil {
					va.log.Infof("Remote VA %q.IsCAAValid returned malformed problem: %s", rva.Address, err)
					result.Problem = probs.ServerInternal(
						fmt.Sprintf("Remote VA IsCAAValid RPC returned malformed result: %s", err))
				} else {
					va.log.Infof("Remote VA %q.IsCAAValid returned problem: %s", rva.Address, prob)
					result.Problem = prob
				}
			}
			results <- result
		}(remoteVA)
	}
}

// checkCAA performs a CAA lookup & validation for the provided identifier. If
// the CAA lookup & validation fail a problem is returned.
func (va *ValidationAuthorityImpl) checkCAA(
	ctx context.Context,
	identifier identifier.ACMEIdentifier,
	params *caaParams) error {
	if core.IsAnyNilOrZero(params, params.validationMethod, params.accountURIID) {
		return probs.ServerInternal("expected validationMethod or accountURIID not provided to checkCAA")
	}

	foundAt, valid, response, err := va.checkCAARecords(ctx, identifier, params)
	if err != nil {
		return berrors.DNSError("%s", err)
	}

	va.log.AuditInfof("Checked CAA records for %s, [Present: %t, Account ID: %d, Challenge: %s, Valid for issuance: %t, Found at: %q] Response=%q",
		identifier.Value, foundAt != "", params.accountURIID, params.validationMethod, valid, foundAt, response)
	if !valid {
		return berrors.CAAError("CAA record for %s prevents issuance", foundAt)
	}
	return nil
}

// caaResult represents the result of querying CAA for a single name. It breaks
// the CAA resource records down by category, keeping only the issue and
// issuewild records. It also records whether any unrecognized RRs were marked
// critical, and stores the raw response text for logging and debugging.
type caaResult struct {
	name            string
	present         bool
	issue           []*dns.CAA
	issuewild       []*dns.CAA
	criticalUnknown bool
	dig             string
	resolvers       bdns.ResolverAddrs
	err             error
}

// filterCAA processes a set of CAA resource records and picks out the only bits
// we care about. It returns two slices of CAA records, representing the issue
// records and the issuewild records respectively, and a boolean indicating
// whether any unrecognized records had the critical bit set.
func filterCAA(rrs []*dns.CAA) ([]*dns.CAA, []*dns.CAA, bool) {
	var issue, issuewild []*dns.CAA
	var criticalUnknown bool

	for _, caaRecord := range rrs {
		switch strings.ToLower(caaRecord.Tag) {
		case "issue":
			issue = append(issue, caaRecord)
		case "issuewild":
			issuewild = append(issuewild, caaRecord)
		case "iodef":
			// We support the iodef property tag insofar as we recognize it, but we
			// never choose to send notifications to the specified addresses. So we
			// do not store the contents of the property tag, but also avoid setting
			// the criticalUnknown bit if there are critical iodef tags.
			continue
		case "issuemail":
			// We support the issuemail property tag insofar as we recognize it and
			// therefore do not bail out if someone has a critical issuemail tag. But
			// of course we do not do any further processing, as we do not issue
			// S/MIME certificates.
			continue
		default:
			// The critical flag is the bit with significance 128. However, many CAA
			// record users have misinterpreted the RFC and concluded that the bit
			// with significance 1 is the critical bit. This is sufficiently
			// widespread that that bit must reasonably be considered an alias for
			// the critical bit. The remaining bits are 0/ignore as proscribed by the
			// RFC.
			if (caaRecord.Flag & (128 | 1)) != 0 {
				criticalUnknown = true
			}
		}
	}

	return issue, issuewild, criticalUnknown
}

// parallelCAALookup makes parallel requests for the target name and all parent
// names. It returns a slice of CAA results, with the results from querying the
// FQDN in the zeroth index, and the results from querying the TLD in the last
// index.
func (va *ValidationAuthorityImpl) parallelCAALookup(ctx context.Context, name string) []caaResult {
	labels := strings.Split(name, ".")
	results := make([]caaResult, len(labels))
	var wg sync.WaitGroup

	for i := range len(labels) {
		// Start the concurrent DNS lookup.
		wg.Add(1)
		go func(name string, r *caaResult) {
			r.name = name
			var records []*dns.CAA
			records, r.dig, r.resolvers, r.err = va.dnsClient.LookupCAA(ctx, name)
			if len(records) > 0 {
				r.present = true
			}
			r.issue, r.issuewild, r.criticalUnknown = filterCAA(records)
			wg.Done()
		}(strings.Join(labels[i:], "."), &results[i])
	}

	wg.Wait()
	return results
}

// selectCAA picks the relevant CAA resource record set to be used, i.e. the set
// for the "closest parent" of the FQDN in question, including the domain
// itself. If we encountered an error for a lookup before we found a successful,
// non-empty response, assume there could have been real records hidden by it,
// and return that error.
func selectCAA(rrs []caaResult) (*caaResult, error) {
	for _, res := range rrs {
		if res.err != nil {
			return nil, res.err
		}
		if res.present {
			return &res, nil
		}
	}
	return nil, nil
}

// getCAA returns the CAA Relevant Resource Set[1] for the given FQDN, i.e. the
// first CAA RRSet found by traversing upwards from the FQDN by removing the
// leftmost label. It returns nil if no RRSet is found on any parent of the
// given FQDN. The returned result also contains the raw CAA response, and an
// error if one is encountered while querying or parsing the records.
//
// [1]: https://datatracker.ietf.org/doc/html/rfc8659#name-relevant-resource-record-se
func (va *ValidationAuthorityImpl) getCAA(ctx context.Context, hostname string) (*caaResult, error) {
	hostname = strings.TrimRight(hostname, ".")

	// See RFC 6844 "Certification Authority Processing" for pseudocode, as
	// amended by https://www.rfc-editor.org/errata/eid5065.
	// Essentially: check CAA records for the FDQN to be issued, and all
	// parent domains.
	//
	// The lookups are performed in parallel in order to avoid timing out
	// the RPC call.
	//
	// We depend on our resolver to snap CNAME and DNAME records.
	results := va.parallelCAALookup(ctx, hostname)
	return selectCAA(results)
}

// checkCAARecords fetches the CAA records for the given identifier and then
// validates them. If the identifier argument's value has a wildcard prefix then
// the prefix is stripped and validation will be performed against the base
// domain, honouring any issueWild CAA records encountered as appropriate.
// checkCAARecords returns four values: the first is a string indicating at
// which name (i.e. FQDN or parent thereof) CAA records were found, if any. The
// second is a bool indicating whether issuance for the identifier is valid. The
// unmodified *dns.CAA records that were processed/filtered are returned as the
// third argument. Any  errors encountered are returned as the fourth return
// value (or nil).
func (va *ValidationAuthorityImpl) checkCAARecords(
	ctx context.Context,
	identifier identifier.ACMEIdentifier,
	params *caaParams) (string, bool, string, error) {
	hostname := strings.ToLower(identifier.Value)
	// If this is a wildcard name, remove the prefix
	var wildcard bool
	if strings.HasPrefix(hostname, `*.`) {
		hostname = strings.TrimPrefix(identifier.Value, `*.`)
		wildcard = true
	}
	caaSet, err := va.getCAA(ctx, hostname)
	if err != nil {
		return "", false, "", err
	}
	raw := ""
	if caaSet != nil {
		raw = caaSet.dig
	}
	valid, foundAt := va.validateCAA(caaSet, wildcard, params)
	return foundAt, valid, raw, nil
}

// validateCAA checks a provided *caaResult. When the wildcard argument is true
// this means the issueWild records must be validated as well. This function
// returns a boolean indicating whether issuance is allowed by this set of CAA
// records, and a string indicating the name at which the CAA records allowing
// issuance were found (if any -- since finding no records at all allows
// issuance).
func (va *ValidationAuthorityImpl) validateCAA(caaSet *caaResult, wildcard bool, params *caaParams) (bool, string) {
	if caaSet == nil {
		// No CAA records found, can issue
		va.metrics.caaCounter.WithLabelValues("no records").Inc()
		return true, ""
	}

	if caaSet.criticalUnknown {
		// Contains unknown critical directives
		va.metrics.caaCounter.WithLabelValues("record with unknown critical directive").Inc()
		return false, caaSet.name
	}

	if len(caaSet.issue) == 0 && !wildcard {
		// Although CAA records exist, none of them pertain to issuance in this case.
		// (e.g. there is only an issuewild directive, but we are checking for a
		// non-wildcard identifier, or there is only an iodef or non-critical unknown
		// directive.)
		va.metrics.caaCounter.WithLabelValues("no relevant records").Inc()
		return true, caaSet.name
	}

	// Per RFC 8659 Section 5.3:
	//   - "Each issuewild Property MUST be ignored when processing a request for
	//     an FQDN that is not a Wildcard Domain Name."; and
	//   - "If at least one issuewild Property is specified in the Relevant RRset
	//     for a Wildcard Domain Name, each issue Property MUST be ignored when
	//     processing a request for that Wildcard Domain Name."
	// So we default to checking the `caaSet.Issue` records and only check
	// `caaSet.Issuewild` when `wildcard` is true and there are 1 or more
	// `Issuewild` records.
	records := caaSet.issue
	if wildcard && len(caaSet.issuewild) > 0 {
		records = caaSet.issuewild
	}

	// There are CAA records pertaining to issuance in our case. Note that this
	// includes the case of the unsatisfiable CAA record value ";", used to
	// prevent issuance by any CA under any circumstance.
	//
	// Our CAA identity must be found in the chosen checkSet.
	for _, caa := range records {
		parsedDomain, parsedParams, err := parseCAARecord(caa)
		if err != nil {
			continue
		}

		if !caaDomainMatches(parsedDomain, va.issuerDomain) {
			continue
		}

		if !caaAccountURIMatches(parsedParams, va.accountURIPrefixes, params.accountURIID) {
			continue
		}

		if !caaValidationMethodMatches(parsedParams, params.validationMethod) {
			continue
		}

		va.metrics.caaCounter.WithLabelValues("authorized").Inc()
		return true, caaSet.name
	}

	// The list of authorized issuers is non-empty, but we are not in it. Fail.
	va.metrics.caaCounter.WithLabelValues("unauthorized").Inc()
	return false, caaSet.name
}

// parseCAARecord extracts the domain and parameters (if any) from a
// issue/issuewild CAA record. This follows RFC 8659 Section 4.2 and Section 4.3
// (https://www.rfc-editor.org/rfc/rfc8659.html#section-4). It returns the
// domain name (which may be the empty string if the record forbids issuance)
// and a tag-value map of CAA parameters, or a descriptive error if the record
// is malformed.
func parseCAARecord(caa *dns.CAA) (string, map[string]string, error) {
	isWSP := func(r rune) bool {
		return r == '\t' || r == ' '
	}

	// Semi-colons (ASCII 0x3B) are prohibited from being specified in the
	// parameter tag or value, hence we can simply split on semi-colons.
	parts := strings.Split(caa.Value, ";")
	domain := strings.TrimFunc(parts[0], isWSP)
	paramList := parts[1:]
	parameters := make(map[string]string)

	// Handle the case where a semi-colon is specified following the domain
	// but no parameters are given.
	if len(paramList) == 1 && strings.TrimFunc(paramList[0], isWSP) == "" {
		return domain, parameters, nil
	}

	for _, parameter := range paramList {
		// A parameter tag cannot include equal signs (ASCII 0x3D),
		// however they are permitted in the value itself.
		tv := strings.SplitN(parameter, "=", 2)
		if len(tv) != 2 {
			return "", nil, fmt.Errorf("parameter not formatted as tag=value: %q", parameter)
		}

		tag := strings.TrimFunc(tv[0], isWSP)
		//lint:ignore S1029,SA6003 we iterate over runes because the RFC specifies ascii codepoints.
		for _, r := range []rune(tag) {
			// ASCII alpha/digits.
			// tag = (ALPHA / DIGIT) *( *("-") (ALPHA / DIGIT))
			if r < 0x30 || (r > 0x39 && r < 0x41) || (r > 0x5a && r < 0x61) || r > 0x7a {
				return "", nil, fmt.Errorf("tag contains disallowed character: %q", tag)
			}
		}

		value := strings.TrimFunc(tv[1], isWSP)
		//lint:ignore S1029,SA6003 we iterate over runes because the RFC specifies ascii codepoints.
		for _, r := range []rune(value) {
			// ASCII without whitespace/semi-colons.
			// value = *(%x21-3A / %x3C-7E)
			if r < 0x21 || (r > 0x3a && r < 0x3c) || r > 0x7e {
				return "", nil, fmt.Errorf("value contains disallowed character: %q", value)
			}
		}

		parameters[tag] = value
	}

	return domain, parameters, nil
}

// caaDomainMatches checks that the issuer domain name listed in the parsed
// CAA record matches the domain name we expect.
func caaDomainMatches(caaDomain string, issuerDomain string) bool {
	return caaDomain == issuerDomain
}

// caaAccountURIMatches checks that the accounturi CAA parameter, if present,
// matches one of the specific account URIs we expect. We support multiple
// account URI prefixes to handle accounts which were registered under ACMEv1.
// See RFC 8657 Section 3: https://www.rfc-editor.org/rfc/rfc8657.html#section-3
func caaAccountURIMatches(caaParams map[string]string, accountURIPrefixes []string, accountID int64) bool {
	accountURI, ok := caaParams["accounturi"]
	if !ok {
		return true
	}

	// If the accounturi is not formatted according to RFC 3986, reject it.
	_, err := url.Parse(accountURI)
	if err != nil {
		return false
	}

	for _, prefix := range accountURIPrefixes {
		if accountURI == fmt.Sprintf("%s%d", prefix, accountID) {
			return true
		}
	}
	return false
}

var validationMethodRegexp = regexp.MustCompile(`^[[:alnum:]-]+$`)

// caaValidationMethodMatches checks that the validationmethods CAA parameter,
// if present, contains the exact name of the ACME validation method used to
// validate this domain.
// See RFC 8657 Section 4: https://www.rfc-editor.org/rfc/rfc8657.html#section-4
func caaValidationMethodMatches(caaParams map[string]string, method core.AcmeChallenge) bool {
	commaSeparatedMethods, ok := caaParams["validationmethods"]
	if !ok {
		return true
	}

	for _, m := range strings.Split(commaSeparatedMethods, ",") {
		// If any listed method does not match the ABNF 1*(ALPHA / DIGIT / "-"),
		// immediately reject the whole record.
		if !validationMethodRegexp.MatchString(m) {
			return false
		}

		caaMethod := core.AcmeChallenge(m)
		if !caaMethod.IsValid() {
			continue
		}

		if caaMethod == method {
			return true
		}
	}
	return false
}
