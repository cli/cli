package client

import (
	"time"

	"github.com/cli/cli/v2/pkg/cmdutil"
)

// SubscriptionFields are the fields of a subscription available to --json.
var SubscriptionFields = []string{
	"id",
	"resourceUrl",
	"eventTypes",
	"createdAt",
	"expiresAt",
	"url",
	"eventsUrl",
}

// Subscription is an event subscription, which delivers events of the selected types for one resource until it
// expires.
type Subscription struct {
	ID          string    `json:"id"`
	ResourceURL string    `json:"resource_url"`
	EventTypes  []string  `json:"event_types"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	URL         string    `json:"url"`
	EventsURL   string    `json:"events_url"`
}

// ExportData returns the selected fields of the subscription for --json output.
func (s *Subscription) ExportData(fields []string) map[string]any {
	return cmdutil.StructExportData(s, fields)
}
