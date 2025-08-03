package notmain

import (
	"bytes"
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"regexp"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	zX509 "github.com/zmap/zcrypto/x509"
	"github.com/zmap/zlint/v3"
	"github.com/zmap/zlint/v3/lint"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	"github.com/letsencrypt/boulder/ctpolicy/loglist"
	"github.com/letsencrypt/boulder/features"
	"github.com/letsencrypt/boulder/goodkey"
	"github.com/letsencrypt/boulder/goodkey/sagoodkey"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/linter"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/policy"
	"github.com/letsencrypt/boulder/precert"
	"github.com/letsencrypt/boulder/sa"
)

// For defense-in-depth in addition to using the PA & its hostnamePolicy to
// check domain names we also perform a check against the regex's from the
// forbiddenDomains array
var forbiddenDomainPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^\s*$`),
	regexp.MustCompile(`\.local$`),
	regexp.MustCompile(`^localhost$`),
	regexp.MustCompile(`\.localhost$`),
}

func isForbiddenDomain(name string) (bool, string) {
	for _, r := range forbiddenDomainPatterns {
		if matches := r.FindAllStringSubmatch(name, -1); len(matches) > 0 {
			return true, r.String()
		}
	}
	return false, ""
}

var batchSize = 1000

type report struct {
	begin     time.Time
	end       time.Time
	GoodCerts int64                  `json:"good-certs"`
	BadCerts  int64                  `json:"bad-certs"`
	DbErrs    int64                  `json:"db-errs"`
	Entries   map[string]reportEntry `json:"entries"`
}

func (r *report) dump() error {
	content, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, string(content))
	return nil
}

type reportEntry struct {
	Valid    bool     `json:"valid"`
	SANs     []string `json:"sans"`
	Problems []string `json:"problems,omitempty"`
}

// certDB is an interface collecting the borp.DbMap functions that the various
// parts of cert-checker rely on. Using this adapter shim allows tests to swap
// out the saDbMap implementation.
type certDB interface {
	Select(ctx context.Context, i interface{}, query string, args ...interface{}) ([]interface{}, error)
	SelectOne(ctx context.Context, i interface{}, query string, args ...interface{}) error
	SelectNullInt(ctx context.Context, query string, args ...interface{}) (sql.NullInt64, error)
}

// A function that looks up a precertificate by serial and returns its DER bytes. Used for
// mocking in tests.
type precertGetter func(context.Context, string) ([]byte, error)

type certChecker struct {
	pa                          core.PolicyAuthority
	kp                          goodkey.KeyPolicy
	dbMap                       certDB
	getPrecert                  precertGetter
	certs                       chan *corepb.Certificate
	clock                       clock.Clock
	rMu                         *sync.Mutex
	issuedReport                report
	checkPeriod                 time.Duration
	acceptableValidityDurations map[time.Duration]bool
	lints                       lint.Registry
	logger                      blog.Logger
}

func newChecker(saDbMap certDB,
	clk clock.Clock,
	pa core.PolicyAuthority,
	kp goodkey.KeyPolicy,
	period time.Duration,
	avd map[time.Duration]bool,
	lints lint.Registry,
	logger blog.Logger,
) certChecker {
	precertGetter := func(ctx context.Context, serial string) ([]byte, error) {
		precertPb, err := sa.SelectPrecertificate(ctx, saDbMap, serial)
		if err != nil {
			return nil, err
		}
		return precertPb.Der, nil
	}
	return certChecker{
		pa:                          pa,
		kp:                          kp,
		dbMap:                       saDbMap,
		getPrecert:                  precertGetter,
		certs:                       make(chan *corepb.Certificate, batchSize),
		rMu:                         new(sync.Mutex),
		clock:                       clk,
		issuedReport:                report{Entries: make(map[string]reportEntry)},
		checkPeriod:                 period,
		acceptableValidityDurations: avd,
		lints:                       lints,
		logger:                      logger,
	}
}

// findStartingID returns the lowest `id` in the certificates table within the
// time window specified. The time window is a half-open interval [begin, end).
func (c *certChecker) findStartingID(ctx context.Context, begin, end time.Time) (int64, error) {
	var output sql.NullInt64
	var err error
	var retries int

	// Rather than querying `MIN(id)` across that whole window, we query it across the first
	// hour of the window. This allows the query planner to use the index on `issued` more
	// effectively. For a busy, actively issuing CA, that will always return results in the
	// first query. For a less busy CA, or during integration tests, there may only exist
	// certificates towards the end of the window, so we try querying later hourly chunks until
	// we find a certificate or hit the end of the window. We also retry transient errors.
	queryBegin := begin
	queryEnd := begin.Add(time.Hour)

	for queryBegin.Compare(end) < 0 {
		output, err = c.dbMap.SelectNullInt(
			ctx,
			`SELECT MIN(id) FROM certificates
				WHERE issued >= :begin AND
					  issued < :end`,
			map[string]interface{}{
				"begin": queryBegin,
				"end":   queryEnd,
			},
		)
		if err != nil {
			c.logger.AuditErrf("finding starting certificate: %s", err)
			retries++
			time.Sleep(core.RetryBackoff(retries, time.Second, time.Minute, 2))
			continue
		}
		// https://mariadb.com/kb/en/min/
		// MIN() returns NULL if there were no matching rows
		// https://pkg.go.dev/database/sql#NullInt64
		// Valid is true if Int64 is not NULL
		if !output.Valid {
			// No matching rows, try the next hour
			queryBegin = queryBegin.Add(time.Hour)
			queryEnd = queryEnd.Add(time.Hour)
			if queryEnd.Compare(end) > 0 {
				queryEnd = end
			}
			continue
		}

		return output.Int64, nil
	}

	// Fell through the loop without finding a valid ID
	return 0, fmt.Errorf("no rows found for certificates issued between %s and %s", begin, end)
}

func (c *certChecker) getCerts(ctx context.Context) error {
	// The end of the report is the current time, rounded up to the nearest second.
	c.issuedReport.end = c.clock.Now().Truncate(time.Second).Add(time.Second)
	// The beginning of the report is the end minus the check period, rounded down to the nearest second.
	c.issuedReport.begin = c.issuedReport.end.Add(-c.checkPeriod).Truncate(time.Second)

	initialID, err := c.findStartingID(ctx, c.issuedReport.begin, c.issuedReport.end)
	if err != nil {
		return err
	}
	if initialID > 0 {
		// decrement the initial ID so that we select below as we aren't using >=
		initialID -= 1
	}

	batchStartID := initialID
	var retries int
	for {
		certs, highestID, err := sa.SelectCertificates(
			ctx,
			c.dbMap,
			`WHERE id > :id AND
			       issued >= :begin AND
				   issued < :end
			 ORDER BY id LIMIT :limit`,
			map[string]interface{}{
				"begin": c.issuedReport.begin,
				"end":   c.issuedReport.end,
				// Retrieve certs in batches of 1000 (the size of the certificate channel)
				// so that we don't eat unnecessary amounts of memory and avoid the 16MB MySQL
				// packet limit.
				"limit": batchSize,
				"id":    batchStartID,
			},
		)
		if err != nil {
			c.logger.AuditErrf("selecting certificates: %s", err)
			retries++
			time.Sleep(core.RetryBackoff(retries, time.Second, time.Minute, 2))
			continue
		}
		retries = 0
		for _, cert := range certs {
			c.certs <- cert
		}
		if len(certs) == 0 {
			break
		}
		lastCert := certs[len(certs)-1]
		if lastCert.Issued.AsTime().After(c.issuedReport.end) {
			break
		}
		batchStartID = highestID
	}

	// Close channel so range operations won't block once the channel empties out
	close(c.certs)
	return nil
}

func (c *certChecker) processCerts(ctx context.Context, wg *sync.WaitGroup, badResultsOnly bool) {
	for cert := range c.certs {
		sans, problems := c.checkCert(ctx, cert)
		valid := len(problems) == 0
		c.rMu.Lock()
		if !badResultsOnly || (badResultsOnly && !valid) {
			c.issuedReport.Entries[cert.Serial] = reportEntry{
				Valid:    valid,
				SANs:     sans,
				Problems: problems,
			}
		}
		c.rMu.Unlock()
		if !valid {
			atomic.AddInt64(&c.issuedReport.BadCerts, 1)
		} else {
			atomic.AddInt64(&c.issuedReport.GoodCerts, 1)
		}
	}
	wg.Done()
}

// Extensions that we allow in certificates
var allowedExtensions = map[string]bool{
	"1.3.6.1.5.5.7.1.1":       true, // Authority info access
	"2.5.29.35":               true, // Authority key identifier
	"2.5.29.19":               true, // Basic constraints
	"2.5.29.32":               true, // Certificate policies
	"2.5.29.31":               true, // CRL distribution points
	"2.5.29.37":               true, // Extended key usage
	"2.5.29.15":               true, // Key usage
	"2.5.29.17":               true, // Subject alternative name
	"2.5.29.14":               true, // Subject key identifier
	"1.3.6.1.4.1.11129.2.4.2": true, // SCT list
	"1.3.6.1.5.5.7.1.24":      true, // TLS feature
}

// For extensions that have a fixed value we check that it contains that value
var expectedExtensionContent = map[string][]byte{
	"1.3.6.1.5.5.7.1.24": {0x30, 0x03, 0x02, 0x01, 0x05}, // Must staple feature
}

// checkValidations checks the database for matching authorizations that were
// likely valid at the time the certificate was issued. Authorizations with
// status = "deactivated" are counted for this, so long as their validatedAt
// is before the issuance and expiration is after.
func (c *certChecker) checkValidations(ctx context.Context, cert *corepb.Certificate, idents identifier.ACMEIdentifiers) error {
	authzs, err := sa.SelectAuthzsMatchingIssuance(ctx, c.dbMap, cert.RegistrationID, cert.Issued.AsTime(), idents)
	if err != nil {
		return fmt.Errorf("error checking authzs for certificate %s: %w", cert.Serial, err)
	}

	if len(authzs) == 0 {
		return fmt.Errorf("no relevant authzs found valid at %s", cert.Issued)
	}

	// We may get multiple authorizations for the same identifier, but that's
	// okay. Any authorization for a given identifier is sufficient.
	identToAuthz := make(map[identifier.ACMEIdentifier]*corepb.Authorization)
	for _, m := range authzs {
		identToAuthz[identifier.FromProto(m.Identifier)] = m
	}

	var errors []error
	for _, ident := range idents {
		_, ok := identToAuthz[ident]
		if !ok {
			errors = append(errors, fmt.Errorf("missing authz for %q", ident.Value))
			continue
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("%s", errors)
	}
	return nil
}

// checkCert returns a list of Subject Alternative Names in the certificate and a list of problems with the certificate.
func (c *certChecker) checkCert(ctx context.Context, cert *corepb.Certificate) ([]string, []string) {
	var problems []string

	// Check that the digests match.
	if cert.Digest != core.Fingerprint256(cert.Der) {
		problems = append(problems, "Stored digest doesn't match certificate digest")
	}

	// Parse the certificate.
	parsedCert, err := zX509.ParseCertificate(cert.Der)
	if err != nil {
		problems = append(problems, fmt.Sprintf("Couldn't parse stored certificate: %s", err))
		// This is a fatal error, we can't do any further processing.
		return nil, problems
	}

	// Now that it's parsed, we can extract the SANs.
	sans := slices.Clone(parsedCert.DNSNames)
	for _, ip := range parsedCert.IPAddresses {
		sans = append(sans, ip.String())
	}

	// Run zlint checks.
	results := zlint.LintCertificateEx(parsedCert, c.lints)
	for name, res := range results.Results {
		if res.Status <= lint.Pass {
			continue
		}
		prob := fmt.Sprintf("zlint %s: %s", res.Status, name)
		if res.Details != "" {
			prob = fmt.Sprintf("%s %s", prob, res.Details)
		}
		problems = append(problems, prob)
	}

	// Check if stored serial is correct.
	storedSerial, err := core.StringToSerial(cert.Serial)
	if err != nil {
		problems = append(problems, "Stored serial is invalid")
	} else if parsedCert.SerialNumber.Cmp(storedSerial) != 0 {
		problems = append(problems, "Stored serial doesn't match certificate serial")
	}

	// Check that we have the correct expiration time.
	if !parsedCert.NotAfter.Equal(cert.Expires.AsTime()) {
		problems = append(problems, "Stored expiration doesn't match certificate NotAfter")
	}

	// Check if basic constraints are set.
	if !parsedCert.BasicConstraintsValid {
		problems = append(problems, "Certificate doesn't have basic constraints set")
	}

	// Check that the cert isn't able to sign other certificates.
	if parsedCert.IsCA {
		problems = append(problems, "Certificate can sign other certificates")
	}

	// Check that the cert has a valid validity period. The validity
	// period is computed inclusive of the whole final second indicated by
	// notAfter.
	validityDuration := parsedCert.NotAfter.Add(time.Second).Sub(parsedCert.NotBefore)
	_, ok := c.acceptableValidityDurations[validityDuration]
	if !ok {
		problems = append(problems, "Certificate has unacceptable validity period")
	}

	// Check that the stored issuance time isn't too far back/forward dated.
	if parsedCert.NotBefore.Before(cert.Issued.AsTime().Add(-6*time.Hour)) || parsedCert.NotBefore.After(cert.Issued.AsTime().Add(6*time.Hour)) {
		problems = append(problems, "Stored issuance date is outside of 6 hour window of certificate NotBefore")
	}

	// Check that the cert doesn't contain any SANs of unexpected types.
	if len(parsedCert.EmailAddresses) != 0 || len(parsedCert.URIs) != 0 {
		problems = append(problems, "Certificate contains SAN of unacceptable type (email or URI)")
	}

	if parsedCert.Subject.CommonName != "" {
		// Check if the CommonName is <= 64 characters.
		if len(parsedCert.Subject.CommonName) > 64 {
			problems = append(
				problems,
				fmt.Sprintf("Certificate has common name >64 characters long (%d)", len(parsedCert.Subject.CommonName)),
			)
		}

		// Check that the CommonName is included in the SANs.
		if !slices.Contains(sans, parsedCert.Subject.CommonName) {
			problems = append(problems, fmt.Sprintf("Certificate Common Name does not appear in Subject Alternative Names: %q !< %v",
				parsedCert.Subject.CommonName, parsedCert.DNSNames))
		}
	}

	// Check that the PA is still willing to issue for each DNS name and IP
	// address in the SANs. We do not check the CommonName here, as (if it exists)
	// we already checked that it is identical to one of the DNSNames in the SAN.
	for _, name := range parsedCert.DNSNames {
		err = c.pa.WillingToIssue(identifier.ACMEIdentifiers{identifier.NewDNS(name)})
		if err != nil {
			problems = append(problems, fmt.Sprintf("Policy Authority isn't willing to issue for '%s': %s", name, err))
			continue
		}
		// For defense-in-depth, even if the PA was willing to issue for a name
		// we double check it against a list of forbidden domains. This way even
		// if the hostnamePolicyFile malfunctions we will flag the forbidden
		// domain matches
		if forbidden, pattern := isForbiddenDomain(name); forbidden {
			problems = append(problems, fmt.Sprintf(
				"Policy Authority was willing to issue but domain '%s' matches "+
					"forbiddenDomains entry %q", name, pattern))
		}
	}
	for _, name := range parsedCert.IPAddresses {
		ip, ok := netip.AddrFromSlice(name)
		if !ok {
			problems = append(problems, fmt.Sprintf("SANs contain malformed IP %q", name))
			continue
		}
		err = c.pa.WillingToIssue(identifier.ACMEIdentifiers{identifier.NewIP(ip)})
		if err != nil {
			problems = append(problems, fmt.Sprintf("Policy Authority isn't willing to issue for '%s': %s", name, err))
			continue
		}
	}

	// Check the cert has the correct key usage extensions
	serverAndClient := slices.Equal(parsedCert.ExtKeyUsage, []zX509.ExtKeyUsage{zX509.ExtKeyUsageServerAuth, zX509.ExtKeyUsageClientAuth})
	serverOnly := slices.Equal(parsedCert.ExtKeyUsage, []zX509.ExtKeyUsage{zX509.ExtKeyUsageServerAuth})
	if !(serverAndClient || serverOnly) {
		problems = append(problems, "Certificate has incorrect key usage extensions")
	}

	for _, ext := range parsedCert.Extensions {
		_, ok := allowedExtensions[ext.Id.String()]
		if !ok {
			problems = append(problems, fmt.Sprintf("Certificate contains an unexpected extension: %s", ext.Id))
		}
		expectedContent, ok := expectedExtensionContent[ext.Id.String()]
		if ok {
			if !bytes.Equal(ext.Value, expectedContent) {
				problems = append(problems, fmt.Sprintf("Certificate extension %s contains unexpected content: has %x, expected %x", ext.Id, ext.Value, expectedContent))
			}
		}
	}

	// Check that the cert has a good key. Note that this does not perform
	// checks which rely on external resources such as weak or blocked key
	// lists, or the list of blocked keys in the database. This only performs
	// static checks, such as against the RSA key size and the ECDSA curve.
	p, err := x509.ParseCertificate(cert.Der)
	if err != nil {
		problems = append(problems, fmt.Sprintf("Couldn't parse stored certificate: %s", err))
	} else {
		err = c.kp.GoodKey(ctx, p.PublicKey)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Key Policy isn't willing to issue for public key: %s", err))
		}
	}

	precertDER, err := c.getPrecert(ctx, cert.Serial)
	if err != nil {
		// Log and continue, since we want the problems slice to only contains
		// problems with the cert itself.
		c.logger.Errf("fetching linting precertificate for %s: %s", cert.Serial, err)
		atomic.AddInt64(&c.issuedReport.DbErrs, 1)
	} else {
		err = precert.Correspond(precertDER, cert.Der)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Certificate does not correspond to precert for %s: %s", cert.Serial, err))
		}
	}

	if features.Get().CertCheckerChecksValidations {
		idents := identifier.FromCert(p)
		err = c.checkValidations(ctx, cert, idents)
		if err != nil {
			if features.Get().CertCheckerRequiresValidations {
				problems = append(problems, err.Error())
			} else {
				var identValues []string
				for _, ident := range idents {
					identValues = append(identValues, ident.Value)
				}
				c.logger.Errf("Certificate %s %s: %s", cert.Serial, identValues, err)
			}
		}
	}

	return sans, problems
}

type Config struct {
	CertChecker struct {
		DB cmd.DBConfig
		cmd.HostnamePolicyConfig

		Workers int `validate:"required,min=1"`
		// Deprecated: this is ignored, and cert checker always checks both expired and unexpired.
		UnexpiredOnly  bool
		BadResultsOnly bool
		CheckPeriod    config.Duration

		// AcceptableValidityDurations is a list of durations which are
		// acceptable for certificates we issue.
		AcceptableValidityDurations []config.Duration

		// GoodKey is an embedded config stanza for the goodkey library. If this
		// is populated, the cert-checker will perform static checks against the
		// public keys in the certs it checks.
		GoodKey goodkey.Config

		// LintConfig is a path to a zlint config file, which can be used to control
		// the behavior of zlint's "customizable lints".
		LintConfig string
		// IgnoredLints is a list of zlint names. Any lint results from a lint in
		// the IgnoredLists list are ignored regardless of LintStatus level.
		IgnoredLints []string

		// CTLogListFile is the path to a JSON file on disk containing the set of
		// all logs trusted by Chrome. The file must match the v3 log list schema:
		// https://www.gstatic.com/ct/log_list/v3/log_list_schema.json
		CTLogListFile string

		Features features.Config
	}
	PA     cmd.PAConfig
	Syslog cmd.SyslogConfig
}

func main() {
	configFile := flag.String("config", "", "File path to the configuration file for this service")
	flag.Parse()
	if *configFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	var config Config
	err := cmd.ReadConfigFile(*configFile, &config)
	cmd.FailOnError(err, "Reading JSON config file into config structure")

	features.Set(config.CertChecker.Features)

	logger := cmd.NewLogger(config.Syslog)
	logger.Info(cmd.VersionString())

	acceptableValidityDurations := make(map[time.Duration]bool)
	if len(config.CertChecker.AcceptableValidityDurations) > 0 {
		for _, entry := range config.CertChecker.AcceptableValidityDurations {
			acceptableValidityDurations[entry.Duration] = true
		}
	} else {
		// For backwards compatibility, assume only a single valid validity
		// period of exactly 90 days if none is configured.
		ninetyDays := (time.Hour * 24) * 90
		acceptableValidityDurations[ninetyDays] = true
	}

	// Validate PA config and set defaults if needed.
	cmd.FailOnError(config.PA.CheckChallenges(), "Invalid PA configuration")
	cmd.FailOnError(config.PA.CheckIdentifiers(), "Invalid PA configuration")

	kp, err := sagoodkey.NewPolicy(&config.CertChecker.GoodKey, nil)
	cmd.FailOnError(err, "Unable to create key policy")

	saDbMap, err := sa.InitWrappedDb(config.CertChecker.DB, prometheus.DefaultRegisterer, logger)
	cmd.FailOnError(err, "While initializing dbMap")

	checkerLatency := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "cert_checker_latency",
		Help: "Histogram of latencies a cert-checker worker takes to complete a batch",
	})
	prometheus.DefaultRegisterer.MustRegister(checkerLatency)

	pa, err := policy.New(config.PA.Identifiers, config.PA.Challenges, logger)
	cmd.FailOnError(err, "Failed to create PA")

	err = pa.LoadHostnamePolicyFile(config.CertChecker.HostnamePolicyFile)
	cmd.FailOnError(err, "Failed to load HostnamePolicyFile")

	if config.CertChecker.CTLogListFile != "" {
		err = loglist.InitLintList(config.CertChecker.CTLogListFile)
		cmd.FailOnError(err, "Failed to load CT Log List")
	}

	lints, err := linter.NewRegistry(config.CertChecker.IgnoredLints)
	cmd.FailOnError(err, "Failed to create zlint registry")
	if config.CertChecker.LintConfig != "" {
		lintconfig, err := lint.NewConfigFromFile(config.CertChecker.LintConfig)
		cmd.FailOnError(err, "Failed to load zlint config file")
		lints.SetConfiguration(lintconfig)
	}

	checker := newChecker(
		saDbMap,
		cmd.Clock(),
		pa,
		kp,
		config.CertChecker.CheckPeriod.Duration,
		acceptableValidityDurations,
		lints,
		logger,
	)
	fmt.Fprintf(os.Stderr, "# Getting certificates issued in the last %s\n", config.CertChecker.CheckPeriod)

	// Since we grab certificates in batches we don't want this to block, when it
	// is finished it will close the certificate channel which allows the range
	// loops in checker.processCerts to break
	go func() {
		err := checker.getCerts(context.TODO())
		cmd.FailOnError(err, "Batch retrieval of certificates failed")
	}()

	fmt.Fprintf(os.Stderr, "# Processing certificates using %d workers\n", config.CertChecker.Workers)
	wg := new(sync.WaitGroup)
	for range config.CertChecker.Workers {
		wg.Add(1)
		go func() {
			s := checker.clock.Now()
			checker.processCerts(context.TODO(), wg, config.CertChecker.BadResultsOnly)
			checkerLatency.Observe(checker.clock.Since(s).Seconds())
		}()
	}
	wg.Wait()
	fmt.Fprintf(
		os.Stderr,
		"# Finished processing certificates, report length: %d, good: %d, bad: %d\n",
		len(checker.issuedReport.Entries),
		checker.issuedReport.GoodCerts,
		checker.issuedReport.BadCerts,
	)
	err = checker.issuedReport.dump()
	cmd.FailOnError(err, "Failed to dump results: %s\n")
}

func init() {
	cmd.RegisterCommand("cert-checker", main, &cmd.ConfigValidator{Config: &Config{}})
}
