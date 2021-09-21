package ghcs

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/github/ghcs/cmd/ghcs/output"
	"github.com/github/ghcs/internal/api"
)

type mockAPIClient struct {
	getCodespaceToken func(context.Context, string, string) (string, error)
	getCodespace      func(context.Context, string, string, string) (*api.Codespace, error)
}

func (m *mockAPIClient) GetCodespaceToken(ctx context.Context, userLogin, codespaceName string) (string, error) {
	if m.getCodespaceToken == nil {
		return "", errors.New("mock api client GetCodespaceToken not implemented")
	}

	return m.getCodespaceToken(ctx, userLogin, codespaceName)
}

func (m *mockAPIClient) GetCodespace(ctx context.Context, token, userLogin, codespaceName string) (*api.Codespace, error) {
	if m.getCodespace == nil {
		return nil, errors.New("mock api client GetCodespace not implemented")
	}

	return m.getCodespace(ctx, token, userLogin, codespaceName)
}

func TestPollForCodespace(t *testing.T) {
	logger := output.NewLogger(nil, nil, false)
	user := &api.User{Login: "test"}
	tmpCodespace := &api.Codespace{Name: "tmp-codespace"}
	codespaceToken := "codespace-token"
	ctx := context.Background()

	pollInterval := 50 * time.Millisecond
	pollTimeout := 100 * time.Millisecond

	api := &mockAPIClient{
		getCodespaceToken: func(ctx context.Context, userLogin, codespace string) (string, error) {
			if userLogin != user.Login {
				return "", fmt.Errorf("user does not match, got: %s, expected: %s", userLogin, user.Login)
			}
			if codespace != tmpCodespace.Name {
				return "", fmt.Errorf("codespace does not match, got: %s, expected: %s", codespace, tmpCodespace.Name)
			}
			return codespaceToken, nil
		},
		getCodespace: func(ctx context.Context, token, userLogin, codespace string) (*api.Codespace, error) {
			if token != codespaceToken {
				return nil, fmt.Errorf("token does not match, got: %s, expected: %s", token, codespaceToken)
			}
			if userLogin != user.Login {
				return nil, fmt.Errorf("user does not match, got: %s, expected: %s", userLogin, user.Login)
			}
			if codespace != tmpCodespace.Name {
				return nil, fmt.Errorf("codespace does not match, got: %s, expected: %s", codespace, tmpCodespace.Name)
			}
			return tmpCodespace, nil
		},
	}

	codespace, err := pollForCodespace(ctx, api, logger, pollTimeout, pollInterval, user.Login, tmpCodespace.Name)
	if err != nil {
		t.Error(err)
	}
	if tmpCodespace.Name != codespace.Name {
		t.Errorf("returned codespace does not match, got: %s, expected: %s", codespace.Name, tmpCodespace.Name)
	}

	// swap the durations to trigger a timeout
	pollTimeout, pollInterval = pollInterval, pollTimeout
	_, err = pollForCodespace(ctx, api, logger, pollTimeout, pollInterval, user.Login, tmpCodespace.Name)
	if err != context.DeadlineExceeded {
		t.Error("expected context deadline exceeded error, got nil")
	}
}
