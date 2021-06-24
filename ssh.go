package liveshare

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
)

type SSHSession struct {
	Session              *Session
	VersionExchangeError chan error
}

func NewSSHSession(session *Session) *SSHSession {
	return &SSHSession{
		Session: session,
	}
}

func (s *SSHSession) Connect() error {
	socketStream, err := s.socketStream()
	if err != nil {
		return fmt.Errorf("error creating socket stream: %v", err)
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
	}

	sshClientConn, chans, reqs, err := ssh.NewClientConn(socketStream, "", &clientConfig)
	if err != nil {
		return fmt.Errorf("error creating ssh client connection: %v", err)
	}

	fmt.Println(sshClientConn, chans, reqs)

	return nil
}

// Reference:
// https://github.com/Azure/azure-relay-node/blob/7b57225365df3010163bf4b9e640868a02737eb6/hyco-ws/index.js#L107-L137
func (s *SSHSession) relayURI(action string) string {
	relaySas := url.QueryEscape(s.Session.WorkspaceAccess.RelaySas)
	relayURI := s.Session.WorkspaceAccess.RelayLink
	relayURI = strings.Replace(relayURI, "sb:", "wss:", -1)
	relayURI = strings.Replace(relayURI, ".net/", ".net:443/$hc/", 1)
	relayURI = relayURI + "?sb-hc-action=" + action + "&sb-hc-token=" + relaySas
	return relayURI
}

func (s *SSHSession) socketStream() (*websocket.Conn, error) {
	uri := s.relayURI("connect")
	ws, err := websocket.Dial(uri, "", uri)
	if err != nil {
		return nil, fmt.Errorf("error dialing relay connection: %v", err)
	}

	return ws, nil
}
