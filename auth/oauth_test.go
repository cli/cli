package auth

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"
	"time"
)

type roundTripper func(*http.Request) (*http.Response, error)

func (rt roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt(req)
}

func TestObtainAccessToken_deviceFlow(t *testing.T) {
	requestCount := 0
	rt := func(req *http.Request) (*http.Response, error) {
		route := fmt.Sprintf("%s %s", req.Method, req.URL)
		switch route {
		case "POST https://github.com/login/device/code":
			if err := req.ParseForm(); err != nil {
				return nil, err
			}
			if req.PostForm.Get("client_id") != "CLIENT-ID" {
				t.Errorf("expected POST /login/device/code to supply client_id=%q, got %q", "CLIENT-ID", req.PostForm.Get("client_id"))
			}
			if req.PostForm.Get("scope") != "repo gist" {
				t.Errorf("expected POST /login/device/code to supply scope=%q, got %q", "repo gist", req.PostForm.Get("scope"))
			}

			responseData := url.Values{}
			responseData.Set("device_code", "DEVICE-CODE")
			responseData.Set("user_code", "1234-ABCD")
			responseData.Set("verification_uri", "https://github.com/login/device")
			responseData.Set("interval", "5")
			responseData.Set("expires_in", "899")

			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(bytes.NewBufferString(responseData.Encode())),
				Header: http.Header{
					"Content-Type": []string{"application/x-www-form-urlencoded; charset=utf-8"},
				},
			}, nil
		case "POST https://github.com/login/oauth/access_token":
			if err := req.ParseForm(); err != nil {
				return nil, err
			}
			if req.PostForm.Get("client_id") != "CLIENT-ID" {
				t.Errorf("expected POST /login/oauth/access_token to supply client_id=%q, got %q", "CLIENT-ID", req.PostForm.Get("client_id"))
			}
			if req.PostForm.Get("device_code") != "DEVICE-CODE" {
				t.Errorf("expected POST /login/oauth/access_token to supply device_code=%q, got %q", "DEVICE-CODE", req.PostForm.Get("scope"))
			}
			if req.PostForm.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" {
				t.Errorf("expected POST /login/oauth/access_token to supply grant_type=%q, got %q", "urn:ietf:params:oauth:grant-type:device_code", req.PostForm.Get("grant_type"))
			}

			responseData := url.Values{}
			requestCount++
			if requestCount == 1 {
				responseData.Set("error", "authorization_pending")
			} else {
				responseData.Set("access_token", "OTOKEN")
			}

			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(bytes.NewBufferString(responseData.Encode())),
			}, nil
		default:
			return nil, fmt.Errorf("unstubbed HTTP request: %v", route)
		}
	}
	httpClient := &http.Client{
		Transport: roundTripper(rt),
	}

	slept := time.Duration(0)
	var browseURL string
	var browseCode string

	oa := &OAuthFlow{
		Hostname:     "github.com",
		ClientID:     "CLIENT-ID",
		ClientSecret: "CLIENT-SEKRIT",
		Scopes:       []string{"repo", "gist"},
		OpenInBrowser: func(url, code string) error {
			browseURL = url
			browseCode = code
			return nil
		},
		HTTPClient: httpClient,
		TimeNow:    time.Now,
		TimeSleep: func(d time.Duration) {
			slept += d
		},
	}

	token, err := oa.ObtainAccessToken()
	if err != nil {
		t.Fatalf("ObtainAccessToken error: %v", err)
	}

	if token != "OTOKEN" {
		t.Errorf("expected token %q, got %q", "OTOKEN", token)
	}
	if requestCount != 2 {
		t.Errorf("expected 2 HTTP pings for token, got %d", requestCount)
	}
	if slept.String() != "10s" {
		t.Errorf("expected total sleep duration of %s, got %s", "10s", slept.String())
	}
	if browseURL != "https://github.com/login/device" {
		t.Errorf("expected to open browser at %s, got %s", "https://github.com/login/device", browseURL)
	}
	if browseCode != "1234-ABCD" {
		t.Errorf("expected to provide user with one-time code %q, got %q", "1234-ABCD", browseCode)
	}
}

func Test_detectDeviceFlow(t *testing.T) {
	type args struct {
		statusCode int
		values     url.Values
	}
	tests := []struct {
		name       string
		args       args
		doFallback bool
		wantErr    string
	}{
		{
			name: "success",
			args: args{
				statusCode: 200,
				values:     url.Values{},
			},
			doFallback: false,
			wantErr:    "",
		},
		{
			name: "wrong response type",
			args: args{
				statusCode: 200,
				values:     nil,
			},
			doFallback: true,
			wantErr:    "",
		},
		{
			name: "401 unauthorized",
			args: args{
				statusCode: 401,
				values:     nil,
			},
			doFallback: true,
			wantErr:    "",
		},
		{
			name: "403 forbidden",
			args: args{
				statusCode: 403,
				values:     nil,
			},
			doFallback: true,
			wantErr:    "",
		},
		{
			name: "404 not found",
			args: args{
				statusCode: 404,
				values:     nil,
			},
			doFallback: true,
			wantErr:    "",
		},
		{
			name: "422 unprocessable",
			args: args{
				statusCode: 422,
				values:     nil,
			},
			doFallback: true,
			wantErr:    "",
		},
		{
			name: "402 payment required",
			args: args{
				statusCode: 402,
				values:     nil,
			},
			doFallback: false,
			wantErr:    "error: HTTP 402",
		},
		{
			name: "400 bad request",
			args: args{
				statusCode: 400,
				values:     nil,
			},
			doFallback: false,
			wantErr:    "error: HTTP 400",
		},
		{
			name: "400 with values",
			args: args{
				statusCode: 400,
				values: url.Values{
					"error": []string{"blah"},
				},
			},
			doFallback: false,
			wantErr:    "error: HTTP 400",
		},
		{
			name: "400 with unauthorized_client",
			args: args{
				statusCode: 400,
				values: url.Values{
					"error": []string{"unauthorized_client"},
				},
			},
			doFallback: true,
			wantErr:    "",
		},
		{
			name: "400 with error_description",
			args: args{
				statusCode: 400,
				values: url.Values{
					"error_description": []string{"HI"},
				},
			},
			doFallback: false,
			wantErr:    "HTTP 400: HI",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := detectDeviceFlow(tt.args.statusCode, tt.args.values)
			if (err != nil) != (tt.wantErr != "") {
				t.Errorf("detectDeviceFlow() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr != "" && err.Error() != tt.wantErr {
				t.Errorf("error = %q, wantErr = %q", err, tt.wantErr)
			}
			if got != tt.doFallback {
				t.Errorf("detectDeviceFlow() = %v, want %v", got, tt.doFallback)
			}
		})
	}
}
