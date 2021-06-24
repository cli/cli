package liveshare

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"
)

type Session struct {
	WorkspaceAccess *WorkspaceAccessResponse
	WorkspaceInfo   *WorkspaceInfoResponse
}

func GetSession(ctx context.Context, configuration *Configuration) (*Session, error) {
	api := NewAPI(configuration)
	session := new(Session)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		workspaceAccess, err := api.WorkspaceAccess()
		if err != nil {
			return fmt.Errorf("error getting workspace access: %v", err)
		}
		session.WorkspaceAccess = workspaceAccess
		return nil
	})

	g.Go(func() error {
		workspaceInfo, err := api.WorkspaceInfo()
		if err != nil {
			return fmt.Errorf("error getting workspace info: %v", err)
		}
		session.WorkspaceInfo = workspaceInfo
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return session, nil
}
