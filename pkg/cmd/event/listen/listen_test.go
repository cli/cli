package listen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmd/event/client"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdListen(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		wantOpts ListenOptions
		wantErr  string
	}{
		{
			name: "resource and events",
			args: "--resource https://github.com/OWNER/REPO/pull/123/files --event foo,bar --count 1 --timeout 5m",
			wantOpts: ListenOptions{
				Resource: "https://github.com/OWNER/REPO/pull/123/files",
				Events:   []string{fooType, barType},
				Count:    1,
				Timeout:  5 * time.Minute,
			},
		},
		{
			name:     "subscription",
			args:     "--subscription sub_1",
			wantOpts: ListenOptions{Subscription: "sub_1"},
		},
		{
			name:    "neither resource nor subscription",
			args:    "--event foo",
			wantErr: "either `--resource` or `--subscription` is required",
		},
		{
			name:    "resource and subscription",
			args:    "--resource https://github.com/OWNER/REPO/pull/1 --subscription sub_1",
			wantErr: "specify only one of `--resource` or `--subscription`",
		},
		{
			name:    "resource without events",
			args:    "--resource https://github.com/OWNER/REPO/pull/1",
			wantErr: "`--event` is required with `--resource`",
		},
		{
			name:    "events with subscription",
			args:    "--subscription sub_1 --event foo",
			wantErr: "`--event` cannot be used with `--subscription`",
		},
		{
			name:    "jq and template",
			args:    "--subscription sub_1 --jq .id --template {{.id}}",
			wantErr: "specify only one of `--jq` or `--template`",
		},
		{
			name:     "follow",
			args:     "--subscription sub_1 --follow",
			wantOpts: ListenOptions{Subscription: "sub_1", Follow: true},
		},
		{
			name:    "zero count",
			args:    "--resource https://github.com/OWNER/REPO/pull/1 --event foo --count 0",
			wantErr: "`--count` must be at least 1",
		},
		{
			name:    "count and follow",
			args:    "--subscription sub_1 --count 2 --follow",
			wantErr: "specify only one of `--count` or `--follow`",
		},
		{
			name:    "zero timeout",
			args:    "--subscription sub_1 --timeout 0s",
			wantErr: "`--timeout` must be positive",
		},
		{
			name:     "timeout in seconds",
			args:     "--subscription sub_1 --timeout 600",
			wantOpts: ListenOptions{Subscription: "sub_1", Timeout: 10 * time.Minute},
		},
		{
			name:    "invalid timeout",
			args:    "--subscription sub_1 --timeout soon",
			wantErr: "invalid argument \"soon\" for \"--timeout\" flag: invalid duration \"soon\"; use seconds or a duration such as 30s or 10m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			var gotOpts *ListenOptions
			cmd := NewCmdListen(&cmdutil.Factory{IOStreams: ios}, func(opts *ListenOptions) error {
				gotOpts = opts
				return nil
			})
			args, err := shlex.Split(tt.args)
			require.NoError(t, err)
			cmd.SetArgs(args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err = cmd.ExecuteC()

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOpts.Resource, gotOpts.Resource)
			assert.Equal(t, tt.wantOpts.Events, gotOpts.Events)
			assert.Equal(t, tt.wantOpts.Subscription, gotOpts.Subscription)
			assert.Equal(t, tt.wantOpts.Count, gotOpts.Count)
			assert.Equal(t, tt.wantOpts.Follow, gotOpts.Follow)
			assert.Equal(t, tt.wantOpts.Timeout, gotOpts.Timeout)
			assert.Equal(t, tt.wantOpts.Jq, gotOpts.Jq)
		})
	}
}

const (
	fooType = "foo"
	barType = "bar"
	fooLine = `{"id":"e1","data":{"api_url":"https://api.github.com/repos/OWNER/REPO/pulls/1/reviews/7"}}` + "\n"
	barLine = `{"id":"e2","data":{"api_url":"https://api.github.com/repos/OWNER/REPO/pulls/1"}}` + "\n"

	resubscribeAdviceMsg = "To keep listening, run the command again with `--resource` and the same `--event` types the subscription was created with."
)

var (
	fooEvent = heredoc.Doc(`
		id: e1
		event: foo
		data: {"id": "e1", "data": {"api_url": "https://api.github.com/repos/OWNER/REPO/pulls/1/reviews/7"}}

	`)
	barEvent = heredoc.Doc(`
		id: e2
		event: bar
		data: {"id":"e2","data":{"api_url":"https://api.github.com/repos/OWNER/REPO/pulls/1"}}

	`)
)

var (
	notFoundErr    = httpError(http.StatusNotFound, "Not Found", nil)
	unavailableErr = httpError(http.StatusServiceUnavailable, "Service Unavailable", nil)
	errLocal       = errors.New(`path component ".." would traverse the URL path`)
	// The client tags API errors with sentinels; these stand in for the tagged errors.
	subNotFoundErr = fmt.Errorf("%w: %w", notFoundErr, client.ErrSubscriptionNotFound)
)

func httpError(status int, message string, headers http.Header) api.HTTPError {
	return api.HTTPError{HTTPError: &ghAPI.HTTPError{
		StatusCode: status,
		Message:    message,
		Headers:    headers,
		RequestURL: &url.URL{Scheme: "https", Host: "api.github.com", Path: "/event-subscriptions"},
	}}
}

// blockStream in the streams of a test case is an idle stream that delivers nothing until it is closed.
const blockStream = "<block>"

func endFrame(outcome, reason string) string {
	return heredoc.Docf(`
		event: github.stream.end
		data: {"outcome":%q,"reason":%q}

	`, outcome, reason)
}

// blockingStream stands in for an idle stream: reads block until the request context is done or it is closed.
type blockingStream struct {
	ctx    context.Context
	closed chan struct{}
}

func newBlockingStream(ctx context.Context) *blockingStream {
	return &blockingStream{ctx: ctx, closed: make(chan struct{})}
}

func (s *blockingStream) Read([]byte) (int, error) {
	select {
	case <-s.ctx.Done():
		return 0, s.ctx.Err()
	case <-s.closed:
		return 0, errors.New("read on closed body")
	}
}

func (s *blockingStream) Close() error {
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}
	return nil
}

func TestListenRun(t *testing.T) {
	origSleep := sleepFunc
	sleepFunc = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	t.Cleanup(func() { sleepFunc = origSleep })

	const resource = "https://github.com/OWNER/REPO/pull/1"

	tests := []struct {
		name         string
		opts         ListenOptions
		color        bool
		streams      []string
		openErrs     []error
		createErrs   []error
		wantErr      error
		wantErrMsg   string
		wantStdout   string
		wantStderr   string
		wantCreates  int
		wantCreateTo []string
	}{
		{
			name:       "prints events as compact JSON lines until the count is reached",
			opts:       ListenOptions{Resource: resource, Events: []string{fooType, barType}, Count: 2},
			streams:    []string{": heartbeat\n\n" + fooEvent + barEvent + fooEvent},
			wantStdout: fooLine + barLine,
			wantStderr: heredoc.Doc(`
				Listening for events.
				Changes made before this point are not reported; read the current state now.
			`),
			wantCreates:  1,
			wantCreateTo: []string{fooType, barType},
		},
		{
			name:       "exits after the first event by default and explains how to wait for the next one",
			opts:       ListenOptions{Resource: resource, Events: []string{fooType, barType}},
			streams:    []string{fooEvent + barEvent},
			wantStdout: fooLine,
			wantStderr: heredoc.Doc(`
				Listening for events.
				Changes made before this point are not reported; read the current state now.
				Exited after the first event. Run the command again to wait for the next event.
			`),
			wantCreates: 1,
		},
		{
			name:       "keeps listening with follow until the stream ends",
			opts:       ListenOptions{Subscription: "sub_1", Follow: true},
			streams:    []string{fooEvent + barEvent + endFrame("subscription_ended", "expired")},
			wantStdout: fooLine + barLine,
			wantErrMsg: "subscription sub_1 expired\n" + resubscribeAdviceMsg,
			wantStderr: heredoc.Doc(`
				Listening for events.
				Changes made before this point are not reported; read the current state now.
			`),
		},
		{
			name:       "reconnects when the stream ends",
			opts:       ListenOptions{Resource: resource, Events: []string{fooType}, Count: 2},
			streams:    []string{fooEvent + endFrame("reconnect", "shutdown"), "", barEvent},
			wantStdout: fooLine + barLine,
			wantStderr: heredoc.Doc(`
				Listening for events.
				Changes made before this point are not reported; read the current state now.
				Event stream ended (shutdown), reconnecting.
				Listening for events.
				Changes made before this point are not reported; read the current state now.
				Event stream closed, reconnecting.
				Listening for events.
				Changes made before this point are not reported; read the current state now.
			`),
			wantCreates: 1,
		},
		{
			name:       "creates the subscription again when it expires",
			opts:       ListenOptions{Resource: resource, Events: []string{fooType}, Count: 1},
			streams:    []string{endFrame("subscription_ended", "expired"), fooEvent},
			wantStdout: fooLine,
			wantStderr: heredoc.Doc(`
				Listening for events.
				Changes made before this point are not reported; read the current state now.
				Subscription expired, creating it again.
				Listening for events on a new subscription.
				Changes made since the previous subscription ended are not reported; read the current state now.
			`),
			wantCreates: 2,
		},
		{
			name:       "stops and advises listening by resource when a subscription passed by ID expires",
			opts:       ListenOptions{Subscription: "sub_1"},
			streams:    []string{endFrame("subscription_ended", "expired")},
			wantErrMsg: "subscription sub_1 expired\n" + resubscribeAdviceMsg,
			wantStderr: heredoc.Doc(`
				Listening for events.
				Changes made before this point are not reported; read the current state now.
			`),
		},
		{
			name:       "stops without creating the subscription again when it is deleted",
			opts:       ListenOptions{Resource: resource, Events: []string{fooType}},
			streams:    []string{endFrame("subscription_ended", "deleted")},
			wantErrMsg: "subscription sub_1 was deleted\nTo keep listening, run the command again to create a new subscription.",
			wantStderr: heredoc.Doc(`
				Listening for events.
				Changes made before this point are not reported; read the current state now.
			`),
			wantCreates: 1,
		},
		{
			name:       "stops and advises listening by resource when a subscription passed by ID is deleted",
			opts:       ListenOptions{Subscription: "sub_1"},
			streams:    []string{endFrame("subscription_ended", "deleted")},
			wantErrMsg: "subscription sub_1 was deleted\n" + resubscribeAdviceMsg,
			wantStderr: heredoc.Doc(`
				Listening for events.
				Changes made before this point are not reported; read the current state now.
			`),
		},
		{
			name:       "stops when access is revoked",
			opts:       ListenOptions{Resource: resource, Events: []string{fooType}},
			streams:    []string{endFrame("subscription_ended", "access_revoked")},
			wantErrMsg: "the subscription ended because access to the resource was revoked",
			wantStderr: heredoc.Doc(`
				Listening for events.
				Changes made before this point are not reported; read the current state now.
			`),
			wantCreates: 1,
		},
		{
			name:        "stops when the stream cannot be attached",
			opts:        ListenOptions{Resource: resource, Events: []string{fooType}},
			openErrs:    []error{notFoundErr},
			wantErr:     notFoundErr,
			wantCreates: 1,
		},
		{
			name:       "creates the subscription again when the stream reports it missing",
			opts:       ListenOptions{Resource: resource, Events: []string{fooType}, Count: 1},
			openErrs:   []error{subNotFoundErr},
			streams:    []string{fooEvent},
			wantStdout: fooLine,
			wantStderr: heredoc.Doc(`
				Subscription not found, creating it again.
				Listening for events on a new subscription.
				Changes made since the previous subscription ended are not reported; read the current state now.
			`),
			wantCreates: 2,
		},
		{
			name:        "stops when the subscription is still missing after creating it again",
			opts:        ListenOptions{Resource: resource, Events: []string{fooType}},
			openErrs:    []error{subNotFoundErr, subNotFoundErr},
			wantErr:     client.ErrSubscriptionNotFound,
			wantStderr:  "Subscription not found, creating it again.\n",
			wantCreates: 2,
		},
		{
			name:       "stops when a subscription passed by ID is not found",
			opts:       ListenOptions{Subscription: "sub_1"},
			openErrs:   []error{subNotFoundErr},
			wantErrMsg: "subscription sub_1 was not found; it may have expired: " + subNotFoundErr.Error() + "\n" + resubscribeAdviceMsg,
		},
		{
			name:       "retries attaching the stream when the server is unavailable",
			opts:       ListenOptions{Resource: resource, Events: []string{fooType}, Count: 1},
			openErrs:   []error{unavailableErr},
			streams:    []string{fooEvent},
			wantStdout: fooLine,
			wantStderr: heredoc.Doc(`
				Could not attach the event stream, retrying: HTTP 503: Service Unavailable (https://api.github.com/event-subscriptions)
				Listening for events.
				Changes made before this point are not reported; read the current state now.
			`),
			wantCreates: 1,
		},
		{
			name: "retries attaching the stream after a network failure",
			opts: ListenOptions{Resource: resource, Events: []string{fooType}, Count: 1},
			openErrs: []error{&url.Error{
				Op: "Get", URL: "https://api.github.com/event-subscriptions/sub_1/events", Err: errors.New("connection reset"),
			}},
			streams:    []string{fooEvent},
			wantStdout: fooLine,
			wantStderr: heredoc.Doc(`
				Could not attach the event stream, retrying: Get "https://api.github.com/event-subscriptions/sub_1/events": connection reset
				Listening for events.
				Changes made before this point are not reported; read the current state now.
			`),
			wantCreates: 1,
		},
		{
			name:        "stops when the request fails before it is sent",
			opts:        ListenOptions{Resource: resource, Events: []string{fooType}, Count: 1},
			openErrs:    []error{errLocal},
			streams:     []string{fooEvent},
			wantErr:     errLocal,
			wantCreates: 1,
		},
		{
			name:       "retries creating the subscription when the server is unavailable",
			opts:       ListenOptions{Resource: resource, Events: []string{fooType}, Count: 1},
			createErrs: []error{unavailableErr},
			streams:    []string{fooEvent},
			wantStdout: fooLine,
			wantStderr: heredoc.Doc(`
				Could not create the subscription, retrying: HTTP 503: Service Unavailable (https://api.github.com/event-subscriptions)
				Listening for events.
				Changes made before this point are not reported; read the current state now.
			`),
			wantCreates: 2,
		},
		{
			name:       "names the commands that free a slot when the subscription limit is reached",
			opts:       ListenOptions{Resource: resource, Events: []string{fooType}, Count: 1},
			createErrs: []error{client.ErrSubscriptionLimit},
			wantErrMsg: "request rejected, or the subscription limit may have been reached\n" +
				"To free a slot, run `gh event subscription list` and delete one with `gh event subscription delete <id>`. " +
				"Deleting a subscription only stops its events, and every subscription expires within 24 hours anyway",
			wantCreates: 1,
		},
		{
			name:       "reconnects when the stream delivers nothing for the idle timeout",
			opts:       ListenOptions{Resource: resource, Events: []string{fooType}, Count: 1, idleTimeout: 20 * time.Millisecond},
			streams:    []string{blockStream, fooEvent},
			wantStdout: fooLine,
			wantStderr: heredoc.Doc(`
				Listening for events.
				Changes made before this point are not reported; read the current state now.
				Event stream failed, reconnecting: no data received for 20ms
				Listening for events.
				Changes made before this point are not reported; read the current state now.
			`),
			wantCreates: 1,
		},
		{
			name:    "filters each event with jq",
			opts:    ListenOptions{Resource: resource, Events: []string{fooType, barType}, Count: 2, Jq: ".data.api_url"},
			streams: []string{fooEvent + barEvent},
			wantStdout: heredoc.Doc(`
				https://api.github.com/repos/OWNER/REPO/pulls/1/reviews/7
				https://api.github.com/repos/OWNER/REPO/pulls/1
			`),
			wantStderr: heredoc.Doc(`
				Listening for events.
				Changes made before this point are not reported; read the current state now.
			`),
			wantCreates: 1,
		},
		{
			name: "sanitizes escape sequences in formatted output",
			opts: ListenOptions{Resource: resource, Events: []string{fooType}, Count: 1, Jq: ".data.title"},
			streams: []string{heredoc.Doc(`
				event: foo
				data: {"data": {"title": "\u001b[31mred"}}

			`)},
			wantStdout: "^[[31mred\n",
			wantStderr: heredoc.Doc(`
				Listening for events.
				Changes made before this point are not reported; read the current state now.
			`),
			wantCreates: 1,
		},
		{
			name:  "keeps colors from template helpers while sanitizing event data",
			opts:  ListenOptions{Resource: resource, Events: []string{fooType}, Count: 1, Template: `{{color "red" .data.title}}`},
			color: true,
			streams: []string{heredoc.Doc(`
				event: foo
				data: {"data": {"title": "\u001b[31mred"}}

			`)},
			wantStdout: "\x1b[0;31m^[[31mred\x1b[0m",
			wantStderr: heredoc.Doc(`
				Listening for events.
				Changes made before this point are not reported; read the current state now.
			`),
			wantCreates: 1,
		},
		{
			name:    "formats each event with a template",
			opts:    ListenOptions{Resource: resource, Events: []string{fooType, barType}, Count: 2, Template: "{{.id}} {{.data.api_url}}\n"},
			streams: []string{fooEvent + barEvent},
			wantStdout: heredoc.Doc(`
				e1 https://api.github.com/repos/OWNER/REPO/pulls/1/reviews/7
				e2 https://api.github.com/repos/OWNER/REPO/pulls/1
			`),
			wantStderr: heredoc.Doc(`
				Listening for events.
				Changes made before this point are not reported; read the current state now.
			`),
			wantCreates: 1,
		},
		{
			name: "rejects an invalid jq expression before subscribing",
			opts: ListenOptions{Resource: resource, Events: []string{fooType}, Jq: ".data |"},
			wantErrMsg: heredoc.Doc(`
				failed to parse jq expression (line 1, column 8)
				    .data |
				           ^  unexpected EOF`),
		},
		{
			name:       "stops when an event cannot be formatted",
			opts:       ListenOptions{Resource: resource, Events: []string{fooType}, Jq: ".id | tonumber"},
			streams:    []string{fooEvent, fooEvent},
			wantErrMsg: "tonumber cannot be applied to \"e1\": invalid number",
			wantStderr: heredoc.Doc(`
				Listening for events.
				Changes made before this point are not reported; read the current state now.
			`),
			wantCreates: 1,
		},
		{
			name:    "exits with the pending status when the timeout is reached",
			opts:    ListenOptions{Resource: resource, Events: []string{fooType}, Timeout: 20 * time.Millisecond},
			wantErr: cmdutil.PendingError,
			wantStderr: heredoc.Doc(`
				Listening for events.
				Changes made before this point are not reported; read the current state now.
				Timed out waiting for events.
			`),
			wantCreates: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetColorEnabled(tt.color)
			opened, failed, created := 0, 0, 0
			var createdTypes []string
			mock := &client.EventClientMock{
				CreateSubscriptionFunc: func(_ context.Context, resourceURL string, eventTypes []string) (*client.Subscription, bool, error) {
					assert.Equal(t, resource, resourceURL)
					createdTypes = eventTypes
					if created < len(tt.createErrs) {
						created++
						return nil, false, tt.createErrs[created-1]
					}
					return &client.Subscription{ID: "sub_1"}, true, nil
				},
				OpenStreamFunc: func(ctx context.Context, id string) (io.ReadCloser, error) {
					assert.Equal(t, "sub_1", id)
					if failed < len(tt.openErrs) {
						failed++
						return nil, tt.openErrs[failed-1]
					}
					opened++
					if opened > len(tt.streams) || tt.streams[opened-1] == blockStream {
						return newBlockingStream(ctx), nil
					}
					return io.NopCloser(strings.NewReader(tt.streams[opened-1])), nil
				},
			}
			opts := tt.opts
			opts.IO = ios
			opts.Config = func() (gh.Config, error) { return config.NewMockConfig(), nil }
			opts.EventClient = func(host string) (client.EventClient, error) {
				assert.Equal(t, "github.com", host)
				return mock, nil
			}

			err := listenRun(context.Background(), &opts)

			switch {
			case tt.wantErr != nil:
				require.ErrorIs(t, err, tt.wantErr)
			case tt.wantErrMsg != "":
				require.EqualError(t, err, tt.wantErrMsg)
			default:
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
			assert.Len(t, mock.CreateSubscriptionCalls(), tt.wantCreates)
			if tt.wantCreateTo != nil {
				assert.Equal(t, tt.wantCreateTo, createdTypes)
			}
		})
	}
}

func TestListenRunBackoffResetsOnlyAfterDelivery(t *testing.T) {
	var delays []time.Duration
	origSleep := sleepFunc
	sleepFunc = func(ctx context.Context, d time.Duration) error {
		delays = append(delays, d)
		return ctx.Err()
	}
	t.Cleanup(func() { sleepFunc = origSleep })

	// Streams closed before delivering anything keep backing off; a heartbeat or an event resets the delay.
	streams := []string{"", "", ": keep-alive\n\n", "", "", fooEvent, "", barEvent}
	opened := 0
	mock := &client.EventClientMock{
		CreateSubscriptionFunc: func(context.Context, string, []string) (*client.Subscription, bool, error) {
			return &client.Subscription{ID: "sub_1"}, true, nil
		},
		OpenStreamFunc: func(context.Context, string) (io.ReadCloser, error) {
			opened++
			return io.NopCloser(strings.NewReader(streams[opened-1])), nil
		},
	}
	ios, _, _, _ := iostreams.Test()
	opts := &ListenOptions{
		IO:          ios,
		Resource:    "https://github.com/OWNER/REPO/pull/1",
		Events:      []string{fooType},
		Count:       2,
		Config:      func() (gh.Config, error) { return config.NewMockConfig(), nil },
		EventClient: func(string) (client.EventClient, error) { return mock, nil },
	}

	require.NoError(t, listenRun(context.Background(), opts))
	s := time.Second
	assert.Equal(t, []time.Duration{1 * s, 2 * s, 1 * s, 2 * s, 4 * s, 1 * s, 2 * s}, delays)
}

func TestReadEventsSkipsInvalidJSON(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()
	l := &listener{opts: &ListenOptions{IO: ios, Follow: true}}

	invalidEvent := heredoc.Doc(`
		event: x
		data: {not json

	`)
	end, err := l.readEvents(bytes.NewBufferString(invalidEvent + fooEvent))

	require.NoError(t, err)
	assert.Nil(t, end)
	assert.Equal(t, fooLine, stdout.String())
	assert.Contains(t, stderr.String(), "warning: skipped an event that is not valid JSON")
}

func TestReadEventsPrintsEventBeforeMoreBytesArrive(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()
	l := &listener{opts: &ListenOptions{IO: ios}}

	// The stream sends one complete frame and then nothing more, as a server does between heartbeats.
	body, w := io.Pipe()
	t.Cleanup(func() { w.Close() })
	go func() { _, _ = w.Write([]byte(fooEvent)) }()

	done := make(chan error, 1)
	go func() {
		_, err := l.readEvents(body)
		done <- err
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, errCountReached)
		assert.Equal(t, fooLine, stdout.String())
	case <-time.After(5 * time.Second):
		w.Close()
		<-done
		t.Fatal("the event was not printed until the stream sent more bytes")
	}
}

func TestReadEventsSanitizesEachFrame(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()
	l := &listener{opts: &ListenOptions{IO: ios, Follow: true}}

	stream := "event: x\ndata: {\"title\": \"\x1b[31mraw\", \"escaped\": \"\\u001b[31m\"}\n\n" +
		"event: x\ndata: {\"title\": \"\xff\"}\n\n" +
		fooEvent
	end, err := l.readEvents(strings.NewReader(stream))

	require.NoError(t, err)
	assert.Nil(t, end)
	assert.Equal(t, `{"title":"^[[31mraw","escaped":"^[[31m"}`+"\n"+fooLine, stdout.String())
	assert.Contains(t, stderr.String(), "warning: skipped an event that is not valid UTF-8")
}

func TestReadEventsSanitizesEndFrame(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()
	l := &listener{opts: &ListenOptions{IO: ios}}

	stream := "event: github.stream.end\ndata: {\"outcome\":\"subscription_ended\",\"reason\":\"\x1b[31mraw \\u001b[31mescaped\"}\n\n"
	end, err := l.readEvents(strings.NewReader(stream))

	require.NoError(t, err)
	assert.Equal(t, &streamEnd{Outcome: "subscription_ended", Reason: "^[[31mraw ^[[31mescaped"}, end)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestReadEventsEndFrameNotValidUTF8(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()
	l := &listener{opts: &ListenOptions{IO: ios, Follow: true}}

	stream := "event: github.stream.end\ndata: {\"outcome\":\"subscription_ended\",\"reason\":\"\xff\"}\n\n" + fooEvent
	end, err := l.readEvents(strings.NewReader(stream))

	require.ErrorContains(t, err, "invalid end of stream frame: invalid UTF-8 string")
	assert.Nil(t, end)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestRetryAfter(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want time.Duration
	}{
		{
			name: "seconds from the header",
			err:  httpError(http.StatusServiceUnavailable, "", http.Header{"Retry-After": {"3"}}),
			want: 3 * time.Second,
		},
		{
			name: "wrapped HTTP error",
			err:  fmt.Errorf("wrapped: %w", httpError(http.StatusServiceUnavailable, "", http.Header{"Retry-After": {"2"}})),
			want: 2 * time.Second,
		},
		{
			name: "no header",
			err:  unavailableErr,
		},
		{
			name: "date in the past",
			err:  httpError(http.StatusServiceUnavailable, "", http.Header{"Retry-After": {"Wed, 21 Oct 2015 07:28:00 GMT"}}),
		},
		{
			name: "not a number or a date",
			err:  httpError(http.StatusServiceUnavailable, "", http.Header{"Retry-After": {"soon"}}),
		},
		{
			name: "not an HTTP error",
			err:  errors.New("connection reset"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, retryAfter(tt.err))
		})
	}
}

func TestRetryAfterFutureDate(t *testing.T) {
	date := time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)
	err := httpError(http.StatusServiceUnavailable, "", http.Header{"Retry-After": {date}})

	got := retryAfter(err)

	// The header has a resolution of one second, so up to a second of the hour may already have passed.
	assert.Greater(t, got, time.Hour-2*time.Second)
	assert.LessOrEqual(t, got, time.Hour)
}

func TestListenRunResourceHost(t *testing.T) {
	tests := []struct {
		name     string
		resource string
		wantHost string
	}{
		{name: "host of the resource URL", resource: "https://GHE.example.com/OWNER/REPO/pull/1", wantHost: "ghe.example.com"},
		{name: "default host for a resource that is not a URL", resource: "not-a-url", wantHost: "github.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			errStop := errors.New("stop")
			var gotHost string
			opts := &ListenOptions{
				IO:       ios,
				Resource: tt.resource,
				Events:   []string{fooType},
				Config:   func() (gh.Config, error) { return config.NewMockConfig(), nil },
				EventClient: func(host string) (client.EventClient, error) {
					gotHost = host
					return nil, errStop
				},
			}

			err := listenRun(context.Background(), opts)

			require.ErrorIs(t, err, errStop)
			assert.Equal(t, tt.wantHost, gotHost)
		})
	}
}
