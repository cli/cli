package mail

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math/big"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/jmhodges/clock"

	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/test"
)

var (
	// These variables are populated by init(), and then referenced by setup() and
	// listenForever(). smtpCert is the TLS certificate which will be served by
	// the fake SMTP server, and smtpRoot is the issuer of that certificate which
	// will be trusted by the SMTP client under test.
	smtpRoot *x509.CertPool
	smtpCert *tls.Certificate
)

func init() {
	// Populate the global smtpRoot and smtpCert variables. We use a single self
	// signed cert for both, for ease of generation. It has to assert the name
	// localhost to appease the mailer, which is connecting to localhost.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	fmt.Println(err)
	template := x509.Certificate{
		DNSNames:     []string{"localhost"},
		SerialNumber: big.NewInt(123),
		NotBefore:    time.Now().Add(-24 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, key.Public(), key)
	fmt.Println(err)
	cert, err := x509.ParseCertificate(certDER)
	fmt.Println(err)

	smtpRoot = x509.NewCertPool()
	smtpRoot.AddCert(cert)

	smtpCert = &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
		Leaf:        cert,
	}
}

type fakeSource struct{}

func (f fakeSource) generate() *big.Int {
	return big.NewInt(1991)
}

func TestGenerateMessage(t *testing.T) {
	fc := clock.NewFake()
	fromAddress, _ := mail.ParseAddress("happy sender <send@email.com>")
	log := blog.UseMock()
	m := New("", "", "", "", nil, *fromAddress, log, metrics.NoopRegisterer, 0, 0)
	m.clk = fc
	m.csprgSource = fakeSource{}
	messageBytes, err := m.generateMessage([]string{"recv@email.com"}, "test subject", "this is the body\n")
	test.AssertNotError(t, err, "Failed to generate email body")
	message := string(messageBytes)
	fields := strings.Split(message, "\r\n")
	test.AssertEquals(t, len(fields), 12)
	fmt.Println(message)
	test.AssertEquals(t, fields[0], "To: \"recv@email.com\"")
	test.AssertEquals(t, fields[1], "From: \"happy sender\" <send@email.com>")
	test.AssertEquals(t, fields[2], "Subject: test subject")
	test.AssertEquals(t, fields[3], "Date: 01 Jan 70 00:00 UTC")
	test.AssertEquals(t, fields[4], "Message-Id: <19700101T000000.1991.send@email.com>")
	test.AssertEquals(t, fields[5], "MIME-Version: 1.0")
	test.AssertEquals(t, fields[6], "Content-Type: text/plain; charset=UTF-8")
	test.AssertEquals(t, fields[7], "Content-Transfer-Encoding: quoted-printable")
	test.AssertEquals(t, fields[8], "")
	test.AssertEquals(t, fields[9], "this is the body")
}

func TestFailNonASCIIAddress(t *testing.T) {
	log := blog.UseMock()
	fromAddress, _ := mail.ParseAddress("send@email.com")
	m := New("", "", "", "", nil, *fromAddress, log, metrics.NoopRegisterer, 0, 0)
	_, err := m.generateMessage([]string{"遗憾@email.com"}, "test subject", "this is the body\n")
	test.AssertError(t, err, "Allowed a non-ASCII to address incorrectly")
}

func expect(t *testing.T, buf *bufio.Reader, expected string) error {
	line, _, err := buf.ReadLine()
	if err != nil {
		t.Errorf("readline: %s expected: %s\n", err, expected)
		return err
	}
	if string(line) != expected {
		t.Errorf("Expected %s, got %s", expected, line)
		return fmt.Errorf("Expected %s, got %s", expected, line)
	}
	return nil
}

type connHandler func(int, *testing.T, net.Conn, *net.TCPConn)

func listenForever(l *net.TCPListener, t *testing.T, handler connHandler) {
	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{*smtpCert},
	}
	connID := 0
	for {
		tcpConn, err := l.AcceptTCP()
		if err != nil {
			return
		}

		tlsConn := tls.Server(tcpConn, tlsConf)
		connID++
		go handler(connID, t, tlsConn, tcpConn)
	}
}

func authenticateClient(t *testing.T, conn net.Conn) {
	buf := bufio.NewReader(conn)
	// we can ignore write errors because any
	// failures will be caught on the connecting
	// side
	_, _ = conn.Write([]byte("220 smtp.example.com ESMTP\n"))
	err := expect(t, buf, "EHLO localhost")
	if err != nil {
		return
	}

	_, _ = conn.Write([]byte("250-PIPELINING\n"))
	_, _ = conn.Write([]byte("250-AUTH PLAIN LOGIN\n"))
	_, _ = conn.Write([]byte("250 8BITMIME\n"))
	// Base64 encoding of "\0user@example.com\0passwd"
	err = expect(t, buf, "AUTH PLAIN AHVzZXJAZXhhbXBsZS5jb20AcGFzc3dk")
	if err != nil {
		return
	}
	_, _ = conn.Write([]byte("235 2.7.0 Authentication successful\n"))
}

// The normal handler authenticates the client and then disconnects without
// further command processing. It is sufficient for TestConnect()
func normalHandler(connID int, t *testing.T, tlsConn net.Conn, tcpConn *net.TCPConn) {
	defer func() {
		err := tlsConn.Close()
		if err != nil {
			t.Errorf("conn.Close: %s", err)
		}
	}()
	authenticateClient(t, tlsConn)
}

// The disconnectHandler authenticates the client like the normalHandler but
// additionally processes an email flow (e.g. MAIL, RCPT and DATA commands).
// When the `connID` is <= `closeFirst` the connection is closed immediately
// after the MAIL command is received and prior to issuing a 250 response. If
// a `goodbyeMsg` is provided, it is written to the client immediately before
// closing. In this way the first `closeFirst` connections will not complete
// normally and can be tested for reconnection logic.
func disconnectHandler(closeFirst int, goodbyeMsg string) connHandler {
	return func(connID int, t *testing.T, conn net.Conn, _ *net.TCPConn) {
		defer func() {
			err := conn.Close()
			if err != nil {
				t.Errorf("conn.Close: %s", err)
			}
		}()
		authenticateClient(t, conn)

		buf := bufio.NewReader(conn)
		err := expect(t, buf, "MAIL FROM:<<you-are-a-winner@example.com>> BODY=8BITMIME")
		if err != nil {
			return
		}

		if connID <= closeFirst {
			// If there was a `goodbyeMsg` specified, write it to the client before
			// closing the connection. This is a good way to deliver a SMTP error
			// before closing
			if goodbyeMsg != "" {
				_, _ = fmt.Fprintf(conn, "%s\r\n", goodbyeMsg)
				t.Logf("Wrote goodbye msg: %s", goodbyeMsg)
			}
			t.Log("Cutting off client early")
			return
		}
		_, _ = conn.Write([]byte("250 Sure. Go on. \r\n"))

		err = expect(t, buf, "RCPT TO:<hi@bye.com>")
		if err != nil {
			return
		}
		_, _ = conn.Write([]byte("250 Tell Me More \r\n"))

		err = expect(t, buf, "DATA")
		if err != nil {
			return
		}
		_, _ = conn.Write([]byte("354 Cool Data\r\n"))
		_, _ = conn.Write([]byte("250 Peace Out\r\n"))
	}
}

func badEmailHandler(messagesToProcess int) connHandler {
	return func(_ int, t *testing.T, conn net.Conn, _ *net.TCPConn) {
		defer func() {
			err := conn.Close()
			if err != nil {
				t.Errorf("conn.Close: %s", err)
			}
		}()
		authenticateClient(t, conn)

		buf := bufio.NewReader(conn)
		err := expect(t, buf, "MAIL FROM:<<you-are-a-winner@example.com>> BODY=8BITMIME")
		if err != nil {
			return
		}

		_, _ = conn.Write([]byte("250 Sure. Go on. \r\n"))

		err = expect(t, buf, "RCPT TO:<hi@bye.com>")
		if err != nil {
			return
		}
		_, _ = conn.Write([]byte("401 4.1.3 Bad recipient address syntax\r\n"))
		err = expect(t, buf, "RSET")
		if err != nil {
			return
		}
		_, _ = conn.Write([]byte("250 Ok yr rset now\r\n"))
	}
}

// The rstHandler authenticates the client like the normalHandler but
// additionally processes an email flow (e.g. MAIL, RCPT and DATA
// commands). When the `connID` is <= `rstFirst` the socket of the
// listening connection is set to abruptively close (sends TCP RST but
// no FIN). The listening connection is closed immediately after the
// MAIL command is received and prior to issuing a 250 response. In this
// way the first `rstFirst` connections will not complete normally and
// can be tested for reconnection logic.
func rstHandler(rstFirst int) connHandler {
	return func(connID int, t *testing.T, tlsConn net.Conn, tcpConn *net.TCPConn) {
		defer func() {
			err := tcpConn.Close()
			if err != nil {
				t.Errorf("conn.Close: %s", err)
			}
		}()
		authenticateClient(t, tlsConn)

		buf := bufio.NewReader(tlsConn)
		err := expect(t, buf, "MAIL FROM:<<you-are-a-winner@example.com>> BODY=8BITMIME")
		if err != nil {
			return
		}
		// Set the socket of the listening connection to abruptively
		// close.
		if connID <= rstFirst {
			err := tcpConn.SetLinger(0)
			if err != nil {
				t.Error(err)
				return
			}
			t.Log("Socket set for abruptive close. Cutting off client early")
			return
		}
		_, _ = tlsConn.Write([]byte("250 Sure. Go on. \r\n"))

		err = expect(t, buf, "RCPT TO:<hi@bye.com>")
		if err != nil {
			return
		}
		_, _ = tlsConn.Write([]byte("250 Tell Me More \r\n"))

		err = expect(t, buf, "DATA")
		if err != nil {
			return
		}
		_, _ = tlsConn.Write([]byte("354 Cool Data\r\n"))
		_, _ = tlsConn.Write([]byte("250 Peace Out\r\n"))
	}
}

func setup(t *testing.T) (*mailerImpl, *net.TCPListener, func()) {
	fromAddress, _ := mail.ParseAddress("you-are-a-winner@example.com")
	log := blog.UseMock()

	// Listen on port 0 to get any free available port
	tcpAddr, err := net.ResolveTCPAddr("tcp", ":0")
	if err != nil {
		t.Fatalf("resolving tcp addr: %s", err)
	}
	tcpl, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		t.Fatalf("listen: %s", err)
	}

	cleanUp := func() {
		err := tcpl.Close()
		if err != nil {
			t.Errorf("listen.Close: %s", err)
		}
	}

	// We can look at the listener Addr() to figure out which free port was
	// assigned by the operating system

	_, port, err := net.SplitHostPort(tcpl.Addr().String())
	if err != nil {
		t.Fatal("failed parsing port from tcp listen")
	}

	m := New(
		"localhost",
		port,
		"user@example.com",
		"passwd",
		smtpRoot,
		*fromAddress,
		log,
		metrics.NoopRegisterer,
		time.Second*2, time.Second*10)

	return m, tcpl, cleanUp
}

func TestConnect(t *testing.T) {
	m, l, cleanUp := setup(t)
	defer cleanUp()

	go listenForever(l, t, normalHandler)
	conn, err := m.Connect()
	if err != nil {
		t.Errorf("Failed to connect: %s", err)
	}
	err = conn.Close()
	if err != nil {
		t.Errorf("Failed to clean up: %s", err)
	}
}

func TestReconnectSuccess(t *testing.T) {
	m, l, cleanUp := setup(t)
	defer cleanUp()
	const closedConns = 5

	// Configure a test server that will disconnect the first `closedConns`
	// connections after the MAIL cmd
	go listenForever(l, t, disconnectHandler(closedConns, ""))

	// With a mailer client that has a max attempt > `closedConns` we expect no
	// error. The message should be delivered after `closedConns` reconnect
	// attempts.
	conn, err := m.Connect()
	if err != nil {
		t.Errorf("Failed to connect: %s", err)
	}
	err = conn.SendMail([]string{"hi@bye.com"}, "You are already a winner!", "Just kidding")
	if err != nil {
		t.Errorf("Expected SendMail() to not fail. Got err: %s", err)
	}
}

func TestBadEmailError(t *testing.T) {
	m, l, cleanUp := setup(t)
	defer cleanUp()
	const messages = 3

	go listenForever(l, t, badEmailHandler(messages))

	conn, err := m.Connect()
	if err != nil {
		t.Errorf("Failed to connect: %s", err)
	}

	err = conn.SendMail([]string{"hi@bye.com"}, "You are already a winner!", "Just kidding")
	// We expect there to be an error
	if err == nil {
		t.Errorf("Expected SendMail() to return an BadAddressSMTPError, got nil")
	}
	expected := "401: 4.1.3 Bad recipient address syntax"
	var badAddrErr BadAddressSMTPError
	test.AssertErrorWraps(t, err, &badAddrErr)
	test.AssertEquals(t, badAddrErr.Message, expected)
}

func TestReconnectSMTP421(t *testing.T) {
	m, l, cleanUp := setup(t)
	defer cleanUp()
	const closedConns = 5

	// A SMTP 421 can be generated when the server times out an idle connection.
	// For more information see https://github.com/letsencrypt/boulder/issues/2249
	smtp421 := "421 1.2.3 green.eggs.and.spam Error: timeout exceeded"

	// Configure a test server that will disconnect the first `closedConns`
	// connections after the MAIL cmd with a SMTP 421 error
	go listenForever(l, t, disconnectHandler(closedConns, smtp421))

	// With a mailer client that has a max attempt > `closedConns` we expect no
	// error. The message should be delivered after `closedConns` reconnect
	// attempts.
	conn, err := m.Connect()
	if err != nil {
		t.Errorf("Failed to connect: %s", err)
	}
	err = conn.SendMail([]string{"hi@bye.com"}, "You are already a winner!", "Just kidding")
	if err != nil {
		t.Errorf("Expected SendMail() to not fail. Got err: %s", err)
	}
}

func TestOtherError(t *testing.T) {
	m, l, cleanUp := setup(t)
	defer cleanUp()

	go listenForever(l, t, func(_ int, t *testing.T, conn net.Conn, _ *net.TCPConn) {
		defer func() {
			err := conn.Close()
			if err != nil {
				t.Errorf("conn.Close: %s", err)
			}
		}()
		authenticateClient(t, conn)

		buf := bufio.NewReader(conn)
		err := expect(t, buf, "MAIL FROM:<<you-are-a-winner@example.com>> BODY=8BITMIME")
		if err != nil {
			return
		}

		_, _ = conn.Write([]byte("250 Sure. Go on. \r\n"))

		err = expect(t, buf, "RCPT TO:<hi@bye.com>")
		if err != nil {
			return
		}

		_, _ = conn.Write([]byte("999 1.1.1 This would probably be bad?\r\n"))

		err = expect(t, buf, "RSET")
		if err != nil {
			return
		}

		_, _ = conn.Write([]byte("250 Ok yr rset now\r\n"))
	})

	conn, err := m.Connect()
	if err != nil {
		t.Errorf("Failed to connect: %s", err)
	}

	err = conn.SendMail([]string{"hi@bye.com"}, "You are already a winner!", "Just kidding")
	// We expect there to be an error
	if err == nil {
		t.Errorf("Expected SendMail() to return an error, got nil")
	}
	expected := "999 1.1.1 This would probably be bad?"
	var rcptErr *textproto.Error
	test.AssertErrorWraps(t, err, &rcptErr)
	test.AssertEquals(t, rcptErr.Error(), expected)

	m, l, cleanUp = setup(t)
	defer cleanUp()

	go listenForever(l, t, func(_ int, t *testing.T, conn net.Conn, _ *net.TCPConn) {
		defer func() {
			err := conn.Close()
			if err != nil {
				t.Errorf("conn.Close: %s", err)
			}
		}()
		authenticateClient(t, conn)

		buf := bufio.NewReader(conn)
		err := expect(t, buf, "MAIL FROM:<<you-are-a-winner@example.com>> BODY=8BITMIME")
		if err != nil {
			return
		}

		_, _ = conn.Write([]byte("250 Sure. Go on. \r\n"))

		err = expect(t, buf, "RCPT TO:<hi@bye.com>")
		if err != nil {
			return
		}

		_, _ = conn.Write([]byte("999 1.1.1 This would probably be bad?\r\n"))

		err = expect(t, buf, "RSET")
		if err != nil {
			return
		}

		_, _ = conn.Write([]byte("nop\r\n"))
	})
	conn, err = m.Connect()
	if err != nil {
		t.Errorf("Failed to connect: %s", err)
	}

	err = conn.SendMail([]string{"hi@bye.com"}, "You are already a winner!", "Just kidding")
	// We expect there to be an error
	test.AssertError(t, err, "SendMail didn't fail as expected")
	test.AssertEquals(t, err.Error(), "999 1.1.1 This would probably be bad? (also, on sending RSET: short response: nop)")
}

func TestReconnectAfterRST(t *testing.T) {
	m, l, cleanUp := setup(t)
	defer cleanUp()
	const rstConns = 5

	// Configure a test server that will RST and disconnect the first
	// `closedConns` connections
	go listenForever(l, t, rstHandler(rstConns))

	// With a mailer client that has a max attempt > `closedConns` we expect no
	// error. The message should be delivered after `closedConns` reconnect
	// attempts.
	conn, err := m.Connect()
	if err != nil {
		t.Errorf("Failed to connect: %s", err)
	}
	err = conn.SendMail([]string{"hi@bye.com"}, "You are already a winner!", "Just kidding")
	if err != nil {
		t.Errorf("Expected SendMail() to not fail. Got err: %s", err)
	}
}
