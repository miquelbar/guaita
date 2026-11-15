package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"guaita/herdr"
	"guaita/notify"
	"github.com/gorilla/websocket"
)

func TestSecurity_OneTimePairingAndNoTwoSessions(t *testing.T) {
	srv := New(Config{SessionToken: "pair-token-xyz", SessionDays: 7})

	// 0. HEAD or prefetch requests must NOT consume the one-time pairing token
	reqHead := httptest.NewRequest(http.MethodHead, "/?token=pair-token-xyz", nil)
	reqHead.RemoteAddr = "192.168.1.50:54321"
	recHead := httptest.NewRecorder()
	srv.HandleIndex(recHead, reqHead)
	if recHead.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for HEAD, got %d", recHead.Code)
	}

	reqPrefetch := httptest.NewRequest(http.MethodGet, "/?token=pair-token-xyz", nil)
	reqPrefetch.RemoteAddr = "192.168.1.50:54321"
	reqPrefetch.Header.Set("Sec-Purpose", "prefetch")
	recPrefetch := httptest.NewRecorder()
	srv.HandleIndex(recPrefetch, reqPrefetch)
	if recPrefetch.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for prefetch, got %d", recPrefetch.Code)
	}

	// 1. Legitimate device 1 pairs with the one-time token
	cookie1 := pairRemoteClient(t, srv, "pair-token-xyz")

	// 1b. Device 1 reloading the same /?token=... URL WITH its cookie still succeeds
	reqReload := httptest.NewRequest(http.MethodGet, "/?token=pair-token-xyz", nil)
	reqReload.RemoteAddr = "192.168.1.50:54321"
	reqReload.AddCookie(cookie1)
	recReload := httptest.NewRecorder()
	srv.HandleIndex(recReload, reqReload)
	if recReload.Code != http.StatusOK {
		t.Fatalf("expected 200 when paired device reloads URL with valid cookie, got %d", recReload.Code)
	}

	// 2. Attacker / device 2 attempts to reuse the same pairing token
	req2 := httptest.NewRequest(http.MethodGet, "/?token=pair-token-xyz", nil)
	req2.RemoteAddr = "192.168.1.77:12345"
	rec2 := httptest.NewRecorder()
	srv.HandleIndex(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when reusing claimed pairing token, got %d", rec2.Code)
	}

	// 3. Attacker attempts to use the raw pairing token as a session cookie or Bearer token
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.RemoteAddr = "192.168.1.77:12345"
	req3.AddCookie(&http.Cookie{Name: "guaita_session", Value: "pair-token-xyz"})
	rec3 := httptest.NewRecorder()
	srv.HandleIndex(rec3, req3)
	if rec3.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when using pairing token as session cookie, got %d", rec3.Code)
	}

	// 4. Device 1's issued session cookie works
	req4 := httptest.NewRequest(http.MethodGet, "/", nil)
	req4.RemoteAddr = "192.168.1.50:54321"
	req4.AddCookie(cookie1)
	rec4 := httptest.NewRecorder()
	srv.HandleIndex(rec4, req4)
	if rec4.Code != http.StatusOK {
		t.Fatalf("expected 200 for paired session cookie, got %d", rec4.Code)
	}

	// 5. Regenerating token revokes Device 1's session cookie immediately
	reqRegen := httptest.NewRequest(http.MethodPost, "/api/token/regenerate", nil)
	reqRegen.RemoteAddr = "127.0.0.1:50000"
	recRegen := httptest.NewRecorder()
	srv.HandleRegenerateToken(recRegen, reqRegen)
	if recRegen.Code != http.StatusOK {
		t.Fatalf("expected 200 on token regenerate, got %d", recRegen.Code)
	}

	reqAfter := httptest.NewRequest(http.MethodGet, "/", nil)
	reqAfter.RemoteAddr = "192.168.1.50:54321"
	reqAfter.AddCookie(cookie1)
	recAfter := httptest.NewRecorder()
	srv.HandleIndex(recAfter, reqAfter)
	if recAfter.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after token regeneration, got %d", recAfter.Code)
	}
}

func TestSecurity_SingleRemoteWebSocketEnforced(t *testing.T) {
	srv := New(Config{
		SessionToken: "pair-ws-token",
		PollInterval: 20 * time.Millisecond,
	})
	srv.statusProvider = func() (herdr.AgentStatus, error) {
		return herdr.AgentStatus{Output: "ok", HerdrOnline: true}, nil
	}
	cookie := pairRemoteClient(t, srv, "pair-ws-token")

	listener := newMemoryListener()
	defer listener.Close()

	httpServer := &http.Server{Handler: srv.Routes()}
	go func() {
		_ = httpServer.Serve(listener)
	}()
	defer httpServer.Close()

	dialRemote := func() *websocket.Conn {
		dialer := websocket.Dialer{
			NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				clientConn, serverConn := net.Pipe()
				listener.conns <- &pipeAddrConn{
					Conn:       serverConn,
					remoteAddr: &net.TCPAddr{IP: net.ParseIP("192.168.1.50"), Port: 54321},
				}
				return clientConn, nil
			},
		}
		h := http.Header{}
		h.Set("Cookie", cookie.Name+"="+cookie.Value)
		conn, _, err := dialer.Dial("ws://127.0.0.1:8085/ws", h)
		if err != nil {
			t.Fatalf("failed to dial remote WS: %v", err)
		}
		return conn
	}

	conn1 := dialRemote()
	defer conn1.Close()
	time.Sleep(30 * time.Millisecond)
	if srv.hub.RemoteCount() != 1 {
		t.Fatalf("expected 1 remote WS client, got %d", srv.hub.RemoteCount())
	}

	// Second remote WS connection with same active session evicts the first, keeping RemoteCount <= 1
	conn2 := dialRemote()
	defer conn2.Close()
	time.Sleep(30 * time.Millisecond)
	if srv.hub.RemoteCount() != 1 {
		t.Fatalf("expected at most 1 remote WS client, got %d", srv.hub.RemoteCount())
	}
}

func TestSecurity_CSWSH_CrossOriginWebSocketBlocked(t *testing.T) {
	srv := New(Config{SessionToken: "pair-cswsh", PollInterval: 50 * time.Millisecond})

	// Cross-origin WebSocket upgrade request must be rejected by CheckOrigin
	reqEvil := httptest.NewRequest(http.MethodGet, "/ws", nil)
	reqEvil.Host = "localhost:8085"
	reqEvil.RemoteAddr = "127.0.0.1:55555"
	reqEvil.Header.Set("Origin", "https://evil-attacker.example")
	reqEvil.Header.Set("Connection", "Upgrade")
	reqEvil.Header.Set("Upgrade", "websocket")
	reqEvil.Header.Set("Sec-WebSocket-Version", "13")
	reqEvil.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	if srv.upgrader.CheckOrigin(reqEvil) {
		t.Fatalf("expected CheckOrigin to reject cross-origin WebSocket request")
	}

	recEvil := httptest.NewRecorder()
	srv.HandleWebSocket(recEvil, reqEvil)
	if recEvil.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for cross-origin WebSocket upgrade, got %d", recEvil.Code)
	}

	// Same-origin WebSocket request must pass CheckOrigin
	reqSame := httptest.NewRequest(http.MethodGet, "/ws", nil)
	reqSame.Host = "localhost:8085"
	reqSame.Header.Set("Origin", "http://localhost:8085")
	if !srv.upgrader.CheckOrigin(reqSame) {
		t.Errorf("expected CheckOrigin to allow same-origin WebSocket request")
	}
}

func TestSecurity_CSRFAndMethodEnforcement(t *testing.T) {
	actionCalled := false
	srv := New(Config{SessionToken: "csrf-token"})
	srv.actionSender = func(a string) error {
		actionCalled = true
		return nil
	}

	// 1. GET /action must be rejected (405)
	reqGet := httptest.NewRequest(http.MethodGet, "/action?action=approve", nil)
	reqGet.RemoteAddr = "127.0.0.1:12345"
	recGet := httptest.NewRecorder()
	srv.HandleAction(recGet, reqGet)
	if recGet.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET /action, got %d", recGet.Code)
	}

	// 2. POST /action with cross-origin Origin header must be rejected (403)
	reqEvilOrigin := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader("action=approve"))
	reqEvilOrigin.Host = "localhost:8085"
	reqEvilOrigin.RemoteAddr = "127.0.0.1:12345"
	reqEvilOrigin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqEvilOrigin.Header.Set("Origin", "https://evil.example")
	recEvilOrigin := httptest.NewRecorder()
	srv.HandleAction(recEvilOrigin, reqEvilOrigin)
	if recEvilOrigin.Code != http.StatusForbidden {
		t.Errorf("expected 403 for cross-origin POST /action, got %d", recEvilOrigin.Code)
	}

	// 3. POST /action with Sec-Fetch-Site: cross-site must be rejected (403)
	reqCrossSite := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader("action=approve"))
	reqCrossSite.Host = "localhost:8085"
	reqCrossSite.RemoteAddr = "127.0.0.1:12345"
	reqCrossSite.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqCrossSite.Header.Set("Sec-Fetch-Site", "cross-site")
	recCrossSite := httptest.NewRecorder()
	srv.HandleAction(recCrossSite, reqCrossSite)
	if recCrossSite.Code != http.StatusForbidden {
		t.Errorf("expected 403 for Sec-Fetch-Site: cross-site, got %d", recCrossSite.Code)
	}

	// 4. GET /api/token/regenerate must be rejected (405)
	reqRegenGet := httptest.NewRequest(http.MethodGet, "/api/token/regenerate", nil)
	reqRegenGet.RemoteAddr = "127.0.0.1:12345"
	recRegenGet := httptest.NewRecorder()
	srv.HandleRegenerateToken(recRegenGet, reqRegenGet)
	if recRegenGet.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET /api/token/regenerate, got %d", recRegenGet.Code)
	}

	if actionCalled {
		t.Fatalf("actionSender should never have been called by blocked CSRF/GET requests")
	}
}

func TestSecurity_RateLimitingBruteForce(t *testing.T) {
	srv := New(Config{SessionToken: "real-token"})

	for i := 0; i < maxAuthFailures; i++ {
		req := httptest.NewRequest(http.MethodGet, "/?token=wrong-guess", nil)
		req.RemoteAddr = "192.168.1.99:40000"
		rec := httptest.NewRecorder()
		srv.HandleIndex(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, rec.Code)
		}
	}

	// Next attempt from same IP must be rate-limited (429)
	reqBlocked := httptest.NewRequest(http.MethodGet, "/?token=real-token", nil)
	reqBlocked.RemoteAddr = "192.168.1.99:40001"
	recBlocked := httptest.NewRecorder()
	srv.HandleIndex(recBlocked, reqBlocked)
	if recBlocked.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests after %d failures, got %d", maxAuthFailures, recBlocked.Code)
	}
}

func TestSecurity_HeadersAndTemplateHardening(t *testing.T) {
	srv := New(Config{SessionToken: "hdr-token"})
	handler := srv.Routes()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:55555"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	expectedHeaders := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
		"Cache-Control":          "no-store",
	}
	for k, want := range expectedHeaders {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("header %s = %q, want %q", k, got, want)
		}
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Errorf("expected Content-Security-Policy header to be set")
	}

	mobileHTML := RenderMobileHTML(7)
	if strings.Contains(mobileHTML, "localStorage.setItem('guaita_session_token'") {
		t.Errorf("mobile template must not store session token in localStorage")
	}
	if strings.Contains(mobileHTML, "/ws${tokenQuery}") {
		t.Errorf("mobile template must not pass token in WebSocket query string")
	}

	adminHTML := RenderAdminHTML(AdminPageData{SessionDays: 7})
	if strings.Contains(adminHTML, "onclick=\"disconnectClient") {
		t.Errorf("admin template must not use inline onclick string concatenation for client IDs")
	}
}

func TestSecurity_TelegramConfigEndpoint(t *testing.T) {
	disp := notify.NewDispatcher()
	srv := New(Config{SessionToken: "tg-sec", Dispatcher: disp})

	// 1. Remote client cannot configure Telegram (403 Forbidden)
	reqRemote := httptest.NewRequest(http.MethodPost, "/api/telegram/config", strings.NewReader(`{"bot_token":"123:abc","chat_id":"999"}`))
	reqRemote.RemoteAddr = "192.168.1.50:12345"
	recRemote := httptest.NewRecorder()
	srv.HandleTelegramConfig(recRemote, reqRemote)
	if recRemote.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for remote Telegram config, got %d", recRemote.Code)
	}

	// 2. GET method is rejected (405 Method Not Allowed)
	reqGet := httptest.NewRequest(http.MethodGet, "/api/telegram/config", nil)
	reqGet.RemoteAddr = "127.0.0.1:12345"
	recGet := httptest.NewRecorder()
	srv.HandleTelegramConfig(recGet, reqGet)
	if recGet.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET /api/telegram/config, got %d", recGet.Code)
	}

	// 3. Localhost POST enables Telegram without echoing bot_token in response
	reqOK := httptest.NewRequest(http.MethodPost, "/api/telegram/config", strings.NewReader(`{"bot_token":"123:secret","chat_id":"999"}`))
	reqOK.RemoteAddr = "127.0.0.1:12345"
	reqOK.Header.Set("Content-Type", "application/json")
	recOK := httptest.NewRecorder()
	srv.HandleTelegramConfig(recOK, reqOK)
	if recOK.Code != http.StatusOK {
		t.Fatalf("expected 200 for localhost Telegram config, got %d", recOK.Code)
	}
	if strings.Contains(recOK.Body.String(), "123:secret") {
		t.Fatalf("response must never echo bot_token")
	}
	if disp.ChannelCount() != 1 {
		t.Fatalf("expected 1 channel in dispatcher, got %d", disp.ChannelCount())
	}

	// 4. Disable cleanly stops the poller and removes from dispatcher
	reqDisable := httptest.NewRequest(http.MethodPost, "/api/telegram/config", strings.NewReader(`{"disable":true}`))
	reqDisable.RemoteAddr = "127.0.0.1:12345"
	recDisable := httptest.NewRecorder()
	srv.HandleTelegramConfig(recDisable, reqDisable)
	if recDisable.Code != http.StatusOK {
		t.Fatalf("expected 200 for disabling Telegram, got %d", recDisable.Code)
	}
	if disp.ChannelCount() != 0 {
		t.Fatalf("expected 0 channels after disable, got %d", disp.ChannelCount())
	}
}

func TestSecurity_CloudflareTunnelDoesNotBypassLocalhostAuth(t *testing.T) {
	srv := New(Config{SessionToken: "cf-secret", PublicURL: "https://test.trycloudflare.com"})

	// Request arriving from local cloudflared (127.0.0.1) WITH CF-Connecting-IP must NOT get Admin access
	reqCF := httptest.NewRequest(http.MethodGet, "/", nil)
	reqCF.RemoteAddr = "127.0.0.1:44444"
	reqCF.Header.Set("CF-Connecting-IP", "203.0.113.50")
	recCF := httptest.NewRecorder()
	srv.HandleIndex(recCF, reqCF)

	if recCF.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for proxied Cloudflare request without token, got %d", recCF.Code)
	}

	// Admin endpoints must reject Cloudflare-proxied requests even from 127.0.0.1
	reqAdmin := httptest.NewRequest(http.MethodPost, "/api/token/regenerate", nil)
	reqAdmin.RemoteAddr = "127.0.0.1:44444"
	reqAdmin.Header.Set("CF-Connecting-IP", "203.0.113.50")
	recAdmin := httptest.NewRecorder()
	srv.HandleRegenerateToken(recAdmin, reqAdmin)

	if recAdmin.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden on admin endpoint via Cloudflare Tunnel, got %d", recAdmin.Code)
	}
}

func TestSecurity_BanUnbanAndAuditLog(t *testing.T) {
	srv := New(Config{SessionToken: "ban-secret"})

	// Pair a remote client from 203.0.113.99 via Cloudflare header
	reqPair := httptest.NewRequest(http.MethodGet, "/?token=ban-secret", nil)
	reqPair.RemoteAddr = "127.0.0.1:50000"
	reqPair.Header.Set("CF-Connecting-IP", "203.0.113.99")
	reqPair.Header.Set("Cf-Ray", "test-ray-1")
	recPair := httptest.NewRecorder()
	srv.HandleIndex(recPair, reqPair)
	if recPair.Code != http.StatusOK {
		t.Fatalf("expected 200 on pairing, got %d", recPair.Code)
	}
	var sessionCookie *http.Cookie
	for _, c := range recPair.Result().Cookies() {
		if c.Name == "guaita_session" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatalf("expected session cookie")
	}

	// Ban 203.0.113.99 from localhost admin
	reqBan := httptest.NewRequest(http.MethodPost, "/api/clients/ban", strings.NewReader("ip=203.0.113.99"))
	reqBan.RemoteAddr = "127.0.0.1:50001"
	reqBan.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recBan := httptest.NewRecorder()
	srv.HandleBanClient(recBan, reqBan)
	if recBan.Code != http.StatusOK {
		t.Fatalf("expected 200 on ban, got %d", recBan.Code)
	}

	// Subsequent request from banned IP even with valid cookie must get 403 Forbidden
	reqAfterBan := httptest.NewRequest(http.MethodGet, "/", nil)
	reqAfterBan.RemoteAddr = "127.0.0.1:50002"
	reqAfterBan.Header.Set("CF-Connecting-IP", "203.0.113.99")
	reqAfterBan.Header.Set("Cf-Ray", "test-ray-2")
	reqAfterBan.AddCookie(sessionCookie)
	recAfterBan := httptest.NewRecorder()
	srv.HandleIndex(recAfterBan, reqAfterBan)
	if recAfterBan.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for banned IP, got %d", recAfterBan.Code)
	}

	// Verify audit log captured PAIR_SUCCESS, CLIENT_BAN, and BANNED_BLOCKED
	audit := srv.ListAudit()
	if len(audit) < 3 {
		t.Fatalf("expected at least 3 audit entries, got %d", len(audit))
	}

	// Unban IP
	reqUnban := httptest.NewRequest(http.MethodPost, "/api/clients/unban", strings.NewReader("ip=203.0.113.99"))
	reqUnban.RemoteAddr = "127.0.0.1:50001"
	reqUnban.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recUnban := httptest.NewRecorder()
	srv.HandleUnbanClient(recUnban, reqUnban)
	if recUnban.Code != http.StatusOK {
		t.Fatalf("expected 200 on unban, got %d", recUnban.Code)
	}
	if srv.IsBanned("203.0.113.99") {
		t.Errorf("expected IP to be unbanned")
	}
}


func TestSecurity_DNSRebindingHostValidation(t *testing.T) {
	srv := New(Config{
		SessionToken: "host-secret",
		LocalIP:      "192.168.1.40",
		PublicURL:    "https://guaita.example.com",
	})
	handler := srv.Routes()

	// Untrusted Host (DNS Rebinding) must be rejected even from 127.0.0.1
	reqEvil := httptest.NewRequest(http.MethodGet, "/", nil)
	reqEvil.RemoteAddr = "127.0.0.1:50000"
	reqEvil.Host = "evil-rebind.example:8085"
	recEvil := httptest.NewRecorder()
	handler.ServeHTTP(recEvil, reqEvil)
	if recEvil.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for untrusted Host, got %d", recEvil.Code)
	}

	// Localhost must be allowed
	reqLocal := httptest.NewRequest(http.MethodGet, "/", nil)
	reqLocal.RemoteAddr = "127.0.0.1:50000"
	reqLocal.Host = "localhost:8085"
	recLocal := httptest.NewRecorder()
	handler.ServeHTTP(recLocal, reqLocal)
	if recLocal.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for localhost, got %d", recLocal.Code)
	}

	// PublicURL host via 127.0.0.1 WITHOUT proxy headers must NOT get admin access
	reqTunnel := httptest.NewRequest(http.MethodGet, "/", nil)
	reqTunnel.RemoteAddr = "127.0.0.1:50000"
	reqTunnel.Host = "guaita.example.com"
	recTunnel := httptest.NewRecorder()
	handler.ServeHTTP(recTunnel, reqTunnel)
	if recTunnel.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for public tunnel host without token, got %d", recTunnel.Code)
	}
}

func TestSecurity_ActionSanitizationAndCSPNonce(t *testing.T) {
	var sentAction string
	srv := New(Config{SessionToken: "csp-secret"})
	srv.actionSender = func(a string) error {
		sentAction = a
		return nil
	}

	// Control characters (\x03, \x1b, \n) must be stripped
	reqCtrl := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader("action=hello\x03\x1b\nworld"))
	reqCtrl.RemoteAddr = "127.0.0.1:50000"
	reqCtrl.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recCtrl := httptest.NewRecorder()
	srv.HandleAction(recCtrl, reqCtrl)
	if recCtrl.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", recCtrl.Code)
	}
	if sentAction != "helloworld" {
		t.Errorf("expected control chars stripped to 'helloworld', got %q", sentAction)
	}

	// Oversized action (>4096 bytes) must be rejected with 400 Bad Request
	big := strings.Repeat("a", 5000)
	reqBig := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader("action="+big))
	reqBig.RemoteAddr = "127.0.0.1:50000"
	reqBig.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recBig := httptest.NewRecorder()
	srv.HandleAction(recBig, reqBig)
	if recBig.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for oversized action, got %d", recBig.Code)
	}

	// HandleIndex must set a per-request CSP script nonce and Secure cookie under X-Forwarded-Proto: https
	reqPair := httptest.NewRequest(http.MethodGet, "/?token=csp-secret", nil)
	reqPair.RemoteAddr = "127.0.0.1:50001"
	reqPair.Header.Set("X-Forwarded-Proto", "https")
	recPair := httptest.NewRecorder()
	srv.HandleIndex(recPair, reqPair)
	if recPair.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on pairing, got %d", recPair.Code)
	}
	csp := recPair.Header().Get("Content-Security-Policy")
	if strings.Contains(csp, "'unsafe-inline'") && strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
		t.Errorf("expected script-src without unsafe-inline, got %s", csp)
	}
	if !strings.Contains(csp, "'nonce-") {
		t.Errorf("expected CSP nonce in header, got %s", csp)
	}
	var pairedCookie *http.Cookie
	for _, c := range recPair.Result().Cookies() {
		if c.Name == "guaita_session" {
			pairedCookie = c
		}
	}
	if pairedCookie == nil || !pairedCookie.Secure {
		t.Errorf("expected Secure cookie when X-Forwarded-Proto is https")
	}
}


func TestSecurity_LocalhostAdminToken(t *testing.T) {
	srv := New(Config{
		SessionToken: "pair-secret",
		AdminToken:   "admin-secret-123",
	})

	// Direct localhost without admin_token must be rejected with 401
	reqNoAuth := httptest.NewRequest(http.MethodGet, "/", nil)
	reqNoAuth.RemoteAddr = "127.0.0.1:50000"
	recNoAuth := httptest.NewRecorder()
	srv.HandleIndex(recNoAuth, reqNoAuth)
	if recNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without admin_token, got %d", recNoAuth.Code)
	}

	// Admin API endpoint without admin cookie/header must be rejected with 403
	reqAPINoAuth := httptest.NewRequest(http.MethodPost, "/api/token/regenerate", nil)
	reqAPINoAuth.RemoteAddr = "127.0.0.1:50000"
	recAPINoAuth := httptest.NewRecorder()
	srv.HandleRegenerateToken(recAPINoAuth, reqAPINoAuth)
	if recAPINoAuth.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden on admin API without admin auth, got %d", recAPINoAuth.Code)
	}

	// Localhost with valid ?admin_token= must succeed and install guaita_admin HttpOnly cookie
	reqAuth := httptest.NewRequest(http.MethodGet, "/?admin_token=admin-secret-123", nil)
	reqAuth.RemoteAddr = "127.0.0.1:50000"
	recAuth := httptest.NewRecorder()
	srv.HandleIndex(recAuth, reqAuth)
	if recAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with valid admin_token, got %d", recAuth.Code)
	}
	var adminCookie *http.Cookie
	for _, c := range recAuth.Result().Cookies() {
		if c.Name == "guaita_admin" {
			adminCookie = c
		}
	}
	if adminCookie == nil || !adminCookie.HttpOnly {
		t.Fatalf("expected HttpOnly guaita_admin cookie")
	}

	// Subsequent admin API call with guaita_admin cookie must succeed
	reqAPIWithCookie := httptest.NewRequest(http.MethodPost, "/api/token/regenerate", nil)
	reqAPIWithCookie.RemoteAddr = "127.0.0.1:50000"
	reqAPIWithCookie.AddCookie(adminCookie)
	recAPIWithCookie := httptest.NewRecorder()
	srv.HandleRegenerateToken(recAPIWithCookie, reqAPIWithCookie)
	if recAPIWithCookie.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on admin API with guaita_admin cookie, got %d", recAPIWithCookie.Code)
	}
}
