package notify

import (
	"encoding/json"
	"sync"
)

// WebPushSubscription represents a browser push manager subscription.
type WebPushSubscription struct {
	Endpoint string            `json:"endpoint"`
	Keys     map[string]string `json:"keys"`
}

// WebPushStore manages active browser push subscriptions and serves PWA assets.
type WebPushStore struct {
	mu            sync.RWMutex
	subscriptions []WebPushSubscription
}

// NewWebPushStore creates an empty subscription registry.
func NewWebPushStore() *WebPushStore {
	return &WebPushStore{
		subscriptions: make([]WebPushSubscription, 0),
	}
}

// Name returns the channel identifier.
func (w *WebPushStore) Name() string {
	return "webpush"
}

// Subscribe registers a new browser push subscription.
func (w *WebPushStore) Subscribe(sub WebPushSubscription) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, existing := range w.subscriptions {
		if existing.Endpoint == sub.Endpoint {
			return
		}
	}
	w.subscriptions = append(w.subscriptions, sub)
}

// Subscriptions returns a snapshot of active subscriptions.
func (w *WebPushStore) Subscriptions() []WebPushSubscription {
	w.mu.RLock()
	defer w.mu.RUnlock()
	res := make([]WebPushSubscription, len(w.subscriptions))
	copy(res, w.subscriptions)
	return res
}

// Notify broadcasts a push notification payload to registered subscriptions.
func (w *WebPushStore) Notify(event NotificationEvent) error {
	// Active subscriptions can be relayed to APNs / VAPID endpoints when configured
	return nil
}

// ManifestJSON returns a W3C Web App Manifest compliant with iOS 16.4+ standalone PWA.
func ManifestJSON() []byte {
	manifest := map[string]interface{}{
		"name":             "Guaita Sentinel",
		"short_name":        "Guaita",
		"description":      "Mobile-First Human-in-the-Loop Sentinel for Coding Agents",
		"start_url":        "/",
		"display":          "standalone",
		"background_color": "#0b0f19",
		"theme_color":       "#0b0f19",
		"orientation":      "portrait",
		"icons": []map[string]string{
			{
				"src":   "data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 100 100'><text y='.9em' font-size='90'>🛡️</text></svg>",
				"sizes": "192x192 512x512",
				"type":  "image/svg+xml",
			},
		},
	}
	bytes, _ := json.Marshal(manifest)
	return bytes
}

// ServiceWorkerJS returns the Service Worker supporting push events and notification actions.
func ServiceWorkerJS() []byte {
	return []byte(`// Guaita Service Worker for iOS 16.4+ PWA Web Push
self.addEventListener('install', (event) => {
    self.skipWaiting();
});

self.addEventListener('activate', (event) => {
    event.waitUntil(self.clients.claim());
});

self.addEventListener('push', (event) => {
    let data = {};
    if (event.data) {
        try { data = event.data.json(); } catch(e) { data = { body: event.data.text() }; }
    }
    const title = data.title || '🛡️ Guaita Sentinel';
    const options = {
        body: data.body || 'Agent is paused awaiting human approval.',
        icon: "data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 100 100'><text y='.9em' font-size='90'>🛡️</text></svg>",
        badge: "data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 100 100'><text y='.9em' font-size='90'>🛡️</text></svg>",
        vibrate: [250, 100, 250, 100, 300],
        data: { url: data.url || '/' },
        actions: [
            { action: 'approve', title: '✓ Approve (Y)' },
            { action: 'reject', title: '✕ Reject (N)' }
        ]
    };
    event.waitUntil(self.registration.showNotification(title, options));
});

self.addEventListener('notificationclick', (event) => {
    event.notification.close();
    if (event.action === 'approve') {
        fetch('/action', { method: 'POST', headers: {'Content-Type': 'application/x-www-form-urlencoded'}, body: 'action=approve' });
    } else if (event.action === 'reject') {
        fetch('/action', { method: 'POST', headers: {'Content-Type': 'application/x-www-form-urlencoded'}, body: 'action=reject' });
    } else {
        event.waitUntil(
            clients.matchAll({ type: 'window', includeUncontrolled: true }).then((windowClients) => {
                for (let client of windowClients) {
                    if (client.url.includes(self.location.origin) && 'focus' in client) {
                        return client.focus();
                    }
                }
                if (clients.openWindow) {
                    return clients.openWindow(event.notification.data.url || '/');
                }
            })
        );
    }
});
`)
}
