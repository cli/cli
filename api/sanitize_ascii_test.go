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

func TestHTTPClient_SanitizeASCIIControlCharacters(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issue := Issue{
			Title: "\u001B[31mRed Title\u001B[0m",
			Body:  "1\u0001 2\u0002 3\u0003 4\u0004 5\u0005 6\u0006 7\u0007 8\u0008 9\t A\r\n B\u000b C\u000c D\r\n E\u000e F\u000f",
			Author: Author{
				ID:    "1",
				Name:  "10\u0010 11\u0011 12\u0012 13\u0013 14\u0014 15\u0015 16\u0016 17\u0017 18\u0018 19\u0019 1A\u001a 1B\u001b 1C\u001c 1D\u001d 1E\u001e 1F\u001f",
				Login: "monalisa",
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
	assert.Equal(t, "monalisa", issue.Author.Login)
	assert.Equal(t, "Escaped ^[ \\^[ \\^[ \\\\^[", issue.ActiveLockReason)
}

func TestSanitizeASCIIReadCloser(t *testing.T) {
	data := []byte(`"Assign},"L`)
	var r io.Reader = bytes.NewReader(data)
	r = &sanitizeASCIIReadCloser{ReadCloser: io.NopCloser(r)}
	r = iotest.OneByteReader(r)
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, data, out)
}
