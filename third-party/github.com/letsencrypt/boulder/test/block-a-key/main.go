// block-a-key is a small utility for creating key blocklist entries.
package main

import (
	"crypto"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/web"
)

const usageHelp = `
block-a-key is utility tool for generating a SHA256 hash of the SubjectPublicKeyInfo
from a certificate or a synthetic SubjectPublicKeyInfo generated from a JWK public key.
It outputs the Base64 encoding of that hash.

The produced encoded digest can be used with Boulder's key blocklist to block
any ACME account creation or certificate requests that use the same public
key.

If you already have an SPKI hash, and it's a SHA256 hash, you can add it directly
to the key blocklist. If it's in hex form you'll need to convert it to base64 first.

installation:
  go install github.com/letsencrypt/boulder/test/block-a-key/...

usage:
  block-a-key -cert <PEM encoded x509 certificate file>
  block-a-key -jwk <JSON encoded JWK file>

output format:
  # <filepath>
  - "<base64 encoded SHA256 subject public key digest>"

examples:
  $> block-a-key -jwk ./test/block-a-key/test/test.ecdsa.jwk.json
  ./test/block-a-key/test/test.ecdsa.jwk.json	cuwGhNNI6nfob5aqY90e7BleU6l7rfxku4X3UTJ3Z7M=
  $> block-a-key -cert ./test/block-a-key/test/test.rsa.cert.pem
  ./test/block-a-key/test/test.rsa.cert.pem	Qebc1V3SkX3izkYRGNJilm9Bcuvf0oox4U2Rn+b4JOE=
`

// keyFromCert returns the public key from a PEM encoded certificate located in
// pemFile or returns an error.
func keyFromCert(pemFile string) (crypto.PublicKey, error) {
	c, err := core.LoadCert(pemFile)
	if err != nil {
		return nil, err
	}
	return c.PublicKey, nil
}

// keyFromJWK returns the public key from a JSON encoded JOSE JWK located in
// jsonFile or returns an error.
func keyFromJWK(jsonFile string) (crypto.PublicKey, error) {
	jwk, err := web.LoadJWK(jsonFile)
	if err != nil {
		return nil, err
	}
	return jwk.Key, nil
}

func main() {
	certFileArg := flag.String("cert", "", "path to a PEM encoded X509 certificate file")
	jwkFileArg := flag.String("jwk", "", "path to a JSON encoded JWK file")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s\n\n", usageHelp)
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if *certFileArg == "" && *jwkFileArg == "" {
		log.Fatalf("error: a -cert or -jwk argument must be provided")
	}

	if *certFileArg != "" && *jwkFileArg != "" {
		log.Fatalf("error: -cert and -jwk arguments are mutually exclusive")
	}

	var file string
	var key crypto.PublicKey
	var err error

	if *certFileArg != "" {
		file = *certFileArg
		key, err = keyFromCert(file)
	} else if *jwkFileArg != "" {
		file = *jwkFileArg
		key, err = keyFromJWK(file)
	} else {
		err = errors.New("unexpected command line state")
	}
	if err != nil {
		log.Fatalf("error loading public key: %v", err)
	}

	spkiHash, err := core.KeyDigestB64(key)
	if err != nil {
		log.Fatalf("error computing spki hash: %v", err)
	}
	fmt.Printf("  # %s\n  - %s\n", file, spkiHash)
}
