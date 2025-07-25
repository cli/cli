package notmain

import (
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/crl/checker"
)

func downloadShard(url string) (*x509.RevocationList, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("downloading crl: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading crl: http status %d", resp.StatusCode)
	}

	crlBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading CRL bytes: %w", err)
	}

	crl, err := x509.ParseRevocationList(crlBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing CRL: %w", err)
	}

	return crl, nil
}

func main() {
	urlFile := flag.String("crls", "", "path to a file containing a JSON Array of CRL URLs")
	issuerFile := flag.String("issuer", "", "path to an issuer certificate on disk, required, '-' to disable validation")
	ageLimitStr := flag.String("ageLimit", "168h", "maximum allowable age of a CRL shard")
	emitRevoked := flag.Bool("emitRevoked", false, "emit revoked serial numbers on stdout, one per line, hex-encoded")
	save := flag.Bool("save", false, "save CRLs to files named after the URL")
	flag.Parse()

	logger := cmd.NewLogger(cmd.SyslogConfig{StdoutLevel: 6, SyslogLevel: -1})
	logger.Info(cmd.VersionString())

	urlFileContents, err := os.ReadFile(*urlFile)
	cmd.FailOnError(err, "Reading CRL URLs file")

	var urls []string
	err = json.Unmarshal(urlFileContents, &urls)
	cmd.FailOnError(err, "Parsing JSON Array of CRL URLs")

	if *issuerFile == "" {
		cmd.Fail("-issuer is required, but may be '-' to disable validation")
	}

	var issuer *x509.Certificate
	if *issuerFile != "-" {
		issuer, err = core.LoadCert(*issuerFile)
		cmd.FailOnError(err, "Loading issuer certificate")
	} else {
		logger.Warning("CRL signature validation disabled")
	}

	ageLimit, err := time.ParseDuration(*ageLimitStr)
	cmd.FailOnError(err, "Parsing age limit")

	errCount := 0
	seenSerials := make(map[string]struct{})
	totalBytes := 0
	oldestTimestamp := time.Time{}
	for _, u := range urls {
		crl, err := downloadShard(u)
		if err != nil {
			errCount += 1
			logger.Errf("fetching CRL %q failed: %s", u, err)
			continue
		}

		if *save {
			parsedURL, err := url.Parse(u)
			if err != nil {
				logger.Errf("parsing url: %s", err)
				continue
			}
			filename := fmt.Sprintf("%s%s", parsedURL.Host, strings.ReplaceAll(parsedURL.Path, "/", "_"))
			err = os.WriteFile(filename, crl.Raw, 0660)
			if err != nil {
				logger.Errf("writing file: %s", err)
				continue
			}
		}

		totalBytes += len(crl.Raw)

		zcrl, err := x509.ParseRevocationList(crl.Raw)
		if err != nil {
			errCount += 1
			logger.Errf("parsing CRL %q failed: %s", u, err)
			continue
		}

		err = checker.Validate(zcrl, issuer, ageLimit)
		if err != nil {
			errCount += 1
			logger.Errf("checking CRL %q failed: %s", u, err)
			continue
		}

		if oldestTimestamp.IsZero() || crl.ThisUpdate.Before(oldestTimestamp) {
			oldestTimestamp = crl.ThisUpdate
		}

		for _, c := range crl.RevokedCertificateEntries {
			serial := core.SerialToString(c.SerialNumber)
			if _, seen := seenSerials[serial]; seen {
				errCount += 1
				logger.Errf("serial seen in multiple shards: %s", serial)
				continue
			}
			seenSerials[serial] = struct{}{}
		}
	}

	if *emitRevoked {
		for serial := range seenSerials {
			fmt.Println(serial)
		}
	}

	if errCount != 0 {
		cmd.Fail(fmt.Sprintf("Encountered %d errors", errCount))
	}

	logger.AuditInfof(
		"Validated %d CRLs, %d serials, %d bytes. Oldest CRL: %s",
		len(urls), len(seenSerials), totalBytes, oldestTimestamp.Format(time.RFC3339))
}

func init() {
	cmd.RegisterCommand("crl-checker", main, nil)
}
