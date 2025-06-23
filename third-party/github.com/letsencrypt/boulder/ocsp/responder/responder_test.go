/*
This code was originally forked from https://github.com/cloudflare/cfssl/blob/1a911ca1b1d6e899bf97dcfa4a14b38db0d31134/ocsp/responder_test.go

Copyright (c) 2014 CloudFlare Inc.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions
are met:

Redistributions of source code must retain the above copyright notice,
this list of conditions and the following disclaimer.

Redistributions in binary form must reproduce the above copyright notice,
this list of conditions and the following disclaimer in the documentation
and/or other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED
TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR
PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF
LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING
NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

package responder

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ocsp"

	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/test"
)

const (
	responseFile       = "testdata/resp64.pem"
	binResponseFile    = "testdata/response.der"
	brokenResponseFile = "testdata/response_broken.pem"
	mixResponseFile    = "testdata/response_mix.pem"
)

type testSource struct{}

func (ts testSource) Response(_ context.Context, r *ocsp.Request) (*Response, error) {
	respBytes, err := hex.DecodeString("3082031D0A0100A08203163082031206092B060105050730010104820303308202FF3081E8A1453043310B300906035504061302555331123010060355040A1309676F6F6420677579733120301E06035504031317434120696E7465726D6564696174652028525341292041180F32303230303631393030333730305A30818D30818A304C300906052B0E03021A0500041417779CF67D84CD4449A2FC7EAC431F9823D8575A04149F2970E80CF9C75ECC1F2871D8C390CD19F40108021300FF8B2AEC5293C6B31D0BC0BA329CF594E7BAA116180F32303230303631393030333733305AA0030A0101180F32303230303631393030303030305AA011180F32303230303632333030303030305A300D06092A864886F70D01010B0500038202010011688303203098FC522D2C599A234B136930E3C4680F2F3192188B98D6EE90E8479449968C51335FADD1636584ACEA9D01A30790BD90190FA35A47E793718128B19E9ED156382C1B68245A6887F547B0B86C44C2354B8DBA94D8BFCAA768EB55FA84AEB4026DBEFC687DB280D21C0B3497A11909804A20F402BDD95E4843C02E30435C2570FFC4EB152FE2785B8D268AC996619644AEC9CF50959D46DEB21DFE96B4D2881D61ABBCA9B6BFEC2DB9132801CAE737C862F0AEAB4948B63F35740CE93FCDBC148F5070790D7BBA1A87E15078CD8335F83686142CE8AC3AD21FAE45B87A7B12562D9F245352A83E3901E97E5EC77E9817990712D8BE60860ABA58804DDE4ECDCA6AEFD3D8764FDBABF0AB1902FA9A7C4C3F5814C25C5E78E0754469E087CAED81E50A5873CADFCAC42963AB38CFD11096BE4201DE4589B57EC48B3DA05A65800D654160E022F6748CD93B431A17270C1B27E313734FCF85F22547D060F23F594BD68C6330C2705190A04905FBD2389E2DD21C0188809E03D713F56BF95953C9897DA6D4D074D70F164270C41BFB386B69E86EB3B9192FEA8F43CE5368CC9AF8687DEE567672A8580BA6A9F76E6E6705DD2F76F48C2C180C763CF4C48AF78C25D40EA7278CB2FBC78958B3179301825B420A7CAE7ACE4C41B5BA7D567AABC9C2701EE75A28F9181E044EDAAA55A31538AA9C526D4C324B9AE58D2922")
	if err != nil {
		return nil, err
	}
	resp, err := ocsp.ParseResponse(respBytes, nil)
	if err != nil {
		return nil, err
	}
	return &Response{resp, respBytes}, nil
}

type expiredSource struct{}

func (es expiredSource) Response(_ context.Context, r *ocsp.Request) (*Response, error) {
	return nil, errOCSPResponseExpired
}

type testCase struct {
	method, path string
	expected     int
}

func TestResponseExpired(t *testing.T) {
	cases := []testCase{
		{"GET", "/MFQwUjBQME4wTDAJBgUrDgMCGgUABBQ55F6w46hhx%2Fo6OXOHa%2BYfe32YhgQU%2B3hPEvlgFYMsnxd%2FNBmzLjbqQYkCEwD6Wh0MaVKu9gJ3By9DI%2F%2Fxsd4%3D", 533},
	}

	responder := Responder{
		Source: expiredSource{},
		responseTypes: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "ocspResponses-test",
			},
			[]string{"type"},
		),
		clk: clock.NewFake(),
		log: blog.NewMock(),
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s %s", tc.method, tc.path), func(t *testing.T) {
			rw := httptest.NewRecorder()
			responder.responseTypes.Reset()

			responder.ServeHTTP(rw, &http.Request{
				Method: tc.method,
				URL: &url.URL{
					Path: tc.path,
				},
			})
			if rw.Code != tc.expected {
				t.Errorf("Incorrect response code: got %d, wanted %d", rw.Code, tc.expected)
			}
			test.AssertByteEquals(t, ocsp.InternalErrorErrorResponse, rw.Body.Bytes())
		})
	}
}

func TestOCSP(t *testing.T) {
	cases := []testCase{
		{"OPTIONS", "/", http.StatusMethodNotAllowed},
		{"GET", "/", http.StatusBadRequest},
		// Bad URL encoding
		{"GET", "%ZZFQwUjBQME4wTDAJBgUrDgMCGgUABBQ55F6w46hhx%2Fo6OXOHa%2BYfe32YhgQU%2B3hPEvlgFYMsnxd%2FNBmzLjbqQYkCEwD6Wh0MaVKu9gJ3By9DI%2F%2Fxsd4%3D", http.StatusBadRequest},
		// Bad URL encoding
		{"GET", "%%FQwUjBQME4wTDAJBgUrDgMCGgUABBQ55F6w46hhx%2Fo6OXOHa%2BYfe32YhgQU%2B3hPEvlgFYMsnxd%2FNBmzLjbqQYkCEwD6Wh0MaVKu9gJ3By9DI%2F%2Fxsd4%3D", http.StatusBadRequest},
		// Bad base64 encoding
		{"GET", "==MFQwUjBQME4wTDAJBgUrDgMCGgUABBQ55F6w46hhx%2Fo6OXOHa%2BYfe32YhgQU%2B3hPEvlgFYMsnxd%2FNBmzLjbqQYkCEwD6Wh0MaVKu9gJ3By9DI%2F%2Fxsd4%3D", http.StatusBadRequest},
		// Bad OCSP DER encoding
		{"GET", "AAAMFQwUjBQME4wTDAJBgUrDgMCGgUABBQ55F6w46hhx%2Fo6OXOHa%2BYfe32YhgQU%2B3hPEvlgFYMsnxd%2FNBmzLjbqQYkCEwD6Wh0MaVKu9gJ3By9DI%2F%2Fxsd4%3D", http.StatusBadRequest},
		// Good encoding all around, including a double slash
		{"GET", "MFQwUjBQME4wTDAJBgUrDgMCGgUABBQ55F6w46hhx%2Fo6OXOHa%2BYfe32YhgQU%2B3hPEvlgFYMsnxd%2FNBmzLjbqQYkCEwD6Wh0MaVKu9gJ3By9DI%2F%2Fxsd4%3D", http.StatusOK},
		// Good request, leading slash
		{"GET", "/MFQwUjBQME4wTDAJBgUrDgMCGgUABBQ55F6w46hhx%2Fo6OXOHa%2BYfe32YhgQU%2B3hPEvlgFYMsnxd%2FNBmzLjbqQYkCEwD6Wh0MaVKu9gJ3By9DI%2F%2Fxsd4%3D", http.StatusOK},
	}

	responder := Responder{
		Source: testSource{},
		responseTypes: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "ocspResponses-test",
			},
			[]string{"type"},
		),
		responseAges: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "ocspAges-test",
				Buckets: []float64{43200},
			},
		),
		clk: clock.NewFake(),
		log: blog.NewMock(),
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s %s", tc.method, tc.path), func(t *testing.T) {
			rw := httptest.NewRecorder()
			responder.responseTypes.Reset()

			responder.ServeHTTP(rw, &http.Request{
				Method: tc.method,
				URL: &url.URL{
					Path: tc.path,
				},
			})
			if rw.Code != tc.expected {
				t.Errorf("Incorrect response code: got %d, wanted %d", rw.Code, tc.expected)
			}
			if rw.Code == http.StatusOK {
				test.AssertMetricWithLabelsEquals(
					t, responder.responseTypes, prometheus.Labels{"type": "Success"}, 1)
			} else if rw.Code == http.StatusBadRequest {
				test.AssertMetricWithLabelsEquals(
					t, responder.responseTypes, prometheus.Labels{"type": "Malformed"}, 1)
			}
		})
	}
	// Exactly two of the cases above result in an OCSP response being sent.
	test.AssertMetricWithLabelsEquals(t, responder.responseAges, prometheus.Labels{}, 2)
}

func TestRequestTooBig(t *testing.T) {
	responder := Responder{
		Source: testSource{},
		responseTypes: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "ocspResponses-test",
			},
			[]string{"type"},
		),
		responseAges: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "ocspAges-test",
				Buckets: []float64{43200},
			},
		),
		clk: clock.NewFake(),
		log: blog.NewMock(),
	}

	rw := httptest.NewRecorder()

	responder.ServeHTTP(rw, httptest.NewRequest("POST", "/",
		bytes.NewBuffer([]byte(strings.Repeat("a", 10001)))))
	expected := 400
	if rw.Code != expected {
		t.Errorf("Incorrect response code: got %d, wanted %d", rw.Code, expected)
	}
}

func TestCacheHeaders(t *testing.T) {
	source, err := NewMemorySourceFromFile(responseFile, blog.NewMock())
	if err != nil {
		t.Fatalf("Error constructing source: %s", err)
	}

	fc := clock.NewFake()
	fc.Set(time.Date(2015, 11, 12, 0, 0, 0, 0, time.UTC))
	responder := Responder{
		Source: source,
		responseTypes: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "ocspResponses-test",
			},
			[]string{"type"},
		),
		responseAges: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "ocspAges-test",
				Buckets: []float64{43200},
			},
		),
		clk: fc,
		log: blog.NewMock(),
	}

	rw := httptest.NewRecorder()
	responder.ServeHTTP(rw, &http.Request{
		Method: "GET",
		URL: &url.URL{
			Path: "MEMwQTA/MD0wOzAJBgUrDgMCGgUABBSwLsMRhyg1dJUwnXWk++D57lvgagQU6aQ/7p6l5vLV13lgPJOmLiSOl6oCAhJN",
		},
	})
	if rw.Code != http.StatusOK {
		t.Errorf("Unexpected HTTP status code %d", rw.Code)
	}
	testCases := []struct {
		header string
		value  string
	}{
		{"Last-Modified", "Tue, 20 Oct 2015 00:00:00 UTC"},
		{"Expires", "Sun, 20 Oct 2030 00:00:00 UTC"},
		{"Cache-Control", "max-age=471398400, public, no-transform, must-revalidate"},
		{"Etag", "\"8169FB0843B081A76E9F6F13FD70C8411597BEACF8B182136FFDD19FBD26140A\""},
	}
	for _, tc := range testCases {
		headers, ok := rw.Result().Header[tc.header]
		if !ok {
			t.Errorf("Header %s missing from HTTP response", tc.header)
			continue
		}
		if len(headers) != 1 {
			t.Errorf("Wrong number of headers in HTTP response. Wanted 1, got %d", len(headers))
			continue
		}
		actual := headers[0]
		if actual != tc.value {
			t.Errorf("Got header %s: %s. Expected %s", tc.header, actual, tc.value)
		}
	}

	rw = httptest.NewRecorder()
	headers := http.Header{}
	headers.Add("If-None-Match", "\"8169FB0843B081A76E9F6F13FD70C8411597BEACF8B182136FFDD19FBD26140A\"")
	responder.ServeHTTP(rw, &http.Request{
		Method: "GET",
		URL: &url.URL{
			Path: "MEMwQTA/MD0wOzAJBgUrDgMCGgUABBSwLsMRhyg1dJUwnXWk++D57lvgagQU6aQ/7p6l5vLV13lgPJOmLiSOl6oCAhJN",
		},
		Header: headers,
	})
	if rw.Code != http.StatusNotModified {
		t.Fatalf("Got wrong status code: expected %d, got %d", http.StatusNotModified, rw.Code)
	}
}

func TestNewSourceFromFile(t *testing.T) {
	logger := blog.NewMock()
	_, err := NewMemorySourceFromFile("", logger)
	if err == nil {
		t.Fatal("Didn't fail on non-file input")
	}

	// expected case
	_, err = NewMemorySourceFromFile(responseFile, logger)
	if err != nil {
		t.Fatal(err)
	}

	// binary-formatted file
	_, err = NewMemorySourceFromFile(binResponseFile, logger)
	if err != nil {
		t.Fatal(err)
	}

	// the response file from before, with stuff deleted
	_, err = NewMemorySourceFromFile(brokenResponseFile, logger)
	if err != nil {
		t.Fatal(err)
	}

	// mix of a correct and malformed responses
	_, err = NewMemorySourceFromFile(mixResponseFile, logger)
	if err != nil {
		t.Fatal(err)
	}
}
