package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/letsencrypt/boulder/akamai"
	"github.com/letsencrypt/boulder/cmd"
)

func main() {
	listenAddr := flag.String("listen", "localhost:6789", "Address to listen on")
	secret := flag.String("secret", "", "Akamai client secret")
	flag.Parse()

	v3Purges := [][]string{}
	mu := sync.Mutex{}

	http.HandleFunc("/debug/get-purges", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		body, err := json.Marshal(struct {
			V3 [][]string
		}{V3: v3Purges})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write(body)
	})

	http.HandleFunc("/debug/reset-purges", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		v3Purges = [][]string{}
		w.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/ccu/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			fmt.Println("Wrong method:", r.Method)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		var purgeRequest struct {
			Objects []string `json:"objects"`
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Println("Can't read body:", err)
			return
		}
		if err = akamai.CheckSignature(*secret, "http://"+*listenAddr, r, body); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Println("Bad signature:", err)
			return
		}
		if err = json.Unmarshal(body, &purgeRequest); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Println("Can't unmarshal:", err)
			return
		}
		if len(purgeRequest.Objects) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Println("Bad parameters:", purgeRequest)
			return
		}
		v3Purges = append(v3Purges, purgeRequest.Objects)

		respObj := struct {
			PurgeID          string
			HTTPStatus       int
			EstimatedSeconds int
		}{
			PurgeID:          "welcome-to-the-purge",
			HTTPStatus:       http.StatusCreated,
			EstimatedSeconds: 153,
		}
		w.WriteHeader(http.StatusCreated)
		resp, err := json.Marshal(respObj)
		if err != nil {
			return
		}
		w.Write(resp)
	})

	s := http.Server{
		ReadTimeout: 30 * time.Second,
		Addr:        *listenAddr,
	}

	go func() {
		err := s.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			cmd.FailOnError(err, "Running TLS server")
		}
	}()

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
	}()

	cmd.WaitForSignal()
}
