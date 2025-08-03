package responder

import (
	"context"
	"encoding/base64"
	"os"
	"regexp"

	blog "github.com/letsencrypt/boulder/log"
	"golang.org/x/crypto/ocsp"
)

// inMemorySource wraps a map from serialNumber to Response and just looks up
// Responses from that map with no safety checks. Useful for testing.
type inMemorySource struct {
	responses map[string]*Response
	log       blog.Logger
}

// NewMemorySource returns an initialized InMemorySource which simply looks up
// responses from an in-memory map based on the serial number in the request.
func NewMemorySource(responses map[string]*Response, logger blog.Logger) (*inMemorySource, error) {
	return &inMemorySource{
		responses: responses,
		log:       logger,
	}, nil
}

// NewMemorySourceFromFile reads the named file into an InMemorySource.
// The file read by this function must contain whitespace-separated OCSP
// responses. Each OCSP response must be in base64-encoded DER form (i.e.,
// PEM without headers or whitespace).  Invalid responses are ignored.
// This function pulls the entire file into an InMemorySource.
func NewMemorySourceFromFile(responseFile string, logger blog.Logger) (*inMemorySource, error) {
	fileContents, err := os.ReadFile(responseFile)
	if err != nil {
		return nil, err
	}

	responsesB64 := regexp.MustCompile(`\s`).Split(string(fileContents), -1)
	responses := make(map[string]*Response, len(responsesB64))
	for _, b64 := range responsesB64 {
		// if the line/space is empty just skip
		if b64 == "" {
			continue
		}
		der, tmpErr := base64.StdEncoding.DecodeString(b64)
		if tmpErr != nil {
			logger.Errf("Base64 decode error %s on: %s", tmpErr, b64)
			continue
		}

		response, tmpErr := ocsp.ParseResponse(der, nil)
		if tmpErr != nil {
			logger.Errf("OCSP decode error %s on: %s", tmpErr, b64)
			continue
		}

		responses[response.SerialNumber.String()] = &Response{
			Response: response,
			Raw:      der,
		}
	}

	logger.Infof("Read %d OCSP responses", len(responses))
	return NewMemorySource(responses, logger)
}

// Response looks up an OCSP response to provide for a given request.
// InMemorySource looks up a response purely based on serial number,
// without regard to what issuer the request is asking for.
func (src inMemorySource) Response(_ context.Context, request *ocsp.Request) (*Response, error) {
	response, present := src.responses[request.SerialNumber.String()]
	if !present {
		return nil, ErrNotFound
	}
	return response, nil
}
