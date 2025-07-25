package va

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"google.golang.org/protobuf/proto"

	"github.com/letsencrypt/boulder/bdns"
	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/probs"
	vapb "github.com/letsencrypt/boulder/va/proto"
)

type caaParams struct {
	accountURIID     int64
	validationMethod core.AcmeChallenge
}

// DoCAA conducts a CAA check for the specified dnsName. When invoked on the
// primary Validation Authority (VA) and the local check succeeds, it also
// performs CAA checks using the configured remote VAs. Failed checks are
// indicated by a non-nil Problems in the returned ValidationResult. DoCAA
// returns error only for internal logic errors (and the client may receive
// errors from gRPC in the event of a communication problem). This method
// implements the CAA portion of Multi-Perspective Issuance Corroboration as
// defined in BRs Sections 3.2.2.9 and 5.4.1.
func (va *ValidationAuthorityImpl) DoCAA(ctx context.Context, req *vapb.IsCAAValidRequest) (*vapb.IsCAAValidResponse, error) {
	if core.IsAnyNilOrZero(req.Identifier, req.ValidationMethod, req.AccountURIID) {
		return nil, berrors.InternalServerError("incomplete IsCAAValid request")
	}

	ident := identifier.FromProto(req.Identifier)
	if ident.Type != identifier.TypeDNS {
		return nil, berrors.MalformedError("Identifier type for CAA check was not DNS")
	}

	logEvent := validationLogEvent{
		AuthzID:    req.AuthzID,
		Requester:  req.AccountURIID,
		Identifier: ident,
	}

	challType := core.AcmeChallenge(req.ValidationMethod)
	if !challType.IsValid() {
		return nil, berrors.InternalServerError("unrecognized validation method %q", req.ValidationMethod)
	}

	params := &caaParams{
		accountURIID:     req.AccountURIID,
		validationMethod: challType,
	}

	// Initialize variables and a deferred function to handle check latency
	// metrics, log check errors, and log an MPIC summary. Avoid using := to
	// redeclare `prob`, `localLatency`, or `summary` below this point.
	var prob *probs.ProblemDetails
	var summary *mpicSummary
	var internalErr error
	var localLatency time.Duration
	start := va.clk.Now()

	defer func() {
		probType := ""
		outcome := fail
		if prob != nil {
			// CAA check failed.
			probType = string(prob.Type)
			logEvent.Error = prob.String()
		} else {
			// CAA check passed.
			outcome = pass
		}
		// Observe local check latency (primary|remote).
		va.observeLatency(opCAA, va.perspective, string(challType), probType, outcome, localLatency)
		if va.isPrimaryVA() {
			// Observe total check latency (primary+remote).
			va.observeLatency(opCAA, allPerspectives, string(challType), probType, outcome, va.clk.Since(start))
			logEvent.Summary = summary
		}
		// Log the total check latency.
		logEvent.Latency = va.clk.Since(start).Round(time.Millisecond).Seconds()

		va.log.AuditObject("CAA check result", logEvent)
	}()

	internalErr = va.checkCAA(ctx, ident, params)

	// Stop the clock for local check latency.
	localLatency = va.clk.Since(start)

	if internalErr != nil {
		logEvent.InternalError = internalErr.Error()
		prob = detailedError(internalErr)
		prob.Detail = fmt.Sprintf("While processing CAA for %s: %s", ident.Value, prob.Detail)
	}

	if va.isPrimaryVA() {
		op := func(ctx context.Context, remoteva RemoteVA, req proto.Message) (remoteResult, error) {
			checkRequest, ok := req.(*vapb.IsCAAValidRequest)
			if !ok {
				return nil, fmt.Errorf("got type %T, want *vapb.IsCAAValidRequest", req)
			}
			return remoteva.DoCAA(ctx, checkRequest)
		}
		var remoteProb *probs.ProblemDetails
		summary, remoteProb = va.doRemoteOperation(ctx, op, req)
		// If the remote result was a non-nil problem then fail the CAA check
		if remoteProb != nil {
			prob = remoteProb
			va.log.Infof("CAA check failed due to remote failures: identifier=%v err=%s",
				ident.Value, remoteProb)
		}
	}

	if prob != nil {
		// The ProblemDetails will be serialized through gRPC, which requires UTF-8.
		// It will also later be serialized in JSON, which defaults to UTF-8. Make
		// sure it is UTF-8 clean now.
		prob = filterProblemDetails(prob)
		return &vapb.IsCAAValidResponse{
			Problem: &corepb.ProblemDetails{
				ProblemType: string(prob.Type),
				Detail:      replaceInvalidUTF8([]byte(prob.Detail)),
			},
			Perspective: va.perspective,
			Rir:         va.rir,
		}, nil
	} else {
		return &vapb.IsCAAValidResponse{
			Perspective: va.perspective,
			Rir:         va.rir,
		}, nil
	}
}

// checkCAA performs a CAA lookup & validation for the provided identifier. If
// the CAA lookup & validation fail a problem is returned.
func (va *ValidationAuthorityImpl) checkCAA(
	ctx context.Context,
	ident identifier.ACMEIdentifier,
	params *caaParams) error {
	if core.IsAnyNilOrZero(params, params.validationMethod, params.accountURIID) {
		return errors.New("expected validationMethod or accountURIID not provided to checkCAA")
	}

	foundAt, valid, response, err := va.checkCAARecords(ctx, ident, params)
	if err != nil {
		return berrors.DNSError("%s", err)
	}

	va.log.AuditInfof("Checked CAA records for %s, [Present: %t, Account ID: %d, Challenge: %s, Valid for issuance: %t, Found at: %q] Response=%q",
		ident.Value, foundAt != "", params.accountURIID, params.validationMethod, valid, foundAt, response)
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
	ident identifier.ACMEIdentifier,
	params *caaParams) (string, bool, string, error) {
	hostname := strings.ToLower(ident.Value)
	// If this is a wildcard name, remove the prefix
	var wildcard bool
	if strings.HasPrefix(hostname, `*.`) {
		hostname = strings.TrimPrefix(ident.Value, `*.`)
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

	if len(records) == 0 {
		// Although CAA records exist, none of them pertain to issuance in this case.
		// (e.g. there is only an issuewild directive, but we are checking for a
		// non-wildcard identifier, or there is only an iodef or non-critical unknown
		// directive.)
		va.metrics.caaCounter.WithLabelValues("no relevant records").Inc()
		return true, caaSet.name
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

// caaParameter is a key-value pair parsed from a single CAA RR.
type caaParameter struct {
	tag string
	val string
}

// parseCAARecord extracts the domain and parameters (if any) from a
// issue/issuewild CAA record. This follows RFC 8659 Section 4.2 and Section 4.3
// (https://www.rfc-editor.org/rfc/rfc8659.html#section-4). It returns the
// domain name (which may be the empty string if the record forbids issuance)
// and a slice of CAA parameters, or a descriptive error if the record is
// malformed.
func parseCAARecord(caa *dns.CAA) (string, []caaParameter, error) {
	isWSP := func(r rune) bool {
		return r == '\t' || r == ' '
	}

	// Semi-colons (ASCII 0x3B) are prohibited from being specified in the
	// parameter tag or value, hence we can simply split on semi-colons.
	parts := strings.Split(caa.Value, ";")

	// See https://www.rfc-editor.org/rfc/rfc8659.html#section-4.2
	//
	// 		issuer-domain-name = label *("." label)
	// 		label = (ALPHA / DIGIT) *( *("-") (ALPHA / DIGIT))
	issuerDomainName := strings.TrimFunc(parts[0], isWSP)
	paramList := parts[1:]

	// Handle the case where a semi-colon is specified following the domain
	// but no parameters are given.
	if len(paramList) == 1 && strings.TrimFunc(paramList[0], isWSP) == "" {
		return issuerDomainName, nil, nil
	}

	var caaParameters []caaParameter
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

		caaParameters = append(caaParameters, caaParameter{
			tag: tag,
			val: value,
		})
	}

	return issuerDomainName, caaParameters, nil
}

// caaDomainMatches checks that the issuer domain name listed in the parsed
// CAA record matches the domain name we expect.
func caaDomainMatches(caaDomain string, issuerDomain string) bool {
	return caaDomain == issuerDomain
}

// caaAccountURIMatches checks that the accounturi CAA parameter, if present,
// matches one of the specific account URIs we expect. We support multiple
// account URI prefixes to handle accounts which were registered under ACMEv1.
// We accept only a single "accounturi" parameter and will fail if multiple are
// found in the CAA RR.
// See RFC 8657 Section 3: https://www.rfc-editor.org/rfc/rfc8657.html#section-3
func caaAccountURIMatches(caaParams []caaParameter, accountURIPrefixes []string, accountID int64) bool {
	var found bool
	var accountURI string
	for _, c := range caaParams {
		if c.tag == "accounturi" {
			if found {
				// A Property with multiple "accounturi" parameters is
				// unsatisfiable.
				return false
			}
			accountURI = c.val
			found = true
		}
	}

	if !found {
		// A Property without an "accounturi" parameter matches any account.
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
// validate this domain. We accept only a single "validationmethods" parameter
// and will fail if multiple are found in the CAA RR, even if all tag-value
// pairs would be valid. See RFC 8657 Section 4:
// https://www.rfc-editor.org/rfc/rfc8657.html#section-4.
func caaValidationMethodMatches(caaParams []caaParameter, method core.AcmeChallenge) bool {
	var validationMethods string
	var found bool
	for _, param := range caaParams {
		if param.tag == "validationmethods" {
			if found {
				// RFC 8657 does not define what behavior to take when multiple
				// "validationmethods" parameters exist, but we make the
				// conscious choice to fail validation similar to how multiple
				// "accounturi" parameters are "unsatisfiable". Subscribers
				// should be aware of RFC 8657 Section 5.8:
				// https://www.rfc-editor.org/rfc/rfc8657.html#section-5.8
				return false
			}
			validationMethods = param.val
			found = true
		}
	}

	if !found {
		return true
	}

	for _, m := range strings.Split(validationMethods, ",") {
		// The value of the "validationmethods" parameter MUST comply with the
		// following ABNF [RFC5234]:
		//
		//      value = [*(label ",") label]
		//      label = 1*(ALPHA / DIGIT / "-")
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
