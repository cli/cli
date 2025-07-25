package sfe

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"google.golang.org/grpc"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/features"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/mocks"
	"github.com/letsencrypt/boulder/must"
	"github.com/letsencrypt/boulder/test"
	"github.com/letsencrypt/boulder/unpause"

	rapb "github.com/letsencrypt/boulder/ra/proto"
)

type MockRegistrationAuthority struct {
	rapb.RegistrationAuthorityClient
}

func (ra *MockRegistrationAuthority) UnpauseAccount(context.Context, *rapb.UnpauseAccountRequest, ...grpc.CallOption) (*rapb.UnpauseAccountResponse, error) {
	return &rapb.UnpauseAccountResponse{}, nil
}

func mustParseURL(s string) *url.URL {
	return must.Do(url.Parse(s))
}

func setupSFE(t *testing.T) (SelfServiceFrontEndImpl, clock.FakeClock) {
	features.Reset()

	fc := clock.NewFake()
	// Set to some non-zero time.
	fc.Set(time.Date(2020, 10, 10, 0, 0, 0, 0, time.UTC))

	stats := metrics.NoopRegisterer

	mockSA := mocks.NewStorageAuthorityReadOnly(fc)

	hmacKey := cmd.HMACKeyConfig{KeyFile: "../test/secrets/sfe_unpause_key"}
	key, err := hmacKey.Load()
	test.AssertNotError(t, err, "Unable to load HMAC key")

	sfe, err := NewSelfServiceFrontEndImpl(
		stats,
		fc,
		blog.NewMock(),
		10*time.Second,
		&MockRegistrationAuthority{},
		mockSA,
		key,
	)
	test.AssertNotError(t, err, "Unable to create SFE")

	return sfe, fc
}

func TestIndexPath(t *testing.T) {
	t.Parallel()
	sfe, _ := setupSFE(t)
	responseWriter := httptest.NewRecorder()
	sfe.Index(responseWriter, &http.Request{
		Method: "GET",
		URL:    mustParseURL("/"),
	})

	test.AssertEquals(t, responseWriter.Code, http.StatusOK)
	test.AssertContains(t, responseWriter.Body.String(), "<title>Let's Encrypt - Portal</title>")
}

func TestBuildIDPath(t *testing.T) {
	t.Parallel()
	sfe, _ := setupSFE(t)
	responseWriter := httptest.NewRecorder()
	sfe.BuildID(responseWriter, &http.Request{
		Method: "GET",
		URL:    mustParseURL("/build"),
	})

	test.AssertEquals(t, responseWriter.Code, http.StatusOK)
	test.AssertContains(t, responseWriter.Body.String(), "Boulder=(")
}

func TestUnpausePaths(t *testing.T) {
	t.Parallel()
	sfe, fc := setupSFE(t)
	unpauseSigner, err := unpause.NewJWTSigner(cmd.HMACKeyConfig{KeyFile: "../test/secrets/sfe_unpause_key"})
	test.AssertNotError(t, err, "Should have been able to create JWT signer, but could not")

	// GET with no JWT
	responseWriter := httptest.NewRecorder()
	sfe.UnpauseForm(responseWriter, &http.Request{
		Method: "GET",
		URL:    mustParseURL(unpause.GetForm),
	})
	test.AssertEquals(t, responseWriter.Code, http.StatusOK)
	test.AssertContains(t, responseWriter.Body.String(), "Invalid unpause URL")

	// GET with an invalid JWT
	responseWriter = httptest.NewRecorder()
	sfe.UnpauseForm(responseWriter, &http.Request{
		Method: "GET",
		URL:    mustParseURL(fmt.Sprintf(unpause.GetForm + "?jwt=x")),
	})
	test.AssertEquals(t, responseWriter.Code, http.StatusOK)
	test.AssertContains(t, responseWriter.Body.String(), "Invalid unpause URL")

	// GET with an expired JWT
	expiredJWT, err := unpause.GenerateJWT(unpauseSigner, 1234567890, []string{"example.net"}, time.Hour, fc)
	test.AssertNotError(t, err, "Should have been able to create JWT, but could not")
	responseWriter = httptest.NewRecorder()
	// Advance the clock by 337 hours to make the JWT expired.
	fc.Add(time.Hour * 337)
	sfe.UnpauseForm(responseWriter, &http.Request{
		Method: "GET",
		URL:    mustParseURL(unpause.GetForm + "?jwt=" + expiredJWT),
	})
	test.AssertEquals(t, responseWriter.Code, http.StatusOK)
	test.AssertContains(t, responseWriter.Body.String(), "Expired unpause URL")

	// GET with a valid JWT and a single identifier
	validJWT, err := unpause.GenerateJWT(unpauseSigner, 1234567890, []string{"example.com"}, time.Hour, fc)
	test.AssertNotError(t, err, "Should have been able to create JWT, but could not")
	responseWriter = httptest.NewRecorder()
	sfe.UnpauseForm(responseWriter, &http.Request{
		Method: "GET",
		URL:    mustParseURL(unpause.GetForm + "?jwt=" + validJWT),
	})
	test.AssertEquals(t, responseWriter.Code, http.StatusOK)
	test.AssertContains(t, responseWriter.Body.String(), "Action required to unpause your account")
	test.AssertContains(t, responseWriter.Body.String(), "example.com")

	// GET with a valid JWT and multiple identifiers
	validJWT, err = unpause.GenerateJWT(unpauseSigner, 1234567890, []string{"example.com", "example.net", "example.org"}, time.Hour, fc)
	test.AssertNotError(t, err, "Should have been able to create JWT, but could not")
	responseWriter = httptest.NewRecorder()
	sfe.UnpauseForm(responseWriter, &http.Request{
		Method: "GET",
		URL:    mustParseURL(unpause.GetForm + "?jwt=" + validJWT),
	})
	test.AssertEquals(t, responseWriter.Code, http.StatusOK)
	test.AssertContains(t, responseWriter.Body.String(), "Action required to unpause your account")
	test.AssertContains(t, responseWriter.Body.String(), "example.com")
	test.AssertContains(t, responseWriter.Body.String(), "example.net")
	test.AssertContains(t, responseWriter.Body.String(), "example.org")

	// POST with an expired JWT
	responseWriter = httptest.NewRecorder()
	sfe.UnpauseSubmit(responseWriter, &http.Request{
		Method: "POST",
		URL:    mustParseURL(unpausePostForm + "?jwt=" + expiredJWT),
	})
	test.AssertEquals(t, responseWriter.Code, http.StatusOK)
	test.AssertContains(t, responseWriter.Body.String(), "Expired unpause URL")

	// POST with no JWT
	responseWriter = httptest.NewRecorder()
	sfe.UnpauseSubmit(responseWriter, &http.Request{
		Method: "POST",
		URL:    mustParseURL(unpausePostForm),
	})
	test.AssertEquals(t, responseWriter.Code, http.StatusOK)
	test.AssertContains(t, responseWriter.Body.String(), "Invalid unpause URL")

	// POST with an invalid JWT, missing one of the three parts
	responseWriter = httptest.NewRecorder()
	sfe.UnpauseSubmit(responseWriter, &http.Request{
		Method: "POST",
		URL:    mustParseURL(unpausePostForm + "?jwt=x.x"),
	})
	test.AssertEquals(t, responseWriter.Code, http.StatusOK)
	test.AssertContains(t, responseWriter.Body.String(), "Invalid unpause URL")

	// POST with an invalid JWT, all parts present but missing some characters
	responseWriter = httptest.NewRecorder()
	sfe.UnpauseSubmit(responseWriter, &http.Request{
		Method: "POST",
		URL:    mustParseURL(unpausePostForm + "?jwt=x.x.x"),
	})
	test.AssertEquals(t, responseWriter.Code, http.StatusOK)
	test.AssertContains(t, responseWriter.Body.String(), "Invalid unpause URL")

	// POST with a valid JWT redirects to a success page
	responseWriter = httptest.NewRecorder()
	sfe.UnpauseSubmit(responseWriter, &http.Request{
		Method: "POST",
		URL:    mustParseURL(unpausePostForm + "?jwt=" + validJWT),
	})
	test.AssertEquals(t, responseWriter.Code, http.StatusFound)
	test.AssertEquals(t, unpauseStatus+"?count=0", responseWriter.Result().Header.Get("Location"))

	// Redirecting after a successful unpause POST displays the success page.
	responseWriter = httptest.NewRecorder()
	sfe.UnpauseStatus(responseWriter, &http.Request{
		Method: "GET",
		URL:    mustParseURL(unpauseStatus + "?count=1"),
	})
	test.AssertEquals(t, responseWriter.Code, http.StatusOK)
	test.AssertContains(t, responseWriter.Body.String(), "Successfully unpaused all 1 identifier(s)")

	// Redirecting after a successful unpause POST with a count of 0 displays
	// the already unpaused page.
	responseWriter = httptest.NewRecorder()
	sfe.UnpauseStatus(responseWriter, &http.Request{
		Method: "GET",
		URL:    mustParseURL(unpauseStatus + "?count=0"),
	})
	test.AssertEquals(t, responseWriter.Code, http.StatusOK)
	test.AssertContains(t, responseWriter.Body.String(), "Account already unpaused")

	// Redirecting after a successful unpause POST with a count equal to the
	// maximum number of identifiers displays the success with caveat page.
	responseWriter = httptest.NewRecorder()
	sfe.UnpauseStatus(responseWriter, &http.Request{
		Method: "GET",
		URL:    mustParseURL(unpauseStatus + "?count=" + fmt.Sprintf("%d", unpause.RequestLimit)),
	})
	test.AssertEquals(t, responseWriter.Code, http.StatusOK)
	test.AssertContains(t, responseWriter.Body.String(), "Some identifiers were unpaused")
}
