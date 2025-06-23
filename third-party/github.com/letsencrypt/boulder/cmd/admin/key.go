package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"sync"
	"sync/atomic"

	"golang.org/x/exp/maps"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/letsencrypt/boulder/core"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/privatekey"
	sapb "github.com/letsencrypt/boulder/sa/proto"
)

// subcommandBlockKey encapsulates the "admin block-key" command.
type subcommandBlockKey struct {
	parallelism uint
	comment     string
	privKey     string
	spkiFile    string
	certFile    string
}

var _ subcommand = (*subcommandBlockKey)(nil)

func (s *subcommandBlockKey) Desc() string {
	return "Block a keypair from any future issuance"
}

func (s *subcommandBlockKey) Flags(flag *flag.FlagSet) {
	// General flags relevant to all key input methods.
	flag.UintVar(&s.parallelism, "parallelism", 10, "Number of concurrent workers to use while blocking keys")
	flag.StringVar(&s.comment, "comment", "", "Additional context to add to database comment column")

	// Flags specifying the input method for the keys to be blocked.
	flag.StringVar(&s.privKey, "private-key", "", "Block issuance for the pubkey corresponding to this private key")
	flag.StringVar(&s.spkiFile, "spki-file", "", "Block issuance for all keys listed in this file as SHA256 hashes of SPKI, hex encoded, one per line")
	flag.StringVar(&s.certFile, "cert-file", "", "Block issuance for the public key of the single PEM-formatted certificate in this file")
}

func (s *subcommandBlockKey) Run(ctx context.Context, a *admin) error {
	// This is a map of all input-selection flags to whether or not they were set
	// to a non-default value. We use this to ensure that exactly one input
	// selection flag was given on the command line.
	setInputs := map[string]bool{
		"-private-key": s.privKey != "",
		"-spki-file":   s.spkiFile != "",
		"-cert-file":   s.certFile != "",
	}
	maps.DeleteFunc(setInputs, func(_ string, v bool) bool { return !v })
	if len(setInputs) == 0 {
		return errors.New("at least one input method flag must be specified")
	} else if len(setInputs) > 1 {
		return fmt.Errorf("more than one input method flag specified: %v", maps.Keys(setInputs))
	}

	var spkiHashes [][]byte
	var err error
	switch maps.Keys(setInputs)[0] {
	case "-private-key":
		var spkiHash []byte
		spkiHash, err = a.spkiHashFromPrivateKey(s.privKey)
		spkiHashes = [][]byte{spkiHash}
	case "-spki-file":
		spkiHashes, err = a.spkiHashesFromFile(s.spkiFile)
	case "-cert-file":
		spkiHashes, err = a.spkiHashesFromCertPEM(s.certFile)
	default:
		return errors.New("no recognized input method flag set (this shouldn't happen)")
	}
	if err != nil {
		return fmt.Errorf("collecting spki hashes to block: %w", err)
	}

	err = a.blockSPKIHashes(ctx, spkiHashes, s.comment, s.parallelism)
	if err != nil {
		return err
	}

	return nil
}

func (a *admin) spkiHashFromPrivateKey(keyFile string) ([]byte, error) {
	_, publicKey, err := privatekey.Load(keyFile)
	if err != nil {
		return nil, fmt.Errorf("loading private key file: %w", err)
	}

	spkiHash, err := core.KeyDigest(publicKey)
	if err != nil {
		return nil, fmt.Errorf("computing SPKI hash: %w", err)
	}

	return spkiHash[:], nil
}

func (a *admin) spkiHashesFromFile(filePath string) ([][]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening spki hashes file: %w", err)
	}

	var spkiHashes [][]byte
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		spkiHex := scanner.Text()
		if spkiHex == "" {
			continue
		}
		spkiHash, err := hex.DecodeString(spkiHex)
		if err != nil {
			return nil, fmt.Errorf("decoding hex spki hash %q: %w", spkiHex, err)
		}

		if len(spkiHash) != 32 {
			return nil, fmt.Errorf("got spki hash of unexpected length: %q (%d)", spkiHex, len(spkiHash))
		}

		spkiHashes = append(spkiHashes, spkiHash)
	}

	return spkiHashes, nil
}

func (a *admin) spkiHashesFromCertPEM(filename string) ([][]byte, error) {
	cert, err := core.LoadCert(filename)
	if err != nil {
		return nil, fmt.Errorf("loading certificate pem: %w", err)
	}

	spkiHash, err := core.KeyDigest(cert.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("computing SPKI hash: %w", err)
	}

	return [][]byte{spkiHash[:]}, nil
}

func (a *admin) blockSPKIHashes(ctx context.Context, spkiHashes [][]byte, comment string, parallelism uint) error {
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("getting admin username: %w", err)
	}

	var errCount atomic.Uint64
	wg := new(sync.WaitGroup)
	work := make(chan []byte, parallelism)
	for i := uint(0); i < parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for spkiHash := range work {
				err = a.blockSPKIHash(ctx, spkiHash, u, comment)
				if err != nil {
					errCount.Add(1)
					if errors.Is(err, berrors.AlreadyRevoked) {
						a.log.Errf("not blocking %x: already blocked", spkiHash)
					} else {
						a.log.Errf("failed to block %x: %s", spkiHash, err)
					}
				}
			}
		}()
	}

	for _, spkiHash := range spkiHashes {
		work <- spkiHash
	}
	close(work)
	wg.Wait()

	if errCount.Load() > 0 {
		return fmt.Errorf("encountered %d errors while revoking certs; see logs above for details", errCount.Load())
	}

	return nil
}

func (a *admin) blockSPKIHash(ctx context.Context, spkiHash []byte, u *user.User, comment string) error {
	exists, err := a.saroc.KeyBlocked(ctx, &sapb.SPKIHash{KeyHash: spkiHash})
	if err != nil {
		return fmt.Errorf("checking if key is already blocked: %w", err)
	}
	if exists.Exists {
		return berrors.AlreadyRevokedError("the provided key already exists in the 'blockedKeys' table")
	}

	stream, err := a.saroc.GetSerialsByKey(ctx, &sapb.SPKIHash{KeyHash: spkiHash})
	if err != nil {
		return fmt.Errorf("setting up stream of serials from SA: %s", err)
	}

	var count int
	for {
		_, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("streaming serials from SA: %s", err)
		}
		count++
	}

	a.log.Infof("Found %d unexpired certificates matching the provided key", count)

	_, err = a.sac.AddBlockedKey(ctx, &sapb.AddBlockedKeyRequest{
		KeyHash:   spkiHash[:],
		Added:     timestamppb.New(a.clk.Now()),
		Source:    "admin-revoker",
		Comment:   fmt.Sprintf("%s: %s", u.Username, comment),
		RevokedBy: 0,
	})
	if err != nil {
		return fmt.Errorf("blocking key: %w", err)
	}

	return nil
}
