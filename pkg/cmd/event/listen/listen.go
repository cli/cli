// Package listen implements the gh event listen command.
package listen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/eventstream"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmd/event/client"
	"github.com/cli/cli/v2/pkg/cmd/event/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/go-gh/v2/pkg/asciisanitizer"
	"github.com/cli/go-gh/v2/pkg/jq"
	"github.com/cli/go-gh/v2/pkg/template"
	"github.com/spf13/cobra"
	"golang.org/x/text/transform"
)

const (
	readyMessage      = "Listening for events."
	resubscribedReady = "Listening for events on a new subscription."
	readyNote         = "Changes made before this point are not reported; read the current state now."
	resubscribedNote  = "Changes made since the previous subscription ended are not reported; read the current state now."
	firstEventNote    = "Exited after the first event. Run the command again to wait for the next event."
	// The command does not fetch the subscription, so it cannot name its event types.
	resubscribeAdvice = "To keep listening, run the command again with `--resource` and the same `--event` types the subscription was created with."
	rerunAdvice       = "To keep listening, run the command again to create a new subscription."
	endFrameEvent     = "github.stream.end"
	minReconnectDelay = time.Second
	maxReconnectDelay = 30 * time.Second
	// The server sends a heartbeat every 9 seconds, so a stream without data for this long is dead.
	defaultIdleTimeout = 45 * time.Second
)

var errCountReached = errors.New("event count reached")

// ListenOptions holds the options of gh event listen.
type ListenOptions struct {
	IO          *iostreams.IOStreams
	Config      func() (gh.Config, error)
	EventClient func(host string) (client.EventClient, error)

	Resource     string
	Events       []string
	Subscription string
	Count        int
	Follow       bool
	Timeout      time.Duration
	Jq           string
	Template     string

	idleTimeout time.Duration
}

func NewCmdListen(f *cmdutil.Factory, runF func(*ListenOptions) error) *cobra.Command {
	opts := &ListenOptions{
		IO:          f.IOStreams,
		Config:      f.Config,
		EventClient: shared.EventClientFunc(f),
		idleTimeout: defaultIdleTimeout,
	}

	cmd := &cobra.Command{
		Use:        "listen",
		SuggestFor: []string{"watch"},
		Short:      "Wait for events on a specific resource",
		Long: heredoc.Docf(`
			Listen to events on a pull request and print each event to stdout as it happens, as one
			CloudEvent JSON object per line.

			Status messages go to stderr, so stdout carries only events. Use this to wait for changes instead of polling.

			By default, the command exits after the first event. Use %[1]s--count%[1]s to exit after a number of
			events, or %[1]s--follow%[1]s to keep listening until it is stopped. Use %[1]s--timeout%[1]s to limit how
			long it listens. When the timeout is reached first, the command exits with status 8.

			Pass a pull request URL with %[1]s--resource%[1]s and the event types to listen to with %[1]s--event%[1]s, repeating the flag
			or separating types with commas:

			%[2]s

			Events are thin single line JSON objects with their %[1]sdata.api_url%[1]s pointing at the object that changed,
			such as the review, the pull request, or the check suite. Fetch it to learn what changed.

			An event can occasionally be delivered more than once. Reading the current state after each event, as above,
			keeps a repeated event harmless.

			A pull request usually has several check suites, and each one sends its own event when it completes, with
			its %[1]sdata.conclusion%[1]s, such as %[1]ssuccess%[1]s or %[1]sfailure%[1]s. To wait for all checks,
			read the pull request checks after each event, for example with %[1]sgh pr checks%[1]s.

			Use %[1]s--jq%[1]s or %[1]s--template%[1]s to format each event; see %[1]sgh help formatting%[1]s.

			A status message is printed each time the event stream is attached. Earlier events are not
			replayed: only events that happen after that are delivered, so read the current state of the pull
			request once the message is printed, even if you read it before.

			The command creates a subscription for the resource and event types, or reuses an
			existing one. Subscriptions expire after 24 hours, and each account can have a limited number
			of them at a time. To listen to an existing subscription, pass its ID with %[1]s--subscription%[1]s.

			If the event stream breaks, the command reconnects. Events that happen while it is disconnected are not delivered.

			Events are only available for pull requests on %[1]sgithub.com%[1]s, not on GitHub Enterprise Server or GHE.com.
		`, "`", shared.EventTypesHelp()),
		Example: heredoc.Doc(`
			# Wait for the next review on a pull request, for up to 30 minutes
			$ gh event listen --resource https://github.com/OWNER/REPO/pull/123 --event com.github.pull_request.review.submitted.v1 --timeout 30m

			# Wait for a pull request to be merged or closed
			$ gh event listen --resource https://github.com/OWNER/REPO/pull/123 --event com.github.pull_request.state.changed.v1

			# Wait for the next check suite to complete and print its conclusion
			$ gh event listen --resource https://github.com/OWNER/REPO/pull/123 --event com.github.pull_request.check_suite.status.changed.v1 --jq .data.conclusion

			# Keep printing reviews and state changes until stopped
			$ gh event listen --resource https://github.com/OWNER/REPO/pull/123 --follow --event com.github.pull_request.review.submitted.v1,com.github.pull_request.state.changed.v1

			# Print the type and API URL of each event until stopped
			$ gh event listen --resource https://github.com/OWNER/REPO/pull/123 --follow --event com.github.pull_request.review.submitted.v1,com.github.pull_request.state.changed.v1 --jq '[.type, .data.api_url] | @tsv'
		`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmdutil.MutuallyExclusive(
				"specify only one of `--resource` or `--subscription`",
				opts.Resource != "", opts.Subscription != "",
			); err != nil {
				return err
			}
			if opts.Subscription == "" {
				if opts.Resource == "" {
					return cmdutil.FlagErrorf("either `--resource` or `--subscription` is required")
				}
				if len(opts.Events) == 0 {
					return cmdutil.FlagErrorf("`--event` is required with `--resource`")
				}
			} else if len(opts.Events) > 0 {
				return cmdutil.FlagErrorf("`--event` cannot be used with `--subscription`")
			}
			if err := cmdutil.MutuallyExclusive(
				"specify only one of `--jq` or `--template`",
				opts.Jq != "", opts.Template != "",
			); err != nil {
				return err
			}
			if err := cmdutil.MutuallyExclusive(
				"specify only one of `--count` or `--follow`",
				cmd.Flags().Changed("count"), opts.Follow,
			); err != nil {
				return err
			}
			if cmd.Flags().Changed("count") && opts.Count < 1 {
				return cmdutil.FlagErrorf("`--count` must be at least 1")
			}
			if cmd.Flags().Changed("timeout") && opts.Timeout <= 0 {
				return cmdutil.FlagErrorf("`--timeout` must be positive")
			}

			if runF != nil {
				return runF(opts)
			}
			return listenRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.Resource, "resource", "", "URL of the pull request to listen to")
	cmd.Flags().StringSliceVar(&opts.Events, "event", nil, "Event `type` to listen to; repeat or separate with commas for more than one")
	shared.RegisterEventTypesCompletion(cmd, "event")
	cmd.Flags().StringVar(&opts.Subscription, "subscription", "", "ID of an existing subscription to listen to")
	cmd.Flags().IntVar(&opts.Count, "count", 0, "Exit after this many events (default 1)")
	cmd.Flags().BoolVar(&opts.Follow, "follow", false, "Keep listening for events until stopped")
	cmd.Flags().Var((*secondsDuration)(&opts.Timeout), "timeout",
		"Exit with status 8 after this long, in seconds or as a duration such as 30s or 10m")
	cmd.Flags().StringVarP(&opts.Jq, "jq", "q", "", "Filter each event using a jq `expression`")
	cmd.Flags().StringVarP(&opts.Template, "template", "t", "", "Format each event using a Go template; see \"gh help formatting\"")
	return cmd
}

// secondsDuration is a duration flag that also accepts a bare number of seconds, such as 600.
type secondsDuration time.Duration

func (d *secondsDuration) String() string {
	// pflag hides the default only for zero-like strings, which do not include "0s".
	if *d == 0 {
		return "0"
	}
	return time.Duration(*d).String()
}

func (d *secondsDuration) Set(s string) error {
	if seconds, err := strconv.ParseFloat(s, 64); err == nil {
		*d = secondsDuration(seconds * float64(time.Second))
		return nil
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q; use seconds or a duration such as 30s or 10m", s)
	}
	*d = secondsDuration(v)
	return nil
}

func (d *secondsDuration) Type() string {
	return "duration"
}

func listenRun(ctx context.Context, opts *ListenOptions) error {
	l := &listener{opts: opts}

	// Check the output format before subscribing, so a typo does not wait for the first event to fail.
	if opts.Jq != "" {
		// An empty input fails to decode only after the expression was parsed and compiled.
		err := jq.Evaluate(strings.NewReader(""), io.Discard, opts.Jq)
		if _, isInputErr := errors.AsType[*json.SyntaxError](err); err != nil && !isInputErr {
			return err
		}
	}
	if opts.Template != "" {
		l.template = template.New(opts.IO.Out, opts.IO.TerminalWidth(), opts.IO.ColorEnabled())
		if err := l.template.Parse(opts.Template); err != nil {
			return err
		}
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	host, _ := cfg.Authentication().DefaultHost()
	if opts.Subscription == "" {
		host = shared.ResourceHost(opts.Resource, host)
		l.eventTypes = shared.NormalizeEventTypes(opts.Events)
	}

	if l.client, err = opts.EventClient(host); err != nil {
		return err
	}

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	err = l.listen(ctx)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		fmt.Fprintln(opts.IO.ErrOut, "Timed out waiting for events.")
		return cmdutil.PendingError
	}
	return err
}

type streamEnd struct {
	Outcome string `json:"outcome"`
	Reason  string `json:"reason"`
}

type listener struct {
	opts           *ListenOptions
	client         client.EventClient
	eventTypes     []string
	subscriptionID string
	received       int
	template       *template.Template
	// delivered records that the current stream delivered a heartbeat or an event, which proves it was working.
	delivered bool
}

// outputError is an error writing an event to stdout, such as a failing jq expression. It ends the command instead
// of reconnecting, since a new stream would fail the same way.
type outputError struct{ err error }

func (e outputError) Error() string { return e.err.Error() }
func (e outputError) Unwrap() error { return e.err }

func (l *listener) listen(ctx context.Context) error {
	errOut := l.opts.IO.ErrOut
	l.subscriptionID = l.opts.Subscription
	if l.subscriptionID == "" {
		if err := l.subscribe(ctx); err != nil {
			return err
		}
	}

	delay := minReconnectDelay
	// resubscribed records that the subscription was created again since the last attach. It changes the ready
	// message, and stops a loop when a subscription that was just created cannot be found.
	var resubscribed bool
	for {
		body, err := l.client.OpenStream(ctx, l.subscriptionID)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, client.ErrSubscriptionNotFound) {
				if l.opts.Subscription != "" {
					return fmt.Errorf("subscription %s was not found; it may have expired: %w\n%s",
						l.opts.Subscription, err, resubscribeAdvice)
				}
				if !resubscribed {
					// The subscription may have expired while the stream was detached.
					fmt.Fprintln(errOut, "Subscription not found, creating it again.")
					if err := l.subscribe(ctx); err != nil {
						return err
					}
					resubscribed = true
					continue
				}
			}
			if !isRetryable(err) {
				return err
			}

			fmt.Fprintf(errOut, "Could not attach the event stream, retrying: %v\n", err)
			if err := sleepFunc(ctx, max(delay, retryAfter(err))); err != nil {
				return err
			}
			delay = min(delay*2, maxReconnectDelay)
			continue
		}

		if resubscribed {
			fmt.Fprintln(errOut, resubscribedReady)
			fmt.Fprintln(errOut, resubscribedNote)
		} else {
			fmt.Fprintln(errOut, readyMessage)
			fmt.Fprintln(errOut, readyNote)
		}
		resubscribed = false

		stream := l.watchIdle(body)
		end, err := l.readEvents(stream)
		stream.Close()
		if errors.Is(err, errCountReached) {
			if l.opts.Count == 0 {
				fmt.Fprintln(errOut, firstEventNote)
			}
			return nil
		}
		if outErr, ok := errors.AsType[outputError](err); ok {
			return outErr.err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		switch {
		case err != nil:
			fmt.Fprintf(errOut, "Event stream failed, reconnecting: %v\n", err)
		case end == nil:
			fmt.Fprintln(errOut, "Event stream closed, reconnecting.")
		case end.Outcome == "reconnect":
			fmt.Fprintf(errOut, "Event stream ended (%s), reconnecting.\n", end.Reason)
		case end.Outcome == "subscription_ended" && end.Reason == "expired" && l.opts.Subscription == "":
			fmt.Fprintln(errOut, "Subscription expired, creating it again.")
			if err := l.subscribe(ctx); err != nil {
				return err
			}
			resubscribed = true
		case end.Outcome == "subscription_ended" && end.Reason == "expired":
			return fmt.Errorf("subscription %s expired\n%s", l.opts.Subscription, resubscribeAdvice)
		case end.Outcome == "subscription_ended" && end.Reason == "deleted":
			// A deleted subscription is not created again: deleting it is how its listeners are stopped on purpose.
			advice := rerunAdvice
			if l.opts.Subscription != "" {
				advice = resubscribeAdvice
			}
			return fmt.Errorf("subscription %s was deleted\n%s", l.subscriptionID, advice)
		case end.Outcome == "subscription_ended" && end.Reason == "access_revoked":
			return errors.New("the subscription ended because access to the resource was revoked")
		default:
			return fmt.Errorf("event stream ended: %s (%s)", end.Outcome, end.Reason)
		}

		// Attaching alone does not reset the backoff: a server that accepts streams and closes them right away
		// would otherwise be reconnected to every second. The server sends its first heartbeat 9 seconds after the
		// stream opens, so a heartbeat or an event shows the stream stayed up and worked.
		if l.delivered {
			delay = minReconnectDelay
		}
		if err := sleepFunc(ctx, delay); err != nil {
			return err
		}
		delay = min(delay*2, maxReconnectDelay)
	}
}

// subscribe creates the subscription, retrying while the API is unavailable.
func (l *listener) subscribe(ctx context.Context) error {
	delay := minReconnectDelay
	for {
		sub, _, err := l.client.CreateSubscription(ctx, l.opts.Resource, l.eventTypes)
		switch {
		case err == nil:
			l.subscriptionID = sub.ID
			return nil
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.Is(err, client.ErrResourceNotFound):
			return fmt.Errorf("could not subscribe to %s; the pull request was not found, or you cannot access it: %w",
				l.opts.Resource, err)
		case !isRetryable(err):
			return shared.SubscriptionLimitError(err)
		}

		fmt.Fprintf(l.opts.IO.ErrOut, "Could not create the subscription, retrying: %v\n", err)
		if err := sleepFunc(ctx, max(delay, retryAfter(err))); err != nil {
			return err
		}
		delay = min(delay*2, maxReconnectDelay)
	}
}

// watchIdle closes body when it delivers no data, not even a heartbeat, for the idle timeout, so a connection that
// died without being closed is detected.
func (l *listener) watchIdle(body io.ReadCloser) io.ReadCloser {
	if l.opts.idleTimeout <= 0 {
		return body
	}
	return newIdleReader(body, l.opts.idleTimeout)
}

// readEvents prints each event of the stream as one line of JSON until the stream ends. It returns the end frame,
// or nil when the stream ended without one, and errCountReached once the event limit was printed.
func (l *listener) readEvents(body io.Reader) (*streamEnd, error) {
	l.delivered = false
	reader := eventstream.NewFrameReader(body)
	reader.OnComment = func(string) { l.delivered = true }
	for {
		frame, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}

		// Event data is JSON, which the API client sanitizes only for JSON responses. Sanitizing it here, rather than
		// printing through ContentOut, keeps the escape sequences that --template helpers such as color generate.
		// Each frame's data is sanitized as a whole: sanitizing the stream instead would hold back the end of a frame
		// until the server sends more bytes, delaying each event until the next heartbeat.
		data, _, sanitizeErr := transform.String(&asciisanitizer.Sanitizer{JSON: true}, frame.Data)

		if frame.Event == endFrameEvent {
			if sanitizeErr != nil {
				return nil, fmt.Errorf("invalid end of stream frame: %w", sanitizeErr)
			}
			var end streamEnd
			if err := json.Unmarshal([]byte(data), &end); err != nil {
				return nil, fmt.Errorf("invalid end of stream frame: %w", err)
			}
			return &end, nil
		}

		l.delivered = true
		if sanitizeErr != nil {
			fmt.Fprintf(l.opts.IO.ErrOut, "warning: skipped an event that is not valid UTF-8: %v\n", sanitizeErr)
			continue
		}
		var event bytes.Buffer
		if err := json.Compact(&event, []byte(data)); err != nil {
			fmt.Fprintf(l.opts.IO.ErrOut, "warning: skipped an event that is not valid JSON: %v\n", err)
			continue
		}
		if err := l.writeEvent(event.Bytes()); err != nil {
			return nil, outputError{err}
		}

		l.received++
		if limit := l.eventLimit(); limit > 0 && l.received >= limit {
			return nil, errCountReached
		}
	}
}

// eventLimit returns the number of events to print before exiting, or zero to keep listening.
func (l *listener) eventLimit() int {
	switch {
	case l.opts.Follow:
		return 0
	case l.opts.Count > 0:
		return l.opts.Count
	default:
		return 1
	}
}

// writeEvent prints a compact JSON event as one line, or formatted with --jq or --template.
func (l *listener) writeEvent(event []byte) error {
	out := l.opts.IO.Out
	switch {
	case l.opts.Jq != "":
		return jq.EvaluateFormatted(bytes.NewReader(event), out, l.opts.Jq, "", false)
	case l.template != nil:
		if err := l.template.Execute(bytes.NewReader(event)); err != nil {
			return err
		}
		return l.template.Flush()
	default:
		_, err := out.Write(append(event, '\n'))
		return err
	}
}

// isRetryable reports whether a request may succeed later. Network and server errors are retried; anything else,
// such as a missing subscription or a request that cannot be built, is final.
func isRetryable(err error) bool {
	if httpErr, ok := errors.AsType[api.HTTPError](err); ok {
		return httpErr.StatusCode >= 500
	}
	_, ok := errors.AsType[*url.Error](err)
	return ok
}

// retryAfter returns the delay requested by the Retry-After header of an HTTP error, or zero. The header holds
// either a number of seconds or an HTTP date to retry after.
func retryAfter(err error) time.Duration {
	httpErr, ok := errors.AsType[api.HTTPError](err)
	if !ok || httpErr.HTTPError == nil {
		return 0
	}
	value := httpErr.Headers.Get("Retry-After")
	if seconds, convErr := strconv.Atoi(value); convErr == nil {
		return max(time.Duration(seconds)*time.Second, 0)
	}
	if date, parseErr := http.ParseTime(value); parseErr == nil {
		return max(time.Until(date), 0)
	}
	return 0
}

// idleReader closes the underlying stream once it has been idle for the timeout. Reads then fail with an idle
// error, instead of blocking forever on a dead connection.
type idleReader struct {
	body    io.ReadCloser
	timeout time.Duration
	timer   *time.Timer
	idle    atomic.Bool
}

func newIdleReader(body io.ReadCloser, timeout time.Duration) *idleReader {
	r := &idleReader{body: body, timeout: timeout}
	r.timer = time.AfterFunc(timeout, func() {
		r.idle.Store(true)
		body.Close()
	})
	return r
}

func (r *idleReader) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)
	if r.idle.Load() {
		r.timer.Stop()
		return n, fmt.Errorf("no data received for %s", r.timeout)
	}
	if n > 0 {
		r.timer.Reset(r.timeout)
	}
	return n, err
}

func (r *idleReader) Close() error {
	r.timer.Stop()
	return r.body.Close()
}

// sleepFunc is sleep, replaced in tests to avoid waiting between retries.
var sleepFunc = sleep

// sleep waits for d plus up to 50% jitter, so clients told to reconnect at the same time do not retry in lockstep.
func sleep(ctx context.Context, d time.Duration) error {
	if jitter := d / 2; jitter > 0 {
		d += rand.N(jitter)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
