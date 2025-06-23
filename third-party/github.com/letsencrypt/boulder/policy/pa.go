package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/mail"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"

	"golang.org/x/net/idna"
	"golang.org/x/text/unicode/norm"

	"github.com/letsencrypt/boulder/core"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/iana"
	"github.com/letsencrypt/boulder/identifier"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/strictyaml"
)

// AuthorityImpl enforces CA policy decisions.
type AuthorityImpl struct {
	log blog.Logger

	blocklist              map[string]bool
	exactBlocklist         map[string]bool
	wildcardExactBlocklist map[string]bool
	blocklistMu            sync.RWMutex

	enabledChallenges map[core.AcmeChallenge]bool
	pseudoRNG         *rand.Rand
	rngMu             sync.Mutex
}

// New constructs a Policy Authority.
func New(challengeTypes map[core.AcmeChallenge]bool, log blog.Logger) (*AuthorityImpl, error) {

	pa := AuthorityImpl{
		log:               log,
		enabledChallenges: challengeTypes,
		// We don't need real randomness for this.
		pseudoRNG: rand.New(rand.NewSource(99)),
	}

	return &pa, nil
}

// blockedNamesPolicy is a struct holding lists of blocked domain names. One for
// exact blocks and one for blocks including all subdomains.
type blockedNamesPolicy struct {
	// ExactBlockedNames is a list of domain names. Issuance for names exactly
	// matching an entry in the list will be forbidden. (e.g. `ExactBlockedNames`
	// containing `www.example.com` will not block `example.com` or
	// `mail.example.com`).
	ExactBlockedNames []string `yaml:"ExactBlockedNames"`
	// HighRiskBlockedNames is like ExactBlockedNames except that issuance is
	// blocked for subdomains as well. (e.g. BlockedNames containing `example.com`
	// will block `www.example.com`).
	//
	// This list typically doesn't change with much regularity.
	HighRiskBlockedNames []string `yaml:"HighRiskBlockedNames"`

	// AdminBlockedNames operates the same as BlockedNames but is changed with more
	// frequency based on administrative blocks/revocations that are added over
	// time above and beyond the high-risk domains. Managing these entries separately
	// from HighRiskBlockedNames makes it easier to vet changes accurately.
	AdminBlockedNames []string `yaml:"AdminBlockedNames"`
}

// LoadHostnamePolicyFile will load the given policy file, returning an error if
// it fails.
func (pa *AuthorityImpl) LoadHostnamePolicyFile(f string) error {
	configBytes, err := os.ReadFile(f)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(configBytes)
	pa.log.Infof("loading hostname policy, sha256: %s", hex.EncodeToString(hash[:]))
	var policy blockedNamesPolicy
	err = strictyaml.Unmarshal(configBytes, &policy)
	if err != nil {
		return err
	}
	if len(policy.HighRiskBlockedNames) == 0 {
		return fmt.Errorf("No entries in HighRiskBlockedNames.")
	}
	if len(policy.ExactBlockedNames) == 0 {
		return fmt.Errorf("No entries in ExactBlockedNames.")
	}
	return pa.processHostnamePolicy(policy)
}

// processHostnamePolicy handles loading a new blockedNamesPolicy into the PA.
// All of the policy.ExactBlockedNames will be added to the
// wildcardExactBlocklist by processHostnamePolicy to ensure that wildcards for
// exact blocked names entries are forbidden.
func (pa *AuthorityImpl) processHostnamePolicy(policy blockedNamesPolicy) error {
	nameMap := make(map[string]bool)
	for _, v := range policy.HighRiskBlockedNames {
		nameMap[v] = true
	}
	for _, v := range policy.AdminBlockedNames {
		nameMap[v] = true
	}
	exactNameMap := make(map[string]bool)
	wildcardNameMap := make(map[string]bool)
	for _, v := range policy.ExactBlockedNames {
		exactNameMap[v] = true
		// Remove the leftmost label of the exact blocked names entry to make an exact
		// wildcard block list entry that will prevent issuing a wildcard that would
		// include the exact blocklist entry. e.g. if "highvalue.example.com" is on
		// the exact blocklist we want "example.com" to be in the
		// wildcardExactBlocklist so that "*.example.com" cannot be issued.
		//
		// First, split the domain into two parts: the first label and the rest of the domain.
		parts := strings.SplitN(v, ".", 2)
		// if there are less than 2 parts then this entry is malformed! There should
		// at least be a "something." and a TLD like "com"
		if len(parts) < 2 {
			return fmt.Errorf(
				"Malformed ExactBlockedNames entry, only one label: %q", v)
		}
		// Add the second part, the domain minus the first label, to the
		// wildcardNameMap to block issuance for `*.`+parts[1]
		wildcardNameMap[parts[1]] = true
	}
	pa.blocklistMu.Lock()
	pa.blocklist = nameMap
	pa.exactBlocklist = exactNameMap
	pa.wildcardExactBlocklist = wildcardNameMap
	pa.blocklistMu.Unlock()
	return nil
}

// The values of maxDNSIdentifierLength, maxLabelLength and maxLabels are hard coded
// into the error messages errNameTooLong, errLabelTooLong and errTooManyLabels.
// If their values change, the related error messages should be updated.

const (
	maxLabels = 10

	// RFC 1034 says DNS labels have a max of 63 octets, and names have a max of 255
	// octets: https://tools.ietf.org/html/rfc1035#page-10. Since two of those octets
	// are taken up by the leading length byte and the trailing root period the actual
	// max length becomes 253.
	maxLabelLength         = 63
	maxDNSIdentifierLength = 253
)

var dnsLabelCharacterRegexp = regexp.MustCompile("^[a-z0-9-]+$")

func isDNSCharacter(ch byte) bool {
	return ('a' <= ch && ch <= 'z') ||
		('A' <= ch && ch <= 'Z') ||
		('0' <= ch && ch <= '9') ||
		ch == '.' || ch == '-'
}

// In these error messages:
//   253 is the value of maxDNSIdentifierLength
//   63 is the value of maxLabelLength
//   10 is the value of maxLabels
// If these values change, the related error messages should be updated.

var (
	errNonPublic            = berrors.MalformedError("Domain name does not end with a valid public suffix (TLD)")
	errICANNTLD             = berrors.MalformedError("Domain name is an ICANN TLD")
	errPolicyForbidden      = berrors.RejectedIdentifierError("The ACME server refuses to issue a certificate for this domain name, because it is forbidden by policy")
	errInvalidDNSCharacter  = berrors.MalformedError("Domain name contains an invalid character")
	errNameTooLong          = berrors.MalformedError("Domain name is longer than 253 bytes")
	errIPAddress            = berrors.MalformedError("The ACME server can not issue a certificate for an IP address")
	errTooManyLabels        = berrors.MalformedError("Domain name has more than 10 labels (parts)")
	errEmptyName            = berrors.MalformedError("Domain name is empty")
	errNameEndsInDot        = berrors.MalformedError("Domain name ends in a dot")
	errTooFewLabels         = berrors.MalformedError("Domain name needs at least one dot")
	errLabelTooShort        = berrors.MalformedError("Domain name can not have two dots in a row")
	errLabelTooLong         = berrors.MalformedError("Domain has a label (component between dots) longer than 63 bytes")
	errMalformedIDN         = berrors.MalformedError("Domain name contains malformed punycode")
	errInvalidRLDH          = berrors.RejectedIdentifierError("Domain name contains an invalid label in a reserved format (R-LDH: '??--')")
	errTooManyWildcards     = berrors.MalformedError("Domain name has more than one wildcard")
	errMalformedWildcard    = berrors.MalformedError("Domain name contains an invalid wildcard. A wildcard is only permitted before the first dot in a domain name")
	errICANNTLDWildcard     = berrors.MalformedError("Domain name is a wildcard for an ICANN TLD")
	errWildcardNotSupported = berrors.MalformedError("Wildcard domain names are not supported")
)

// validNonWildcardDomain checks that a domain isn't:
//   - empty
//   - prefixed with the wildcard label `*.`
//   - made of invalid DNS characters
//   - longer than the maxDNSIdentifierLength
//   - an IPv4 or IPv6 address
//   - suffixed with just "."
//   - made of too many DNS labels
//   - made of any invalid DNS labels
//   - suffixed with something other than an IANA registered TLD
//   - exactly equal to an IANA registered TLD
//
// It does NOT ensure that the domain is absent from any PA blocked lists.
func validNonWildcardDomain(domain string) error {
	if domain == "" {
		return errEmptyName
	}

	if strings.HasPrefix(domain, "*.") {
		return errWildcardNotSupported
	}

	for _, ch := range []byte(domain) {
		if !isDNSCharacter(ch) {
			return errInvalidDNSCharacter
		}
	}

	if len(domain) > maxDNSIdentifierLength {
		return errNameTooLong
	}

	if ip := net.ParseIP(domain); ip != nil {
		return errIPAddress
	}

	if strings.HasSuffix(domain, ".") {
		return errNameEndsInDot
	}

	labels := strings.Split(domain, ".")
	if len(labels) > maxLabels {
		return errTooManyLabels
	}
	if len(labels) < 2 {
		return errTooFewLabels
	}
	for _, label := range labels {
		// Check that this is a valid LDH Label: "A string consisting of ASCII
		// letters, digits, and the hyphen with the further restriction that the
		// hyphen cannot appear at the beginning or end of the string. Like all DNS
		// labels, its total length must not exceed 63 octets." (RFC 5890, 2.3.1)
		if len(label) < 1 {
			return errLabelTooShort
		}
		if len(label) > maxLabelLength {
			return errLabelTooLong
		}
		if !dnsLabelCharacterRegexp.MatchString(label) {
			return errInvalidDNSCharacter
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return errInvalidDNSCharacter
		}

		// Check if this is a Reserved LDH Label: "[has] the property that they
		// contain "--" in the third and fourth characters but which otherwise
		// conform to LDH label rules." (RFC 5890, 2.3.1)
		if len(label) >= 4 && label[2:4] == "--" {
			// Check if this is an XN-Label: "labels that begin with the prefix "xn--"
			// (case independent), but otherwise conform to the rules for LDH labels."
			// (RFC 5890, 2.3.1)
			if label[0:2] != "xn" {
				return errInvalidRLDH
			}

			// Check if this is a P-Label: "A XN-Label that contains valid output of
			// the Punycode algorithm (as defined in RFC 3492, Section 6.3) from the
			// fifth and subsequent positions." (Baseline Requirements, 1.6.1)
			ulabel, err := idna.ToUnicode(label)
			if err != nil {
				return errMalformedIDN
			}
			if !norm.NFC.IsNormalString(ulabel) {
				return errMalformedIDN
			}
		}
	}

	// Names must end in an ICANN TLD, but they must not be equal to an ICANN TLD.
	icannTLD, err := iana.ExtractSuffix(domain)
	if err != nil {
		return errNonPublic
	}
	if icannTLD == domain {
		return errICANNTLD
	}

	return nil
}

// ValidDomain checks that a domain is valid and that it doesn't contain any
// invalid wildcard characters. It does NOT ensure that the domain is absent
// from any PA blocked lists.
func ValidDomain(domain string) error {
	if strings.Count(domain, "*") <= 0 {
		return validNonWildcardDomain(domain)
	}

	// Names containing more than one wildcard are invalid.
	if strings.Count(domain, "*") > 1 {
		return errTooManyWildcards
	}

	// If the domain has a wildcard character, but it isn't the first most
	// label of the domain name then the wildcard domain is malformed
	if !strings.HasPrefix(domain, "*.") {
		return errMalformedWildcard
	}

	// The base domain is the wildcard request with the `*.` prefix removed
	baseDomain := strings.TrimPrefix(domain, "*.")

	// Names must end in an ICANN TLD, but they must not be equal to an ICANN TLD.
	icannTLD, err := iana.ExtractSuffix(baseDomain)
	if err != nil {
		return errNonPublic
	}
	// Names must have a non-wildcard label immediately adjacent to the ICANN
	// TLD. No `*.com`!
	if baseDomain == icannTLD {
		return errICANNTLDWildcard
	}
	return validNonWildcardDomain(baseDomain)
}

// forbiddenMailDomains is a map of domain names we do not allow after the
// @ symbol in contact mailto addresses. These are frequently used when
// copy-pasting example configurations and would not result in expiration
// messages and subscriber communications reaching the user that created the
// registration if allowed.
var forbiddenMailDomains = map[string]bool{
	// https://tools.ietf.org/html/rfc2606#section-3
	"example.com": true,
	"example.net": true,
	"example.org": true,
}

// ValidEmail returns an error if the input doesn't parse as an email address,
// the domain isn't a valid hostname in Preferred Name Syntax, or its on the
// list of domains forbidden for mail (because they are often used in examples).
func ValidEmail(address string) error {
	email, err := mail.ParseAddress(address)
	if err != nil {
		if len(address) > 254 {
			address = address[:254] + "..."
		}
		return berrors.InvalidEmailError("%q is not a valid e-mail address", address)
	}
	splitEmail := strings.SplitN(email.Address, "@", -1)
	domain := strings.ToLower(splitEmail[len(splitEmail)-1])
	err = validNonWildcardDomain(domain)
	if err != nil {
		return berrors.InvalidEmailError(
			"contact email %q has invalid domain : %s",
			email.Address, err)
	}
	if forbiddenMailDomains[domain] {
		return berrors.InvalidEmailError(
			"invalid contact domain. Contact emails @%s are forbidden",
			domain)
	}
	return nil
}

// subError returns an appropriately typed error based on the input error
func subError(name string, err error) berrors.SubBoulderError {
	var bErr *berrors.BoulderError
	if errors.As(err, &bErr) {
		return berrors.SubBoulderError{
			Identifier:   identifier.DNSIdentifier(name),
			BoulderError: bErr,
		}
	} else {
		return berrors.SubBoulderError{
			Identifier: identifier.DNSIdentifier(name),
			BoulderError: &berrors.BoulderError{
				Type:   berrors.RejectedIdentifier,
				Detail: err.Error(),
			},
		}
	}
}

// WillingToIssue determines whether the CA is willing to issue for the provided
// domain names.
//
// It checks the criteria checked by `WellFormedDomainNames`, and additionally checks
// whether any domain is on a blocklist.
//
// If multiple domains are invalid, the error will contain suberrors specific to
// each domain.
//
// Precondition: all input domain names must be in lowercase.
func (pa *AuthorityImpl) WillingToIssue(domains []string) error {
	err := WellFormedDomainNames(domains)
	if err != nil {
		return err
	}

	var subErrors []berrors.SubBoulderError
	for _, domain := range domains {
		if strings.Count(domain, "*") > 0 {
			// The base domain is the wildcard request with the `*.` prefix removed
			baseDomain := strings.TrimPrefix(domain, "*.")

			// The base domain can't be in the wildcard exact blocklist
			err = pa.checkWildcardHostList(baseDomain)
			if err != nil {
				subErrors = append(subErrors, subError(domain, err))
				continue
			}
		}

		// For both wildcard and non-wildcard domains, check whether any parent domain
		// name is on the regular blocklist.
		err := pa.checkHostLists(domain)
		if err != nil {
			subErrors = append(subErrors, subError(domain, err))
			continue
		}
	}
	return combineSubErrors(subErrors)
}

// WellFormedDomainNames returns an error if any of the provided domains do not meet these criteria:
//
//   - MUST contains only lowercase characters, numbers, hyphens, and dots
//   - MUST NOT have more than maxLabels labels
//   - MUST follow the DNS hostname syntax rules in RFC 1035 and RFC 2181
//
// In particular, it:
//   - MUST NOT contain underscores
//   - MUST NOT match the syntax of an IP address
//   - MUST end in a public suffix
//   - MUST have at least one label in addition to the public suffix
//   - MUST NOT be a label-wise suffix match for a name on the block list,
//     where comparison is case-independent (normalized to lower case)
//
// If a domain contains a *, we additionally require:
//   - There is at most one `*` wildcard character
//   - That the wildcard character is the leftmost label
//   - That the wildcard label is not immediately adjacent to a top level ICANN
//     TLD
//
// If multiple domains are invalid, the error will contain suberrors specific to
// each domain.
func WellFormedDomainNames(domains []string) error {
	var subErrors []berrors.SubBoulderError
	for _, domain := range domains {
		err := ValidDomain(domain)
		if err != nil {
			subErrors = append(subErrors, subError(domain, err))
		}
	}
	return combineSubErrors(subErrors)
}

func combineSubErrors(subErrors []berrors.SubBoulderError) error {
	if len(subErrors) > 0 {
		// If there was only one error, then use it as the top level error that is
		// returned.
		if len(subErrors) == 1 {
			return berrors.RejectedIdentifierError(
				"Cannot issue for %q: %s",
				subErrors[0].Identifier.Value,
				subErrors[0].BoulderError.Detail,
			)
		}

		detail := fmt.Sprintf(
			"Cannot issue for %q: %s (and %d more problems. Refer to sub-problems for more information.)",
			subErrors[0].Identifier.Value,
			subErrors[0].BoulderError.Detail,
			len(subErrors)-1,
		)
		return (&berrors.BoulderError{
			Type:   berrors.RejectedIdentifier,
			Detail: detail,
		}).WithSubErrors(subErrors)
	}
	return nil
}

// checkWildcardHostList checks the wildcardExactBlocklist for a given domain.
// If the domain is not present on the list nil is returned, otherwise
// errPolicyForbidden is returned.
func (pa *AuthorityImpl) checkWildcardHostList(domain string) error {
	pa.blocklistMu.RLock()
	defer pa.blocklistMu.RUnlock()

	if pa.wildcardExactBlocklist == nil {
		return fmt.Errorf("Hostname policy not yet loaded.")
	}

	if pa.wildcardExactBlocklist[domain] {
		return errPolicyForbidden
	}

	return nil
}

func (pa *AuthorityImpl) checkHostLists(domain string) error {
	pa.blocklistMu.RLock()
	defer pa.blocklistMu.RUnlock()

	if pa.blocklist == nil {
		return fmt.Errorf("Hostname policy not yet loaded.")
	}

	labels := strings.Split(domain, ".")
	for i := range labels {
		joined := strings.Join(labels[i:], ".")
		if pa.blocklist[joined] {
			return errPolicyForbidden
		}
	}

	if pa.exactBlocklist[domain] {
		return errPolicyForbidden
	}
	return nil
}

// challengeTypesFor determines which challenge types are acceptable for the
// given identifier.
func (pa *AuthorityImpl) challengeTypesFor(identifier identifier.ACMEIdentifier) ([]core.AcmeChallenge, error) {
	var challenges []core.AcmeChallenge

	// If the identifier is for a DNS wildcard name we only
	// provide a DNS-01 challenge as a matter of CA policy.
	if strings.HasPrefix(identifier.Value, "*.") {
		// We must have the DNS-01 challenge type enabled to create challenges for
		// a wildcard identifier per LE policy.
		if !pa.ChallengeTypeEnabled(core.ChallengeTypeDNS01) {
			return nil, fmt.Errorf(
				"Challenges requested for wildcard identifier but DNS-01 " +
					"challenge type is not enabled")
		}
		// Only provide a DNS-01-Wildcard challenge
		challenges = []core.AcmeChallenge{core.ChallengeTypeDNS01}
	} else {
		// Otherwise we collect up challenges based on what is enabled.
		if pa.ChallengeTypeEnabled(core.ChallengeTypeHTTP01) {
			challenges = append(challenges, core.ChallengeTypeHTTP01)
		}

		if pa.ChallengeTypeEnabled(core.ChallengeTypeTLSALPN01) {
			challenges = append(challenges, core.ChallengeTypeTLSALPN01)
		}

		if pa.ChallengeTypeEnabled(core.ChallengeTypeDNS01) {
			challenges = append(challenges, core.ChallengeTypeDNS01)
		}
	}

	return challenges, nil
}

// ChallengesFor determines which challenge types are acceptable for the given
// identifier, and constructs new challenge objects for those challenge types.
// The resulting challenge objects all share a single challenge token and are
// returned in a random order.
func (pa *AuthorityImpl) ChallengesFor(identifier identifier.ACMEIdentifier) ([]core.Challenge, error) {
	challTypes, err := pa.challengeTypesFor(identifier)
	if err != nil {
		return nil, err
	}

	challenges := make([]core.Challenge, len(challTypes))

	token := core.NewToken()

	for i, t := range challTypes {
		c, err := core.NewChallenge(t, token)
		if err != nil {
			return nil, err
		}

		challenges[i] = c
	}

	// We shuffle the challenges to prevent ACME clients from relying on the
	// specific order that boulder returns them in.
	shuffled := make([]core.Challenge, len(challenges))

	pa.rngMu.Lock()
	defer pa.rngMu.Unlock()
	for i, challIdx := range pa.pseudoRNG.Perm(len(challenges)) {
		shuffled[i] = challenges[challIdx]
	}

	return shuffled, nil
}

// ChallengeTypeEnabled returns whether the specified challenge type is enabled
func (pa *AuthorityImpl) ChallengeTypeEnabled(t core.AcmeChallenge) bool {
	pa.blocklistMu.RLock()
	defer pa.blocklistMu.RUnlock()
	return pa.enabledChallenges[t]
}

// CheckAuthz determines that an authorization was fulfilled by a challenge
// that was appropriate for the kind of identifier in the authorization.
func (pa *AuthorityImpl) CheckAuthz(authz *core.Authorization) error {
	chall, err := authz.SolvedBy()
	if err != nil {
		return err
	}

	challTypes, err := pa.challengeTypesFor(authz.Identifier)
	if err != nil {
		return err
	}

	if !slices.Contains(challTypes, chall) {
		return errors.New("authorization fulfilled by invalid challenge")
	}

	return nil
}
