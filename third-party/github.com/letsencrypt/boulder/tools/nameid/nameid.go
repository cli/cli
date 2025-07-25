package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/letsencrypt/boulder/issuance"
)

func usage() {
	fmt.Printf("Usage: %s [OPTIONS] [ISSUER CERTIFICATE(S)]\n", os.Args[0])
}

func main() {
	var shorthandFlag = flag.Bool("s", false, "Display only the nameid for each given issuer certificate")
	flag.Parse()

	if len(os.Args) <= 1 {
		usage()
		os.Exit(1)
	}

	for _, certFile := range flag.Args() {
		issuer, err := issuance.LoadCertificate(certFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			os.Exit(1)
		}

		if *shorthandFlag {
			fmt.Println(issuer.NameID())
		} else {
			fmt.Printf("%s: %d\n", certFile, issuer.NameID())
		}
	}
}
