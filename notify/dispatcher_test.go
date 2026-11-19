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

type mockResolutionNotifier struct {
	mockNotifier
	resolved chan string
}

func (m *mockResolutionNotifier) Resolve(paneID, action string) error {
	m.resolved <- paneID + ":" + action
	return nil
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

func TestDispatcher_SyncWaitingPanes(t *testing.T) {
	d := NewDispatcher()
	mock := &mockNotifier{}
	d.Register(mock)

	getCalls := func() int {
		mock.mu.Lock()
		defer mock.mu.Unlock()
		return mock.calls
	}

	// Two distinct panes become waiting simultaneously -> 2 notifications
	d.SyncWaitingPanes([]NotificationEvent{
		{Title: "Pane 1", PaneID: "w1:p1"},
		{Title: "Pane 2", PaneID: "w2:p1"},
	})
	time.Sleep(30 * time.Millisecond)
	if getCalls() != 2 {
		t.Fatalf("expected 2 notifications for 2 waiting panes, got %d", getCalls())
	}

	// Same 2 panes still waiting -> deduplicated (still 2)
	d.SyncWaitingPanes([]NotificationEvent{
		{Title: "Pane 1", PaneID: "w1:p1"},
		{Title: "Pane 2", PaneID: "w2:p1"},
	})
	time.Sleep(30 * time.Millisecond)
	if getCalls() != 2 {
		t.Fatalf("expected deduplicated count 2, got %d", getCalls())
	}

	// w1:p1 resolved, w2:p1 still waiting -> still 2
	d.SyncWaitingPanes([]NotificationEvent{
		{Title: "Pane 2", PaneID: "w2:p1"},
	})
	time.Sleep(30 * time.Millisecond)
	if getCalls() != 2 {
		t.Fatalf("expected count 2 after w1:p1 resolved, got %d", getCalls())
	}

	// w1:p1 becomes waiting again -> fires 3rd notification
	d.SyncWaitingPanes([]NotificationEvent{
		{Title: "Pane 1", PaneID: "w1:p1"},
		{Title: "Pane 2", PaneID: "w2:p1"},
	})
	time.Sleep(30 * time.Millisecond)
	if getCalls() != 3 {
		t.Fatalf("expected 3 notifications after w1:p1 re-blocked, got %d", getCalls())
	}
}

func TestDispatcher_ApprovalCanOnlyBeConsumedOnce(t *testing.T) {
	d := NewDispatcher()
	notifier := &mockResolutionNotifier{resolved: make(chan string, 1)}
	d.Register(notifier)
	d.SyncWaitingPanes([]NotificationEvent{{PaneID: "%7", Body: "Approve?"}})

	if got := d.ClaimWaiting("%7"); got != ClaimAcquired {
		t.Fatalf("expected first claim to acquire request, got %v", got)
	}
	if got := d.ClaimWaiting("%7"); got != ClaimConsumed {
		t.Fatalf("expected concurrent claim to be consumed, got %v", got)
	}
	d.FinishWaiting("%7", "approve", true)
	if !d.IsWaitingConsumed("%7") {
		t.Fatal("expected successful request to remain consumed while source is still waiting")
	}

	select {
	case got := <-notifier.resolved:
		if got != "%7:approve" {
			t.Fatalf("unexpected resolution: %s", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected notifier resolution update")
	}

	d.SyncWaitingPanes([]NotificationEvent{{PaneID: "%7", Body: "Still stale"}})
	if got := d.ClaimWaiting("%7"); got != ClaimConsumed {
		t.Fatalf("expected stale waiting state to stay consumed, got %v", got)
	}
	d.SyncWaitingPanes(nil)
	d.SyncWaitingPanes([]NotificationEvent{{PaneID: "%7", Body: "New request"}})
	if got := d.ClaimWaiting("%7"); got != ClaimAcquired {
		t.Fatalf("expected a new request after source cleared, got %v", got)
	}
}

func TestDispatcher_FailedActionReopensRequest(t *testing.T) {
	d := NewDispatcher()
	d.SyncWaitingPanes([]NotificationEvent{{PaneID: "%3"}})
	if got := d.ClaimWaiting("%3"); got != ClaimAcquired {
		t.Fatalf("expected claim, got %v", got)
	}
	d.FinishWaiting("%3", "reject", false)
	if got := d.ClaimWaiting("%3"); got != ClaimAcquired {
		t.Fatalf("expected retry after failure, got %v", got)
	}
}

func TestTelegramNotifier_ResolveEditsMessageAndRemovesButtons(t *testing.T) {
	var requests []struct {
		path    string
		payload map[string]interface{}
	}
	client := &http.Client{Transport: mockTransport(func(r *http.Request) (*http.Response, error) {
		var payload map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		requests = append(requests, struct {
			path    string
			payload map[string]interface{}
		}{path: r.URL.Path, payload: payload})
		body := `{"ok":true}`
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			body = `{"ok":true,"result":{"message_id":42}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	tg := &TelegramNotifier{Token: "token", ChatID: "123", Client: client, messages: make(map[string]telegramMessage), resolved: make(map[string]string)}

	if err := tg.Notify(NotificationEvent{PaneID: "%3", Body: "Approve changes?"}); err != nil {
		t.Fatalf("notify failed: %v", err)
	}
	if err := tg.Resolve("%3", "approve"); err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if len(requests) != 2 || !strings.HasSuffix(requests[1].path, "/editMessageText") {
		t.Fatalf("expected send followed by edit, got %+v", requests)
	}
	if !strings.Contains(requests[1].payload["text"].(string), "APPROVED") {
		t.Fatalf("expected approved label, got %v", requests[1].payload["text"])
	}
	markup := requests[1].payload["reply_markup"].(map[string]interface{})
	if buttons := markup["inline_keyboard"].([]interface{}); len(buttons) != 0 {
		t.Fatalf("expected inline buttons to be removed, got %v", buttons)
	}
}

func TestTelegramNotifier_ResolutionBeforeMessageRegistration(t *testing.T) {
	var paths []string
	client := &http.Client{Transport: mockTransport(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		body := `{"ok":true}`
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			body = `{"ok":true,"result":{"message_id":73}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	tg := &TelegramNotifier{Token: "token", ChatID: "123", Client: client, messages: make(map[string]telegramMessage), resolved: make(map[string]string)}
	tg.BeginPending("%5")
	if err := tg.Resolve("%5", "reject"); err != nil {
		t.Fatalf("early resolution failed: %v", err)
	}
	if err := tg.Notify(NotificationEvent{PaneID: "%5", Body: "Reject changes?"}); err != nil {
		t.Fatalf("notify failed: %v", err)
	}
	if len(paths) != 2 || !strings.HasSuffix(paths[0], "/sendMessage") || !strings.HasSuffix(paths[1], "/editMessageText") {
		t.Fatalf("expected late message registration to trigger resolution edit, got %v", paths)
	}
}

func TestParseTelegramAction(t *testing.T) {
	cases := []struct {
		raw      string
		wantPane string
		wantAct  string
		wantOK   bool
	}{
		{"approve", "", "approve", true},
		{"reject", "", "reject", true},
		{"approve:w1:p2", "w1:p2", "approve", true},
		{"reject:%3", "%3", "reject", true},
		{"act:1:%4", "%4", "1", true},
		{"act:p:w1:p2", "w1:p2", "p", true},
		{"act:esc", "", "esc", true},
		{"act:approve:%1", "%1", "approve", true},
		{"drop-database", "", "", false},
	}
	for _, tc := range cases {
		p, a, ok := ParseTelegramAction(tc.raw)
		if p != tc.wantPane || a != tc.wantAct || ok != tc.wantOK {
			t.Errorf("ParseTelegramAction(%q) = (%q, %q, %v), want (%q, %q, %v)", tc.raw, p, a, ok, tc.wantPane, tc.wantAct, tc.wantOK)
		}
	}
}

func TestTelegramNotifier_CustomOptionsKeyboard(t *testing.T) {
	var requests []struct {
		path    string
		payload map[string]interface{}
	}
	client := &http.Client{Transport: mockTransport(func(r *http.Request) (*http.Response, error) {
		var payload map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		requests = append(requests, struct {
			path    string
			payload map[string]interface{}
		}{path: r.URL.Path, payload: payload})
		body := `{"ok":true}`
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			body = `{"ok":true,"result":{"message_id":99}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	tg := &TelegramNotifier{Token: "token", ChatID: "123", Client: client, messages: make(map[string]telegramMessage), resolved: make(map[string]string)}

	event := NotificationEvent{
		PaneID: "%4",
		Body:   "Select action:",
		Options: []NotificationOption{
			{Key: "1", Label: "Yes, proceed", Style: "approve"},
			{Key: "2", Label: "Always allow", Style: "always"},
			{Key: "esc", Label: "Cancel", Style: "reject"},
		},
	}
	if err := tg.Notify(event); err != nil {
		t.Fatalf("notify failed: %v", err)
	}

	if len(requests) != 1 || !strings.HasSuffix(requests[0].path, "/sendMessage") {
		t.Fatalf("expected 1 sendMessage request, got %+v", requests)
	}
	markup := requests[0].payload["reply_markup"].(map[string]interface{})
	keyboard := markup["inline_keyboard"].([]interface{})
	// 3 option rows + 1 bottom row (enter/esc) = 4 rows
	if len(keyboard) != 4 {
		t.Fatalf("expected 4 keyboard rows, got %d: %+v", len(keyboard), keyboard)
	}

	firstRow := keyboard[0].([]interface{})
	firstBtn := firstRow[0].(map[string]interface{})
	if firstBtn["callback_data"] != "act:1:%4" {
		t.Errorf("expected callback_data 'act:1:%%4', got %v", firstBtn["callback_data"])
	}

	secondRow := keyboard[1].([]interface{})
	secondBtn := secondRow[0].(map[string]interface{})
	if secondBtn["callback_data"] != "act:2:%4" {
		t.Errorf("expected callback_data 'act:2:%%4', got %v", secondBtn["callback_data"])
	}

	if err := tg.Resolve("%4", "2"); err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if len(requests) != 2 || !strings.HasSuffix(requests[1].path, "/editMessageText") {
		t.Fatalf("expected 2nd request to be editMessageText, got %+v", requests)
	}
	text := requests[1].payload["text"].(string)
	if !strings.Contains(text, "OPTION SELECTED (2)") {
		t.Errorf("expected resolution text to contain 'OPTION SELECTED (2)', got %q", text)
	}
}

type flakyNotifier struct {
	mu       sync.Mutex
	attempts int
	failTime int
}

func (f *flakyNotifier) Name() string { return "flaky" }
func (f *flakyNotifier) Notify(e NotificationEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts++
	if f.attempts <= f.failTime {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func TestDispatcher_RetriesFailedDeliveryAndNewPromptKey(t *testing.T) {
	d := NewDispatcher()
	flaky := &flakyNotifier{failTime: 1}
	d.Register(flaky)

	ev1 := NotificationEvent{PaneID: "w1:p2", PromptKey: "confirm:Allow file A?", Body: "Allow file A?"}
	// First poll fails delivery
	d.SyncWaitingPanes([]NotificationEvent{ev1})
	time.Sleep(30 * time.Millisecond)

	// Second poll retries and succeeds
	d.SyncWaitingPanes([]NotificationEvent{ev1})
	time.Sleep(30 * time.Millisecond)

	// Third poll with same promptKey is deduplicated
	d.SyncWaitingPanes([]NotificationEvent{ev1})
	time.Sleep(30 * time.Millisecond)

	flaky.mu.Lock()
	if flaky.attempts != 2 {
		t.Fatalf("expected 2 attempts (1 fail + 1 retry), got %d", flaky.attempts)
	}
	flaky.mu.Unlock()

	// New promptKey on the same pane while still waitingPending triggers a new notification
	ev2 := NotificationEvent{PaneID: "w1:p2", PromptKey: "question:What file name?", Body: "What file name?"}
	d.SyncWaitingPanes([]NotificationEvent{ev2})
	time.Sleep(30 * time.Millisecond)

	flaky.mu.Lock()
	if flaky.attempts != 3 {
		t.Fatalf("expected 3rd delivery on new PromptKey, got %d", flaky.attempts)
	}
	flaky.mu.Unlock()
}
