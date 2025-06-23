package mail

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/letsencrypt/boulder/core"
	blog "github.com/letsencrypt/boulder/log"
)

type idGenerator interface {
	generate() *big.Int
}

var maxBigInt = big.NewInt(math.MaxInt64)

type realSource struct{}

func (s realSource) generate() *big.Int {
	randInt, err := rand.Int(rand.Reader, maxBigInt)
	if err != nil {
		panic(err)
	}
	return randInt
}

// Mailer is an interface that allows creating Conns. Implementations must
// be safe for concurrent use.
type Mailer interface {
	Connect() (Conn, error)
}

// Conn is an interface that allows sending mail. When you are done with a
// Conn, call Close(). Implementations are not required to be safe for
// concurrent use.
type Conn interface {
	SendMail([]string, string, string) error
	Close() error
}

// connImpl represents a single connection to a mail server. It is not safe
// for concurrent use.
type connImpl struct {
	config
	client smtpClient
}

// mailerImpl defines a mail transfer agent to use for sending mail. It is
// safe for concurrent us.
type mailerImpl struct {
	config
}

type config struct {
	log              blog.Logger
	dialer           dialer
	from             mail.Address
	clk              clock.Clock
	csprgSource      idGenerator
	reconnectBase    time.Duration
	reconnectMax     time.Duration
	sendMailAttempts *prometheus.CounterVec
}

type dialer interface {
	Dial() (smtpClient, error)
}

type smtpClient interface {
	Mail(string) error
	Rcpt(string) error
	Data() (io.WriteCloser, error)
	Reset() error
	Close() error
}

type dryRunClient struct {
	log blog.Logger
}

func (d dryRunClient) Dial() (smtpClient, error) {
	return d, nil
}

func (d dryRunClient) Mail(from string) error {
	d.log.Debugf("MAIL FROM:<%s>", from)
	return nil
}

func (d dryRunClient) Rcpt(to string) error {
	d.log.Debugf("RCPT TO:<%s>", to)
	return nil
}

func (d dryRunClient) Close() error {
	return nil
}

func (d dryRunClient) Data() (io.WriteCloser, error) {
	return d, nil
}

func (d dryRunClient) Write(p []byte) (n int, err error) {
	for _, line := range strings.Split(string(p), "\n") {
		d.log.Debugf("data: %s", line)
	}
	return len(p), nil
}

func (d dryRunClient) Reset() (err error) {
	d.log.Debugf("RESET")
	return nil
}

// New constructs a Mailer to represent an account on a particular mail
// transfer agent.
func New(
	server,
	port,
	username,
	password string,
	rootCAs *x509.CertPool,
	from mail.Address,
	logger blog.Logger,
	stats prometheus.Registerer,
	reconnectBase time.Duration,
	reconnectMax time.Duration) *mailerImpl {

	sendMailAttempts := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "send_mail_attempts",
		Help: "A counter of send mail attempts labelled by result",
	}, []string{"result", "error"})
	stats.MustRegister(sendMailAttempts)

	return &mailerImpl{
		config: config{
			dialer: &dialerImpl{
				username: username,
				password: password,
				server:   server,
				port:     port,
				rootCAs:  rootCAs,
			},
			log:              logger,
			from:             from,
			clk:              clock.New(),
			csprgSource:      realSource{},
			reconnectBase:    reconnectBase,
			reconnectMax:     reconnectMax,
			sendMailAttempts: sendMailAttempts,
		},
	}
}

// NewDryRun constructs a Mailer suitable for doing a dry run. It simply logs
// each command that would have been run, at debug level.
func NewDryRun(from mail.Address, logger blog.Logger) *mailerImpl {
	return &mailerImpl{
		config: config{
			dialer:      dryRunClient{logger},
			from:        from,
			clk:         clock.New(),
			csprgSource: realSource{},
			sendMailAttempts: prometheus.NewCounterVec(prometheus.CounterOpts{
				Name: "send_mail_attempts",
				Help: "A counter of send mail attempts labelled by result",
			}, []string{"result", "error"}),
		},
	}
}

func (c config) generateMessage(to []string, subject, body string) ([]byte, error) {
	mid := c.csprgSource.generate()
	now := c.clk.Now().UTC()
	addrs := []string{}
	for _, a := range to {
		if !core.IsASCII(a) {
			return nil, fmt.Errorf("Non-ASCII email address")
		}
		addrs = append(addrs, strconv.Quote(a))
	}
	headers := []string{
		fmt.Sprintf("To: %s", strings.Join(addrs, ", ")),
		fmt.Sprintf("From: %s", c.from.String()),
		fmt.Sprintf("Subject: %s", subject),
		fmt.Sprintf("Date: %s", now.Format(time.RFC822)),
		fmt.Sprintf("Message-Id: <%s.%s.%s>", now.Format("20060102T150405"), mid.String(), c.from.Address),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: quoted-printable",
	}
	for i := range headers[1:] {
		// strip LFs
		headers[i] = strings.Replace(headers[i], "\n", "", -1)
	}
	bodyBuf := new(bytes.Buffer)
	mimeWriter := quotedprintable.NewWriter(bodyBuf)
	_, err := mimeWriter.Write([]byte(body))
	if err != nil {
		return nil, err
	}
	err = mimeWriter.Close()
	if err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf(
		"%s\r\n\r\n%s\r\n",
		strings.Join(headers, "\r\n"),
		bodyBuf.String(),
	)), nil
}

func (c *connImpl) reconnect() {
	for i := 0; ; i++ {
		sleepDuration := core.RetryBackoff(i, c.reconnectBase, c.reconnectMax, 2)
		c.log.Infof("sleeping for %s before reconnecting mailer", sleepDuration)
		c.clk.Sleep(sleepDuration)
		c.log.Info("attempting to reconnect mailer")
		client, err := c.dialer.Dial()
		if err != nil {
			c.log.Warningf("reconnect error: %s", err)
			continue
		}
		c.client = client
		break
	}
	c.log.Info("reconnected successfully")
}

// Connect opens a connection to the specified mail server. It must be called
// before SendMail.
func (m *mailerImpl) Connect() (Conn, error) {
	client, err := m.dialer.Dial()
	if err != nil {
		return nil, err
	}
	return &connImpl{m.config, client}, nil
}

type dialerImpl struct {
	username, password, server, port string
	rootCAs                          *x509.CertPool
}

func (di *dialerImpl) Dial() (smtpClient, error) {
	hostport := net.JoinHostPort(di.server, di.port)
	var conn net.Conn
	var err error
	conn, err = tls.Dial("tcp", hostport, &tls.Config{
		RootCAs: di.rootCAs,
	})
	if err != nil {
		return nil, err
	}
	client, err := smtp.NewClient(conn, di.server)
	if err != nil {
		return nil, err
	}
	auth := smtp.PlainAuth("", di.username, di.password, di.server)
	if err = client.Auth(auth); err != nil {
		return nil, err
	}
	return client, nil
}

// resetAndError resets the current mail transaction and then returns its
// argument as an error. If the reset command also errors, it combines both
// errors and returns them. Without this we would get `nested MAIL command`.
// https://github.com/letsencrypt/boulder/issues/3191
func (c *connImpl) resetAndError(err error) error {
	if err == io.EOF {
		return err
	}
	if err2 := c.client.Reset(); err2 != nil {
		return fmt.Errorf("%s (also, on sending RSET: %s)", err, err2)
	}
	return err
}

func (c *connImpl) sendOne(to []string, subject, msg string) error {
	if c.client == nil {
		return errors.New("call Connect before SendMail")
	}
	body, err := c.generateMessage(to, subject, msg)
	if err != nil {
		return err
	}
	if err = c.client.Mail(c.from.String()); err != nil {
		return err
	}
	for _, t := range to {
		if err = c.client.Rcpt(t); err != nil {
			return c.resetAndError(err)
		}
	}
	w, err := c.client.Data()
	if err != nil {
		return c.resetAndError(err)
	}
	_, err = w.Write(body)
	if err != nil {
		return c.resetAndError(err)
	}
	err = w.Close()
	if err != nil {
		return c.resetAndError(err)
	}
	return nil
}

// BadAddressSMTPError is returned by SendMail when the server rejects a message
// but for a reason that doesn't prevent us from continuing to send mail. The
// error message contains the error code and the error message returned from the
// server.
type BadAddressSMTPError struct {
	Message string
}

func (e BadAddressSMTPError) Error() string {
	return e.Message
}

// Based on reading of various SMTP documents these are a handful
// of errors we are likely to be able to continue sending mail after
// receiving. The majority of these errors boil down to 'bad address'.
var badAddressErrorCodes = map[int]bool{
	401: true, // Invalid recipient
	422: true, // Recipient mailbox is full
	441: true, // Recipient server is not responding
	450: true, // User's mailbox is not available
	501: true, // Bad recipient address syntax
	510: true, // Invalid recipient
	511: true, // Invalid recipient
	513: true, // Address type invalid
	541: true, // Recipient rejected message
	550: true, // Non-existent address
	553: true, // Non-existent address
}

// SendMail sends an email to the provided list of recipients. The email body
// is simple text.
func (c *connImpl) SendMail(to []string, subject, msg string) error {
	var protoErr *textproto.Error
	for {
		err := c.sendOne(to, subject, msg)
		if err == nil {
			// If the error is nil, we sent the mail without issue. nice!
			break
		} else if err == io.EOF {
			c.sendMailAttempts.WithLabelValues("failure", "EOF").Inc()
			// If the error is an EOF, we should try to reconnect on a backoff
			// schedule, sleeping between attempts.
			c.reconnect()
			// After reconnecting, loop around and try `sendOne` again.
			continue
		} else if errors.Is(err, syscall.ECONNRESET) {
			c.sendMailAttempts.WithLabelValues("failure", "TCP RST").Inc()
			// If the error is `syscall.ECONNRESET`, we should try to reconnect on a backoff
			// schedule, sleeping between attempts.
			c.reconnect()
			// After reconnecting, loop around and try `sendOne` again.
			continue
		} else if errors.Is(err, syscall.EPIPE) {
			// EPIPE also seems to be a common way to signal TCP RST.
			c.sendMailAttempts.WithLabelValues("failure", "EPIPE").Inc()
			c.reconnect()
			continue
		} else if errors.As(err, &protoErr) && protoErr.Code == 421 {
			c.sendMailAttempts.WithLabelValues("failure", "SMTP 421").Inc()
			/*
			 *  If the error is an instance of `textproto.Error` with a SMTP error code,
			 *  and that error code is 421 then treat this as a reconnect-able event.
			 *
			 *  The SMTP RFC defines this error code as:
			 *   421 <domain> Service not available, closing transmission channel
			 *   (This may be a reply to any command if the service knows it
			 *   must shut down)
			 *
			 * In practice we see this code being used by our production SMTP server
			 * when the connection has gone idle for too long. For more information
			 * see issue #2249[0].
			 *
			 * [0] - https://github.com/letsencrypt/boulder/issues/2249
			 */
			c.reconnect()
			// After reconnecting, loop around and try `sendOne` again.
			continue
		} else if errors.As(err, &protoErr) && badAddressErrorCodes[protoErr.Code] {
			c.sendMailAttempts.WithLabelValues("failure", fmt.Sprintf("SMTP %d", protoErr.Code)).Inc()
			return BadAddressSMTPError{fmt.Sprintf("%d: %s", protoErr.Code, protoErr.Msg)}
		} else {
			// If it wasn't an EOF error or a recoverable SMTP error it is unexpected and we
			// return from SendMail() with the error
			c.sendMailAttempts.WithLabelValues("failure", "unexpected").Inc()
			return err
		}
	}

	c.sendMailAttempts.WithLabelValues("success", "").Inc()
	return nil
}

// Close closes the connection.
func (c *connImpl) Close() error {
	err := c.client.Close()
	if err != nil {
		return err
	}
	c.client = nil
	return nil
}
