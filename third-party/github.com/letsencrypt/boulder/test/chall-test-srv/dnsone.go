package main

import "net/http"

// addDNS01 handles an HTTP POST request to add a new DNS-01 challenge TXT
// record for a given host/value.
//
// The POST body is expected to have two non-empty parameters:
// "host" - the hostname to add the mock TXT response under.
// "value" - the key authorization value to return in the TXT response.
//
// A successful POST will write http.StatusOK to the client.
func (srv *managementServer) addDNS01(w http.ResponseWriter, r *http.Request) {
	// Unmarshal the request body JSON as a request object
	var request struct {
		Host  string
		Value string
	}
	if err := mustParsePOST(&request, r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// If the request has an empty host or value it's a bad request
	if request.Host == "" || request.Value == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Add the DNS-01 challenge response TXT to the challenge server
	srv.challSrv.AddDNSOneChallenge(request.Host, request.Value)
	srv.log.Printf("Added DNS-01 TXT challenge for Host %q - Value %q\n",
		request.Host, request.Value)
	w.WriteHeader(http.StatusOK)
}

// delDNS01 handles an HTTP POST request to delete an existing DNS-01 challenge
// TXT record for a given host.
//
// The POST body is expected to have one non-empty parameter:
// "host" - the hostname to remove the mock TXT response for.
//
// A successful POST will write http.StatusOK to the client.
func (srv *managementServer) delDNS01(w http.ResponseWriter, r *http.Request) {
	// Unmarshal the request body JSON as a request object
	var request struct {
		Host string
	}
	if err := mustParsePOST(&request, r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// If the request has an empty host value it's a bad request
	if request.Host == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Delete the DNS-01 challenge response TXT for the given host from the
	// challenge server
	srv.challSrv.DeleteDNSOneChallenge(request.Host)
	srv.log.Printf("Removed DNS-01 TXT challenge for Host %q\n", request.Host)
	w.WriteHeader(http.StatusOK)
}
