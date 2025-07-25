package rocsp

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/ocsp"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/metrics"
)

func makeClient() (*RWClient, clock.Clock) {
	CACertFile := "../test/certs/ipki/minica.pem"
	CertFile := "../test/certs/ipki/localhost/cert.pem"
	KeyFile := "../test/certs/ipki/localhost/key.pem"
	tlsConfig := cmd.TLSConfig{
		CACertFile: CACertFile,
		CertFile:   CertFile,
		KeyFile:    KeyFile,
	}
	tlsConfig2, err := tlsConfig.Load(metrics.NoopRegisterer)
	if err != nil {
		panic(err)
	}

	rdb := redis.NewRing(&redis.RingOptions{
		Addrs: map[string]string{
			"shard1": "10.77.77.2:4218",
			"shard2": "10.77.77.3:4218",
		},
		Username:  "unittest-rw",
		Password:  "824968fa490f4ecec1e52d5e34916bdb60d45f8d",
		TLSConfig: tlsConfig2,
	})
	clk := clock.NewFake()
	return NewWritingClient(rdb, 5*time.Second, clk, metrics.NoopRegisterer), clk
}

func TestSetAndGet(t *testing.T) {
	client, _ := makeClient()
	fmt.Println(client.Ping(context.Background()))

	respBytes, err := os.ReadFile("testdata/ocsp.response")
	if err != nil {
		t.Fatal(err)
	}

	response, err := ocsp.ParseResponse(respBytes, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = client.StoreResponse(context.Background(), response)
	if err != nil {
		t.Fatalf("storing response: %s", err)
	}

	serial := "ffaa13f9c34be80b8e2532b83afe063b59a6"
	resp2, err := client.GetResponse(context.Background(), serial)
	if err != nil {
		t.Fatalf("getting response: %s", err)
	}
	if !bytes.Equal(resp2, respBytes) {
		t.Errorf("response written and response retrieved were not equal")
	}
}
