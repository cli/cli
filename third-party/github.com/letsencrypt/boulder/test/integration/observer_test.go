//go:build integration

package integration

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eggsampler/acme/v3"
)

func streamOutput(t *testing.T, c *exec.Cmd) (<-chan string, func()) {
	t.Helper()
	outChan := make(chan string)

	stdout, err := c.StdoutPipe()
	if err != nil {
		t.Fatalf("getting stdout handle: %s", err)
	}

	outScanner := bufio.NewScanner(stdout)
	go func() {
		for outScanner.Scan() {
			outChan <- outScanner.Text()
		}
	}()

	stderr, err := c.StderrPipe()
	if err != nil {
		t.Fatalf("getting stderr handle: %s", err)
	}

	errScanner := bufio.NewScanner(stderr)
	go func() {
		for errScanner.Scan() {
			outChan <- errScanner.Text()
		}
	}()

	err = c.Start()
	if err != nil {
		t.Fatalf("starting cmd: %s", err)
	}

	return outChan, func() {
		c.Cancel()
		c.Wait()
	}
}

func TestTLSProbe(t *testing.T) {
	t.Parallel()

	// We can't use random_domain(), because the observer needs to be able to
	// resolve this hostname within the docker-compose environment.
	hostname := "integration.trust"
	tempdir := t.TempDir()

	// Create the certificate that the prober will inspect.
	client, err := makeClient()
	if err != nil {
		t.Fatalf("creating test acme client: %s", err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating test key: %s", err)
	}

	res, err := authAndIssue(client, key, []acme.Identifier{{Type: "dns", Value: hostname}}, true, "")
	if err != nil {
		t.Fatalf("issuing test cert: %s", err)
	}

	// Set up the HTTP server that the prober will be pointed at.
	certFile, err := os.Create(path.Join(tempdir, "fullchain.pem"))
	if err != nil {
		t.Fatalf("creating cert file: %s", err)
	}

	err = pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: res.certs[0].Raw})
	if err != nil {
		t.Fatalf("writing test cert to file: %s", err)
	}

	err = pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: res.certs[1].Raw})
	if err != nil {
		t.Fatalf("writing test issuer cert to file: %s", err)
	}

	err = certFile.Close()
	if err != nil {
		t.Errorf("closing cert file: %s", err)
	}

	keyFile, err := os.Create(path.Join(tempdir, "privkey.pem"))
	if err != nil {
		t.Fatalf("creating key file: %s", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshalling test key: %s", err)
	}

	err = pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err != nil {
		t.Fatalf("writing test key to file: %s", err)
	}

	err = keyFile.Close()
	if err != nil {
		t.Errorf("closing key file: %s", err)
	}

	go http.ListenAndServeTLS(":8675", certFile.Name(), keyFile.Name(), http.DefaultServeMux)

	// Kick off the prober, pointed at the server presenting our test cert.
	configFile, err := os.Create(path.Join(tempdir, "observer.yml"))
	if err != nil {
		t.Fatalf("creating config file: %s", err)
	}

	_, err = configFile.WriteString(fmt.Sprintf(`---
buckets: [.001, .002, .005, .01, .02, .05, .1, .2, .5, 1, 2, 5, 10]
syslog:
  stdoutlevel: 6
  sysloglevel: 0
monitors:
  -
    period: 1s
    kind: TLS
    settings:
      response: valid
      hostname: "%s:8675"`, hostname))
	if err != nil {
		t.Fatalf("writing test config: %s", err)
	}

	binPath, err := filepath.Abs("bin/boulder")
	if err != nil {
		t.Fatalf("computing boulder binary path: %s", err)
	}

	c := exec.CommandContext(context.Background(), binPath, "boulder-observer", "-config", configFile.Name(), "-debug-addr", ":8024")
	output, cancel := streamOutput(t, c)
	defer cancel()

	timeout := time.NewTimer(5 * time.Second)

	for {
		select {
		case <-timeout.C:
			t.Fatalf("timed out before getting desired log line from boulder-observer")
		case line := <-output:
			t.Log(line)
			if strings.Contains(line, "name=[integration.trust:8675]") && strings.Contains(line, "success=[true]") {
				return
			}
		}
	}
}
