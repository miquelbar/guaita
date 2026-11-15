package notify

import (
	"log"
	"sync"
)

// NotificationEvent holds details for an agent approval request.
type NotificationEvent struct {
	Title            string
	Body             string
	PaneID           string
	PairingURL       string
	ActionApproveURL string
	ActionRejectURL  string
}

// Notifier defines the common interface for notification channels.
type Notifier interface {
	Name() string
	Notify(event NotificationEvent) error
}

// Dispatcher coordinates multi-channel delivery with state transition deduplication.
type Dispatcher struct {
	mu         sync.Mutex
	notifiers  []Notifier
	wasWaiting bool
}

// NewDispatcher creates an empty notification dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		notifiers: make([]Notifier, 0),
	}
}

// Register adds a notifier channel to the dispatcher.
func (d *Dispatcher) Register(n Notifier) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.notifiers = append(d.notifiers, n)
}

// DispatchOnWaiting dispatches notifications only on transition from non-waiting to waiting.
func (d *Dispatcher) DispatchOnWaiting(isWaiting bool, event NotificationEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if isWaiting {
		if !d.wasWaiting {
			d.wasWaiting = true
			// Broadcast asynchronously across all registered notifiers
			for _, n := range d.notifiers {
				go func(target Notifier) {
					if err := target.Notify(event); err != nil {
						log.Printf("[%s] notification delivery failed: %v", target.Name(), err)
					}
				}(n)
			}
		}
	} else {
		d.wasWaiting = false
	}
}

// ChannelCount returns the number of active notification channels.
func (d *Dispatcher) ChannelCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.notifiers)
}
