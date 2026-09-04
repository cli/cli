//go:build acceptance

package acceptance_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-internal/testscript"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixtureRepositoryClient interface {
	create(name string) error
	delete(name string) error
}

type liveFixtureRepositoryClient struct {
	apiClient *ghAPI.RESTClient
	org       string
}

func newLiveFixtureRepositoryClient(tsEnv testScriptEnv) (*liveFixtureRepositoryClient, error) {
	return newLiveFixtureRepositoryClientWithTransport(tsEnv, nil)
}

func newLiveFixtureRepositoryClientWithTransport(tsEnv testScriptEnv, transport http.RoundTripper) (*liveFixtureRepositoryClient, error) {
	apiClient, err := ghAPI.NewRESTClient(ghAPI.ClientOptions{
		Host:         tsEnv.host,
		APIHost:      tsEnv.apiHost,
		AuthToken:    tsEnv.token,
		LogIgnoreEnv: true,
		Timeout:      30 * time.Second,
		Transport:    transport,
	})
	if err != nil {
		return nil, err
	}
	return &liveFixtureRepositoryClient{
		apiClient: apiClient,
		org:       tsEnv.org,
	}, nil
}

func (c *liveFixtureRepositoryClient) create(name string) error {
	body, err := json.Marshal(struct {
		Name     string `json:"name"`
		Private  bool   `json:"private"`
		AutoInit bool   `json:"auto_init"`
	}{
		Name:     name,
		Private:  true,
		AutoInit: true,
	})
	if err != nil {
		return err
	}
	path := fmt.Sprintf("orgs/%s/repos", url.PathEscape(c.org))
	return c.apiClient.Do(http.MethodPost, path, bytes.NewReader(body), nil)
}

func (c *liveFixtureRepositoryClient) delete(name string) error {
	path := fmt.Sprintf("repos/%s/%s", url.PathEscape(c.org), url.PathEscape(name))
	err := c.apiClient.Do(http.MethodDelete, path, nil, nil)
	var httpErr *ghAPI.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		return nil
	}
	return err
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type fixtureRepositoryManager struct {
	client fixtureRepositoryClient

	mu           sync.Mutex
	shared       string
	repositories []string
}

func newFixtureRepositoryManager(tsEnv testScriptEnv) (*fixtureRepositoryManager, error) {
	client, err := newLiveFixtureRepositoryClient(tsEnv)
	if err != nil {
		return nil, err
	}
	return &fixtureRepositoryManager{client: client}, nil
}

func (m *fixtureRepositoryManager) repository(mode string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if mode == "shared" && m.shared != "" {
		return m.shared, nil
	}

	name := fmt.Sprintf("gh-acceptance-%s-%s", mode, strings.ToLower(randomString(16)))
	m.repositories = append(m.repositories, name)
	if err := m.client.create(name); err != nil {
		return "", fmt.Errorf("creating %s fixture repository: %w", mode, err)
	}
	if mode == "shared" {
		m.shared = name
	}
	return name, nil
}

func (m *fixtureRepositoryManager) cleanup() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for i := len(m.repositories) - 1; i >= 0; i-- {
		if err := m.client.delete(m.repositories[i]); err != nil {
			errs = append(errs, fmt.Errorf("deleting %s: %w", m.repositories[i], err))
		}
	}
	return errors.Join(errs...)
}

func (m *fixtureRepositoryManager) delete(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.client.delete(name)
}

type fakeFixtureRepositoryClient struct {
	created []string
	deleted []string
}

func (c *fakeFixtureRepositoryClient) create(name string) error {
	c.created = append(c.created, name)
	return nil
}

func (c *fakeFixtureRepositoryClient) delete(name string) error {
	c.deleted = append(c.deleted, name)
	return nil
}

func TestFixtureRepositoryManager(t *testing.T) {
	client := &fakeFixtureRepositoryClient{}
	manager := &fixtureRepositoryManager{client: client}

	firstShared, err := manager.repository("shared")
	require.NoError(t, err)
	secondShared, err := manager.repository("shared")
	require.NoError(t, err)
	firstIsolated, err := manager.repository("isolated")
	require.NoError(t, err)
	secondIsolated, err := manager.repository("isolated")
	require.NoError(t, err)

	assert.Equal(t, firstShared, secondShared)
	assert.NotEqual(t, firstIsolated, secondIsolated)
	assert.Len(t, client.created, 3)

	require.NoError(t, manager.cleanup())
	assert.Equal(t, []string{secondIsolated, firstIsolated, firstShared}, client.deleted)
}

func TestLiveFixtureRepositoryClientUsesAPIHost(t *testing.T) {
	var request *http.Request
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		request = req
		return &http.Response{
			StatusCode: http.StatusCreated,
			Status:     "201 Created",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Request:    req,
		}, nil
	})
	client, err := newLiveFixtureRepositoryClientWithTransport(testScriptEnv{
		host:    "github.com",
		apiHost: "gateway.example.com",
		org:     "example",
		token:   "ghs_token",
	}, transport)
	require.NoError(t, err)

	require.NoError(t, client.create("fixture"))
	require.NotNil(t, request)
	assert.Equal(t, "gateway.example.com", request.URL.Host)
	assert.Equal(t, "/orgs/example/repos", request.URL.Path)
	assert.Equal(t, "token ghs_token", request.Header.Get("Authorization"))
}

func TestDeferredRepositoryCleanup(t *testing.T) {
	file := filepath.Join(t.TempDir(), "cleanup.txt")
	require.NoError(t, os.WriteFile(file, []byte("fixture-repo none\ndefer cleanup-repo stale-repo\n"), 0o600))

	client := &fakeFixtureRepositoryClient{}
	manager := &fixtureRepositoryManager{client: client}
	tsEnv := testScriptEnv{
		host:  "github.com",
		org:   "example",
		token: "ghs_token",
	}
	t.Run("script", func(t *testing.T) {
		testscript.Run(t, testscript.Params{
			Files:               []string{file},
			Setup:               sharedSetup(tsEnv),
			Cmds:                sharedCmds(tsEnv, manager),
			RequireExplicitExec: true,
		})
	})

	assert.Equal(t, []string{"stale-repo"}, client.deleted)
}

func TestManagedFixtureRejectsRepositoryCreation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "managed.txt")
	script := "fixture-repo shared REPO\n" +
		"! exec gh repo create example --private\n" +
		"stderr 'requires.*fixture-repo none'\n"
	require.NoError(t, os.WriteFile(file, []byte(script), 0o600))

	client := &fakeFixtureRepositoryClient{}
	manager := &fixtureRepositoryManager{client: client}
	tsEnv := testScriptEnv{
		host:  "github.com",
		org:   "example",
		token: "ghs_token",
	}
	testscript.Run(t, testscript.Params{
		Files:               []string{file},
		Setup:               sharedSetup(tsEnv),
		Cmds:                sharedCmds(tsEnv, manager),
		RequireExplicitExec: true,
	})
}
