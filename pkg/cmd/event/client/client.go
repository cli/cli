// Package client provides access to the GitHub Event Subscriptions API. The EventClient interface defines the
// supported operations and can be replaced with a mock in tests.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/safeurl"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

var (
	// ErrUnsupportedHost is returned when the host is not github.com, the only host serving event subscriptions.
	ErrUnsupportedHost = errors.New("event subscriptions are only available on github.com")
	// ErrResourceNotFound is returned when the resource to subscribe to does not exist or is not accessible.
	ErrResourceNotFound = errors.New("resource not found")
	// ErrSubscriptionNotFound is returned when the subscription does not exist, for example because it expired.
	ErrSubscriptionNotFound = errors.New("subscription not found")
	// ErrPreviewNotEnabled is returned when the account is not enrolled in the Event Subscriptions preview.
	ErrPreviewNotEnabled = errors.New("event subscriptions are in preview and not enabled for this account")
	// ErrSubscriptionLimit is returned when the API rejects a new subscription without naming an invalid field, which
	// most likely means the caller holds as many subscriptions as it may.
	ErrSubscriptionLimit = errors.New("request rejected, or the subscription limit may have been reached")
)

// previewNotEnabledMessage is the message of the 403 response the API returns on every route to accounts that are
// not enrolled in the preview.
const previewNotEnabledMessage = "Event subscriptions are not enabled for this account"

//go:generate moq -rm -out client_mock.go . EventClient

// EventClient defines operations on event subscriptions.
type EventClient interface {
	// CreateSubscription subscribes to events of the given types for the resource at resourceURL. Creation is
	// idempotent: an active subscription for the same resource and set of event types is returned as is, and created
	// reports whether the subscription is new.
	CreateSubscription(ctx context.Context, resourceURL string, eventTypes []string) (sub *Subscription, created bool, err error)
	// GetSubscription returns the active subscription with the given ID.
	GetSubscription(ctx context.Context, id string) (*Subscription, error)
	// ListSubscriptions returns up to limit of the caller's active subscriptions, newest first. The limit must be
	// positive.
	ListSubscriptions(ctx context.Context, limit int) ([]Subscription, error)
	// DeleteSubscription deletes the subscription with the given ID.
	DeleteSubscription(ctx context.Context, id string) error
	// OpenStream opens the server-sent events stream of the subscription with the given ID. The caller must close
	// the returned body. The stream is live only: events published while no stream is open are not delivered.
	OpenStream(ctx context.Context, id string) (io.ReadCloser, error)
}

type eventClient struct {
	api  *api.Client
	host string
}

// NewEventClient returns an EventClient that sends requests for host through httpClient.
func NewEventClient(httpClient *http.Client, host string) EventClient {
	return &eventClient{api: api.NewClientFromHTTP(httpClient), host: host}
}

func (c *eventClient) CreateSubscription(ctx context.Context, resourceURL string, eventTypes []string) (*Subscription, bool, error) {
	if !SupportedHost(c.host) {
		return nil, false, ErrUnsupportedHost
	}

	body, err := json.Marshal(struct {
		ResourceURL string   `json:"resource_url"`
		EventTypes  []string `json:"event_types"`
	}{
		ResourceURL: resourceURL,
		EventTypes:  eventTypes,
	})
	if err != nil {
		return nil, false, err
	}

	path, err := safeurl.JoinPath("event-subscriptions")
	if err != nil {
		return nil, false, err
	}

	resp, err := c.api.RequestWithContext(ctx, c.host, http.MethodPost, path.String(), bytes.NewReader(body))
	if err != nil {
		if httpErr, ok := errors.AsType[api.HTTPError](err); ok {
			switch {
			case httpErr.StatusCode == http.StatusNotFound:
				return nil, false, fmt.Errorf("%w: %w", ErrResourceNotFound, err)
			case httpErr.StatusCode == http.StatusForbidden:
				return nil, false, forbiddenError(httpErr, err)
			case httpErr.StatusCode == http.StatusUnprocessableEntity && len(httpErr.Errors) > 0:
				return nil, false, fmt.Errorf("invalid subscription request: %w", err)
			case httpErr.StatusCode == http.StatusUnprocessableEntity:
				return nil, false, fmt.Errorf("%w: %w", ErrSubscriptionLimit, err)
			}
		}
		return nil, false, err
	}
	created := resp.StatusCode == http.StatusCreated
	var sub Subscription
	if err := decodeJSON(resp, &sub); err != nil {
		return nil, false, err
	}
	return &sub, created, nil
}

func (c *eventClient) GetSubscription(ctx context.Context, id string) (*Subscription, error) {
	if !SupportedHost(c.host) {
		return nil, ErrUnsupportedHost
	}

	path, err := safeurl.JoinPath("event-subscriptions", id)
	if err != nil {
		return nil, err
	}

	var sub Subscription
	if err := c.api.REST(c.host, http.MethodGet, path.String(), nil, &sub); err != nil {
		if httpErr, ok := errors.AsType[api.HTTPError](err); ok {
			switch {
			case httpErr.StatusCode == http.StatusNotFound:
				return nil, fmt.Errorf("%w: %w", ErrSubscriptionNotFound, err)
			case httpErr.StatusCode == http.StatusForbidden:
				return nil, forbiddenError(httpErr, err)
			}
		}
		return nil, err
	}
	return &sub, nil
}

// maxPerPage is the largest page size the list endpoint accepts.
const maxPerPage = 100

func (c *eventClient) ListSubscriptions(ctx context.Context, limit int) ([]Subscription, error) {
	if !SupportedHost(c.host) {
		return nil, ErrUnsupportedHost
	}

	path, err := safeurl.JoinPath("event-subscriptions")
	if err != nil {
		return nil, err
	}
	path.SetQuery("per_page", strconv.Itoa(min(limit, maxPerPage)))

	subs := []Subscription{}
	for next := path.String(); next != "" && len(subs) < limit; {
		var page struct {
			Subscriptions []Subscription `json:"subscriptions"`
		}
		next, err = c.api.RESTWithNext(c.host, http.MethodGet, next, nil, &page)
		if err != nil {
			if httpErr, ok := errors.AsType[api.HTTPError](err); ok && httpErr.StatusCode == http.StatusForbidden {
				return nil, forbiddenError(httpErr, err)
			}
			return nil, err
		}
		// Pages past the last one are empty, so stop there even if a link points further.
		if len(page.Subscriptions) == 0 {
			break
		}
		subs = append(subs, page.Subscriptions...)
	}
	return subs[:min(len(subs), limit)], nil
}

// decodeJSON reads a JSON response body into v and closes it. A body cut short is reported as a *url.Error, like a
// request that failed in transit, so callers can tell it apart from a response of an unexpected shape.
func decodeJSON(resp *http.Response, v any) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &url.Error{Op: resp.Request.Method, URL: resp.Request.URL.String(), Err: err}
	}

	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	return nil
}

func (c *eventClient) DeleteSubscription(ctx context.Context, id string) error {
	if !SupportedHost(c.host) {
		return ErrUnsupportedHost
	}

	path, err := safeurl.JoinPath("event-subscriptions", id)
	if err != nil {
		return err
	}

	if err := c.api.REST(c.host, http.MethodDelete, path.String(), nil, nil); err != nil {
		if httpErr, ok := errors.AsType[api.HTTPError](err); ok {
			switch {
			case httpErr.StatusCode == http.StatusNotFound:
				return fmt.Errorf("%w: %w", ErrSubscriptionNotFound, err)
			case httpErr.StatusCode == http.StatusForbidden:
				return forbiddenError(httpErr, err)
			}
		}
		return err
	}
	return nil
}

func (c *eventClient) OpenStream(ctx context.Context, id string) (io.ReadCloser, error) {
	if !SupportedHost(c.host) {
		return nil, ErrUnsupportedHost
	}

	path, err := safeurl.JoinPath("event-subscriptions", id, "events")
	if err != nil {
		return nil, err
	}

	resp, err := c.api.RequestWithContext(ctx, c.host, http.MethodGet, path.String(), nil, api.WithHeader("Accept", "text/event-stream"))
	if err != nil {
		if httpErr, ok := errors.AsType[api.HTTPError](err); ok {
			switch {
			case httpErr.StatusCode == http.StatusNotFound:
				return nil, fmt.Errorf("%w: %w", ErrSubscriptionNotFound, err)
			case httpErr.StatusCode == http.StatusForbidden:
				return nil, forbiddenError(httpErr, err)
			}
		}
		return nil, err
	}
	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if mediaType != "text/event-stream" {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected event stream content type %q", resp.Header.Get("Content-Type"))
	}
	return resp.Body, nil
}

// forbiddenError tells a 403 response for an account without preview access apart from other denials. It keeps the
// HTTP error wrapped, so callers treat it as final rather than retrying it.
func forbiddenError(httpErr api.HTTPError, err error) error {
	// This check is temporary and can be removed once event subscriptions are enabled for all users.
	if httpErr.HTTPError != nil && httpErr.Message == previewNotEnabledMessage {
		return fmt.Errorf("%w: %w", ErrPreviewNotEnabled, err)
	}
	return fmt.Errorf("access forbidden: %w", err)
}

// SupportedHost reports whether event subscriptions are available on host. They are only available on github.com,
// not on GitHub Enterprise Server or GHE.com.
func SupportedHost(host string) bool {
	return !ghauth.IsEnterprise(host) && !ghauth.IsTenancy(host)
}
