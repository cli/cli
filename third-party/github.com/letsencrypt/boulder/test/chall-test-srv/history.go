package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/letsencrypt/challtestsrv"
)

// clearHistory handles an HTTP POST request to clear the challenge server
// request history for a specific hostname and type of event.
//
// The POST body is expected to have two parameters:
// "host" - the hostname to clear history for.
// "type" - the type of event to clear. May be "http", "dns", or "tlsalpn".
//
// A successful POST will write http.StatusOK to the client.
func (srv *managementServer) clearHistory(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Host string
		Type string `json:"type"`
	}
	if err := mustParsePOST(&request, r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	typeMap := map[string]challtestsrv.RequestEventType{
		"http":    challtestsrv.HTTPRequestEventType,
		"dns":     challtestsrv.DNSRequestEventType,
		"tlsalpn": challtestsrv.TLSALPNRequestEventType,
	}
	if request.Host == "" {
		http.Error(w, "host parameter must not be empty", http.StatusBadRequest)
		return
	}
	if code, ok := typeMap[request.Type]; ok {
		srv.challSrv.ClearRequestHistory(request.Host, code)
		srv.log.Printf("Cleared challenge server request history for %q %q events\n",
			request.Host, request.Type)
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Error(w, fmt.Sprintf("%q event type unknown", request.Type), http.StatusBadRequest)
}

// getHTTPHistory returns only the HTTPRequestEvents for the given hostname
// from the challenge server's request history in JSON form.
func (srv *managementServer) getHTTPHistory(w http.ResponseWriter, r *http.Request) {
	host, err := requestHost(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	srv.writeHistory(
		srv.challSrv.RequestHistory(host, challtestsrv.HTTPRequestEventType),
		w)
}

// getDNSHistory returns only the DNSRequestEvents from the challenge
// server's request history in JSON form.
func (srv *managementServer) getDNSHistory(w http.ResponseWriter, r *http.Request) {
	host, err := requestHost(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	srv.writeHistory(
		srv.challSrv.RequestHistory(host, challtestsrv.DNSRequestEventType),
		w)
}

// getTLSALPNHistory returns only the TLSALPNRequestEvents from the challenge
// server's request history in JSON form.
func (srv *managementServer) getTLSALPNHistory(w http.ResponseWriter, r *http.Request) {
	host, err := requestHost(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	srv.writeHistory(
		srv.challSrv.RequestHistory(host, challtestsrv.TLSALPNRequestEventType),
		w)
}

// requestHost extracts the Host parameter of a JSON POST body in the provided
// request, or returns an error.
func requestHost(r *http.Request) (string, error) {
	var request struct {
		Host string
	}
	if err := mustParsePOST(&request, r); err != nil {
		return "", err
	}
	if request.Host == "" {
		return "", errors.New("host parameter of POST body must not be empty")
	}
	return request.Host, nil
}

// writeHistory writes the provided list of challtestsrv.RequestEvents to the
// provided http.ResponseWriter in JSON form.
func (srv *managementServer) writeHistory(
	history []challtestsrv.RequestEvent, w http.ResponseWriter,
) {
	// Always write an empty JSON list instead of `null`
	if history == nil {
		history = []challtestsrv.RequestEvent{}
	}
	jsonHistory, err := json.MarshalIndent(history, "", "   ")
	if err != nil {
		srv.log.Printf("Error marshaling history: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(jsonHistory)
}
