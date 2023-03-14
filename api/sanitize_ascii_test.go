package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPClientSanitizeASCIIControlCharactersC0(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issue := Issue{
			Title: "\u001B[31mRed Title\u001B[0m",
			Body:  "1\u0001 2\u0002 3\u0003 4\u0004 5\u0005 6\u0006 7\u0007 8\u0008 9\t A\r\n B\u000b C\u000c D\r\n E\u000e F\u000f",
			Author: Author{
				ID:    "1",
				Name:  "10\u0010 11\u0011 12\u0012 13\u0013 14\u0014 15\u0015 16\u0016 17\u0017 18\u0018 19\u0019 1A\u001a 1B\u001b 1C\u001c 1D\u001d 1E\u001e 1F\u001f",
				Login: "monalisa \\u00\u001b",
			},
			ActiveLockReason: "Escaped \u001B \\u001B \\\u001B \\\\u001B",
		}
		responseData, _ := json.Marshal(issue)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprint(w, string(responseData))
	}))
	defer ts.Close()

	client, err := NewHTTPClient(HTTPClientOptions{})
	require.NoError(t, err)
	req, err := http.NewRequest("GET", ts.URL, nil)
	require.NoError(t, err)
	res, err := client.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	require.NoError(t, err)
	var issue Issue
	err = json.Unmarshal(body, &issue)
	require.NoError(t, err)
	assert.Equal(t, "^[[31mRed Title^[[0m", issue.Title)
	assert.Equal(t, "1^A 2^B 3^C 4^D 5^E 6^F 7^G 8^H 9\t A\r\n B^K C^L D\r\n E^N F^O", issue.Body)
	assert.Equal(t, "10^P 11^Q 12^R 13^S 14^T 15^U 16^V 17^W 18^X 19^Y 1A^Z 1B^[ 1C^\\ 1D^] 1E^^ 1F^_", issue.Author.Name)
	assert.Equal(t, "monalisa \\u00^[", issue.Author.Login)
	assert.Equal(t, "Escaped ^[ \\^[ \\^[ \\\\^[", issue.ActiveLockReason)
}

func TestHTTPClientSanitizeASCIIControlCharactersC1(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issue := Issue{
			Title: "\xC2\x9B[31mRed Title\xC2\x9B[0m",
			Body:  "80\xC2\x80 81\xC2\x81 82\xC2\x82 83\xC2\x83 84\xC2\x84 85\xC2\x85 86\xC2\x86 87\xC2\x87 88\xC2\x88 89\xC2\x89 8A\xC2\x8A 8B\xC2\x8B 8C\xC2\x8C 8D\xC2\x8D 8E\xC2\x8E 8F\xC2\x8F",
			Author: Author{
				ID:    "1",
				Name:  "90\xC2\x90 91\xC2\x91 92\xC2\x92 93\xC2\x93 94\xC2\x94 95\xC2\x95 96\xC2\x96 97\xC2\x97 98\xC2\x98 99\xC2\x99 9A\xC2\x9A 9B\xC2\x9B 9C\xC2\x9C 9D\xC2\x9D 9E\xC2\x9E 9F\xC2\x9F",
				Login: "monalisa\xC2\xA1",
			},
		}
		responseData, _ := json.Marshal(issue)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprint(w, string(responseData))
	}))
	defer ts.Close()

	client, err := NewHTTPClient(HTTPClientOptions{})
	require.NoError(t, err)
	req, err := http.NewRequest("GET", ts.URL, nil)
	require.NoError(t, err)
	res, err := client.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	require.NoError(t, err)
	var issue Issue
	err = json.Unmarshal(body, &issue)
	require.NoError(t, err)
	assert.Equal(t, "^[[31mRed Title^[[0m", issue.Title)
	assert.Equal(t, "80^@ 81^A 82^B 83^C 84^D 85^E 86^F 87^G 88^H 89^I 8A^J 8B^K 8C^L 8D^M 8E^N 8F^O", issue.Body)
	assert.Equal(t, "90^P 91^Q 92^R 93^S 94^T 95^U 96^V 97^W 98^X 99^Y 9A^Z 9B^[ 9C^\\ 9D^] 9E^^ 9F^_", issue.Author.Name)
	assert.Equal(t, "monalisa¡", issue.Author.Login)
}

func TestSanitizedReadCloser(t *testing.T) {
	data := []byte(`the quick brown fox\njumped over the lazy dog\t`)
	rc := sanitizedReadCloser(io.NopCloser(bytes.NewReader(data)))
	assert.NoError(t, iotest.TestReader(rc, data))
}
