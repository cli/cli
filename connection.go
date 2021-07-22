package liveshare

import (
	"errors"
	"net/url"
	"strings"
)

type Connection struct {
	SessionID     string
	SessionToken  string
	RelaySAS      string
	RelayEndpoint string
}

func (r Connection) validate() error {
	if r.SessionID == "" {
		return errors.New("connection SessionID is required")
	}

	if r.SessionToken == "" {
		return errors.New("connection SessionToken is required")
	}

	if r.RelaySAS == "" {
		return errors.New("connection RelaySAS is required")
	}

	if r.RelayEndpoint == "" {
		return errors.New("connection RelayEndpoint is required")
	}

	return nil
}

func (r Connection) uri(action string) string {
	sas := url.QueryEscape(r.RelaySAS)
	uri := r.RelayEndpoint
	uri = strings.Replace(uri, "sb:", "wss:", -1)
	uri = strings.Replace(uri, ".net/", ".net:443/$hc/", 1)
	uri = uri + "?sb-hc-action=" + action + "&sb-hc-token=" + sas
	return uri
}
