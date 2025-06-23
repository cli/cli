package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/issuance"
)

type aiaTestSrv struct {
	issuersByName map[string]*issuance.Certificate
}

func (srv *aiaTestSrv) handleIssuer(w http.ResponseWriter, r *http.Request) {
	issuerName, err := url.PathUnescape(r.URL.Path[1:])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	issuerName = strings.ReplaceAll(issuerName, "-", " ")

	issuer, ok := srv.issuersByName[issuerName]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(fmt.Sprintf("issuer %q not found", issuerName)))
		return
	}

	w.Header().Set("Content-Type", "application/pkix-cert")
	w.WriteHeader(http.StatusOK)
	w.Write(issuer.Certificate.Raw)
}

// This regex excludes the "...-cross.cert.pem" files, since we don't serve our
// cross-signed certs at AIA URLs.
var issuerCertRegex = regexp.MustCompile(`int-(rsa|ecdsa)-[a-z]\.cert\.pem$`)

func main() {
	listenAddr := flag.String("addr", "", "Address to listen on")
	hierarchyDir := flag.String("hierarchy", "", "Directory to load certs from")
	flag.Parse()

	files, err := os.ReadDir(*hierarchyDir)
	cmd.FailOnError(err, "opening hierarchy directory")

	byName := make(map[string]*issuance.Certificate)
	for _, file := range files {
		if issuerCertRegex.Match([]byte(file.Name())) {
			cert, err := issuance.LoadCertificate(path.Join(*hierarchyDir, file.Name()))
			cmd.FailOnError(err, "loading issuer certificate")

			name := cert.Certificate.Subject.CommonName
			if _, found := byName[name]; found {
				cmd.FailOnError(fmt.Errorf("loaded two certs with CN %q", name), "")
			}
			byName[name] = cert
		}
	}

	srv := aiaTestSrv{
		issuersByName: byName,
	}

	http.HandleFunc("/", srv.handleIssuer)

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
