// generate.go is a helper utility for integration tests.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"text/template"

	"github.com/letsencrypt/boulder/cmd"
	blog "github.com/letsencrypt/boulder/log"
)

// createSlot initializes a SoftHSM slot and token. SoftHSM chooses the highest empty
// slot, initializes it, and then assigns it a new randomly chosen slot ID. Since we can't
// predict this ID we need to parse out the new ID so that we can use it in the ceremony
// configs.
func createSlot(label string) (string, error) {
	output, err := exec.Command("softhsm2-util", "--init-token", "--free", "--label", label, "--pin", "1234", "--so-pin", "5678").CombinedOutput()
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`to slot (\d+)`)
	matches := re.FindSubmatch(output)
	if len(matches) != 2 {
		return "", errors.New("unexpected number of slot matches")
	}
	return string(matches[1]), nil
}

// genKey is used to run a root key ceremony with a given config, replacing
// SlotID in the YAML with a specific slot ID.
func genKey(path string, inSlot string) error {
	tmpPath, err := rewriteConfig(path, map[string]string{"SlotID": inSlot})
	if err != nil {
		return err
	}
	output, err := exec.Command("./bin/ceremony", "-config", tmpPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("error running ceremony for %s: %s:\n%s", tmpPath, err, string(output))
	}
	return nil
}

// rewriteConfig creates a temporary config based on the template at path
// using the variables in rewrites.
func rewriteConfig(path string, rewrites map[string]string) (string, error) {
	tmplBytes, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(os.TempDir(), "ceremony-config")
	if err != nil {
		return "", err
	}
	defer tmp.Close()
	tmpl, err := template.New("config").Parse(string(tmplBytes))
	if err != nil {
		return "", err
	}
	err = tmpl.Execute(tmp, rewrites)
	if err != nil {
		return "", err
	}
	return tmp.Name(), nil
}

// runCeremony is used to run a ceremony with a given config.
func runCeremony(path string) error {
	output, err := exec.Command("./bin/ceremony", "-config", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("error running ceremony for %s: %s:\n%s", path, err, string(output))
	}
	return nil
}

func main() {
	_ = blog.Set(blog.StdoutLogger(6))
	defer cmd.AuditPanic()

	// Create SoftHSM slots for the root signing keys
	rsaRootKeySlot, err := createSlot("Root RSA")
	cmd.FailOnError(err, "failed creating softhsm2 slot for RSA root key")
	ecdsaRootKeySlot, err := createSlot("Root ECDSA")
	cmd.FailOnError(err, "failed creating softhsm2 slot for ECDSA root key")

	// Generate the root signing keys and certificates
	err = genKey("test/certs/root-ceremony-rsa.yaml", rsaRootKeySlot)
	cmd.FailOnError(err, "failed to generate RSA root key + root cert")
	err = genKey("test/certs/root-ceremony-ecdsa.yaml", ecdsaRootKeySlot)
	cmd.FailOnError(err, "failed to generate ECDSA root key + root cert")

	// Do everything for all of the intermediates
	for _, alg := range []string{"rsa", "ecdsa"} {
		rootKeySlot := rsaRootKeySlot
		if alg == "ecdsa" {
			rootKeySlot = ecdsaRootKeySlot
		}

		for _, inst := range []string{"a", "b", "c"} {
			name := fmt.Sprintf("int %s %s", alg, inst)
			// Note: The file names produced by this script (as a combination of this
			// line, and the rest of the file name as specified in the various yaml
			// template files) are meaningful and are consumed by aia-test-srv. If
			// you change the structure of these file names, you will need to change
			// aia-test-srv as well to recognize and consume the resulting files.
			fileName := strings.Replace(name, " ", "-", -1)

			// Create SoftHSM slot
			keySlot, err := createSlot(name)
			cmd.FailOnError(err, "failed to create softhsm2 slot for intermediate key")

			// Generate key
			keyConfigTemplate := fmt.Sprintf("test/certs/intermediate-key-ceremony-%s.yaml", alg)
			keyConfig, err := rewriteConfig(keyConfigTemplate, map[string]string{
				"SlotID":   keySlot,
				"Label":    name,
				"FileName": fileName,
			})
			cmd.FailOnError(err, "failed to rewrite intermediate key ceremony config")

			err = runCeremony(keyConfig)
			cmd.FailOnError(err, "failed to generate intermediate key")

			// Generate cert
			certConfigTemplate := fmt.Sprintf("test/certs/intermediate-cert-ceremony-%s.yaml", alg)
			certConfig, err := rewriteConfig(certConfigTemplate, map[string]string{
				"SlotID":     rootKeySlot,
				"CommonName": name,
				"FileName":   fileName,
			})
			cmd.FailOnError(err, "failed to rewrite intermediate cert ceremony config")

			err = runCeremony(certConfig)
			cmd.FailOnError(err, "failed to generate intermediate cert")

			// Generate cross-certs, if necessary
			if alg == "rsa" {
				continue
			}

			crossConfigTemplate := fmt.Sprintf("test/certs/intermediate-cert-ceremony-%s-cross.yaml", alg)
			crossConfig, err := rewriteConfig(crossConfigTemplate, map[string]string{
				"SlotID":     rsaRootKeySlot,
				"CommonName": name,
				"FileName":   fileName,
			})
			cmd.FailOnError(err, "failed to rewrite intermediate cross-cert ceremony config")

			err = runCeremony(crossConfig)
			cmd.FailOnError(err, "failed to generate intermediate cross-cert")
		}
	}

	// Create CRLs stating that the intermediates are not revoked.
	rsaTmpCRLConfig, err := rewriteConfig("test/certs/root-crl-rsa.yaml", map[string]string{
		"SlotID": rsaRootKeySlot,
	})
	cmd.FailOnError(err, "failed to rewrite RSA root CRL config with key ID")
	err = runCeremony(rsaTmpCRLConfig)
	cmd.FailOnError(err, "failed to generate RSA root CRL")

	ecdsaTmpCRLConfig, err := rewriteConfig("test/certs/root-crl-ecdsa.yaml", map[string]string{
		"SlotID": ecdsaRootKeySlot,
	})
	cmd.FailOnError(err, "failed to rewrite ECDSA root CRL config with key ID")
	err = runCeremony(ecdsaTmpCRLConfig)
	cmd.FailOnError(err, "failed to generate ECDSA root CRL")
}
