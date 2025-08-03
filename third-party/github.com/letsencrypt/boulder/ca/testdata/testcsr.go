// Hack up the x509.CertificateRequest in here, run `go run testcsr.go`, and a
// DER-encoded CertificateRequest will be printed to stdout.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"log"
	"os"
)

func main() {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("Failed to parse private key: %s", err)
	}

	req := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: "CapiTalizedLetters.com",
		},
		DNSNames: []string{
			"moreCAPs.com",
			"morecaps.com",
			"evenMOREcaps.com",
			"Capitalizedletters.COM",
		},
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, req, priv)
	if err != nil {
		log.Fatalf("unable to create CSR: %s", err)
	}
	_, err = os.Stdout.Write(csr)
	if err != nil {
		log.Fatalf("unable to write to stdout: %s", err)
	}
}
