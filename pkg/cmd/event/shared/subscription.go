package shared

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/cli/cli/v2/pkg/cmd/event/client"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

// ResourceHost returns the host to send subscription requests for resourceURL to, or fallback when the URL names no
// host. The API validates the resource; its host only picks where to send the request.
func ResourceHost(resourceURL, fallback string) string {
	if u, err := url.Parse(resourceURL); err == nil && u.Host != "" {
		return ghauth.NormalizeHostname(u.Hostname())
	}
	return fallback
}

// NormalizeEventTypes drops blank and duplicate event types, which the API rejects.
func NormalizeEventTypes(names []string) []string {
	var types []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || slices.Contains(types, name) {
			continue
		}
		types = append(types, name)
	}
	return types
}

// SubscriptionNotFoundError reports a subscription the API did not find. The API does not tell a missing
// subscription apart from an expired, deleted, or another account's one.
func SubscriptionNotFoundError(id string) error {
	return fmt.Errorf("subscription %s was not found; it may have expired or been deleted, or you cannot access it", id)
}

// SubscriptionLimitError adds the commands that free a subscription slot to err when it reports the subscription
// limit, and returns other errors unchanged.
func SubscriptionLimitError(err error) error {
	if !errors.Is(err, client.ErrSubscriptionLimit) {
		return err
	}
	return fmt.Errorf("%w\nTo free a slot, run `gh event subscription list` and delete one with "+
		"`gh event subscription delete <id>`. Deleting a subscription only stops its events, "+
		"and every subscription expires within 24 hours anyway", err)
}
