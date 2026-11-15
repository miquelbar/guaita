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

	"guaita/herdr"
	"github.com/gorilla/websocket"
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

func TestHandleIndex_RemoteAuthorizedWithQueryToken(t *testing.T) {
	srv := New(Config{SessionToken: "secret123", SessionDays: 5})
	req := httptest.NewRequest(http.MethodGet, "/?token=secret123", nil)
	req.RemoteAddr = "192.168.1.50:54321" // remote
	rec := httptest.NewRecorder()

	srv.HandleIndex(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	cookies := rec.Result().Cookies()
	var foundCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "guaita_session" {
			foundCookie = c
			break
		}
	}

	if foundCookie == nil {
		t.Fatalf("expected cookie 'guaita_session' to be set")
	}
	if foundCookie.Value != "secret123" {
		t.Errorf("expected cookie value 'secret123', got %s", foundCookie.Value)
	}
	if !foundCookie.HttpOnly {
		t.Errorf("expected cookie to be HttpOnly")
	}
	if foundCookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("expected SameSiteStrictMode")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Guaita // Mobile Sentinel") {
		t.Errorf("expected body to contain mobile title")
	}
	if !strings.Contains(body, "Expires in 5 days") {
		t.Errorf("expected body to contain 'Expires in 5 days'")
	}
}

func TestHandleIndex_RemoteAuthorizedWithCookie(t *testing.T) {
	srv := New(Config{SessionToken: "secret123", SessionDays: 7})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.50:54321" // remote
	req.AddCookie(&http.Cookie{Name: "guaita_session", Value: "secret123"})
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

	conn, resp, err := dialer.Dial("ws://memory/ws", nil)
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
