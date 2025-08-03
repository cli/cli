package main

import (
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/letsencrypt/boulder/cmd"
)

var contactsCap = 20

type config struct {
	// OAuthAddr is the address (e.g. IP:port) on which the OAuth server will
	// listen.
	OAuthAddr string

	// PardotAddr is the address (e.g. IP:port) on which the Pardot server will
	// listen.
	PardotAddr string

	// ExpectedClientID is the client ID that the server expects to receive in
	// requests to the /services/oauth2/token endpoint.
	ExpectedClientID string `validate:"required"`

	// ExpectedClientSecret is the client secret that the server expects to
	// receive in requests to the /services/oauth2/token endpoint.
	ExpectedClientSecret string `validate:"required"`
}

type contacts struct {
	sync.Mutex
	created []string
}

type testServer struct {
	expectedClientID     string
	expectedClientSecret string
	token                string
	contacts             contacts
}

func (ts *testServer) getTokenHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")

	if clientID != ts.expectedClientID || clientSecret != ts.expectedClientSecret {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	response := map[string]interface{}{
		"access_token": ts.token,
		"token_type":   "Bearer",
		"expires_in":   3600,
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		log.Printf("Failed to encode token response: %v", err)
		http.Error(w, "Failed to encode token response", http.StatusInternalServerError)
	}
}

func (ts *testServer) checkToken(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")
	if token != "Bearer "+ts.token {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
}

func (ts *testServer) createContactsHandler(w http.ResponseWriter, r *http.Request) {
	ts.checkToken(w, r)

	businessUnitId := r.Header.Get("Pardot-Business-Unit-Id")
	if businessUnitId == "" {
		http.Error(w, "Missing 'Pardot-Business-Unit-Id' header", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}

	type contactData struct {
		Email string `json:"email"`
	}

	var contact contactData
	err = json.Unmarshal(body, &contact)
	if err != nil {
		http.Error(w, "Failed to parse request body", http.StatusBadRequest)
		return
	}

	if contact.Email == "" {
		http.Error(w, "Missing 'email' field in request body", http.StatusBadRequest)
		return
	}

	ts.contacts.Lock()
	if len(ts.contacts.created) >= contactsCap {
		// Copying the slice in memory is inefficient, but this is a test server
		// with a small number of contacts, so it's fine.
		ts.contacts.created = ts.contacts.created[1:]
	}
	ts.contacts.created = append(ts.contacts.created, contact.Email)
	ts.contacts.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "success"}`))
}

func (ts *testServer) queryContactsHandler(w http.ResponseWriter, r *http.Request) {
	ts.checkToken(w, r)

	ts.contacts.Lock()
	respContacts := slices.Clone(ts.contacts.created)
	ts.contacts.Unlock()

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(map[string]interface{}{"contacts": respContacts})
	if err != nil {
		log.Printf("Failed to encode contacts query response: %v", err)
		http.Error(w, "Failed to encode contacts query response", http.StatusInternalServerError)
	}
}

func main() {
	oauthAddr := flag.String("oauth-addr", "", "OAuth server listen address override")
	pardotAddr := flag.String("pardot-addr", "", "Pardot server listen address override")
	configFile := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	if *configFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	var c config
	err := cmd.ReadConfigFile(*configFile, &c)
	cmd.FailOnError(err, "Reading JSON config file into config structure")

	if *oauthAddr != "" {
		c.OAuthAddr = *oauthAddr
	}
	if *pardotAddr != "" {
		c.PardotAddr = *pardotAddr
	}

	tokenBytes := make([]byte, 32)
	_, err = rand.Read(tokenBytes)
	if err != nil {
		log.Fatalf("Failed to generate token: %v", err)
	}

	ts := &testServer{
		expectedClientID:     c.ExpectedClientID,
		expectedClientSecret: c.ExpectedClientSecret,
		token:                fmt.Sprintf("%x", tokenBytes),
		contacts:             contacts{created: make([]string, 0, contactsCap)},
	}

	// OAuth Server
	oauthMux := http.NewServeMux()
	oauthMux.HandleFunc("/services/oauth2/token", ts.getTokenHandler)
	oauthServer := &http.Server{
		Addr:        c.OAuthAddr,
		Handler:     oauthMux,
		ReadTimeout: 30 * time.Second,
	}

	log.Printf("pardot-test-srv OAuth server listening at %s", c.OAuthAddr)
	go func() {
		err := oauthServer.ListenAndServe()
		if err != nil {
			log.Fatalf("Failed to start OAuth server: %s", err)
		}
	}()

	// Pardot API Server
	pardotMux := http.NewServeMux()
	pardotMux.HandleFunc("/api/v5/objects/prospects", ts.createContactsHandler)
	pardotMux.HandleFunc("/contacts", ts.queryContactsHandler)

	pardotServer := &http.Server{
		Addr:        c.PardotAddr,
		Handler:     pardotMux,
		ReadTimeout: 30 * time.Second,
	}
	log.Printf("pardot-test-srv Pardot API server listening at %s", c.PardotAddr)
	go func() {
		err := pardotServer.ListenAndServe()
		if err != nil {
			log.Fatalf("Failed to start Pardot API server: %s", err)
		}
	}()

	cmd.WaitForSignal()
}
