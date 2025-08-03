package notmain

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/crypto/ocsp"

	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/ocsp/responder"
	"github.com/letsencrypt/boulder/test"
)

func TestMux(t *testing.T) {
	reqBytes, err := os.ReadFile("./testdata/ocsp.req")
	test.AssertNotError(t, err, "failed to read OCSP request")
	req, err := ocsp.ParseRequest(reqBytes)
	test.AssertNotError(t, err, "failed to parse OCSP request")

	doubleSlashBytes, err := base64.StdEncoding.DecodeString("MFMwUTBPME0wSzAJBgUrDgMCGgUABBR+5mrncpqz/PiiIGRsFqEtYHEIXQQUqEpqYwR93brm0Tm3pkVl7/Oo7KECEgO/AC2R1FW8hePAj4xp//8Jhw==")
	test.AssertNotError(t, err, "failed to decode double slash OCSP request")
	doubleSlashReq, err := ocsp.ParseRequest(doubleSlashBytes)
	test.AssertNotError(t, err, "failed to parse double slash OCSP request")

	respBytes, err := os.ReadFile("./testdata/ocsp.resp")
	test.AssertNotError(t, err, "failed to read OCSP response")
	resp, err := ocsp.ParseResponse(respBytes, nil)
	test.AssertNotError(t, err, "failed to parse OCSP response")

	responses := map[string]*responder.Response{
		req.SerialNumber.String():            {Response: resp, Raw: respBytes},
		doubleSlashReq.SerialNumber.String(): {Response: resp, Raw: respBytes},
	}
	src, err := responder.NewMemorySource(responses, blog.NewMock())
	test.AssertNotError(t, err, "failed to create inMemorySource")

	h := mux("/foobar/", src, time.Second, metrics.NoopRegisterer, []otelhttp.Option{}, blog.NewMock(), 1000)

	type muxTest struct {
		method   string
		path     string
		reqBody  []byte
		respBody []byte
	}
	mts := []muxTest{
		{"POST", "/foobar/", reqBytes, respBytes},
		{"GET", "/", nil, nil},
		{"GET", "/foobar/MFMwUTBPME0wSzAJBgUrDgMCGgUABBR+5mrncpqz/PiiIGRsFqEtYHEIXQQUqEpqYwR93brm0Tm3pkVl7/Oo7KECEgO/AC2R1FW8hePAj4xp//8Jhw==", nil, respBytes},
	}
	for i, mt := range mts {
		w := httptest.NewRecorder()
		r, err := http.NewRequest(mt.method, mt.path, bytes.NewReader(mt.reqBody))
		if err != nil {
			t.Fatalf("#%d, NewRequest: %s", i, err)
		}
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("Code: want %d, got %d", http.StatusOK, w.Code)
		}
		if !bytes.Equal(w.Body.Bytes(), mt.respBody) {
			t.Errorf("Mismatched body: want %#v, got %#v", mt.respBody, w.Body.Bytes())
		}
	}
}
