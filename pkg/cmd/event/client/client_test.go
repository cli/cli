package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const subscriptionJSON = `{
	"id": "sub_1",
	"resource_url": "https://github.com/OWNER/REPO/pull/1",
	"event_types": ["test.a", "test.b"],
	"created_at": "2026-01-02T03:04:05Z",
	"expires_at": "2026-01-03T03:04:05Z",
	"url": "https://api.github.com/event-subscriptions/sub_1",
	"events_url": "https://api.github.com/event-subscriptions/sub_1/events"
}`

var wantSubscription = &Subscription{
	ID:          "sub_1",
	ResourceURL: "https://github.com/OWNER/REPO/pull/1",
	EventTypes:  []string{"test.a", "test.b"},
	CreatedAt:   time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	ExpiresAt:   time.Date(2026, 1, 3, 3, 4, 5, 0, time.UTC),
	URL:         "https://api.github.com/event-subscriptions/sub_1",
	EventsURL:   "https://api.github.com/event-subscriptions/sub_1/events",
}

func newTestClient(t *testing.T, host string) (EventClient, *httpmock.Registry) {
	reg := &httpmock.Registry{}
	t.Cleanup(func() { reg.Verify(t) })
	return NewEventClient(&http.Client{Transport: reg}, host), reg
}

func errorResponse(status int, message string) httpmock.Responder {
	return httpmock.StatusJSONResponse(status, map[string]string{"message": message})
}

func TestCreateSubscription(t *testing.T) {
	for _, status := range []int{http.StatusCreated, http.StatusOK} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client, reg := newTestClient(t, "github.com")
			var gotBody map[string]any
			reg.Register(httpmock.REST("POST", "event-subscriptions"), func(req *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(body, &gotBody))
				return httpmock.StatusStringResponse(status, subscriptionJSON)(req)
			})

			sub, created, err := client.CreateSubscription(context.Background(), "https://github.com/OWNER/REPO/pull/1",
				[]string{"test.a", "test.b"})

			require.NoError(t, err)
			assert.Equal(t, wantSubscription, sub)
			assert.Equal(t, status == http.StatusCreated, created)
			assert.Equal(t, map[string]any{
				"resource_url": "https://github.com/OWNER/REPO/pull/1",
				"event_types":  []any{"test.a", "test.b"},
			}, gotBody)
		})
	}
}

func TestGetSubscription(t *testing.T) {
	client, reg := newTestClient(t, "github.com")
	reg.Register(httpmock.REST("GET", "event-subscriptions/sub_1"), httpmock.StringResponse(subscriptionJSON))

	sub, err := client.GetSubscription(context.Background(), "sub_1")

	require.NoError(t, err)
	assert.Equal(t, wantSubscription, sub)
}

func TestListSubscriptions(t *testing.T) {
	page := func(link string, ids ...string) httpmock.Responder {
		subs := make([]map[string]string, 0, len(ids))
		for _, id := range ids {
			subs = append(subs, map[string]string{"id": id})
		}
		responder := httpmock.JSONResponse(map[string]any{"subscriptions": subs})
		if link == "" {
			return responder
		}
		return httpmock.WithHeader(responder, "Link", link)
	}
	pageMatcher := func(perPage, page string) httpmock.Matcher {
		query := url.Values{"per_page": {perPage}}
		if page != "" {
			query.Set("page", page)
		}
		rest := httpmock.REST("GET", "event-subscriptions")
		paged := httpmock.QueryMatcher("GET", "event-subscriptions", query)
		return func(req *http.Request) bool {
			return rest(req) && paged(req) && req.URL.Query().Has("page") == (page != "")
		}
	}
	nextLink := func(page string) string {
		return `<https://api.github.com/event-subscriptions?page=` + page + `&per_page=3>; rel="next", ` +
			`<https://api.github.com/event-subscriptions?page=9&per_page=3>; rel="last"`
	}

	tests := []struct {
		name    string
		limit   int
		stubs   func(*httpmock.Registry)
		wantIDs []string
	}{
		{
			name:  "follows next links and truncates to the limit",
			limit: 3,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(pageMatcher("3", ""), page(nextLink("2"), "sub_1", "sub_2"))
				reg.Register(pageMatcher("3", "2"), page(nextLink("3"), "sub_3", "sub_4"))
			},
			wantIDs: []string{"sub_1", "sub_2", "sub_3"},
		},
		{
			name:  "stops at the last page",
			limit: 30,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(pageMatcher("30", ""), page("", "sub_1"))
			},
			wantIDs: []string{"sub_1"},
		},
		{
			name:  "stops at an empty page despite a next link",
			limit: 30,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(pageMatcher("30", ""), page(nextLink("2")))
			},
			wantIDs: []string{},
		},
		{
			name:  "caps the page size",
			limit: 150,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(pageMatcher("100", ""), page("", "sub_1"))
			},
			wantIDs: []string{"sub_1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, reg := newTestClient(t, "github.com")
			tt.stubs(reg)

			subs, err := client.ListSubscriptions(context.Background(), tt.limit)

			require.NoError(t, err)
			gotIDs := []string{}
			for _, sub := range subs {
				gotIDs = append(gotIDs, sub.ID)
			}
			assert.Equal(t, tt.wantIDs, gotIDs)
		})
	}
}

func TestListSubscriptionsDecodesFields(t *testing.T) {
	client, reg := newTestClient(t, "github.com")
	reg.Register(httpmock.REST("GET", "event-subscriptions"),
		httpmock.StringResponse(`{"subscriptions": [`+subscriptionJSON+`]}`))

	subs, err := client.ListSubscriptions(context.Background(), 30)

	require.NoError(t, err)
	assert.Equal(t, []Subscription{*wantSubscription}, subs)
}

func TestDeleteSubscription(t *testing.T) {
	client, reg := newTestClient(t, "github.com")
	reg.Register(httpmock.REST("DELETE", "event-subscriptions/sub_1"), httpmock.StatusStringResponse(http.StatusNoContent, ""))

	err := client.DeleteSubscription(context.Background(), "sub_1")

	require.NoError(t, err)
}

func TestClientErrors(t *testing.T) {
	create := func(c EventClient) error {
		_, _, err := c.CreateSubscription(context.Background(), "https://github.com/OWNER/REPO/pull/1", []string{"test.a"})
		return err
	}
	get := func(c EventClient) error {
		_, err := c.GetSubscription(context.Background(), "sub_1")
		return err
	}
	del := func(c EventClient) error {
		return c.DeleteSubscription(context.Background(), "sub_1")
	}
	list := func(c EventClient) error {
		_, err := c.ListSubscriptions(context.Background(), 30)
		return err
	}
	stream := func(c EventClient) error {
		_, err := c.OpenStream(context.Background(), "sub_1")
		return err
	}

	tests := []struct {
		name        string
		call        func(EventClient) error
		matcher     httpmock.Matcher
		responder   httpmock.Responder
		wantErr     error
		wantMessage string
		wantHint    string
	}{
		{
			name:        "create on a missing resource",
			call:        create,
			matcher:     httpmock.REST("POST", "event-subscriptions"),
			responder:   errorResponse(http.StatusNotFound, "Not Found"),
			wantErr:     ErrResourceNotFound,
			wantMessage: "Not Found",
		},
		{
			name:    "create with an invalid field",
			call:    create,
			matcher: httpmock.REST("POST", "event-subscriptions"),
			responder: httpmock.StatusJSONResponse(http.StatusUnprocessableEntity, map[string]any{
				"message": "Validation Failed",
				"errors":  []map[string]string{{"resource": "EventSubscription", "field": "resource_url", "code": "invalid"}},
			}),
			wantMessage: "Validation Failed\nEventSubscription.resource_url is invalid",
			wantHint:    "invalid subscription request",
		},
		{
			name:        "create rejected without invalid fields",
			call:        create,
			matcher:     httpmock.REST("POST", "event-subscriptions"),
			responder:   errorResponse(http.StatusUnprocessableEntity, "Subscription limit exceeded"),
			wantErr:     ErrSubscriptionLimit,
			wantMessage: "Subscription limit exceeded",
			wantHint:    "request rejected, or the subscription limit may have been reached",
		},
		{
			name:        "create forbidden",
			call:        create,
			matcher:     httpmock.REST("POST", "event-subscriptions"),
			responder:   errorResponse(http.StatusForbidden, "Forbidden"),
			wantMessage: "Forbidden",
			wantHint:    "access forbidden",
		},
		{
			name:        "create without preview access",
			call:        create,
			matcher:     httpmock.REST("POST", "event-subscriptions"),
			responder:   errorResponse(http.StatusForbidden, "Event subscriptions are not enabled for this account"),
			wantErr:     ErrPreviewNotEnabled,
			wantMessage: "Event subscriptions are not enabled for this account",
			wantHint:    "event subscriptions are in preview and not enabled for this account",
		},
		{
			name:        "get without preview access",
			call:        get,
			matcher:     httpmock.REST("GET", "event-subscriptions/sub_1"),
			responder:   errorResponse(http.StatusForbidden, "Event subscriptions are not enabled for this account"),
			wantErr:     ErrPreviewNotEnabled,
			wantMessage: "Event subscriptions are not enabled for this account",
			wantHint:    "event subscriptions are in preview and not enabled for this account",
		},
		{
			name:        "list without preview access",
			call:        list,
			matcher:     httpmock.REST("GET", "event-subscriptions"),
			responder:   errorResponse(http.StatusForbidden, "Event subscriptions are not enabled for this account"),
			wantErr:     ErrPreviewNotEnabled,
			wantMessage: "Event subscriptions are not enabled for this account",
			wantHint:    "event subscriptions are in preview and not enabled for this account",
		},
		{
			name:        "delete without preview access",
			call:        del,
			matcher:     httpmock.REST("DELETE", "event-subscriptions/sub_1"),
			responder:   errorResponse(http.StatusForbidden, "Event subscriptions are not enabled for this account"),
			wantErr:     ErrPreviewNotEnabled,
			wantMessage: "Event subscriptions are not enabled for this account",
			wantHint:    "event subscriptions are in preview and not enabled for this account",
		},
		{
			name:        "stream without preview access",
			call:        stream,
			matcher:     httpmock.REST("GET", "event-subscriptions/sub_1/events"),
			responder:   errorResponse(http.StatusForbidden, "Event subscriptions are not enabled for this account"),
			wantErr:     ErrPreviewNotEnabled,
			wantMessage: "Event subscriptions are not enabled for this account",
			wantHint:    "event subscriptions are in preview and not enabled for this account",
		},
		{
			name:        "get a missing subscription",
			call:        get,
			matcher:     httpmock.REST("GET", "event-subscriptions/sub_1"),
			responder:   errorResponse(http.StatusNotFound, "Not Found"),
			wantErr:     ErrSubscriptionNotFound,
			wantMessage: "Not Found",
		},
		{
			name:        "delete a missing subscription",
			call:        del,
			matcher:     httpmock.REST("DELETE", "event-subscriptions/sub_1"),
			responder:   errorResponse(http.StatusNotFound, "Not Found"),
			wantErr:     ErrSubscriptionNotFound,
			wantMessage: "Not Found",
		},
		{
			name:        "stream a missing subscription",
			call:        stream,
			matcher:     httpmock.REST("GET", "event-subscriptions/sub_1/events"),
			responder:   errorResponse(http.StatusNotFound, "Not Found"),
			wantErr:     ErrSubscriptionNotFound,
			wantMessage: "Not Found",
		},
	}

	sentinels := []error{ErrUnsupportedHost, ErrResourceNotFound, ErrSubscriptionNotFound, ErrPreviewNotEnabled}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, reg := newTestClient(t, "github.com")
			reg.Register(tt.matcher, tt.responder)

			err := tt.call(client)

			var httpErr api.HTTPError
			require.ErrorAs(t, err, &httpErr)
			assert.Equal(t, tt.wantMessage, httpErr.Message)
			if tt.wantHint != "" {
				require.ErrorContains(t, err, tt.wantHint+": HTTP ")
			}
			for _, sentinel := range sentinels {
				if sentinel == tt.wantErr {
					require.ErrorIs(t, err, sentinel)
				} else {
					require.NotErrorIs(t, err, sentinel)
				}
			}
		})
	}
}

func TestSubscriptionResponseBody(t *testing.T) {
	calls := map[string]struct {
		method, path string
		call         func(EventClient) error
	}{
		"create": {"POST", "event-subscriptions", func(c EventClient) error {
			_, _, err := c.CreateSubscription(context.Background(), "https://github.com/OWNER/REPO/pull/1", []string{"test.a"})
			return err
		}},
	}
	for name, c := range calls {
		t.Run(name+" with a body cut short fails like a request in transit", func(t *testing.T) {
			client, reg := newTestClient(t, "github.com")
			reg.Register(httpmock.REST(c.method, c.path), func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK, Request: req,
					Body: io.NopCloser(io.MultiReader(strings.NewReader(`{"id": "sub`), iotest.ErrReader(io.ErrUnexpectedEOF))),
				}, nil
			})

			err := c.call(client)

			_, ok := errors.AsType[*url.Error](err)
			assert.True(t, ok, "got %v", err)
			assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
		})
		t.Run(name+" with a body that is not a subscription fails to decode", func(t *testing.T) {
			client, reg := newTestClient(t, "github.com")
			reg.Register(httpmock.REST(c.method, c.path), httpmock.StringResponse(`{"id": "sub`))

			err := c.call(client)

			require.ErrorContains(t, err, "failed to decode response")
			_, ok := errors.AsType[*url.Error](err)
			assert.False(t, ok)
		})
	}
}

func TestOpenStream(t *testing.T) {
	streamResponse := func(contentType string) httpmock.Responder {
		return func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "text/event-stream", req.Header.Get("Accept"))
			resp, err := httpmock.StringResponse(": heartbeat\n\n")(req)
			resp.Header.Set("Content-Type", contentType)
			return resp, err
		}
	}

	t.Run("returns the stream body", func(t *testing.T) {
		client, reg := newTestClient(t, "github.com")
		reg.Register(httpmock.REST("GET", "event-subscriptions/sub_1/events"),
			streamResponse("text/event-stream; charset=utf-8"))

		body, err := client.OpenStream(context.Background(), "sub_1")

		require.NoError(t, err)
		defer body.Close()
		got, err := io.ReadAll(body)
		require.NoError(t, err)
		assert.Equal(t, ": heartbeat\n\n", string(got))
	})

	t.Run("rejects a response that is not an event stream", func(t *testing.T) {
		client, reg := newTestClient(t, "github.com")
		reg.Register(httpmock.REST("GET", "event-subscriptions/sub_1/events"), streamResponse("application/json"))

		_, err := client.OpenStream(context.Background(), "sub_1")

		require.ErrorContains(t, err, `unexpected event stream content type "application/json"`)
	})
}

func TestClientUnsupportedHost(t *testing.T) {
	for _, host := range []string{"ghe.example.com", "example.ghe.com"} {
		t.Run(host, func(t *testing.T) {
			client, _ := newTestClient(t, host)

			_, err := client.GetSubscription(context.Background(), "sub_1")

			require.ErrorIs(t, err, ErrUnsupportedHost)
		})
	}
}
