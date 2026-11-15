package server

import (
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

func TestHandleIndex_Unauthorized(t *testing.T) {
	srv := New(Config{SessionToken: "secret123", SessionDays: 7})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.HandleIndex(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
	}
}

func TestHandleIndex_AuthorizedWithQueryToken(t *testing.T) {
	srv := New(Config{SessionToken: "secret123", SessionDays: 5})
	req := httptest.NewRequest(http.MethodGet, "/?token=secret123", nil)
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
	if !strings.Contains(body, "Guaita // Control Center") {
		t.Errorf("expected body to contain title")
	}
	if !strings.Contains(body, "Caduca en 5 días") {
		t.Errorf("expected body to contain 'Caduca en 5 días'")
	}
}

func TestHandleIndex_AuthorizedWithCookie(t *testing.T) {
	srv := New(Config{SessionToken: "secret123", SessionDays: 7})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "guaita_session", Value: "secret123"})
	rec := httptest.NewRecorder()

	srv.HandleIndex(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
}

func TestHandleAction_Unauthorized(t *testing.T) {
	srv := New(Config{SessionToken: "secret123", SessionDays: 7})
	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader("action=approve"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.HandleAction(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
	}
}

func TestHandleAction_Authorized(t *testing.T) {
	var capturedAction string
	srv := New(Config{SessionToken: "secret123", SessionDays: 7})
	srv.actionSender = func(action string) error {
		capturedAction = action
		return nil
	}

	form := url.Values{}
	form.Set("action", "approve")
	req := httptest.NewRequest(http.MethodPost, "/action?token=secret123", strings.NewReader(form.Encode()))
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

func TestHandleWebSocket_Unauthorized(t *testing.T) {
	srv := New(Config{SessionToken: "secret123", SessionDays: 7})
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rec := httptest.NewRecorder()

	srv.HandleWebSocket(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
	}
}

// In-memory net.Listener for fully sandboxed testing without TCP sockets
type memoryListener struct {
	conns  chan net.Conn
	closed bool
	mu     sync.Mutex
}

func newMemoryListener() *memoryListener {
	return &memoryListener{
		conns: make(chan net.Conn, 10),
	}
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

func TestHandleWebSocket_AuthorizedPipe(t *testing.T) {
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
			listener.conns <- serverConn
			return clientConn, nil
		},
	}

	// 1. Dial without token -> expect 401
	_, resp, err := dialer.Dial("ws://memory/ws", nil)
	if err == nil {
		t.Fatalf("expected error on unauthorized WS dial")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", resp.StatusCode)
	}

	// 2. Dial with valid token
	conn, resp, err := dialer.Dial("ws://memory/ws?token=secret123", nil)
	if err != nil {
		t.Fatalf("unexpected WS dial error: %v (resp: %v)", err, resp)
	}
	defer conn.Close()

	var status herdr.AgentStatus
	if err := conn.ReadJSON(&status); err != nil {
		t.Fatalf("failed to read JSON from websocket: %v", err)
	}

	if !status.Waiting {
		t.Errorf("expected status.Waiting to be true")
	}
	if status.Output != "Prompt active" {
		t.Errorf("expected output 'Prompt active', got %s", status.Output)
	}
}
