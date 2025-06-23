package mocks

import (
	"sync"

	"github.com/letsencrypt/boulder/mail"
)

// Mailer is a mock
type Mailer struct {
	sync.Mutex
	Messages []MailerMessage
}

var _ mail.Mailer = &Mailer{}

// mockMailerConn is a mock that satisfies the mail.Conn interface
type mockMailerConn struct {
	parent *Mailer
}

var _ mail.Conn = &mockMailerConn{}

// MailerMessage holds the captured emails from SendMail()
type MailerMessage struct {
	To      string
	Subject string
	Body    string
}

// Clear removes any previously recorded messages
func (m *Mailer) Clear() {
	m.Lock()
	defer m.Unlock()
	m.Messages = nil
}

// SendMail is a mock
func (m *mockMailerConn) SendMail(to []string, subject, msg string) error {
	m.parent.Lock()
	defer m.parent.Unlock()
	for _, rcpt := range to {
		m.parent.Messages = append(m.parent.Messages, MailerMessage{
			To:      rcpt,
			Subject: subject,
			Body:    msg,
		})
	}
	return nil
}

// Close is a mock
func (m *mockMailerConn) Close() error {
	return nil
}

// Connect is a mock
func (m *Mailer) Connect() (mail.Conn, error) {
	return &mockMailerConn{parent: m}, nil
}
