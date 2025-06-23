package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"

	"github.com/letsencrypt/boulder/test/ocsp/helper"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `
checkocsp [OPTION]... FILE [FILE]...

OCSP-checking tool. Provide a list of filenames for certificates in PEM format,
and this tool will check OCSP for each certificate based on its AIA field.
It will return an error if the OCSP server fails to respond for any request,
if any response is invalid or has a bad signature, or if any response is too
stale.

`)
		flag.PrintDefaults()
	}
	helper.RegisterFlags()
	serials := flag.Bool("serials", false, "Parameters are hex-encoded serial numbers instead of filenames. Requires --issuer-file and --url.")
	flag.Parse()
	var errors bool
	if len(flag.Args()) == 0 {
		flag.Usage()
		os.Exit(0)
	}
	config, err := helper.ConfigFromFlags()
	if err != nil {
		log.Fatal(err)
	}
	for _, a := range flag.Args() {
		var err error
		var bytes []byte
		if *serials {
			bytes, err = hex.DecodeString(strings.Replace(a, ":", "", -1))
			if err != nil {
				log.Printf("error for %s: %s\n", a, err)
			}
			serialNumber := big.NewInt(0).SetBytes(bytes)
			_, err = helper.ReqSerial(serialNumber, config)

		} else {
			_, err = helper.ReqFile(a, config)
		}
		if err != nil {
			log.Printf("error for %s: %s\n", a, err)
			errors = true
		}
	}
	if errors {
		os.Exit(1)
	}
}
