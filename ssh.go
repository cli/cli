package liveshare

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

type SSH struct {
	Session *Session
}

func NewSSH(session *Session) *SSH {
	return &SSH{
		Session: session,
	}
}

// Reference:
// https://github.com/Azure/azure-relay-node/blob/7b57225365df3010163bf4b9e640868a02737eb6/hyco-ws/index.js#L107-L137
func (s *SSH) relayURI(action string) string {
	relaySas := url.QueryEscape(s.Session.WorkspaceAccess.RelaySas)
	relayURI := s.Session.WorkspaceAccess.RelayLink
	relayURI = strings.Replace(relayURI, "sb:", "wss:", -1)
	relayURI = strings.Replace(relayURI, ".net/", ".net:443/$hc/", 1)
	relayURI = relayURI + "?sb-hc-action=" + action + "&sb-hc-token=" + relaySas
	return relayURI
}

func (s *SSH) socketStream() (net.Conn, error) {
	uri := s.relayURI("connect")

	ws, _, err := websocket.DefaultDialer.Dial(uri, nil)
	if err != nil {
		return nil, fmt.Errorf("error dialing websocket connection: %v", err)
	}

	return NewAdapter(ws), nil
}

type SSHSession struct {
	*ssh.Session
	reader io.Reader
	writer io.Writer
}

func (s SSHSession) Read(p []byte) (n int, err error) {
	return s.reader.Read(p)
}

func (s SSHSession) Write(p []byte) (n int, err error) {
	return s.writer.Write(p)
}

func (s *SSH) NewSession() (*SSHSession, error) {
	socketStream, err := s.socketStream()
	if err != nil {
		return nil, fmt.Errorf("error creating socket stream: %v", err)
	}

	clientConfig := ssh.ClientConfig{
		User: "",
		Auth: []ssh.AuthMethod{
			ssh.Password(s.Session.WorkspaceAccess.SessionToken),
		},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			// TODO(josebalius): implement
			return nil
		},
		Timeout: 10 * time.Second,
	}

	sshClientConn, chans, reqs, err := ssh.NewClientConn(socketStream, "", &clientConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating ssh client connection: %v", err)
	}

	sshClient := ssh.NewClient(sshClientConn, chans, reqs)
	sshSession, err := sshClient.NewSession()
	if err != nil {
		return nil, fmt.Errorf("error creating ssh client session: %v", err)
	}

	reader, err := sshSession.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("error creating ssh session reader: %v", err)
	}

	writer, err := sshSession.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("error creating ssh session writer: %v", err)
	}

	return &SSHSession{Session: sshSession, reader: reader, writer: writer}, nil
}
