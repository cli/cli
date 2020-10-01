package api

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_CacheReponse(t *testing.T) {
	counter := 0
	fakeHTTP := funcTripper{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			counter += 1
			body := fmt.Sprintf("%d: %s %s", counter, req.Method, req.URL.String())
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(bytes.NewBufferString(body)),
			}, nil
		},
	}

	cacheDir := filepath.Join(t.TempDir(), "gh-cli-cache")
	httpClient := NewHTTPClient(ReplaceTripper(fakeHTTP), CacheReponse(time.Minute, cacheDir))

	do := func(method, url string, body io.Reader) (string, error) {
		req, err := http.NewRequest(method, url, body)
		if err != nil {
			return "", err
		}
		res, err := httpClient.Do(req)
		if err != nil {
			return "", err
		}
		resBody, err := ioutil.ReadAll(res.Body)
		if err != nil {
			err = fmt.Errorf("ReadAll: %w", err)
		}
		return string(resBody), err
	}

	res1, err := do("GET", "http://example.com/path", nil)
	require.NoError(t, err)
	assert.Equal(t, "1: GET http://example.com/path", res1)
	res2, err := do("GET", "http://example.com/path", nil)
	require.NoError(t, err)
	assert.Equal(t, "1: GET http://example.com/path", res2)

	res3, err := do("GET", "http://example.com/path2", nil)
	require.NoError(t, err)
	assert.Equal(t, "2: GET http://example.com/path2", res3)

	res4, err := do("POST", "http://example.com/path", bytes.NewBufferString(`hello`))
	require.NoError(t, err)
	assert.Equal(t, "3: POST http://example.com/path", res4)
	res5, err := do("POST", "http://example.com/path", bytes.NewBufferString(`hello`))
	require.NoError(t, err)
	assert.Equal(t, "3: POST http://example.com/path", res5)

	res6, err := do("POST", "http://example.com/path", bytes.NewBufferString(`hello2`))
	require.NoError(t, err)
	assert.Equal(t, "4: POST http://example.com/path", res6)
}
