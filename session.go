package liveshare

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/sync/errgroup"
)

type session struct {
	api *api

	workspaceAccess *workspaceAccessResponse
	workspaceInfo   *workspaceInfoResponse
}

func newSession(api *api) *session {
	return &session{api: api}
}

func (s *session) init(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		workspaceAccess, err := s.api.workspaceAccess()
		if err != nil {
			return fmt.Errorf("error getting workspace access: %v", err)
		}
		s.workspaceAccess = workspaceAccess
		return nil
	})

	g.Go(func() error {
		workspaceInfo, err := s.api.workspaceInfo()
		if err != nil {
			return fmt.Errorf("error getting workspace info: %v", err)
		}
		s.workspaceInfo = workspaceInfo
		return nil
	})

	if err := g.Wait(); err != nil {
		return err
	}

	return nil
}

// Reference:
// https://github.com/Azure/azure-relay-node/blob/7b57225365df3010163bf4b9e640868a02737eb6/hyco-ws/index.js#L107-L137
func (s *session) relayURI(action string) string {
	relaySas := url.QueryEscape(s.workspaceAccess.RelaySas)
	relayURI := s.workspaceAccess.RelayLink
	relayURI = strings.Replace(relayURI, "sb:", "wss:", -1)
	relayURI = strings.Replace(relayURI, ".net/", ".net:443/$hc/", 1)
	relayURI = relayURI + "?sb-hc-action=" + action + "&sb-hc-token=" + relaySas
	return relayURI
}
