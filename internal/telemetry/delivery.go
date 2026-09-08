package telemetry

import "sync"

// Delivery buffers completed invocation payloads until they can be sent.
type Delivery struct {
	mu       sync.Mutex
	send     func(SendTelemetryPayload)
	payloads []SendTelemetryPayload
}

// NewDelivery creates a delivery queue using send to transmit each payload.
func NewDelivery(send func(SendTelemetryPayload)) *Delivery {
	return &Delivery{send: send}
}

func (d *Delivery) enqueue(payload SendTelemetryPayload) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.payloads = append(d.payloads, payload)
}

// Flush sends queued payloads. It does not complete in-progress invocations.
func (d *Delivery) Flush() {
	d.mu.Lock()
	payloads := d.payloads
	d.payloads = nil
	d.mu.Unlock()

	for _, payload := range payloads {
		d.send(payload)
	}
}
