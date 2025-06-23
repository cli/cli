package main

import (
	"context"
	"crypto/x509"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/revocation"
)

type s3TestSrv struct {
	sync.RWMutex
	allSerials map[string]revocation.Reason
	allShards  map[string][]byte
}

func (srv *s3TestSrv) handleS3(w http.ResponseWriter, r *http.Request) {
	if r.Method == "PUT" {
		srv.handleUpload(w, r)
	} else if r.Method == "GET" {
		srv.handleDownload(w, r)
	} else {
		w.WriteHeader(405)
	}
}

func (srv *s3TestSrv) handleUpload(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("failed to read request body"))
		return
	}

	crl, err := x509.ParseRevocationList(body)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(fmt.Sprintf("failed to parse body: %s", err)))
		return
	}

	srv.Lock()
	defer srv.Unlock()
	srv.allShards[r.URL.Path] = body
	for _, rc := range crl.RevokedCertificateEntries {
		srv.allSerials[core.SerialToString(rc.SerialNumber)] = revocation.Reason(rc.ReasonCode)
	}

	w.WriteHeader(200)
	w.Write([]byte("{}"))
}

func (srv *s3TestSrv) handleDownload(w http.ResponseWriter, r *http.Request) {
	srv.RLock()
	defer srv.RUnlock()
	body, ok := srv.allShards[r.URL.Path]
	if !ok {
		w.WriteHeader(404)
		return
	}
	w.WriteHeader(200)
	w.Write(body)
}

func (srv *s3TestSrv) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(405)
		return
	}

	serial := r.URL.Query().Get("serial")
	if serial == "" {
		w.WriteHeader(400)
		return
	}

	srv.RLock()
	defer srv.RUnlock()
	reason, ok := srv.allSerials[serial]
	if !ok {
		w.WriteHeader(404)
		return
	}

	w.WriteHeader(200)
	w.Write([]byte(fmt.Sprintf("%d", reason)))
}

func main() {
	listenAddr := flag.String("listen", "0.0.0.0:4501", "Address to listen on")
	flag.Parse()

	srv := s3TestSrv{
		allSerials: make(map[string]revocation.Reason),
		allShards:  make(map[string][]byte),
	}

	http.HandleFunc("/", srv.handleS3)
	http.HandleFunc("/query", srv.handleQuery)

	s := http.Server{
		ReadTimeout: 30 * time.Second,
		Addr:        *listenAddr,
	}

	go func() {
		err := s.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			cmd.FailOnError(err, "Running TLS server")
		}
	}()

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
	}()

	cmd.WaitForSignal()
}
