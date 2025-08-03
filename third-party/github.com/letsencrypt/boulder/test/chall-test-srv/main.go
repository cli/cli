package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/letsencrypt/challtestsrv"

	"github.com/letsencrypt/boulder/cmd"
)

// managementServer is a small HTTP server that can control a challenge server,
// adding and deleting challenge responses as required
type managementServer struct {
	// A managementServer is a http.Server
	*http.Server
	log *log.Logger
	// The challenge server that is under control by the management server
	challSrv *challtestsrv.ChallSrv
}

func (srv *managementServer) Run() {
	srv.log.Printf("Starting management server on %s", srv.Server.Addr)
	// Start the HTTP server in its own dedicated Go routine
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !strings.Contains(err.Error(), "Server closed") {
			srv.log.Print(err)
		}
	}()
}

func (srv *managementServer) Shutdown() {
	if err := srv.Server.Shutdown(context.Background()); err != nil {
		srv.log.Printf("Err shutting down management server")
	}
}

func filterEmpty(input []string) []string {
	var output []string
	for _, val := range input {
		trimmed := strings.TrimSpace(val)
		if trimmed != "" {
			output = append(output, trimmed)
		}
	}
	return output
}

func main() {
	httpOneBind := flag.String("http01", ":5002",
		"Comma separated bind addresses/ports for HTTP-01 challenges. Set empty to disable.")
	httpsOneBind := flag.String("https01", ":5003",
		"Comma separated bind addresses/ports for HTTPS HTTP-01 challenges. Set empty to disable.")
	dohBind := flag.String("doh", ":8443",
		"Comma separated bind addresses/ports for DoH queries. Set empty to disable.")
	dohCert := flag.String("doh-cert", "", "Path to certificate file for DoH server.")
	dohCertKey := flag.String("doh-cert-key", "", "Path to certificate key file for DoH server.")
	dnsOneBind := flag.String("dns01", ":8053",
		"Comma separated bind addresses/ports for DNS-01 challenges and fake DNS data. Set empty to disable.")
	tlsAlpnOneBind := flag.String("tlsalpn01", ":5001",
		"Comma separated bind addresses/ports for TLS-ALPN-01 and HTTPS HTTP-01 challenges. Set empty to disable.")
	managementBind := flag.String("management", ":8055",
		"Bind address/port for management HTTP interface")
	defaultIPv4 := flag.String("defaultIPv4", "127.0.0.1",
		"Default IPv4 address for mock DNS responses to A queries")
	defaultIPv6 := flag.String("defaultIPv6", "::1",
		"Default IPv6 address for mock DNS responses to AAAA queries")

	flag.Parse()

	if len(flag.Args()) > 0 {
		fmt.Printf("invalid command line arguments: %s\n", strings.Join(flag.Args(), " "))
		flag.Usage()
		os.Exit(1)
	}

	httpOneAddresses := filterEmpty(strings.Split(*httpOneBind, ","))
	httpsOneAddresses := filterEmpty(strings.Split(*httpsOneBind, ","))
	dohAddresses := filterEmpty(strings.Split(*dohBind, ","))
	dnsOneAddresses := filterEmpty(strings.Split(*dnsOneBind, ","))
	tlsAlpnOneAddresses := filterEmpty(strings.Split(*tlsAlpnOneBind, ","))

	logger := log.New(os.Stdout, "chall-test-srv - ", log.Ldate|log.Ltime)

	// Create a new challenge server with the provided config
	srv, err := challtestsrv.New(challtestsrv.Config{
		HTTPOneAddrs:    httpOneAddresses,
		HTTPSOneAddrs:   httpsOneAddresses,
		DOHAddrs:        dohAddresses,
		DOHCert:         *dohCert,
		DOHCertKey:      *dohCertKey,
		DNSOneAddrs:     dnsOneAddresses,
		TLSALPNOneAddrs: tlsAlpnOneAddresses,
		Log:             logger,
	})
	cmd.FailOnError(err, "Unable to construct challenge server")

	// Create a new management server with the provided config
	oobSrv := managementServer{
		Server: &http.Server{
			Addr:        *managementBind,
			ReadTimeout: 30 * time.Second,
		},
		challSrv: srv,
		log:      logger,
	}
	// Register handlers on the management server for adding challenge responses
	// for the configured challenges.
	if *httpOneBind != "" || *httpsOneBind != "" {
		http.HandleFunc("/add-http01", oobSrv.addHTTP01)
		http.HandleFunc("/del-http01", oobSrv.delHTTP01)
		http.HandleFunc("/add-redirect", oobSrv.addHTTPRedirect)
		http.HandleFunc("/del-redirect", oobSrv.delHTTPRedirect)
	}
	if *dnsOneBind != "" {
		http.HandleFunc("/set-default-ipv4", oobSrv.setDefaultDNSIPv4)
		http.HandleFunc("/set-default-ipv6", oobSrv.setDefaultDNSIPv6)
		// TODO(@cpu): It might make sense to revisit this API in the future to have
		// one endpoint that accepts the mock type required (A, AAAA, CNAME, etc)
		// instead of having separate endpoints per type.
		http.HandleFunc("/set-txt", oobSrv.addDNS01)
		http.HandleFunc("/clear-txt", oobSrv.delDNS01)
		http.HandleFunc("/add-a", oobSrv.addDNSARecord)
		http.HandleFunc("/clear-a", oobSrv.delDNSARecord)
		http.HandleFunc("/add-aaaa", oobSrv.addDNSAAAARecord)
		http.HandleFunc("/clear-aaaa", oobSrv.delDNSAAAARecord)
		http.HandleFunc("/add-caa", oobSrv.addDNSCAARecord)
		http.HandleFunc("/clear-caa", oobSrv.delDNSCAARecord)
		http.HandleFunc("/set-cname", oobSrv.addDNSCNAMERecord)
		http.HandleFunc("/clear-cname", oobSrv.delDNSCNAMERecord)
		http.HandleFunc("/set-servfail", oobSrv.addDNSServFailRecord)
		http.HandleFunc("/clear-servfail", oobSrv.delDNSServFailRecord)

		srv.SetDefaultDNSIPv4(*defaultIPv4)
		srv.SetDefaultDNSIPv6(*defaultIPv6)
		if *defaultIPv4 != "" {
			logger.Printf("Answering A queries with %s by default",
				*defaultIPv4)
		}
		if *defaultIPv6 != "" {
			logger.Printf("Answering AAAA queries with %s by default",
				*defaultIPv6)
		}
	}
	if *tlsAlpnOneBind != "" {
		http.HandleFunc("/add-tlsalpn01", oobSrv.addTLSALPN01)
		http.HandleFunc("/del-tlsalpn01", oobSrv.delTLSALPN01)
	}

	http.HandleFunc("/clear-request-history", oobSrv.clearHistory)
	http.HandleFunc("/http-request-history", oobSrv.getHTTPHistory)
	http.HandleFunc("/dns-request-history", oobSrv.getDNSHistory)
	http.HandleFunc("/tlsalpn01-request-history", oobSrv.getTLSALPNHistory)

	// Start all of the sub-servers in their own Go routines so that the main Go
	// routine can spin forever looking for signals to catch.
	go srv.Run()
	go oobSrv.Run()

	cmd.CatchSignals(func() {
		srv.Shutdown()
		oobSrv.Shutdown()
	})
}
