package server

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"guaita/herdr"
	"guaita/notify"
)

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		addr     string
		expected bool
	}{
		{"127.0.0.1:8080", true},
		{"127.0.0.1", true},
		{"[::1]:9090", true},
		{"::1", true},
		{"localhost:8085", true},
		{"192.168.1.50:8085", false},
		{"10.0.0.1:443", false},
		{"172.16.0.1:80", false},
	}

	for _, tt := range tests {
		got := IsLoopback(tt.addr)
		if got != tt.expected {
			t.Errorf("IsLoopback(%q) = %v; expected %v", tt.addr, got, tt.expected)
		}
	}
}

func TestHandleIndex_LocalhostAdmin(t *testing.T) {
	srv := New(Config{SessionToken: "secret123", SessionDays: 7, LocalIP: "192.168.1.100", Port: "8085"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:54321" // loopback
	rec := httptest.NewRecorder()

	srv.HandleIndex(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "LOCALHOST ADMIN") {
		t.Errorf("expected localhost admin badge in body")
	}
	if !strings.Contains(body, "Mobile Pairing") {
		t.Errorf("expected mobile pairing card")
	}
	if !strings.Contains(body, "Connected Clients") {
		t.Errorf("expected connected clients table")
	}
}

func TestHandleIndex_RemoteUnauthorized(t *testing.T) {
	srv := New(Config{SessionToken: "secret123", SessionDays: 7})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.50:54321" // remote
	rec := httptest.NewRecorder()

	srv.HandleIndex(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
	}
}

func pairRemoteClient(t *testing.T, srv *Server, token string) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/?token="+token, nil)
	req.RemoteAddr = "192.168.1.50:54321"
	rec := httptest.NewRecorder()
	srv.HandleIndex(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on pairing, got %d", rec.Code)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "guaita_session" {
			return c
		}
	}
	t.Fatalf("expected guaita_session cookie after pairing")
	return nil
}

func TestHandleIndex_RemoteAuthorizedWithQueryToken(t *testing.T) {
	srv := New(Config{SessionToken: "secret123", SessionDays: 5})
	foundCookie := pairRemoteClient(t, srv, "secret123")

	if foundCookie.Value == "" || foundCookie.Value == "secret123" {
		t.Errorf("expected unique opaque session cookie distinct from pairing token, got %q", foundCookie.Value)
	}
	if !foundCookie.HttpOnly {
		t.Errorf("expected cookie to be HttpOnly")
	}
	if foundCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("expected SameSiteLaxMode")
	}

	// Follow-up request to "/" (or reload of "/?token=secret123") with the issued cookie succeeds
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "192.168.1.50:54321"
	req2.AddCookie(foundCookie)
	rec2 := httptest.NewRecorder()
	srv.HandleIndex(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on follow-up request, got %d", rec2.Code)
	}
	body := rec2.Body.String()
	if !strings.Contains(body, "Guaita // Mobile Sentinel") {
		t.Errorf("expected body to contain mobile title")
	}
	if !strings.Contains(body, "Expires in 5 days") {
		t.Errorf("expected body to contain 'Expires in 5 days'")
	}
}

func TestHandleIndex_RemoteAuthorizedWithCookie(t *testing.T) {
	srv := New(Config{SessionToken: "secret123", SessionDays: 7})
	cookie := pairRemoteClient(t, srv, "secret123")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.50:54321" // remote
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	srv.HandleIndex(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
}

func TestHandleAction_RemoteUnauthorized(t *testing.T) {
	srv := New(Config{SessionToken: "secret123", SessionDays: 7})
	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader("action=approve"))
	req.RemoteAddr = "192.168.1.50:54321" // remote
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.HandleAction(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
	}
}

func TestHandleAction_LocalhostAuthorized(t *testing.T) {
	var capturedAction string
	srv := New(Config{SessionToken: "secret123", SessionDays: 7})
	srv.actionSender = func(action string) error {
		capturedAction = action
		return nil
	}

	form := url.Values{}
	form.Set("action", "approve")
	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(form.Encode()))
	req.RemoteAddr = "127.0.0.1:44444" // loopback
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.HandleAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if capturedAction != "approve" {
		t.Errorf("expected captured action 'approve', got %s", capturedAction)
	}
}

func TestActionsSharePendingStateAcrossWebAndTelegram(t *testing.T) {
	dispatcher := notify.NewDispatcher()
	srv := New(Config{SessionToken: "secret123", Dispatcher: dispatcher})
	var calls int
	srv.actionPaneSender = func(paneID, action string) error {
		calls++
		if paneID != "%4" || action != "approve" {
			t.Fatalf("unexpected action (%q, %q)", paneID, action)
		}
		return nil
	}
	dispatcher.SyncWaitingPanes([]notify.NotificationEvent{{PaneID: "%4", Body: "Approve?"}})

	form := url.Values{"action": {"approve"}, "pane_id": {"%4"}}
	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(form.Encode()))
	req.RemoteAddr = "127.0.0.1:44444"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.HandleAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected web action to succeed, got %d", rec.Code)
	}
	if err := srv.HandleTelegramAction("approve:%4"); !errors.Is(err, errApprovalConsumed) {
		t.Fatalf("expected stale Telegram action to be rejected, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly one agent action, got %d", calls)
	}

	status := herdr.AgentStatus{
		Waiting: true,
		PaneID:  "%4",
		Panes:   []herdr.PaneSummary{{PaneID: "%4", Waiting: true}},
	}
	srv.maskConsumedWaiting(&status)
	if status.Waiting || status.Panes[0].Waiting {
		t.Fatal("expected web status to hide the consumed request")
	}
}

func TestTelegramActionPreventsRepeatedWebApproval(t *testing.T) {
	dispatcher := notify.NewDispatcher()
	srv := New(Config{SessionToken: "secret123", Dispatcher: dispatcher})
	var calls int
	srv.actionPaneSender = func(_, _ string) error {
		calls++
		return nil
	}
	dispatcher.SyncWaitingPanes([]notify.NotificationEvent{{PaneID: "%8"}})
	if err := srv.HandleTelegramAction("reject:%8"); err != nil {
		t.Fatalf("expected Telegram action to succeed: %v", err)
	}

	form := url.Values{"action": {"reject"}, "pane_id": {"%8"}}
	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(form.Encode()))
	req.RemoteAddr = "127.0.0.1:44444"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.HandleAction(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected duplicate web action to return 409, got %d", rec.Code)
	}
	if calls != 1 {
		t.Fatalf("expected one dispatch, got %d", calls)
	}
}

func TestHandleAction_RemoteAuthorizedWithHeadersAndJSON(t *testing.T) {
	var capturedAction string
	srv := New(Config{SessionToken: "secret123", SessionDays: 7})
	srv.actionSender = func(action string) error {
		capturedAction = action
		return nil
	}
	cookie := pairRemoteClient(t, srv, "secret123")

	// 1. With X-Guaita-Token header (using active session ID) and JSON body
	reqJSON := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(`{"action":"reject"}`))
	reqJSON.RemoteAddr = "192.168.1.50:54321"
	reqJSON.Header.Set("X-Guaita-Token", cookie.Value)
	recJSON := httptest.NewRecorder()
	srv.HandleAction(recJSON, reqJSON)

	if recJSON.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for X-Guaita-Token, got %d", recJSON.Code)
	}
	if capturedAction != "reject" {
		t.Errorf("expected 'reject', got %s", capturedAction)
	}

	// 2. With Authorization Bearer and form body
	reqBearer := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader("action=enter"))
	reqBearer.RemoteAddr = "192.168.1.50:54321"
	reqBearer.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqBearer.Header.Set("Authorization", "Bearer "+cookie.Value)
	recBearer := httptest.NewRecorder()
	srv.HandleAction(recBearer, reqBearer)

	if recBearer.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for Bearer token, got %d", recBearer.Code)
	}
	if capturedAction != "enter" {
		t.Errorf("expected 'enter', got %s", capturedAction)
	}
}

func TestHandleDisconnectClient_Security(t *testing.T) {
	srv := New(Config{SessionToken: "secret123"})

	// 1. Remote request -> 403 Forbidden
	reqRemote := httptest.NewRequest(http.MethodPost, "/api/clients/disconnect", strings.NewReader("id=abc"))
	reqRemote.RemoteAddr = "192.168.1.50:12345"
	reqRemote.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recRemote := httptest.NewRecorder()
	srv.HandleDisconnectClient(recRemote, reqRemote)
	if recRemote.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for remote disconnect, got %d", recRemote.Code)
	}

	// 2. Localhost request -> 200 OK
	reqLocal := httptest.NewRequest(http.MethodPost, "/api/clients/disconnect", strings.NewReader("id=abc"))
	reqLocal.RemoteAddr = "127.0.0.1:12345"
	reqLocal.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recLocal := httptest.NewRecorder()
	srv.HandleDisconnectClient(recLocal, reqLocal)
	if recLocal.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for localhost disconnect, got %d", recLocal.Code)
	}
}

func TestHandleRegenerateToken_Security(t *testing.T) {
	srv := New(Config{SessionToken: "initial-token", LocalIP: "192.168.1.100", Port: "8085"})

	// 1. Remote request -> 403 Forbidden
	reqRemote := httptest.NewRequest(http.MethodPost, "/api/token/regenerate", nil)
	reqRemote.RemoteAddr = "192.168.1.50:12345"
	recRemote := httptest.NewRecorder()
	srv.HandleRegenerateToken(recRemote, reqRemote)
	if recRemote.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for remote token regeneration, got %d", recRemote.Code)
	}

	// 2. Localhost request -> 200 OK with new token
	reqLocal := httptest.NewRequest(http.MethodPost, "/api/token/regenerate", nil)
	reqLocal.RemoteAddr = "127.0.0.1:12345"
	recLocal := httptest.NewRecorder()
	srv.HandleRegenerateToken(recLocal, reqLocal)
	if recLocal.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for localhost token regeneration, got %d", recLocal.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(recLocal.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if resp["token"] == "initial-token" {
		t.Errorf("token should have changed from initial-token")
	}
	if srv.Token() != resp["token"] {
		t.Errorf("server token mismatch: %s != %s", srv.Token(), resp["token"])
	}
	if resp["qr_base64"] == "" {
		t.Errorf("expected non-empty qr_base64 in response")
	}
}

type memoryListener struct {
	conns  chan net.Conn
	closed bool
	mu     sync.Mutex
}

func newMemoryListener() *memoryListener {
	return &memoryListener{conns: make(chan net.Conn, 10)}
}

func (l *memoryListener) Accept() (net.Conn, error) {
	conn, ok := <-l.conns
	if !ok {
		return nil, errors.New("listener closed")
	}
	return conn, nil
}

func (l *memoryListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.closed {
		l.closed = true
		close(l.conns)
	}
	return nil
}

func (l *memoryListener) Addr() net.Addr {
	return &net.IPAddr{IP: net.IPv4(127, 0, 0, 1)}
}

type pipeAddrConn struct {
	net.Conn
	remoteAddr net.Addr
}

func (p *pipeAddrConn) RemoteAddr() net.Addr {
	return p.remoteAddr
}

func TestHandleWebSocket_StreamAndHub(t *testing.T) {
	srv := New(Config{
		SessionToken: "secret123",
		SessionDays:  7,
		PollInterval: 10 * time.Millisecond,
	})
	srv.statusProvider = func() (herdr.AgentStatus, error) {
		return herdr.AgentStatus{Output: "Prompt active", Waiting: true}, nil
	}

	listener := newMemoryListener()
	defer listener.Close()

	httpServer := &http.Server{Handler: srv.Routes()}
	go func() {
		_ = httpServer.Serve(listener)
	}()
	defer httpServer.Close()

	dialer := &websocket.Dialer{
		NetDial: func(network, addr string) (net.Conn, error) {
			serverConn, clientConn := net.Pipe()
			wrappedServerConn := &pipeAddrConn{
				Conn:       serverConn,
				remoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 54321},
			}
			listener.conns <- wrappedServerConn
			return clientConn, nil
		},
	}

	conn, resp, err := dialer.Dial("ws://localhost/ws", nil)
	if err != nil {
		t.Fatalf("unexpected WS dial error: %v (resp: %v)", err, resp)
	}
	defer conn.Close()

	var payload wsPayload
	if err := conn.ReadJSON(&payload); err != nil {
		t.Fatalf("failed to read JSON from websocket: %v", err)
	}

	if !payload.Waiting {
		t.Errorf("expected payload.Waiting to be true")
	}
	if payload.Output != "Prompt active" {
		t.Errorf("expected output 'Prompt active', got %s", payload.Output)
	}
	// Localhost WS should receive the clients list
	if len(payload.Clients) != 1 {
		t.Errorf("expected 1 connected client, got %d", len(payload.Clients))
	}
}

func TestHandleManifestAndSW(t *testing.T) {
	srv := New(Config{})

	// Manifest
	reqM := httptest.NewRequest(http.MethodGet, "/manifest.json", nil)
	recM := httptest.NewRecorder()
	srv.HandleManifest(recM, reqM)
	if recM.Code != http.StatusOK {
		t.Errorf("expected 200 OK for manifest, got %d", recM.Code)
	}
	if recM.Header().Get("Content-Type") != "application/manifest+json" {
		t.Errorf("unexpected content-type: %s", recM.Header().Get("Content-Type"))
	}

	// Service Worker
	reqSW := httptest.NewRequest(http.MethodGet, "/sw.js", nil)
	recSW := httptest.NewRecorder()
	srv.HandleServiceWorker(recSW, reqSW)
	if recSW.Code != http.StatusOK {
		t.Errorf("expected 200 OK for sw.js, got %d", recSW.Code)
	}
	if recSW.Header().Get("Content-Type") != "application/javascript" {
		t.Errorf("unexpected content-type: %s", recSW.Header().Get("Content-Type"))
	}
}

func TestHandleSelectPane_AndPaneRouting(t *testing.T) {
	srv := New(Config{SessionToken: "pane-secret"})
	var gotPane, gotAction string
	srv.actionPaneSender = func(paneID, action string) error {
		gotPane = paneID
		gotAction = action
		return nil
	}

	// Select a specific pane from localhost
	reqSel := httptest.NewRequest(http.MethodPost, "/api/panes/select", strings.NewReader("pane_id=w2:p1"))
	reqSel.RemoteAddr = "127.0.0.1:50000"
	reqSel.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recSel := httptest.NewRecorder()
	srv.HandleSelectPane(recSel, reqSel)
	if recSel.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on select pane, got %d", recSel.Code)
	}

	// Action without explicit pane_id must route to the pinned pane
	reqAct := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader("action=approve"))
	reqAct.RemoteAddr = "127.0.0.1:50000"
	reqAct.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recAct := httptest.NewRecorder()
	srv.HandleAction(recAct, reqAct)
	if recAct.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on action, got %d", recAct.Code)
	}
	if gotPane != "w2:p1" || gotAction != "approve" {
		t.Errorf("expected routed action (w2:p1, approve), got (%q, %q)", gotPane, gotAction)
	}

	// Action with explicit pane_id overrides pinned pane
	reqAct2 := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader("action=reject&pane_id=%253"))
	reqAct2.RemoteAddr = "127.0.0.1:50000"
	reqAct2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recAct2 := httptest.NewRecorder()
	srv.HandleAction(recAct2, reqAct2)
	if gotPane != "%3" || gotAction != "reject" {
		t.Errorf("expected explicit pane override (%%3, reject), got (%q, %q)", gotPane, gotAction)
	}
}

func TestHandleWebSocket_StreamsPromptTypeAndQuestionHighlight(t *testing.T) {
	srv := New(Config{
		SessionToken: "secret123",
		SessionDays:  7,
		PollInterval: 10 * time.Millisecond,
	})
	srv.statusProvider = func() (herdr.AgentStatus, error) {
		return herdr.AgentStatus{
			Output:      "Which environment should we deploy to?",
			Waiting:     true,
			PromptType:  "question",
			PromptText:  "Which environment should we deploy to?",
			AgentState:  "idle",
			PaneID:      "w1:p1",
			HerdrOnline: true,
		}, nil
	}

	listener := newMemoryListener()
	defer listener.Close()

	httpServer := &http.Server{Handler: srv.Routes()}
	go func() {
		_ = httpServer.Serve(listener)
	}()
	defer httpServer.Close()

	dialer := &websocket.Dialer{
		NetDial: func(network, addr string) (net.Conn, error) {
			serverConn, clientConn := net.Pipe()
			wrappedServerConn := &pipeAddrConn{
				Conn:       serverConn,
				remoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 54321},
			}
			listener.conns <- wrappedServerConn
			return clientConn, nil
		},
	}

	conn, _, err := dialer.Dial("ws://localhost/ws", nil)
	if err != nil {
		t.Fatalf("unexpected WS dial error: %v", err)
	}
	defer conn.Close()

	var payload wsPayload
	if err := conn.ReadJSON(&payload); err != nil {
		t.Fatalf("failed to read JSON from websocket: %v", err)
	}
	if !payload.Waiting || payload.PromptType != "question" {
		t.Errorf("expected waiting question payload, got waiting=%v type=%q", payload.Waiting, payload.PromptType)
	}
	if payload.PromptText != "Which environment should we deploy to?" {
		t.Errorf("unexpected prompt_text: %q", payload.PromptText)
	}

	// Verify mobile and admin HTML templates include state banners, question quote boxes, and scroll tracker
	mobileHTML := RenderMobileHTMLWithNonce(7, "testnonce")
	if strings.Contains(mobileHTML, "%!") {
		t.Errorf("mobileHTML contains broken fmt.Sprintf verb")
	}
	for _, needle := range []string{"agentStateBanner", "promptQuoteMobile", "questionPillsMobile", "createScrollTracker"} {
		if !strings.Contains(mobileHTML, needle) {
			t.Errorf("expected mobileHTML to contain %q", needle)
		}
	}

	adminHTML := RenderAdminHTML(AdminPageData{SessionDays: 7, CSPNonce: "testnonce"})
	if strings.Contains(adminHTML, "%!") {
		t.Errorf("adminHTML contains broken fmt.Sprintf verb")
	}
	for _, needle := range []string{"adminStateBanner", "promptQuoteAdmin", "questionActionsAdmin", "createAdminScrollTracker"} {
		if !strings.Contains(adminHTML, needle) {
			t.Errorf("expected adminHTML to contain %q", needle)
		}
	}
}

func TestHandleWebSocket_StreamsPromptOptionsAndDispatchesOptionActions(t *testing.T) {
	var capturedAction string
	var capturedPane string

	srv := New(Config{
		SessionToken: "secret123",
		SessionDays:  7,
		PollInterval: 10 * time.Millisecond,
	})
	srv.statusProvider = func() (herdr.AgentStatus, error) {
		return herdr.AgentStatus{
			Output:     "Would you like to run the following command?\n› 1. Yes, proceed\n  2. Always allow",
			Waiting:    true,
			PromptType: "confirm",
			PromptText: "Would you like to run the following command?",
			PromptOptions: []herdr.PromptOption{
				{Key: "1", Label: "Yes, proceed (y)", Style: "approve"},
				{Key: "2", Label: "Yes, and don't ask again (p)", Style: "always"},
				{Key: "esc", Label: "No, and tell Codex what to do differently (esc)", Style: "reject"},
			},
			AgentState:  "idle",
			PaneID:      "w1:p2",
			HerdrOnline: true,
		}, nil
	}
	srv.actionPaneSender = func(paneID, action string) error {
		capturedPane = paneID
		capturedAction = action
		return nil
	}

	listener := newMemoryListener()
	defer listener.Close()

	httpServer := &http.Server{Handler: srv.Routes()}
	go func() {
		_ = httpServer.Serve(listener)
	}()
	defer httpServer.Close()

	dialer := &websocket.Dialer{
		NetDial: func(network, addr string) (net.Conn, error) {
			serverConn, clientConn := net.Pipe()
			wrappedServerConn := &pipeAddrConn{
				Conn:       serverConn,
				remoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 54321},
			}
			listener.conns <- wrappedServerConn
			return clientConn, nil
		},
	}

	conn, _, err := dialer.Dial("ws://localhost/ws", nil)
	if err != nil {
		t.Fatalf("unexpected WS dial error: %v", err)
	}
	defer conn.Close()

	var payload wsPayload
	if err := conn.ReadJSON(&payload); err != nil {
		t.Fatalf("failed to read JSON from websocket: %v", err)
	}
	if !payload.Waiting || len(payload.PromptOptions) != 3 {
		t.Fatalf("expected 3 prompt options, got %d (waiting=%v)", len(payload.PromptOptions), payload.Waiting)
	}
	if payload.PromptOptions[0].Key != "1" || payload.PromptOptions[0].Style != "approve" {
		t.Errorf("unexpected option 0: %+v", payload.PromptOptions[0])
	}
	if payload.PromptOptions[1].Key != "2" || payload.PromptOptions[1].Style != "always" {
		t.Errorf("unexpected option 1: %+v", payload.PromptOptions[1])
	}
	if payload.PromptOptions[2].Key != "esc" || payload.PromptOptions[2].Style != "reject" {
		t.Errorf("unexpected option 2: %+v", payload.PromptOptions[2])
	}

	// Verify template HTML elements and CSS classes for prompt options
	mobileHTML := RenderMobileHTMLWithNonce(7, "testnonce")
	if strings.Contains(mobileHTML, "%!") {
		t.Errorf("mobileHTML contains broken fmt.Sprintf verb")
	}
	for _, needle := range []string{"optionsPillsMobile", "pill-option", "pill-always"} {
		if !strings.Contains(mobileHTML, needle) {
			t.Errorf("expected mobileHTML to contain %q", needle)
		}
	}

	adminHTML := RenderAdminHTML(AdminPageData{SessionDays: 7, CSPNonce: "testnonce"})
	if strings.Contains(adminHTML, "%!") {
		t.Errorf("adminHTML contains broken fmt.Sprintf verb")
	}
	for _, needle := range []string{"optionsActionsAdmin", "btn-act-option", "btn-act-always"} {
		if !strings.Contains(adminHTML, needle) {
			t.Errorf("expected adminHTML to contain %q", needle)
		}
	}

	// Dispatch option "2" with pane_id
	form := url.Values{"action": {"2"}, "pane_id": {"w1:p2"}}
	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(form.Encode()))
	req.RemoteAddr = "127.0.0.1:44444"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.HandleAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on option action dispatch, got %d", rec.Code)
	}
	if capturedAction != "2" || capturedPane != "w1:p2" {
		t.Errorf("expected dispatched action ('2', 'w1:p2'), got (%q, %q)", capturedAction, capturedPane)
	}
}

func TestNormalizePromptKey_StableAcrossTimerAndSpinnerUpdates(t *testing.T) {
	opts := []notify.NotificationOption{
		{Key: "1", Label: "Yes"},
		{Key: "2", Label: "No"},
	}

	key1 := normalizePromptKey("confirm", "Allow file edit? [0:12] · 128tokens\n❯ 1. Yes\n  2. No", opts)
	key2 := normalizePromptKey("confirm", "Allow file edit? [0:15] · 140tokens\n› 1. Yes\n  2. No", opts)

	if key1 != key2 {
		t.Fatalf("expected keys to be identical despite timer/spinner differences:\nkey1=%q\nkey2=%q", key1, key2)
	}
}

func TestDispatchWaitingStatuses_SuppressesPushAlertsForIdleQuestionsWithoutOptions(t *testing.T) {
	dispatcher := notify.NewDispatcher()
	srv := New(Config{SessionToken: "test", Dispatcher: dispatcher})

	// 1. Idle conversational question without choices is filtered out from push dispatcher
	idleQuestion := []herdr.AgentStatus{
		{
			PaneID:        "w1:p1",
			PromptType:    "question",
			PromptText:    "Any other questions?",
			PromptOptions: nil,
			AgentState:    "idle",
			Waiting:       true,
		},
	}
	srv.dispatchWaitingStatuses(idleQuestion)
	if dispatcher.IsWaitingConsumed("w1:p1") || dispatcher.ClaimWaiting("w1:p1") != notify.ClaimMissing {
		t.Errorf("expected idle conversational question to NOT be registered in notification dispatcher")
	}

	// 2. Blocked question from orchestrator IS registered
	blockedQuestion := []herdr.AgentStatus{
		{
			PaneID:        "w1:p2",
			PromptType:    "question",
			PromptText:    "Enter API key:",
			PromptOptions: nil,
			AgentState:    "blocked",
			Waiting:       true,
		},
	}
	srv.dispatchWaitingStatuses(blockedQuestion)
	if dispatcher.ClaimWaiting("w1:p2") != notify.ClaimAcquired {
		t.Errorf("expected blocked question from orchestrator to be registered in notification dispatcher")
	}

	// 3. Question with interactive options IS registered
	questionWithOptions := []herdr.AgentStatus{
		{
			PaneID:     "w1:p3",
			PromptType: "question",
			PromptText: "Which engine?",
			PromptOptions: []herdr.PromptOption{
				{Key: "1", Label: "PostgreSQL"},
			},
			AgentState: "idle",
			Waiting:    true,
		},
	}
	srv.dispatchWaitingStatuses(questionWithOptions)
	if dispatcher.ClaimWaiting("w1:p3") != notify.ClaimAcquired {
		t.Errorf("expected question with options to be registered in notification dispatcher")
	}
}
