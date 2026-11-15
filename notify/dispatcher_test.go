package notify

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
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
	expectedRedacted := "Agent paused waiting for human approval. Open Guaita to inspect and respond."
	if capturedBody != expectedRedacted {
		t.Errorf("expected redacted body for public ntfy.sh, got: %s", capturedBody)
	}
	if len(capturedActions) == 0 {
		t.Errorf("expected actions header to be set")
	}
	if strings.Contains(capturedActions, "token=123") {
		t.Errorf("expected ntfy Actions header to strip sensitive token query parameter, got: %s", capturedActions)
	}
}

func TestTelegramNotifier_Authorization(t *testing.T) {
	tg := &TelegramNotifier{
		Token:  "test-token",
		ChatID: "999999",
	}

	// Authorized chat ID
	msgAuth := &struct {
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	}{}
	msgAuth.Chat.ID = 999999
	if !tg.isAuthorizedCallback(111111, msgAuth) {
		t.Errorf("expected authorized chat ID to pass")
	}

	// Unauthorized chat ID and user ID
	msgEvil := &struct {
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	}{}
	msgEvil.Chat.ID = 888888
	if tg.isAuthorizedCallback(777777, msgEvil) {
		t.Errorf("expected unauthorized chat/user ID to be rejected")
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
	if capturedBody["parse_mode"] != "HTML" {
		t.Errorf("expected parse_mode HTML, got %v", capturedBody["parse_mode"])
	}
}

func TestTelegramNotifier_HTMLAndErrorDetailsRegression(t *testing.T) {
	var capturedPayload map[string]interface{}

	client := &http.Client{
		Transport: mockTransport(func(r *http.Request) (*http.Response, error) {
			_ = json.NewDecoder(r.Body).Decode(&capturedPayload)
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"ok":false,"description":"Bad Request: chat not found"}`))),
			}, nil
		}),
	}

	tg := &TelegramNotifier{
		Token:  "test-token",
		ChatID: "12345",
		Client: client,
	}

	err := tg.Notify(NotificationEvent{
		Title:      "Alert",
		Body:       "check <tag> & _underscore_ *star* ```backticks```",
		PaneID:     "w1:p1",
		PairingURL: "https://192.168.10.1:8085/",
	})
	if err == nil || !strings.Contains(err.Error(), "chat not found") {
		t.Fatalf("expected error containing Telegram description, got: %v", err)
	}

	text, _ := capturedPayload["text"].(string)
	if !strings.Contains(text, "&lt;tag&gt; &amp;") {
		t.Errorf("expected HTML-escaped body in Telegram payload, got: %s", text)
	}

	markup, _ := capturedPayload["reply_markup"].(map[string]interface{})
	kb, _ := markup["inline_keyboard"].([]interface{})
	if len(kb) != 1 {
		t.Errorf("expected 1 inline_keyboard row (no private-IP url button row), got %d", len(kb))
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
