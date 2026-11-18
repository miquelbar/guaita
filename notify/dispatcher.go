package notify

import (
	"log"
	"sync"
)

type waitingState uint8

const (
	waitingPending waitingState = iota + 1
	waitingResolving
	waitingResolved
)

// ClaimResult describes whether an action acquired a pending approval request.
type ClaimResult uint8

const (
	ClaimMissing ClaimResult = iota
	ClaimAcquired
	ClaimConsumed
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

// ResolutionNotifier updates a previously delivered notification after an action wins the request.
type ResolutionNotifier interface {
	Resolve(paneID, action string) error
}

// PendingNotifier resets channel-local state before a new request is delivered.
type PendingNotifier interface {
	BeginPending(paneID string)
}

// Dispatcher coordinates multi-channel delivery with per-pane state transition deduplication.
type Dispatcher struct {
	mu            sync.Mutex
	notifiers     []Notifier
	waitingByPane map[string]waitingState
}

// NewDispatcher creates an empty notification dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		notifiers:     make([]Notifier, 0),
		waitingByPane: make(map[string]waitingState),
	}
}

// Register adds a notifier channel to the dispatcher.
func (d *Dispatcher) Register(n Notifier) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.notifiers = append(d.notifiers, n)
}

// RegisterOrReplace adds a notifier or replaces an existing one with the same name.
func (d *Dispatcher) RegisterOrReplace(n Notifier) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, existing := range d.notifiers {
		if existing.Name() == n.Name() {
			d.notifiers[i] = n
			return
		}
	}
	d.notifiers = append(d.notifiers, n)
}

// Unregister removes any notifier matching the given name.
func (d *Dispatcher) Unregister(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	filtered := d.notifiers[:0]
	for _, existing := range d.notifiers {
		if existing.Name() != name {
			filtered = append(filtered, existing)
		}
	}
	d.notifiers = filtered
}

func (d *Dispatcher) broadcast(event NotificationEvent) {
	for _, n := range d.notifiers {
		go func(target Notifier) {
			if err := target.Notify(event); err != nil {
				log.Printf("[%s] notification delivery failed: %v", target.Name(), err)
			}
		}(n)
	}
}

func (d *Dispatcher) beginPending(event NotificationEvent) {
	for _, notifier := range d.notifiers {
		if pending, ok := notifier.(PendingNotifier); ok {
			pending.BeginPending(event.PaneID)
		}
	}
}

// DispatchOnWaiting dispatches notifications only on transition from non-waiting to waiting for a pane.
func (d *Dispatcher) DispatchOnWaiting(isWaiting bool, event NotificationEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := event.PaneID
	if key == "" {
		key = "_default"
	}

	if isWaiting {
		if d.waitingByPane[key] == 0 {
			d.waitingByPane[key] = waitingPending
			d.beginPending(event)
			d.broadcast(event)
		}
	} else {
		if key == "_default" {
			d.waitingByPane = make(map[string]waitingState)
		} else {
			delete(d.waitingByPane, key)
		}
	}
}

// SyncWaitingPanes updates waiting state across all active panes simultaneously,
// firing independent notifications for every newly blocked pane and clearing resolved ones.
func (d *Dispatcher) SyncWaitingPanes(events []NotificationEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()

	activeKeys := make(map[string]bool, len(events))
	for _, ev := range events {
		key := ev.PaneID
		if key == "" {
			key = "_default"
		}
		activeKeys[key] = true
		if d.waitingByPane[key] == 0 {
			d.waitingByPane[key] = waitingPending
			d.beginPending(ev)
			d.broadcast(ev)
		}
	}
	for k := range d.waitingByPane {
		if !activeKeys[k] {
			delete(d.waitingByPane, k)
		}
	}
}

func paneKey(paneID string) string {
	if paneID == "" {
		return "_default"
	}
	return paneID
}

// ClaimWaiting atomically acquires a pending request so only one control channel can answer it.
func (d *Dispatcher) ClaimWaiting(paneID string) ClaimResult {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := paneKey(paneID)
	switch d.waitingByPane[key] {
	case waitingPending:
		d.waitingByPane[key] = waitingResolving
		return ClaimAcquired
	case waitingResolving, waitingResolved:
		return ClaimConsumed
	default:
		return ClaimMissing
	}
}

// FinishWaiting commits a successful action or releases the request after a delivery failure.
func (d *Dispatcher) FinishWaiting(paneID, action string, success bool) {
	d.mu.Lock()
	key := paneKey(paneID)
	if d.waitingByPane[key] != waitingResolving {
		d.mu.Unlock()
		return
	}
	if !success {
		d.waitingByPane[key] = waitingPending
		d.mu.Unlock()
		return
	}
	d.waitingByPane[key] = waitingResolved
	resolvers := make([]ResolutionNotifier, 0, len(d.notifiers))
	for _, notifier := range d.notifiers {
		if resolver, ok := notifier.(ResolutionNotifier); ok {
			resolvers = append(resolvers, resolver)
		}
	}
	d.mu.Unlock()

	for _, resolver := range resolvers {
		go func(target ResolutionNotifier) {
			if err := target.Resolve(paneID, action); err != nil {
				log.Printf("notification resolution update failed: %v", err)
			}
		}(resolver)
	}
}

// IsWaitingConsumed reports whether an active request is resolving or already resolved.
func (d *Dispatcher) IsWaitingConsumed(paneID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	state := d.waitingByPane[paneKey(paneID)]
	return state == waitingResolving || state == waitingResolved
}

// ChannelCount returns the number of active notification channels.
func (d *Dispatcher) ChannelCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.notifiers)
}
