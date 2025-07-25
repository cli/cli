package helper

import (
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ocsp"
)

var (
	method             *string
	urlOverride        *string
	hostOverride       *string
	tooSoon            *int
	ignoreExpiredCerts *bool
	expectStatus       *int
	expectReason       *int
	issuerFile         *string
)

// Config contains fields which control various behaviors of the
// checker's behavior.
type Config struct {
	method string
	// This URL will always be used in place of the URL in a certificate.
	urlOverride string
	// This URL will be used if no urlOverride is present and no OCSP URL is in the certificate.
	urlFallback        string
	hostOverride       string
	tooSoon            int
	ignoreExpiredCerts bool
	expectStatus       int
	expectReason       int
	output             io.Writer
	issuerFile         string
}

// DefaultConfig is a Config populated with a set of curated default values
// intended for library test usage of this package.
var DefaultConfig = Config{
	method:             "GET",
	urlOverride:        "",
	urlFallback:        "",
	hostOverride:       "",
	tooSoon:            76,
	ignoreExpiredCerts: false,
	expectStatus:       -1,
	expectReason:       -1,
	output:             io.Discard,
	issuerFile:         "",
}

var parseFlagsOnce sync.Once

// RegisterFlags registers command-line flags that affect OCSP checking.
func RegisterFlags() {
	method = flag.String("method", DefaultConfig.method, "Method to use for fetching OCSP")
	urlOverride = flag.String("url", DefaultConfig.urlOverride, "URL of OCSP responder to override")
	hostOverride = flag.String("host", DefaultConfig.hostOverride, "Host header to override in HTTP request")
	tooSoon = flag.Int("too-soon", DefaultConfig.tooSoon, "If NextUpdate is fewer than this many hours in future, warn.")
	ignoreExpiredCerts = flag.Bool("ignore-expired-certs", DefaultConfig.ignoreExpiredCerts, "If a cert is expired, don't bother requesting OCSP.")
	expectStatus = flag.Int("expect-status", DefaultConfig.expectStatus, "Expect response to have this numeric status (0=Good, 1=Revoked, 2=Unknown); or -1 for no enforcement.")
	expectReason = flag.Int("expect-reason", DefaultConfig.expectReason, "Expect response to have this numeric revocation reason (0=Unspecified, 1=KeyCompromise, etc); or -1 for no enforcement.")
	issuerFile = flag.String("issuer-file", DefaultConfig.issuerFile, "Path to issuer file. Use as an alternative to automatic fetch of issuer from the certificate.")
}

// ConfigFromFlags returns a Config whose values are populated from any command
// line flags passed by the user, or default values if not passed.  However, it
// replaces io.Discard with os.Stdout so that CLI usages of this package
// will produce output on stdout by default.
func ConfigFromFlags() (Config, error) {
	parseFlagsOnce.Do(func() {
		flag.Parse()
	})
	if method == nil || urlOverride == nil || hostOverride == nil || tooSoon == nil || ignoreExpiredCerts == nil || expectStatus == nil || expectReason == nil || issuerFile == nil {
		return DefaultConfig, errors.New("ConfigFromFlags was called without registering flags. Call RegisterFlags before flag.Parse()")
	}
	return Config{
		method:             *method,
		urlOverride:        *urlOverride,
		hostOverride:       *hostOverride,
		tooSoon:            *tooSoon,
		ignoreExpiredCerts: *ignoreExpiredCerts,
		expectStatus:       *expectStatus,
		expectReason:       *expectReason,
		output:             os.Stdout,
		issuerFile:         *issuerFile,
	}, nil
}

// WithExpectStatus returns a new Config with the given expectStatus,
// and all other fields the same as the receiver.
func (template Config) WithExpectStatus(status int) Config {
	ret := template
	ret.expectStatus = status
	return ret
}

// WithExpectReason returns a new Config with the given expectReason,
// and all other fields the same as the receiver.
func (template Config) WithExpectReason(reason int) Config {
	ret := template
	ret.expectReason = reason
	return ret
}

func (template Config) WithURLFallback(url string) Config {
	ret := template
	ret.urlFallback = url
	return ret
}

// WithOutput returns a new Config with the given output,
// and all other fields the same as the receiver.
func (template Config) WithOutput(w io.Writer) Config {
	ret := template
	ret.output = w
	return ret
}

func GetIssuerFile(f string) (*x509.Certificate, error) {
	certFileBytes, err := os.ReadFile(f)
	if err != nil {
		return nil, fmt.Errorf("reading issuer file: %w", err)
	}
	block, _ := pem.Decode(certFileBytes)
	if block == nil {
		return nil, fmt.Errorf("no pem data found in issuer file")
	}
	issuer, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing issuer certificate: %w", err)
	}
	return issuer, nil
}

func GetIssuer(cert *x509.Certificate) (*x509.Certificate, error) {
	if cert == nil {
		return nil, fmt.Errorf("nil certificate")
	}
	if len(cert.IssuingCertificateURL) == 0 {
		return nil, fmt.Errorf("No AIA information available, can't get issuer")
	}
	issuerURL := cert.IssuingCertificateURL[0]
	resp, err := http.Get(issuerURL)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("got http status code %d from AIA issuer url %q", resp.StatusCode, resp.Request.URL)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var issuer *x509.Certificate
	contentType := resp.Header.Get("Content-Type")
	if contentType == "application/x-pkcs7-mime" || contentType == "application/pkcs7-mime" {
		issuer, err = parseCMS(body)
	} else {
		issuer, err = parse(body)
	}
	if err != nil {
		return nil, fmt.Errorf("from %s: %w", issuerURL, err)
	}
	return issuer, nil
}

// parse tries to parse the bytes as a PEM or DER-encoded certificate.
func parse(body []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(body)
	var der []byte
	if block == nil {
		der = body
	} else {
		der = block.Bytes
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return cert, nil
}

// parseCMS parses certificates from CMS messages of type SignedData.
func parseCMS(body []byte) (*x509.Certificate, error) {
	type signedData struct {
		Version          int
		Digests          asn1.RawValue
		EncapContentInfo asn1.RawValue
		Certificates     asn1.RawValue
	}
	type cms struct {
		ContentType asn1.ObjectIdentifier
		SignedData  signedData `asn1:"explicit,tag:0"`
	}
	var msg cms
	_, err := asn1.Unmarshal(body, &msg)
	if err != nil {
		return nil, fmt.Errorf("parsing CMS: %s", err)
	}
	cert, err := x509.ParseCertificate(msg.SignedData.Certificates.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing CMS: %s", err)
	}
	return cert, nil
}

// ReqFile makes an OCSP request using the given config for the PEM-encoded
// certificate in fileName, and returns the response.
func ReqFile(fileName string, config Config) (*ocsp.Response, error) {
	contents, err := os.ReadFile(fileName)
	if err != nil {
		return nil, err
	}
	return ReqDER(contents, config)
}

// ReqDER makes an OCSP request using the given config for the given DER-encoded
// certificate, and returns the response.
func ReqDER(der []byte, config Config) (*ocsp.Response, error) {
	cert, err := parse(der)
	if err != nil {
		return nil, fmt.Errorf("parsing certificate: %s", err)
	}
	if time.Now().After(cert.NotAfter) {
		if config.ignoreExpiredCerts {
			return nil, nil
		}
		return nil, fmt.Errorf("certificate expired %s ago: %s", time.Since(cert.NotAfter), cert.NotAfter)
	}
	return Req(cert, config)
}

// ReqSerial makes an OCSP request using the given config for a certificate only identified by
// serial number. It requires that the Config have issuerFile set.
func ReqSerial(serialNumber *big.Int, config Config) (*ocsp.Response, error) {
	if config.issuerFile == "" {
		return nil, errors.New("checking OCSP by serial number requires --issuer-file")
	}
	return Req(&x509.Certificate{SerialNumber: serialNumber}, config)
}

// Req makes an OCSP request using the given config for the given in-memory
// certificate, and returns the response.
func Req(cert *x509.Certificate, config Config) (*ocsp.Response, error) {
	var issuer *x509.Certificate
	var err error
	if config.issuerFile == "" {
		issuer, err = GetIssuer(cert)
		if err != nil {
			return nil, fmt.Errorf("problem getting issuer (try --issuer-file flag instead): %w", err)
		}
	} else {
		issuer, err = GetIssuerFile(config.issuerFile)
	}
	if err != nil {
		return nil, fmt.Errorf("getting issuer: %s", err)
	}
	req, err := ocsp.CreateRequest(cert, issuer, nil)
	if err != nil {
		return nil, fmt.Errorf("creating OCSP request: %s", err)
	}

	ocspURL, err := getOCSPURL(cert, config.urlOverride, config.urlFallback)
	if err != nil {
		return nil, err
	}

	httpResp, err := sendHTTPRequest(req, ocspURL, config.method, config.hostOverride, config.output)
	if err != nil {
		return nil, err
	}
	respBytes, err := io.ReadAll(httpResp.Body)
	defer httpResp.Body.Close()
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(config.output, "HTTP %d\n", httpResp.StatusCode)
	for k, v := range httpResp.Header {
		for _, vv := range v {
			fmt.Fprintf(config.output, "%s: %s\n", k, vv)
		}
	}
	if httpResp.StatusCode != 200 {
		return nil, StatusCodeError{httpResp.StatusCode, respBytes}
	}
	if len(respBytes) == 0 {
		return nil, fmt.Errorf("empty response body")
	}
	return parseAndPrint(respBytes, cert, issuer, config)
}

type StatusCodeError struct {
	Code int
	Body []byte
}

func (e StatusCodeError) Error() string {
	return fmt.Sprintf("HTTP status code %d, body: %s", e.Code, e.Body)
}

func sendHTTPRequest(
	req []byte,
	ocspURL *url.URL,
	method string,
	host string,
	output io.Writer,
) (*http.Response, error) {
	encodedReq := base64.StdEncoding.EncodeToString(req)
	var httpRequest *http.Request
	var err error
	if method == "GET" {
		ocspURL.Path = encodedReq
		fmt.Fprintf(output, "Fetching %s\n", ocspURL.String())
		httpRequest, err = http.NewRequest("GET", ocspURL.String(), http.NoBody)
	} else if method == "POST" {
		fmt.Fprintf(output, "POSTing request, reproduce with: curl -i --data-binary @- %s < <(base64 -d <<<%s)\n",
			ocspURL, encodedReq)
		httpRequest, err = http.NewRequest("POST", ocspURL.String(), bytes.NewBuffer(req))
	} else {
		return nil, fmt.Errorf("invalid method %s, expected GET or POST", method)
	}
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Add("Content-Type", "application/ocsp-request")
	if host != "" {
		httpRequest.Host = host
	}
	client := http.Client{
		Timeout: 5 * time.Second,
	}

	return client.Do(httpRequest)
}

func getOCSPURL(cert *x509.Certificate, urlOverride, urlFallback string) (*url.URL, error) {
	var ocspServer string
	if urlOverride != "" {
		ocspServer = urlOverride
	} else if len(cert.OCSPServer) > 0 {
		ocspServer = cert.OCSPServer[0]
	} else if len(urlFallback) > 0 {
		ocspServer = urlFallback
	} else {
		return nil, fmt.Errorf("no ocsp servers in cert")
	}
	ocspURL, err := url.Parse(ocspServer)
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %s", err)
	}
	return ocspURL, nil
}

// checkSignerTimes checks that the OCSP response is within the
// validity window of whichever certificate signed it, and that that
// certificate is currently valid.
func checkSignerTimes(resp *ocsp.Response, issuer *x509.Certificate, output io.Writer) error {
	var ocspSigner = issuer
	if delegatedSigner := resp.Certificate; delegatedSigner != nil {
		ocspSigner = delegatedSigner

		fmt.Fprintf(output, "Using delegated OCSP signer from response: %s\n",
			base64.StdEncoding.EncodeToString(ocspSigner.Raw))
	}

	if resp.NextUpdate.After(ocspSigner.NotAfter) {
		return fmt.Errorf("OCSP response is valid longer than OCSP signer (%s): %s is after %s",
			ocspSigner.Subject, resp.NextUpdate, ocspSigner.NotAfter)
	}
	if resp.ThisUpdate.Before(ocspSigner.NotBefore) {
		return fmt.Errorf("OCSP response's validity begins before the OCSP signer's (%s): %s is before %s",
			ocspSigner.Subject, resp.ThisUpdate, ocspSigner.NotBefore)
	}

	if time.Now().After(ocspSigner.NotAfter) {
		return fmt.Errorf("OCSP signer (%s) expired at %s", ocspSigner.Subject, ocspSigner.NotAfter)
	}
	if time.Now().Before(ocspSigner.NotBefore) {
		return fmt.Errorf("OCSP signer (%s) not valid until %s", ocspSigner.Subject, ocspSigner.NotBefore)
	}
	return nil
}

func parseAndPrint(respBytes []byte, cert, issuer *x509.Certificate, config Config) (*ocsp.Response, error) {
	fmt.Fprintf(config.output, "\nDecoding body: %s\n", base64.StdEncoding.EncodeToString(respBytes))
	resp, err := ocsp.ParseResponseForCert(respBytes, cert, issuer)
	if err != nil {
		return nil, fmt.Errorf("parsing response: %s", err)
	}

	var errs []error
	if config.expectStatus != -1 && resp.Status != config.expectStatus {
		errs = append(errs, fmt.Errorf("wrong CertStatus %d, expected %d", resp.Status, config.expectStatus))
	}
	if config.expectReason != -1 && resp.RevocationReason != config.expectReason {
		errs = append(errs, fmt.Errorf("wrong RevocationReason %d, expected %d", resp.RevocationReason, config.expectReason))
	}
	timeTilExpiry := time.Until(resp.NextUpdate)
	tooSoonDuration := time.Duration(config.tooSoon) * time.Hour
	if timeTilExpiry < tooSoonDuration {
		errs = append(errs, fmt.Errorf("NextUpdate is too soon: %s", timeTilExpiry))
	}

	err = checkSignerTimes(resp, issuer, config.output)
	if err != nil {
		errs = append(errs, fmt.Errorf("checking signature on delegated signer: %s", err))
	}

	fmt.Fprint(config.output, PrettyResponse(resp))

	if len(errs) > 0 {
		fmt.Fprint(config.output, "Errors:\n")
		err := errs[0]
		fmt.Fprintf(config.output, "  %v\n", err.Error())
		for _, e := range errs[1:] {
			err = fmt.Errorf("%w; %v", err, e)
			fmt.Fprintf(config.output, "  %v\n", e.Error())
		}
		return nil, err
	}
	fmt.Fprint(config.output, "No errors found.\n")
	return resp, nil
}

func PrettyResponse(resp *ocsp.Response) string {
	var builder strings.Builder
	pr := func(s string, v ...interface{}) {
		fmt.Fprintf(&builder, s, v...)
	}

	pr("\n")
	pr("Response:\n")
	pr("  SerialNumber %036x\n", resp.SerialNumber)
	pr("  CertStatus %d\n", resp.Status)
	pr("  RevocationReason %d\n", resp.RevocationReason)
	pr("  RevokedAt %s\n", resp.RevokedAt)
	pr("  ProducedAt %s\n", resp.ProducedAt)
	pr("  ThisUpdate %s\n", resp.ThisUpdate)
	pr("  NextUpdate %s\n", resp.NextUpdate)
	pr("  SignatureAlgorithm %s\n", resp.SignatureAlgorithm)
	pr("  IssuerHash %s\n", resp.IssuerHash)
	if resp.Extensions != nil {
		pr("  Extensions %#v\n", resp.Extensions)
	}
	if resp.Certificate != nil {
		pr("  Certificate:\n")
		pr("    Subject: %s\n", resp.Certificate.Subject)
		pr("    Issuer: %s\n", resp.Certificate.Issuer)
		pr("    NotBefore: %s\n", resp.Certificate.NotBefore)
		pr("    NotAfter: %s\n", resp.Certificate.NotAfter)
	}

	var responder pkix.RDNSequence
	_, err := asn1.Unmarshal(resp.RawResponderName, &responder)
	if err != nil {
		pr("  Responder: error (%s)\n", err)
	} else {
		pr("  Responder: %s\n", responder)
	}

	return builder.String()
}
