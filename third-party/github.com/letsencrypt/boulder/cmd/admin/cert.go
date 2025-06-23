package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	"golang.org/x/crypto/ocsp"
	"golang.org/x/exp/maps"

	core "github.com/letsencrypt/boulder/core"
	berrors "github.com/letsencrypt/boulder/errors"
	rapb "github.com/letsencrypt/boulder/ra/proto"
	"github.com/letsencrypt/boulder/revocation"
	sapb "github.com/letsencrypt/boulder/sa/proto"
)

// subcommandRevokeCert encapsulates the "admin revoke-cert" command. It accepts
// many flags specifying different ways a to-be-revoked certificate can be
// identified. It then gathers the serial numbers of all identified certs, spins
// up a worker pool, and revokes all of those serials individually.
//
// Note that some batch methods (such as -incident-table and -serials-file) can
// result in high memory usage, as this subcommand will gather every serial in
// memory before beginning to revoke any of them. This trades local memory usage
// for shorter database and gRPC query times, so that we don't need massive
// timeouts when collecting serials to revoke.
type subcommandRevokeCert struct {
	parallelism   uint
	reasonStr     string
	skipBlock     bool
	malformed     bool
	serial        string
	incidentTable string
	serialsFile   string
	privKey       string
	regID         uint
	certFile      string
}

var _ subcommand = (*subcommandRevokeCert)(nil)

func (s *subcommandRevokeCert) Desc() string {
	return "Revoke one or more certificates"
}

func (s *subcommandRevokeCert) Flags(flag *flag.FlagSet) {
	// General flags relevant to all certificate input methods.
	flag.UintVar(&s.parallelism, "parallelism", 10, "Number of concurrent workers to use while revoking certs")
	flag.StringVar(&s.reasonStr, "reason", "unspecified", "Revocation reason (unspecified, keyCompromise, superseded, cessationOfOperation, or privilegeWithdrawn)")
	flag.BoolVar(&s.skipBlock, "skip-block-key", false, "Skip blocking the key, if revoked for keyCompromise - use with extreme caution")
	flag.BoolVar(&s.malformed, "malformed", false, "Indicates that the cert cannot be parsed - use with caution")

	// Flags specifying the input method for the certificates to be revoked.
	flag.StringVar(&s.serial, "serial", "", "Revoke the certificate with this hex serial")
	flag.StringVar(&s.incidentTable, "incident-table", "", "Revoke all certificates whose serials are in this table")
	flag.StringVar(&s.serialsFile, "serials-file", "", "Revoke all certificates whose hex serials are in this file")
	flag.StringVar(&s.privKey, "private-key", "", "Revoke all certificates whose pubkey matches this private key")
	flag.UintVar(&s.regID, "reg-id", 0, "Revoke all certificates issued to this account")
	flag.StringVar(&s.certFile, "cert-file", "", "Revoke the single PEM-formatted certificate in this file")
}

func (s *subcommandRevokeCert) Run(ctx context.Context, a *admin) error {
	if s.parallelism == 0 {
		// Why did they override it to 0, instead of just leaving it the default?
		return fmt.Errorf("got unacceptable parallelism %d", s.parallelism)
	}

	reasonCode := revocation.Reason(-1)
	for code := range revocation.AdminAllowedReasons {
		if s.reasonStr == revocation.ReasonToString[code] {
			reasonCode = code
			break
		}
	}
	if reasonCode == revocation.Reason(-1) {
		return fmt.Errorf("got unacceptable revocation reason %q", s.reasonStr)
	}

	if s.skipBlock && reasonCode == ocsp.KeyCompromise {
		// We would only add the SPKI hash of the pubkey to the blockedKeys table if
		// the revocation reason is keyCompromise.
		return errors.New("-skip-block-key only makes sense with -reason=1")
	}

	if s.malformed && reasonCode == ocsp.KeyCompromise {
		// This is because we can't extract and block the pubkey if we can't
		// parse the certificate.
		return errors.New("cannot revoke malformed certs for reason keyCompromise")
	}

	// This is a map of all input-selection flags to whether or not they were set
	// to a non-default value. We use this to ensure that exactly one input
	// selection flag was given on the command line.
	setInputs := map[string]bool{
		"-serial":         s.serial != "",
		"-incident-table": s.incidentTable != "",
		"-serials-file":   s.serialsFile != "",
		"-private-key":    s.privKey != "",
		"-reg-id":         s.regID != 0,
		"-cert-file":      s.certFile != "",
	}
	maps.DeleteFunc(setInputs, func(_ string, v bool) bool { return !v })
	if len(setInputs) == 0 {
		return errors.New("at least one input method flag must be specified")
	} else if len(setInputs) > 1 {
		return fmt.Errorf("more than one input method flag specified: %v", maps.Keys(setInputs))
	}

	var serials []string
	var err error
	switch maps.Keys(setInputs)[0] {
	case "-serial":
		serials, err = []string{s.serial}, nil
	case "-incident-table":
		serials, err = a.serialsFromIncidentTable(ctx, s.incidentTable)
	case "-serials-file":
		serials, err = a.serialsFromFile(ctx, s.serialsFile)
	case "-private-key":
		serials, err = a.serialsFromPrivateKey(ctx, s.privKey)
	case "-reg-id":
		serials, err = a.serialsFromRegID(ctx, int64(s.regID))
	case "-cert-file":
		serials, err = a.serialsFromCertPEM(ctx, s.certFile)
	default:
		return errors.New("no recognized input method flag set (this shouldn't happen)")
	}
	if err != nil {
		return fmt.Errorf("collecting serials to revoke: %w", err)
	}

	if len(serials) == 0 {
		return errors.New("no serials to revoke found")
	}
	a.log.Infof("Found %d certificates to revoke", len(serials))

	err = a.revokeSerials(ctx, serials, reasonCode, s.malformed, s.skipBlock, s.parallelism)
	if err != nil {
		return fmt.Errorf("revoking serials: %w", err)
	}

	return nil
}

func (a *admin) serialsFromIncidentTable(ctx context.Context, tableName string) ([]string, error) {
	stream, err := a.saroc.SerialsForIncident(ctx, &sapb.SerialsForIncidentRequest{IncidentTable: tableName})
	if err != nil {
		return nil, fmt.Errorf("setting up stream of serials from incident table %q: %s", tableName, err)
	}

	var serials []string
	for {
		is, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("streaming serials from incident table %q: %s", tableName, err)
		}
		serials = append(serials, is.Serial)
	}

	return serials, nil
}

func (a *admin) serialsFromFile(_ context.Context, filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening serials file: %w", err)
	}

	var serials []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		serial := scanner.Text()
		if serial == "" {
			continue
		}
		serials = append(serials, serial)
	}

	return serials, nil
}

func (a *admin) serialsFromPrivateKey(ctx context.Context, privkeyFile string) ([]string, error) {
	spkiHash, err := a.spkiHashFromPrivateKey(privkeyFile)
	if err != nil {
		return nil, err
	}

	stream, err := a.saroc.GetSerialsByKey(ctx, &sapb.SPKIHash{KeyHash: spkiHash})
	if err != nil {
		return nil, fmt.Errorf("setting up stream of serials from SA: %s", err)
	}

	var serials []string
	for {
		serial, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("streaming serials from SA: %s", err)
		}
		serials = append(serials, serial.Serial)
	}

	return serials, nil
}

func (a *admin) serialsFromRegID(ctx context.Context, regID int64) ([]string, error) {
	_, err := a.saroc.GetRegistration(ctx, &sapb.RegistrationID{Id: regID})
	if err != nil {
		return nil, fmt.Errorf("couldn't confirm regID exists: %w", err)
	}

	stream, err := a.saroc.GetSerialsByAccount(ctx, &sapb.RegistrationID{Id: regID})
	if err != nil {
		return nil, fmt.Errorf("setting up stream of serials from SA: %s", err)
	}

	var serials []string
	for {
		serial, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("streaming serials from SA: %s", err)
		}
		serials = append(serials, serial.Serial)
	}

	return serials, nil
}

func (a *admin) serialsFromCertPEM(_ context.Context, filename string) ([]string, error) {
	cert, err := core.LoadCert(filename)
	if err != nil {
		return nil, fmt.Errorf("loading certificate pem: %w", err)
	}

	return []string{core.SerialToString(cert.SerialNumber)}, nil
}

func cleanSerial(serial string) (string, error) {
	serialStrip := func(r rune) rune {
		switch {
		case unicode.IsLetter(r):
			return r
		case unicode.IsDigit(r):
			return r
		}
		return rune(-1)
	}
	strippedSerial := strings.Map(serialStrip, serial)
	if !core.ValidSerial(strippedSerial) {
		return "", fmt.Errorf("cleaned serial %q is not valid", strippedSerial)
	}
	return strippedSerial, nil
}

func (a *admin) revokeSerials(ctx context.Context, serials []string, reason revocation.Reason, malformed bool, skipBlockKey bool, parallelism uint) error {
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("getting admin username: %w", err)
	}

	var errCount atomic.Uint64
	wg := new(sync.WaitGroup)
	work := make(chan string, parallelism)
	for i := uint(0); i < parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for serial := range work {
				cleanedSerial, err := cleanSerial(serial)
				if err != nil {
					a.log.Errf("skipping serial %q: %s", serial, err)
					continue
				}
				_, err = a.rac.AdministrativelyRevokeCertificate(
					ctx,
					&rapb.AdministrativelyRevokeCertificateRequest{
						Serial:       cleanedSerial,
						Code:         int64(reason),
						AdminName:    u.Username,
						SkipBlockKey: skipBlockKey,
						Malformed:    malformed,
					},
				)
				if err != nil {
					errCount.Add(1)
					if errors.Is(err, berrors.AlreadyRevoked) {
						a.log.Errf("not revoking %q: already revoked", serial)
					} else {
						a.log.Errf("failed to revoke %q: %s", serial, err)
					}
				}
			}
		}()
	}

	for _, serial := range serials {
		work <- serial
	}
	close(work)
	wg.Wait()

	if errCount.Load() > 0 {
		return fmt.Errorf("encountered %d errors while revoking certs; see logs above for details", errCount.Load())
	}

	return nil
}
