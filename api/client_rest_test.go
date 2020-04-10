package api

import (
	"bytes"
	"testing"
)

func TestRESTGetDelete(t *testing.T) {
	http := &FakeHTTP{}

	client := NewClient(
		ReplaceTripper(http),
		AddHeader("Authorization", "Basic BASE64ENCODE_CLIENT_ID:CLIENT_SECRET"),
		AddHeader("Content-Type", "text/plain"),
	)

	http.StubResponse(204, bytes.NewBuffer([]byte{}))

	r := bytes.NewReader([]byte(`{"access_token": "ACCESS_TOKEN"}`))
	var data interface{}
	err := client.REST("DELETE", "applications/CLIENTID/grant", r, &data)
	eq(t, err, nil)
}
