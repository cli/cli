package main

import "net/http"

// addHTTP01 handles an HTTP POST request to add a new HTTP-01 challenge
// response for a given token.
//
// The POST body is expected to have two non-empty parameters:
// "token" - the HTTP-01 challenge token to add the mock HTTP-01 response under
// in the `/.well-known/acme-challenge/` path.
//
// "content" - the key authorization value to return in the HTTP response.
//
// A successful POST will write http.StatusOK to the client.
func (srv *managementServer) addHTTP01(w http.ResponseWriter, r *http.Request) {
	// Unmarshal the request body JSON as a request object
	var request struct {
		Token   string
		Content string
	}
	if err := mustParsePOST(&request, r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// If the request has an empty token or content it's a bad request
	if request.Token == "" || request.Content == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Add the HTTP-01 challenge to the challenge server
	srv.challSrv.AddHTTPOneChallenge(request.Token, request.Content)
	srv.log.Printf("Added HTTP-01 challenge for token %q - key auth %q\n",
		request.Token, request.Content)
	w.WriteHeader(http.StatusOK)
}

// delHTTP01 handles an HTTP POST request to delete an existing HTTP-01
// challenge response for a given token.
//
// The POST body is expected to have one non-empty parameter:
// "token" - the HTTP-01 challenge token to remove the mock HTTP-01 response
// from.
//
// A successful POST will write http.StatusOK to the client.
func (srv *managementServer) delHTTP01(w http.ResponseWriter, r *http.Request) {
	// Unmarshal the request body JSON as a request object
	var request struct {
		Token string
	}
	if err := mustParsePOST(&request, r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// If the request has an empty token it's a bad request
	if request.Token == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Delete the HTTP-01 challenge for the given token from the challenge server
	srv.challSrv.DeleteHTTPOneChallenge(request.Token)
	srv.log.Printf("Removed HTTP-01 challenge for token %q\n", request.Token)
	w.WriteHeader(http.StatusOK)
}

// addHTTPRedirect handles an HTTP POST request to add a new 301 redirect to be
// served for the given path to the given target URL.
//
// The POST body is expected to have two non-empty parameters:
// "path" - the path that when matched in an HTTP request will return the
// redirect.
//
// "targetURL" - the URL that the client will be redirected to when making HTTP
// requests for the redirected path.
//
// A successful POST will write http.StatusOK to the client.
func (srv *managementServer) addHTTPRedirect(w http.ResponseWriter, r *http.Request) {
	// Unmarshal the request body JSON as a request object
	var request struct {
		Path      string
		TargetURL string
	}
	if err := mustParsePOST(&request, r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// If the request has an empty path or target URL it's a bad request
	if request.Path == "" || request.TargetURL == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Add the HTTP redirect to the challenge server
	srv.challSrv.AddHTTPRedirect(request.Path, request.TargetURL)
	srv.log.Printf("Added HTTP redirect for path %q to %q\n",
		request.Path, request.TargetURL)
	w.WriteHeader(http.StatusOK)
}

// delHTTPRedirect handles an HTTP POST request to delete an existing HTTP
// redirect for a given path.
//
// The POST body is expected to have one non-empty parameter:
// "path" - the path to remove a redirect for.
//
// A successful POST will write http.StatusOK to the client.
func (srv *managementServer) delHTTPRedirect(w http.ResponseWriter, r *http.Request) {
	// Unmarshal the request body JSON as a request object
	var request struct {
		Path string
	}
	if err := mustParsePOST(&request, r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if request.Path == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Delete the HTTP redirect for the given path from the challenge server
	srv.challSrv.DeleteHTTPRedirect(request.Path)
	srv.log.Printf("Removed HTTP redirect for path %q\n", request.Path)
	w.WriteHeader(http.StatusOK)
}
