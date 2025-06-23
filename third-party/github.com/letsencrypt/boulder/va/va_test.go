package va

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"

	"github.com/letsencrypt/boulder/bdns"
	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	"github.com/letsencrypt/boulder/features"
	"github.com/letsencrypt/boulder/identifier"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/probs"
	"github.com/letsencrypt/boulder/test"
	vapb "github.com/letsencrypt/boulder/va/proto"
)

var expectedToken = "LoqXcYV8q5ONbJQxbmR7SCTNo3tiAXDfowyjxAjEuX0"
var expectedThumbprint = "9jg46WB3rR_AHD-EBXdN7cBkH1WOu0tA3M9fm21mqTI"
var expectedKeyAuthorization = ka(expectedToken)

func ka(token string) string {
	return token + "." + expectedThumbprint
}

func bigIntFromB64(b64 string) *big.Int {
	bytes, _ := base64.URLEncoding.DecodeString(b64)
	x := big.NewInt(0)
	x.SetBytes(bytes)
	return x
}

func intFromB64(b64 string) int {
	return int(bigIntFromB64(b64).Int64())
}

var n = bigIntFromB64("n4EPtAOCc9AlkeQHPzHStgAbgs7bTZLwUBZdR8_KuKPEHLd4rHVTeT-O-XV2jRojdNhxJWTDvNd7nqQ0VEiZQHz_AJmSCpMaJMRBSFKrKb2wqVwGU_NsYOYL-QtiWN2lbzcEe6XC0dApr5ydQLrHqkHHig3RBordaZ6Aj-oBHqFEHYpPe7Tpe-OfVfHd1E6cS6M1FZcD1NNLYD5lFHpPI9bTwJlsde3uhGqC0ZCuEHg8lhzwOHrtIQbS0FVbb9k3-tVTU4fg_3L_vniUFAKwuCLqKnS2BYwdq_mzSnbLY7h_qixoR7jig3__kRhuaxwUkRz5iaiQkqgc5gHdrNP5zw==")
var e = intFromB64("AQAB")
var d = bigIntFromB64("bWUC9B-EFRIo8kpGfh0ZuyGPvMNKvYWNtB_ikiH9k20eT-O1q_I78eiZkpXxXQ0UTEs2LsNRS-8uJbvQ-A1irkwMSMkK1J3XTGgdrhCku9gRldY7sNA_AKZGh-Q661_42rINLRCe8W-nZ34ui_qOfkLnK9QWDDqpaIsA-bMwWWSDFu2MUBYwkHTMEzLYGqOe04noqeq1hExBTHBOBdkMXiuFhUq1BU6l-DqEiWxqg82sXt2h-LMnT3046AOYJoRioz75tSUQfGCshWTBnP5uDjd18kKhyv07lhfSJdrPdM5Plyl21hsFf4L_mHCuoFau7gdsPfHPxxjVOcOpBrQzwQ==")
var p = bigIntFromB64("uKE2dh-cTf6ERF4k4e_jy78GfPYUIaUyoSSJuBzp3Cubk3OCqs6grT8bR_cu0Dm1MZwWmtdqDyI95HrUeq3MP15vMMON8lHTeZu2lmKvwqW7anV5UzhM1iZ7z4yMkuUwFWoBvyY898EXvRD-hdqRxHlSqAZ192zB3pVFJ0s7pFc=")
var q = bigIntFromB64("uKE2dh-cTf6ERF4k4e_jy78GfPYUIaUyoSSJuBzp3Cubk3OCqs6grT8bR_cu0Dm1MZwWmtdqDyI95HrUeq3MP15vMMON8lHTeZu2lmKvwqW7anV5UzhM1iZ7z4yMkuUwFWoBvyY898EXvRD-hdqRxHlSqAZ192zB3pVFJ0s7pFc=")

var TheKey = rsa.PrivateKey{
	PublicKey: rsa.PublicKey{N: n, E: e},
	D:         d,
	Primes:    []*big.Int{p, q},
}

var accountKey = &jose.JSONWebKey{Key: TheKey.Public()}

// Return an ACME DNS identifier for the given hostname
func dnsi(hostname string) identifier.ACMEIdentifier {
	return identifier.DNSIdentifier(hostname)
}

var ctx context.Context

func TestMain(m *testing.M) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Minute)
	ret := m.Run()
	cancel()
	os.Exit(ret)
}

var accountURIPrefixes = []string{"http://boulder.service.consul:4000/acme/reg/"}

func createValidationRequest(domain string, challengeType core.AcmeChallenge) *vapb.PerformValidationRequest {
	return &vapb.PerformValidationRequest{
		Domain: domain,
		Challenge: &corepb.Challenge{
			Type:              string(challengeType),
			Status:            string(core.StatusPending),
			Token:             expectedToken,
			Validationrecords: nil,
			KeyAuthorization:  expectedKeyAuthorization,
		},
		Authz: &vapb.AuthzMeta{
			Id:    "",
			RegID: 1,
		},
	}
}

// setup returns an in-memory VA and a mock logger. The default resolver client
// is MockClient{}, but can be overridden.
func setup(srv *httptest.Server, maxRemoteFailures int, userAgent string, remoteVAs []RemoteVA, mockDNSClientOverride bdns.Client) (*ValidationAuthorityImpl, *blog.Mock) {
	features.Reset()
	fc := clock.NewFake()

	logger := blog.NewMock()

	if userAgent == "" {
		userAgent = "user agent 1.0"
	}

	va, err := NewValidationAuthorityImpl(
		&bdns.MockClient{Log: logger},
		nil,
		maxRemoteFailures,
		userAgent,
		"letsencrypt.org",
		metrics.NoopRegisterer,
		fc,
		logger,
		accountURIPrefixes,
	)

	if mockDNSClientOverride != nil {
		va.dnsClient = mockDNSClientOverride
	}

	// Adjusting industry regulated ACME challenge port settings is fine during
	// testing
	if srv != nil {
		port := getPort(srv)
		va.httpPort = port
		va.tlsPort = port
	}

	if err != nil {
		panic(fmt.Sprintf("Failed to create validation authority: %v", err))
	}
	if remoteVAs != nil {
		va.remoteVAs = remoteVAs
	}
	return va, logger
}

func setupRemote(srv *httptest.Server, userAgent string, mockDNSClientOverride bdns.Client) RemoteClients {
	rva, _ := setup(srv, 0, userAgent, nil, mockDNSClientOverride)

	return RemoteClients{VAClient: &inMemVA{*rva}, CAAClient: &inMemVA{*rva}}
}

type multiSrv struct {
	*httptest.Server

	mu         sync.Mutex
	allowedUAs map[string]bool
}

func (s *multiSrv) setAllowedUAs(allowedUAs map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowedUAs = allowedUAs
}

const slowRemoteSleepMillis = 1000

func httpMultiSrv(t *testing.T, token string, allowedUAs map[string]bool) *multiSrv {
	t.Helper()
	m := http.NewServeMux()

	server := httptest.NewUnstartedServer(m)
	ms := &multiSrv{server, sync.Mutex{}, allowedUAs}

	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() == "slow remote" {
			time.Sleep(slowRemoteSleepMillis)
		}
		ms.mu.Lock()
		defer ms.mu.Unlock()
		if ms.allowedUAs[r.UserAgent()] {
			ch := core.Challenge{Token: token}
			keyAuthz, _ := ch.ExpectedKeyAuthorization(accountKey)
			fmt.Fprint(w, keyAuthz, "\n\r \t")
		} else {
			fmt.Fprint(w, "???")
		}
	})

	ms.Start()
	return ms
}

// cancelledVA is a mock that always returns context.Canceled for
// PerformValidation calls
type cancelledVA struct{}

func (v cancelledVA) PerformValidation(_ context.Context, _ *vapb.PerformValidationRequest, _ ...grpc.CallOption) (*vapb.ValidationResult, error) {
	return nil, context.Canceled
}

func (v cancelledVA) IsCAAValid(_ context.Context, _ *vapb.IsCAAValidRequest, _ ...grpc.CallOption) (*vapb.IsCAAValidResponse, error) {
	return nil, context.Canceled
}

// brokenRemoteVA is a mock for the VAClient and CAAClient interfaces that always return
// errors.
type brokenRemoteVA struct{}

// errBrokenRemoteVA is the error returned by a brokenRemoteVA's
// PerformValidation and IsSafeDomain functions.
var errBrokenRemoteVA = errors.New("brokenRemoteVA is broken")

// PerformValidation returns errBrokenRemoteVA unconditionally
func (b brokenRemoteVA) PerformValidation(_ context.Context, _ *vapb.PerformValidationRequest, _ ...grpc.CallOption) (*vapb.ValidationResult, error) {
	return nil, errBrokenRemoteVA
}

func (b brokenRemoteVA) IsCAAValid(_ context.Context, _ *vapb.IsCAAValidRequest, _ ...grpc.CallOption) (*vapb.IsCAAValidResponse, error) {
	return nil, errBrokenRemoteVA
}

// inMemVA is a wrapper which fulfills the VAClient and CAAClient
// interfaces, but then forwards requests directly to its inner
// ValidationAuthorityImpl rather than over the network. This lets a local
// in-memory mock VA act like a remote VA.
type inMemVA struct {
	rva ValidationAuthorityImpl
}

func (inmem inMemVA) PerformValidation(ctx context.Context, req *vapb.PerformValidationRequest, _ ...grpc.CallOption) (*vapb.ValidationResult, error) {
	return inmem.rva.PerformValidation(ctx, req)
}

func (inmem inMemVA) IsCAAValid(ctx context.Context, req *vapb.IsCAAValidRequest, _ ...grpc.CallOption) (*vapb.IsCAAValidResponse, error) {
	return inmem.rva.IsCAAValid(ctx, req)
}

func TestValidateMalformedChallenge(t *testing.T) {
	va, _ := setup(nil, 0, "", nil, nil)

	_, err := va.validateChallenge(ctx, dnsi("example.com"), "fake-type-01", expectedToken, expectedKeyAuthorization)

	prob := detailedError(err)
	test.AssertEquals(t, prob.Type, probs.MalformedProblem)
}

func TestPerformValidationInvalid(t *testing.T) {
	va, _ := setup(nil, 0, "", nil, nil)

	req := createValidationRequest("foo.com", core.ChallengeTypeDNS01)
	res, _ := va.PerformValidation(context.Background(), req)
	test.Assert(t, res.Problems != nil, "validation succeeded")

	test.AssertMetricWithLabelsEquals(t, va.metrics.validationTime, prometheus.Labels{
		"type":         "dns-01",
		"result":       "invalid",
		"problem_type": "unauthorized",
	}, 1)
}

func TestInternalErrorLogged(t *testing.T) {
	va, mockLog := setup(nil, 0, "", nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	req := createValidationRequest("nonexistent.com", core.ChallengeTypeHTTP01)
	_, err := va.PerformValidation(ctx, req)
	test.AssertNotError(t, err, "failed validation should not be an error")
	matchingLogs := mockLog.GetAllMatching(
		`Validation result JSON=.*"InternalError":"127.0.0.1: Get.*nonexistent.com/\.well-known.*: context deadline exceeded`)
	test.AssertEquals(t, len(matchingLogs), 1)
}

func TestPerformValidationValid(t *testing.T) {
	va, mockLog := setup(nil, 0, "", nil, nil)

	// create a challenge with well known token
	req := createValidationRequest("good-dns01.com", core.ChallengeTypeDNS01)
	res, _ := va.PerformValidation(context.Background(), req)
	test.Assert(t, res.Problems == nil, fmt.Sprintf("validation failed: %#v", res.Problems))

	test.AssertMetricWithLabelsEquals(t, va.metrics.validationTime, prometheus.Labels{
		"type":         "dns-01",
		"result":       "valid",
		"problem_type": "",
	}, 1)
	resultLog := mockLog.GetAllMatching(`Validation result`)
	if len(resultLog) != 1 {
		t.Fatalf("Wrong number of matching lines for 'Validation result'")
	}
	if !strings.Contains(resultLog[0], `"Hostname":"good-dns01.com"`) {
		t.Error("PerformValidation didn't log validation hostname.")
	}
}

// TestPerformValidationWildcard tests that the VA properly strips the `*.`
// prefix from a wildcard name provided to the PerformValidation function.
func TestPerformValidationWildcard(t *testing.T) {
	va, mockLog := setup(nil, 0, "", nil, nil)

	// create a challenge with well known token
	req := createValidationRequest("*.good-dns01.com", core.ChallengeTypeDNS01)
	// perform a validation for a wildcard name
	res, _ := va.PerformValidation(context.Background(), req)
	test.Assert(t, res.Problems == nil, fmt.Sprintf("validation failed: %#v", res.Problems))

	test.AssertMetricWithLabelsEquals(t, va.metrics.validationTime, prometheus.Labels{
		"type":         "dns-01",
		"result":       "valid",
		"problem_type": "",
	}, 1)
	resultLog := mockLog.GetAllMatching(`Validation result`)
	if len(resultLog) != 1 {
		t.Fatalf("Wrong number of matching lines for 'Validation result'")
	}

	// We expect that the top level Hostname reflect the wildcard name
	if !strings.Contains(resultLog[0], `"Hostname":"*.good-dns01.com"`) {
		t.Errorf("PerformValidation didn't log correct validation hostname.")
	}
	// We expect that the ValidationRecord contain the correct non-wildcard
	// hostname that was validated
	if !strings.Contains(resultLog[0], `"hostname":"good-dns01.com"`) {
		t.Errorf("PerformValidation didn't log correct validation record hostname.")
	}
}

func TestDCVAndCAASequencing(t *testing.T) {
	va, mockLog := setup(nil, 0, "", nil, nil)

	// When validation succeeds, CAA should be checked.
	mockLog.Clear()
	req := createValidationRequest("good-dns01.com", core.ChallengeTypeDNS01)
	res, err := va.PerformValidation(context.Background(), req)
	test.AssertNotError(t, err, "performing validation")
	test.Assert(t, res.Problems == nil, fmt.Sprintf("validation failed: %#v", res.Problems))
	caaLog := mockLog.GetAllMatching(`Checked CAA records for`)
	test.AssertEquals(t, len(caaLog), 1)

	// When validation fails, CAA should be skipped.
	mockLog.Clear()
	req = createValidationRequest("bad-dns01.com", core.ChallengeTypeDNS01)
	res, err = va.PerformValidation(context.Background(), req)
	test.AssertNotError(t, err, "performing validation")
	test.Assert(t, res.Problems != nil, "validation succeeded")
	caaLog = mockLog.GetAllMatching(`Checked CAA records for`)
	test.AssertEquals(t, len(caaLog), 0)
}

func TestMultiVA(t *testing.T) {
	// Create a new challenge to use for the httpSrv
	req := createValidationRequest("localhost", core.ChallengeTypeHTTP01)

	const (
		remoteUA1 = "remote 1"
		remoteUA2 = "remote 2"
		localUA   = "local 1"
	)
	allowedUAs := map[string]bool{
		localUA:   true,
		remoteUA1: true,
		remoteUA2: true,
	}

	// Create an IPv4 test server
	ms := httpMultiSrv(t, expectedToken, allowedUAs)
	defer ms.Close()

	remoteVA1 := setupRemote(ms.Server, remoteUA1, nil)
	remoteVA2 := setupRemote(ms.Server, remoteUA2, nil)
	remoteVAs := []RemoteVA{
		{remoteVA1, remoteUA1},
		{remoteVA2, remoteUA2},
	}
	brokenVA := RemoteClients{
		VAClient:  brokenRemoteVA{},
		CAAClient: brokenRemoteVA{},
	}
	cancelledVA := RemoteClients{
		VAClient:  cancelledVA{},
		CAAClient: cancelledVA{},
	}

	unauthorized := probs.Unauthorized(fmt.Sprintf(
		`The key authorization file from the server did not match this challenge. Expected %q (got "???")`,
		expectedKeyAuthorization))
	expectedInternalErrLine := fmt.Sprintf(
		`ERR: \[AUDIT\] Remote VA "broken".PerformValidation failed: %s`,
		errBrokenRemoteVA.Error())
	testCases := []struct {
		Name         string
		RemoteVAs    []RemoteVA
		AllowedUAs   map[string]bool
		ExpectedProb *probs.ProblemDetails
		ExpectedLog  string
	}{
		{
			// With local and both remote VAs working there should be no problem.
			Name:       "Local and remote VAs OK",
			RemoteVAs:  remoteVAs,
			AllowedUAs: allowedUAs,
		},
		{
			// If the local VA fails everything should fail
			Name:         "Local VA bad, remote VAs OK",
			RemoteVAs:    remoteVAs,
			AllowedUAs:   map[string]bool{remoteUA1: true, remoteUA2: true},
			ExpectedProb: unauthorized,
		},
		{
			// If a remote VA fails with an internal err it should fail
			Name: "Local VA ok, remote VA internal err",
			RemoteVAs: []RemoteVA{
				{remoteVA1, remoteUA1},
				{brokenVA, "broken"},
			},
			AllowedUAs:   allowedUAs,
			ExpectedProb: probs.ServerInternal("During secondary validation: Remote PerformValidation RPC failed"),
			// The real failure cause should be logged
			ExpectedLog: expectedInternalErrLine,
		},
		{
			// With only one working remote VA there should be a validation failure
			Name:       "Local VA and one remote VA OK",
			RemoteVAs:  remoteVAs,
			AllowedUAs: map[string]bool{localUA: true, remoteUA2: true},
			ExpectedProb: probs.Unauthorized(fmt.Sprintf(
				`During secondary validation: The key authorization file from the server did not match this challenge. Expected %q (got "???")`,
				expectedKeyAuthorization)),
		},
		{
			// Any remote VA cancellations are a problem.
			Name: "Local VA and one remote VA OK, one cancelled VA",
			RemoteVAs: []RemoteVA{
				{remoteVA1, remoteUA1},
				{cancelledVA, remoteUA2},
			},
			AllowedUAs:   allowedUAs,
			ExpectedProb: probs.ServerInternal("During secondary validation: Remote PerformValidation RPC canceled"),
		},
		{
			// Any remote VA cancellations are a problem.
			Name: "Local VA OK, two cancelled remote VAs",
			RemoteVAs: []RemoteVA{
				{cancelledVA, remoteUA1},
				{cancelledVA, remoteUA2},
			},
			AllowedUAs:   allowedUAs,
			ExpectedProb: probs.ServerInternal("During secondary validation: Remote PerformValidation RPC canceled"),
		},
		{
			// With the local and remote VAs seeing diff problems, we expect a problem.
			Name:       "Local and remote VA differential, full results, enforce multi VA",
			RemoteVAs:  remoteVAs,
			AllowedUAs: map[string]bool{localUA: true},
			ExpectedProb: probs.Unauthorized(fmt.Sprintf(
				`During secondary validation: The key authorization file from the server did not match this challenge. Expected %q (got "???")`,
				expectedKeyAuthorization)),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			// Configure the test server with the testcase allowed UAs.
			ms.setAllowedUAs(tc.AllowedUAs)

			// Configure a primary VA with testcase remote VAs.
			localVA, mockLog := setup(ms.Server, 0, localUA, tc.RemoteVAs, nil)

			// Perform all validations
			res, _ := localVA.PerformValidation(ctx, req)
			if res.Problems == nil && tc.ExpectedProb != nil {
				t.Errorf("expected prob %v, got nil", tc.ExpectedProb)
			} else if res.Problems != nil && tc.ExpectedProb == nil {
				t.Errorf("expected no prob, got %v", res.Problems)
			} else if res.Problems != nil && tc.ExpectedProb != nil {
				// That result should match expected.
				test.AssertEquals(t, res.Problems.ProblemType, string(tc.ExpectedProb.Type))
				test.AssertEquals(t, res.Problems.Detail, tc.ExpectedProb.Detail)
			}

			if tc.ExpectedLog != "" {
				lines := mockLog.GetAllMatching(tc.ExpectedLog)
				if len(lines) != 1 {
					t.Fatalf("Got log %v; expected %q", mockLog.GetAll(), tc.ExpectedLog)
				}
			}
		})
	}
}

func TestMultiVAEarlyReturn(t *testing.T) {
	const (
		remoteUA1 = "remote 1"
		remoteUA2 = "slow remote"
		localUA   = "local 1"
	)
	allowedUAs := map[string]bool{
		localUA:   true,
		remoteUA1: false, // forbid UA 1 to provoke early return
		remoteUA2: true,
	}

	ms := httpMultiSrv(t, expectedToken, allowedUAs)
	defer ms.Close()

	remoteVA1 := setupRemote(ms.Server, remoteUA1, nil)
	remoteVA2 := setupRemote(ms.Server, remoteUA2, nil)

	remoteVAs := []RemoteVA{
		{remoteVA1, remoteUA1},
		{remoteVA2, remoteUA2},
	}

	// Create a local test VA with the two remote VAs
	localVA, _ := setup(ms.Server, 0, localUA, remoteVAs, nil)

	// Perform all validations
	start := time.Now()
	req := createValidationRequest("localhost", core.ChallengeTypeHTTP01)
	res, _ := localVA.PerformValidation(ctx, req)

	// It should always fail
	if res.Problems == nil {
		t.Error("expected prob from PerformValidation, got nil")
	}

	elapsed := time.Since(start).Round(time.Millisecond).Milliseconds()

	// The slow UA should sleep for `slowRemoteSleepMillis`. But the first remote
	// VA should fail quickly and the early-return code should cause the overall
	// overall validation to return a prob quickly (i.e. in less than half of
	// `slowRemoteSleepMillis`).
	if elapsed > slowRemoteSleepMillis/2 {
		t.Errorf(
			"Expected an early return from PerformValidation in < %d ms, took %d ms",
			slowRemoteSleepMillis/2, elapsed)
	}
}

func TestMultiVAPolicy(t *testing.T) {
	const (
		remoteUA1 = "remote 1"
		remoteUA2 = "remote 2"
		localUA   = "local 1"
	)
	// Forbid both remote UAs to ensure that multi-va fails
	allowedUAs := map[string]bool{
		localUA:   true,
		remoteUA1: false,
		remoteUA2: false,
	}

	ms := httpMultiSrv(t, expectedToken, allowedUAs)
	defer ms.Close()

	remoteVA1 := setupRemote(ms.Server, remoteUA1, nil)
	remoteVA2 := setupRemote(ms.Server, remoteUA2, nil)

	remoteVAs := []RemoteVA{
		{remoteVA1, remoteUA1},
		{remoteVA2, remoteUA2},
	}

	// Create a local test VA with the two remote VAs
	localVA, _ := setup(ms.Server, 0, localUA, remoteVAs, nil)

	// Perform validation for a domain not in the disabledDomains list
	req := createValidationRequest("letsencrypt.org", core.ChallengeTypeHTTP01)
	res, _ := localVA.PerformValidation(ctx, req)
	// It should fail
	if res.Problems == nil {
		t.Error("expected prob from PerformValidation, got nil")
	}
}

func TestDetailedError(t *testing.T) {
	cases := []struct {
		err      error
		ip       net.IP
		expected string
	}{
		{
			err: ipError{
				ip: net.ParseIP("192.168.1.1"),
				err: &net.OpError{
					Op:  "dial",
					Net: "tcp",
					Err: &os.SyscallError{
						Syscall: "getsockopt",
						Err:     syscall.ECONNREFUSED,
					},
				},
			},
			expected: "192.168.1.1: Connection refused",
		},
		{
			err: &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: &os.SyscallError{
					Syscall: "getsockopt",
					Err:     syscall.ECONNREFUSED,
				},
			},
			expected: "Connection refused",
		},
		{
			err: &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: &os.SyscallError{
					Syscall: "getsockopt",
					Err:     syscall.ECONNRESET,
				},
			},
			ip:       nil,
			expected: "Connection reset by peer",
		},
	}
	for _, tc := range cases {
		actual := detailedError(tc.err).Detail
		if actual != tc.expected {
			t.Errorf("Wrong detail for %v. Got %q, expected %q", tc.err, actual, tc.expected)
		}
	}
}

func TestLogRemoteDifferentials(t *testing.T) {
	// Create some remote VAs
	remoteVA1 := setupRemote(nil, "remote 1", nil)
	remoteVA2 := setupRemote(nil, "remote 2", nil)
	remoteVA3 := setupRemote(nil, "remote 3", nil)
	remoteVAs := []RemoteVA{
		{remoteVA1, "remote 1"},
		{remoteVA2, "remote 2"},
		{remoteVA3, "remote 3"},
	}

	// Set up a local VA that allows a max of 2 remote failures.
	localVA, mockLog := setup(nil, 2, "local 1", remoteVAs, nil)

	egProbA := probs.DNS("root DNS servers closed at 4:30pm")
	egProbB := probs.OrderNotReady("please take a number")

	testCases := []struct {
		name        string
		remoteProbs []*remoteVAResult
		expectedLog string
	}{
		{
			name: "all results equal (nil)",
			remoteProbs: []*remoteVAResult{
				{Problem: nil, VAHostname: "remoteA"},
				{Problem: nil, VAHostname: "remoteB"},
				{Problem: nil, VAHostname: "remoteC"},
			},
		},
		{
			name: "all results equal (not nil)",
			remoteProbs: []*remoteVAResult{
				{Problem: egProbA, VAHostname: "remoteA"},
				{Problem: egProbA, VAHostname: "remoteB"},
				{Problem: egProbA, VAHostname: "remoteC"},
			},
			expectedLog: `INFO: remoteVADifferentials JSON={"Domain":"example.com","AccountID":1999,"ChallengeType":"blorpus-01","RemoteSuccesses":0,"RemoteFailures":[{"VAHostname":"remoteA","Problem":{"type":"dns","detail":"root DNS servers closed at 4:30pm","status":400}},{"VAHostname":"remoteB","Problem":{"type":"dns","detail":"root DNS servers closed at 4:30pm","status":400}},{"VAHostname":"remoteC","Problem":{"type":"dns","detail":"root DNS servers closed at 4:30pm","status":400}}]}`,
		},
		{
			name: "differing results, some non-nil",
			remoteProbs: []*remoteVAResult{
				{Problem: nil, VAHostname: "remoteA"},
				{Problem: egProbB, VAHostname: "remoteB"},
				{Problem: nil, VAHostname: "remoteC"},
			},
			expectedLog: `INFO: remoteVADifferentials JSON={"Domain":"example.com","AccountID":1999,"ChallengeType":"blorpus-01","RemoteSuccesses":2,"RemoteFailures":[{"VAHostname":"remoteB","Problem":{"type":"orderNotReady","detail":"please take a number","status":403}}]}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockLog.Clear()

			localVA.logRemoteResults(
				"example.com", 1999, "blorpus-01", tc.remoteProbs)

			lines := mockLog.GetAllMatching("remoteVADifferentials JSON=.*")
			if tc.expectedLog != "" {
				test.AssertEquals(t, len(lines), 1)
				test.AssertEquals(t, lines[0], tc.expectedLog)
			} else {
				test.AssertEquals(t, len(lines), 0)
			}
		})
	}
}
