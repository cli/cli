package email

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"github.com/letsencrypt/boulder/test"
)

func defaultTokenHandler(w http.ResponseWriter, r *http.Request) {
	err := json.NewEncoder(w).Encode(oauthTokenResp{
		AccessToken: "dummy",
		ExpiresIn:   3600,
	})
	if err != nil {
		// This should never happen.
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("failed to encode token"))
		return
	}
}

func TestSendContactSuccess(t *testing.T) {
	t.Parallel()

	contactHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer dummy" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}

	tokenSrv := httptest.NewServer(http.HandlerFunc(defaultTokenHandler))
	defer tokenSrv.Close()

	contactSrv := httptest.NewServer(http.HandlerFunc(contactHandler))
	defer contactSrv.Close()

	clk := clock.NewFake()
	client, err := NewPardotClientImpl(clk, "biz-unit", "cid", "csec", tokenSrv.URL, contactSrv.URL)
	test.AssertNotError(t, err, "failed to create client")

	err = client.SendContact("test@example.com")
	test.AssertNotError(t, err, "SendContact should succeed")
}

func TestSendContactUpdateTokenFails(t *testing.T) {
	t.Parallel()

	tokenHandlerThatAlwaysErrors := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, "token error")
	}

	contactHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	tokenSrv := httptest.NewServer(http.HandlerFunc(tokenHandlerThatAlwaysErrors))
	defer tokenSrv.Close()

	contactSrv := httptest.NewServer(http.HandlerFunc(contactHandler))
	defer contactSrv.Close()

	clk := clock.NewFake()
	client, err := NewPardotClientImpl(clk, "biz-unit", "cid", "csec", tokenSrv.URL, contactSrv.URL)
	test.AssertNotError(t, err, "Failed to create client")

	err = client.SendContact("test@example.com")
	test.AssertError(t, err, "Expected token update to fail")
	test.AssertContains(t, err.Error(), "failed to update token")
}

func TestSendContact4xx(t *testing.T) {
	t.Parallel()

	contactHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, err := io.WriteString(w, "bad request")
		test.AssertNotError(t, err, "failed to write response")
	}

	tokenSrv := httptest.NewServer(http.HandlerFunc(defaultTokenHandler))
	defer tokenSrv.Close()

	contactSrv := httptest.NewServer(http.HandlerFunc(contactHandler))
	defer contactSrv.Close()

	clk := clock.NewFake()
	client, err := NewPardotClientImpl(clk, "biz-unit", "cid", "csec", tokenSrv.URL, contactSrv.URL)
	test.AssertNotError(t, err, "Failed to create client")

	err = client.SendContact("test@example.com")
	test.AssertError(t, err, "Should fail on 400")
	test.AssertContains(t, err.Error(), "create contact request returned status 400")
}

func TestSendContactTokenExpiry(t *testing.T) {
	t.Parallel()

	// tokenHandler returns "old_token" on the first call and "new_token" on subsequent calls.
	tokenRetrieved := false
	tokenHandler := func(w http.ResponseWriter, r *http.Request) {
		token := "new_token"
		if !tokenRetrieved {
			token = "old_token"
			tokenRetrieved = true
		}
		err := json.NewEncoder(w).Encode(oauthTokenResp{
			AccessToken: token,
			ExpiresIn:   3600,
		})
		test.AssertNotError(t, err, "failed to encode token")
	}

	// contactHandler expects "old_token" for the first request and "new_token" for the next.
	firstRequest := true
	contactHandler := func(w http.ResponseWriter, r *http.Request) {
		expectedToken := "new_token"
		if firstRequest {
			expectedToken = "old_token"
			firstRequest = false
		}
		if r.Header.Get("Authorization") != "Bearer "+expectedToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}

	tokenSrv := httptest.NewServer(http.HandlerFunc(tokenHandler))
	defer tokenSrv.Close()

	contactSrv := httptest.NewServer(http.HandlerFunc(contactHandler))
	defer contactSrv.Close()

	clk := clock.NewFake()
	client, err := NewPardotClientImpl(clk, "biz-unit", "cid", "csec", tokenSrv.URL, contactSrv.URL)
	test.AssertNotError(t, err, "Failed to create client")

	// First call uses the initial token ("old_token").
	err = client.SendContact("test@example.com")
	test.AssertNotError(t, err, "SendContact should succeed with the initial token")

	// Advance time to force token expiry.
	clk.Add(3601 * time.Second)

	// Second call should refresh the token to "new_token".
	err = client.SendContact("test@example.com")
	test.AssertNotError(t, err, "SendContact should succeed after refreshing the token")
}

func TestSendContactServerErrorsAfterMaxAttempts(t *testing.T) {
	t.Parallel()

	gotAttempts := 0
	contactHandler := func(w http.ResponseWriter, r *http.Request) {
		gotAttempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	tokenSrv := httptest.NewServer(http.HandlerFunc(defaultTokenHandler))
	defer tokenSrv.Close()

	contactSrv := httptest.NewServer(http.HandlerFunc(contactHandler))
	defer contactSrv.Close()

	client, _ := NewPardotClientImpl(clock.NewFake(), "biz-unit", "cid", "csec", tokenSrv.URL, contactSrv.URL)

	err := client.SendContact("test@example.com")
	test.AssertError(t, err, "Should fail after retrying all attempts")
	test.AssertEquals(t, maxAttempts, gotAttempts)
	test.AssertContains(t, err.Error(), "create contact request returned status 503")
}

func TestSendContactRedactsEmail(t *testing.T) {
	t.Parallel()

	emailToTest := "test@example.com"

	contactHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		// Intentionally include the request email in the response body.
		resp := fmt.Sprintf("error: %s is invalid", emailToTest)
		_, err := io.WriteString(w, resp)
		test.AssertNotError(t, err, "failed to write response")
	}

	tokenSrv := httptest.NewServer(http.HandlerFunc(defaultTokenHandler))
	defer tokenSrv.Close()

	contactSrv := httptest.NewServer(http.HandlerFunc(contactHandler))
	defer contactSrv.Close()

	clk := clock.NewFake()
	client, err := NewPardotClientImpl(clk, "biz-unit", "cid", "csec", tokenSrv.URL, contactSrv.URL)
	test.AssertNotError(t, err, "failed to create client")

	err = client.SendContact(emailToTest)
	test.AssertError(t, err, "SendContact should fail")
	test.AssertNotContains(t, err.Error(), emailToTest)
	test.AssertContains(t, err.Error(), "[REDACTED]")
}
