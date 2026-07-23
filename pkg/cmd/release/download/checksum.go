package download

import (
	"crypto/sha1" //nolint:gosec // matching publisher-provided checksum files, not a security boundary
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/cli/cli/v2/pkg/iostreams"
)

type checksumEntry struct {
	hash string
	alg  string
}

// checksumLineRE matches the coreutils `sha256sum`/`sha512sum`/`sha1sum` output format:
// a hex digest, whitespace, an optional `*` binary-mode marker, then the file name.
var checksumLineRE = regexp.MustCompile(`^([0-9a-fA-F]+)\s+\*?(.+)$`)

// verifyChecksums reads the downloaded checksum file at dest and confirms that each
// named asset, also already downloaded to dest, hashes to the value recorded there.
func verifyChecksums(dest *destinationWriter, checksumFileName string, assetNames []string, io *iostreams.IOStreams) error {
	data, err := os.ReadFile(dest.makePath(checksumFileName))
	if err != nil {
		return fmt.Errorf("reading checksum file %q: %w", checksumFileName, err)
	}

	checksums := parseChecksumFile(string(data))

	var problems []string
	for _, name := range assetNames {
		entry, ok := checksums[name]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: no checksum entry found in %s", name, checksumFileName))
			continue
		}

		actual, err := hashFile(dest.makePath(name), entry.alg)
		if err != nil {
			return err
		}
		if !strings.EqualFold(actual, entry.hash) {
			problems = append(problems, fmt.Sprintf("%s: checksum mismatch (expected %s, got %s)", name, entry.hash, actual))
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("checksum verification failed:\n%s", strings.Join(problems, "\n"))
	}

	fmt.Fprintf(io.Out, "Verified checksums for %d asset(s) against %s\n", len(assetNames), checksumFileName)
	return nil
}

// parseChecksumFile parses lines of the form "<hex digest>  <filename>", inferring the
// hash algorithm from the digest length (40 hex chars = sha1, 64 = sha256, 128 = sha512).
// Unrecognized or malformed lines are skipped.
func parseChecksumFile(data string) map[string]checksumEntry {
	checksums := make(map[string]checksumEntry)
	for line := range strings.SplitSeq(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		matches := checksumLineRE.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		alg := algForDigestLength(len(matches[1]))
		if alg == "" {
			continue
		}

		checksums[matches[2]] = checksumEntry{hash: matches[1], alg: alg}
	}
	return checksums
}

func algForDigestLength(n int) string {
	switch n {
	case 40:
		return "sha1"
	case 64:
		return "sha256"
	case 128:
		return "sha512"
	default:
		return ""
	}
}

func hashFile(path, alg string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var h hash.Hash
	switch alg {
	case "sha1":
		h = sha1.New() //nolint:gosec // matching publisher-provided checksum files, not a security boundary
	case "sha256":
		h = sha256.New()
	case "sha512":
		h = sha512.New()
	}

	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hashing %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
