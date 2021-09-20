package liveshare

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

type sshSession struct {
	*ssh.Session
	token  string
	socket net.Conn
	conn   ssh.Conn
	reader io.Reader
	writer io.Writer
}

func newSSHSession(token string, socket net.Conn) *sshSession {
	return &sshSession{token: token, socket: socket}
}

func (s *sshSession) connect(ctx context.Context) error {
	clientConfig := ssh.ClientConfig{
		User: "",
		Auth: []ssh.AuthMethod{
			ssh.Password(s.token),
		},
		HostKeyAlgorithms: []string{"rsa-sha2-512", "rsa-sha2-256"},
		HostKeyCallback:   ssh.InsecureIgnoreHostKey(),
		Timeout:           10 * time.Second,
	}

	sshClientConn, chans, reqs, err := ssh.NewClientConn(s.socket, "", &clientConfig)
	if err != nil {
		return fmt.Errorf("error creating ssh client connection: %w", err)
	}
	s.conn = sshClientConn

	sshClient := ssh.NewClient(sshClientConn, chans, reqs)
	s.Session, err = sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("error creating ssh client session: %w", err)
	}

	s.reader, err = s.Session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error creating ssh session reader: %w", err)
	}

	s.writer, err = s.Session.StdinPipe()
	if err != nil {
		return fmt.Errorf("error creating ssh session writer: %w", err)
	}

	return nil
}

func (s *sshSession) Read(p []byte) (n int, err error) {
	return s.reader.Read(p)
}

func (s *sshSession) Write(p []byte) (n int, err error) {
	return s.writer.Write(p)
}
