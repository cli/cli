package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// mustParsePOST will attempt to read a JSON POST body from the provided request
// and unmarshal it into the provided ob. If an error occurs at any point it
// will be returned.
func mustParsePOST(ob interface{}, request *http.Request) error {
	jsonBody, err := io.ReadAll(request.Body)
	if err != nil {
		return err
	}

	if string(jsonBody) == "" {
		return errors.New("Expected JSON POST body, was empty")
	}

	return json.Unmarshal(jsonBody, ob)
}
