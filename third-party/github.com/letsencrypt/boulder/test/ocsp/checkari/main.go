package main

import (
	"crypto"
	_ "crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"

	"github.com/letsencrypt/boulder/core"
)

// certID matches the ASN.1 structure of the CertID sequence defined by RFC6960.
type certID struct {
	HashAlgorithm  pkix.AlgorithmIdentifier
	IssuerNameHash []byte
	IssuerKeyHash  []byte
	SerialNumber   *big.Int
}

func createRequest(cert *x509.Certificate) ([]byte, error) {
	if !crypto.SHA256.Available() {
		return nil, x509.ErrUnsupportedAlgorithm
	}
	h := crypto.SHA256.New()

	h.Write(cert.RawIssuer)
	issuerNameHash := h.Sum(nil)

	req := certID{
		pkix.AlgorithmIdentifier{ // SHA256
			Algorithm:  asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1},
			Parameters: asn1.RawValue{Tag: 5 /* ASN.1 NULL */},
		},
		issuerNameHash,
		cert.AuthorityKeyId,
		cert.SerialNumber,
	}

	return asn1.Marshal(req)
}

func parseResponse(resp *http.Response) (*core.RenewalInfo, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var res core.RenewalInfo
	err = json.Unmarshal(body, &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

func checkARI(baseURL string, certPath string) (*core.RenewalInfo, error) {
	cert, err := core.LoadCert(certPath)
	if err != nil {
		return nil, err
	}

	req, err := createRequest(cert)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s", baseURL, base64.RawURLEncoding.EncodeToString(req))
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	ri, err := parseResponse(resp)
	if err != nil {
		return nil, err
	}

	return ri, nil
}

func getARIURL(directory string) (string, error) {
	resp, err := http.Get(directory)
	if err != nil {
		return "", err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var dir struct {
		RenewalInfo string `json:"renewalInfo"`
	}
	err = json.Unmarshal(body, &dir)
	if err != nil {
		return "", err
	}

	return dir.RenewalInfo, nil
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `
checkari [-url https://acme.api/directory] FILE [FILE]...

Tool for querying ARI. Provide a list of filenames for certificates in PEM
format, and this tool will query for and output the suggested renewal window
for each certificate.

`)
		flag.PrintDefaults()
	}
	directory := flag.String("url", "https://acme-v02.api.letsencrypt.org/directory", "ACME server's Directory URL")
	flag.Parse()
	if len(flag.Args()) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	ariPath, err := getARIURL(*directory)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	for _, cert := range flag.Args() {
		fmt.Printf("%s:\n", cert)
		window, err := checkARI(ariPath, cert)
		if err != nil {
			fmt.Printf("\t%s\n", err)
		} else {
			fmt.Printf("\tRenew after : %s\n", window.SuggestedWindow.Start)
			fmt.Printf("\tRenew before: %s\n", window.SuggestedWindow.End)
		}
	}
}
