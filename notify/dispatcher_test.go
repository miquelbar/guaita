package notify

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

type mockNotifier struct {
	mu     sync.Mutex
	calls  int
	events []NotificationEvent
}

func (m *mockNotifier) Name() string { return "mock" }
func (m *mockNotifier) Notify(e NotificationEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.events = append(m.events, e)
	return nil
}

func TestDispatcher_Deduplication(t *testing.T) {
	d := NewDispatcher()
	mock := &mockNotifier{}
	d.Register(mock)

	event := NotificationEvent{Title: "Test", Body: "Waiting"}

	// 1. Initial true -> should notify
	d.DispatchOnWaiting(true, event)
	time.Sleep(20 * time.Millisecond)

	mock.mu.Lock()
	if mock.calls != 1 {
		t.Fatalf("expected 1 call, got %d", mock.calls)
	}
	mock.mu.Unlock()

	// 2. Continuous true -> should NOT re-notify
	d.DispatchOnWaiting(true, event)
	time.Sleep(20 * time.Millisecond)

	mock.mu.Lock()
	if mock.calls != 1 {
		t.Fatalf("expected still 1 call, got %d", mock.calls)
	}
	mock.mu.Unlock()

	// 3. Reset to false
	d.DispatchOnWaiting(false, event)

	// 4. New transition to true -> should notify again
	d.DispatchOnWaiting(true, event)
	time.Sleep(20 * time.Millisecond)

	mock.mu.Lock()
	if mock.calls != 2 {
		t.Fatalf("expected 2 calls after reset, got %d", mock.calls)
	}
	mock.mu.Unlock()
}

type mockTransport func(req *http.Request) (*http.Response, error)

func (m mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

func TestNtfyNotifier_Delivery(t *testing.T) {
	var capturedTitle, capturedPriority, capturedActions, capturedBody string

	client := &http.Client{
		Transport: mockTransport(func(r *http.Request) (*http.Response, error) {
			capturedTitle = r.Header.Get("Title")
			capturedPriority = r.Header.Get("Priority")
			capturedActions = r.Header.Get("Actions")
			b, _ := io.ReadAll(r.Body)
			capturedBody = string(b)

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte("ok"))),
			}, nil
		}),
	}

	ntfy := NewNtfy("https://ntfy.sh", "test-topic")
	ntfy.Client = client

	err := ntfy.Notify(NotificationEvent{
		Title:            "Approval Needed",
		Body:             "Please confirm task",
		ActionApproveURL: "http://localhost:8085/action?token=123",
		ActionRejectURL:  "http://localhost:8085/action?token=123",
		PairingURL:       "http://localhost:8085/?token=123",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedTitle != "Approval Needed" {
		t.Errorf("unexpected title: %s", capturedTitle)
	}
	if capturedPriority != "high" {
		t.Errorf("expected high priority, got %s", capturedPriority)
	}
	if capturedBody != "Please confirm task" {
		t.Errorf("unexpected body: %s", capturedBody)
	}
	if len(capturedActions) == 0 {
		t.Errorf("expected actions header to be set")
	}
}

func TestTelegramNotifier_Delivery(t *testing.T) {
	var capturedBody map[string]interface{}

	client := &http.Client{
		Transport: mockTransport(func(r *http.Request) (*http.Response, error) {
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"ok": true}`))),
			}, nil
		}),
	}

	tg := &TelegramNotifier{
		Token:  "test-token",
		ChatID: "12345",
		Client: client,
	}

	err := tg.Notify(NotificationEvent{
		Title:      "Agent Alert",
		Body:       "Approve changes?",
		PaneID:     "w1:p1",
		PairingURL: "http://localhost:8085/?token=abc",
	})

	if err != nil {
		t.Fatalf("unexpected telegram error: %v", err)
	}
	if capturedBody["chat_id"] != "12345" {
		t.Errorf("expected chat_id 12345, got %v", capturedBody["chat_id"])
	}
}

func TestWebPushStore_ManifestAndSW(t *testing.T) {
	store := NewWebPushStore()
	store.Subscribe(WebPushSubscription{Endpoint: "https://push.apple.com/test"})
	subs := store.Subscriptions()
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subs))
	}

	manifest := ManifestJSON()
	if len(manifest) == 0 {
		t.Errorf("manifest should not be empty")
	}

	sw := ServiceWorkerJS()
	if len(sw) == 0 {
		t.Errorf("service worker should not be empty")
	}
}
