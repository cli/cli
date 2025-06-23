package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"sync"

	"github.com/letsencrypt/boulder/cmd"
	blog "github.com/letsencrypt/boulder/log"
)

type mailSrv struct {
	closeFirst      uint
	allReceivedMail []rcvdMail
	allMailMutex    sync.Mutex
	connNumber      uint
	connNumberMutex sync.RWMutex
	logger          blog.Logger
}

type rcvdMail struct {
	From string
	To   string
	Mail string
}

func expectLine(buf *bufio.Reader, expected string) error {
	line, _, err := buf.ReadLine()
	if err != nil {
		return fmt.Errorf("readline: %v", err)
	}
	if string(line) != expected {
		return fmt.Errorf("Expected %s, got %s", expected, line)
	}
	return nil
}

var mailFromRegex = regexp.MustCompile(`^MAIL FROM:<(.*)>\s*BODY=8BITMIME\s*$`)
var rcptToRegex = regexp.MustCompile(`^RCPT TO:<(.*)>\s*$`)
var smtpErr501 = []byte("501 syntax error in parameters or arguments \r\n")
var smtpOk250 = []byte("250 OK \r\n")

func (srv *mailSrv) handleConn(conn net.Conn) {
	defer conn.Close()
	srv.connNumberMutex.Lock()
	srv.connNumber++
	srv.connNumberMutex.Unlock()
	srv.logger.Infof("mail-test-srv: Got connection from %s", conn.RemoteAddr())

	readBuf := bufio.NewReader(conn)
	conn.Write([]byte("220 smtp.example.com ESMTP\r\n"))
	err := expectLine(readBuf, "EHLO localhost")
	if err != nil {
		log.Printf("mail-test-srv: %s: %v\n", conn.RemoteAddr(), err)
		return
	}
	conn.Write([]byte("250-PIPELINING\r\n"))
	conn.Write([]byte("250-AUTH PLAIN LOGIN\r\n"))
	conn.Write([]byte("250 8BITMIME\r\n"))
	// This AUTH PLAIN is the output of: echo -en '\0cert-manager@example.com\0password' | base64
	// Must match the mail configs for integration tests.
	err = expectLine(readBuf, "AUTH PLAIN AGNlcnQtbWFuYWdlckBleGFtcGxlLmNvbQBwYXNzd29yZA==")
	if err != nil {
		log.Printf("mail-test-srv: %s: %v\n", conn.RemoteAddr(), err)
		return
	}
	conn.Write([]byte("235 2.7.0 Authentication successful\r\n"))
	srv.logger.Infof("mail-test-srv: Successful auth from %s", conn.RemoteAddr())

	// necessary commands:
	// MAIL RCPT DATA QUIT

	var fromAddr string
	var toAddr []string

	clearState := func() {
		fromAddr = ""
		toAddr = nil
	}

	reader := bufio.NewScanner(readBuf)
scan:
	for reader.Scan() {
		line := reader.Text()
		cmdSplit := strings.SplitN(line, " ", 2)
		cmd := cmdSplit[0]
		switch cmd {
		case "QUIT":
			conn.Write([]byte("221 Bye \r\n"))
			break scan
		case "RSET":
			clearState()
			conn.Write(smtpOk250)
		case "NOOP":
			conn.Write(smtpOk250)
		case "MAIL":
			srv.connNumberMutex.RLock()
			if srv.connNumber <= srv.closeFirst {
				// Half of the time, close cleanly to simulate the server side closing
				// unexpectedly.
				if srv.connNumber%2 == 0 {
					log.Printf(
						"mail-test-srv: connection # %d < -closeFirst parameter %d, disconnecting client. Bye!\n",
						srv.connNumber, srv.closeFirst)
					clearState()
					conn.Close()
				} else {
					// The rest of the time, simulate a stale connection timeout by sending
					// a SMTP 421 message. This replicates the timeout/close from issue
					// 2249 - https://github.com/letsencrypt/boulder/issues/2249
					log.Printf(
						"mail-test-srv: connection # %d < -closeFirst parameter %d, disconnecting with 421. Bye!\n",
						srv.connNumber, srv.closeFirst)
					clearState()
					conn.Write([]byte("421 1.2.3 foo.bar.baz Error: timeout exceeded \r\n"))
					conn.Close()
				}
			}
			srv.connNumberMutex.RUnlock()
			clearState()
			matches := mailFromRegex.FindStringSubmatch(line)
			if matches == nil {
				log.Panicf("mail-test-srv: %s: MAIL FROM parse error\n", conn.RemoteAddr())
			}
			addr, err := mail.ParseAddress(matches[1])
			if err != nil {
				log.Panicf("mail-test-srv: %s: addr parse error: %v\n", conn.RemoteAddr(), err)
			}
			fromAddr = addr.Address
			conn.Write(smtpOk250)
		case "RCPT":
			matches := rcptToRegex.FindStringSubmatch(line)
			if matches == nil {
				conn.Write(smtpErr501)
				continue
			}
			addr, err := mail.ParseAddress(matches[1])
			if err != nil {
				log.Panicf("mail-test-srv: %s: addr parse error: %v\n", conn.RemoteAddr(), err)
			}
			toAddr = append(toAddr, addr.Address)
			conn.Write(smtpOk250)
		case "DATA":
			conn.Write([]byte("354 Start mail input \r\n"))
			var msgBuf bytes.Buffer

			for reader.Scan() {
				line := reader.Text()
				msgBuf.WriteString(line)
				msgBuf.WriteString("\r\n")
				if strings.HasSuffix(msgBuf.String(), "\r\n.\r\n") {
					break
				}
			}
			if reader.Err() != nil {
				log.Printf("mail-test-srv: read from %s: %v\n", conn.RemoteAddr(), reader.Err())
				return
			}

			mailResult := rcvdMail{
				From: fromAddr,
				Mail: msgBuf.String(),
			}
			srv.allMailMutex.Lock()
			for _, rcpt := range toAddr {
				mailResult.To = rcpt
				srv.allReceivedMail = append(srv.allReceivedMail, mailResult)
				log.Printf("mail-test-srv: Got mail: %s -> %s\n", fromAddr, rcpt)
			}
			srv.allMailMutex.Unlock()
			conn.Write([]byte("250 Got mail \r\n"))
			clearState()
		}
	}
	if reader.Err() != nil {
		log.Printf("mail-test-srv: read from %s: %s\n", conn.RemoteAddr(), reader.Err())
	}
}

func (srv *mailSrv) serveSMTP(ctx context.Context, l net.Listener) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			// If the accept call returned an error because the listener has been
			// closed, then the context should have been canceled too. In that case,
			// ignore the error.
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go srv.handleConn(conn)
	}
}

func main() {
	var listenAPI = flag.String("http", "0.0.0.0:9381", "http port to listen on")
	var listenSMTP = flag.String("smtp", "0.0.0.0:9380", "smtp port to listen on")
	var certFilename = flag.String("cert", "", "certificate to serve")
	var privKeyFilename = flag.String("key", "", "private key for certificate")
	var closeFirst = flag.Uint("closeFirst", 0, "close first n connections after MAIL for reconnection tests")

	flag.Parse()

	cert, err := tls.LoadX509KeyPair(*certFilename, *privKeyFilename)
	if err != nil {
		log.Fatal(err)
	}
	l, err := tls.Listen("tcp", *listenSMTP, &tls.Config{
		Certificates: []tls.Certificate{cert},
	})
	if err != nil {
		log.Fatalf("Couldn't bind %q for SMTP: %s", *listenSMTP, err)
	}
	defer l.Close()

	srv := mailSrv{
		closeFirst: *closeFirst,
		logger:     cmd.NewLogger(cmd.SyslogConfig{StdoutLevel: 7}),
	}

	srv.setupHTTP(http.DefaultServeMux)
	go func() {
		// The gosec linter complains that timeouts cannot be set here. That's fine,
		// because this is test-only code.
		////nolint:gosec
		err := http.ListenAndServe(*listenAPI, http.DefaultServeMux)
		if err != nil {
			log.Fatalln("Couldn't start HTTP server", err)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go cmd.FailOnError(srv.serveSMTP(ctx, l), "Failed to accept connection")

	cmd.WaitForSignal()
}
