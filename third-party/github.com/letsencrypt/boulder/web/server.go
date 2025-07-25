package web

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"time"

	blog "github.com/letsencrypt/boulder/log"
)

type errorWriter struct {
	blog.Logger
}

func (ew errorWriter) Write(p []byte) (n int, err error) {
	// log.Logger will append a newline to all messages before calling
	// Write. Our log checksum checker doesn't like newlines, because
	// syslog will strip them out so the calculated checksums will
	// differ. So that we don't hit this corner case for every line
	// logged from inside net/http.Server we strip the newline before
	// we get to the checksum generator.
	p = bytes.TrimRight(p, "\n")
	ew.Logger.Err(fmt.Sprintf("net/http.Server: %s", string(p)))
	return
}

// NewServer returns an http.Server which will listen on the given address, when
// started, for each path in the handler. Errors are sent to the given logger.
func NewServer(listenAddr string, handler http.Handler, logger blog.Logger) http.Server {
	return http.Server{
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
		Addr:         listenAddr,
		ErrorLog:     log.New(errorWriter{logger}, "", 0),
		Handler:      handler,
	}
}
