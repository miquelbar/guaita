package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"guaita/config"
	"guaita/herdr"
	"guaita/notify"
)

const (
	maxBodyBytes      = 64 * 1024
	maxActionBytes    = 4096
	maxAuthFailures   = 10
	authFailureWindow = 1 * time.Minute
	authBlockDuration = 5 * time.Minute
)

// Config specifies runtime parameters for the server.
type Config struct {
	SessionToken     string
	AdminToken       string
	SessionDays      int
	PollInterval     time.Duration
	LocalIP          string
	Port             string
	PublicURL        string
	TrustProxy       bool
	TLSEnabled       bool
	Dispatcher       *notify.Dispatcher
	WebPushStore     *notify.WebPushStore
	TelegramNotifier *notify.TelegramNotifier
	NtfyTopic        string
	NtfyServer       string
	TelegramEnabled  bool
	ConfigPath       string
}

type authFailureRecord struct {
	count        int
	windowStart  time.Time
	blockedUntil time.Time
}

type authRateLimiter struct {
	mu       sync.Mutex
	failures map[string]*authFailureRecord
}

func newAuthRateLimiter() *authRateLimiter {
	return &authRateLimiter{
		failures: make(map[string]*authFailureRecord),
	}
}

func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return strings.TrimPrefix(strings.TrimSuffix(remoteAddr, "]"), "[")
	}
	return strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
}

func (l *authRateLimiter) isBlocked(remoteAddr string) bool {
	ip := clientIP(remoteAddr)
	l.mu.Lock()
	defer l.mu.Unlock()

	rec, ok := l.failures[ip]
	if !ok {
		return false
	}
	now := time.Now()
	if !rec.blockedUntil.IsZero() && now.Before(rec.blockedUntil) {
		return true
	}
	if !rec.blockedUntil.IsZero() && now.After(rec.blockedUntil) {
		delete(l.failures, ip)
	}
	return false
}

func (l *authRateLimiter) recordFailure(remoteAddr string) {
	ip := clientIP(remoteAddr)
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	rec, ok := l.failures[ip]
	if !ok || now.Sub(rec.windowStart) > authFailureWindow {
		l.failures[ip] = &authFailureRecord{
			count:       1,
			windowStart: now,
		}
		return
	}
	rec.count++
	if rec.count >= maxAuthFailures {
		rec.blockedUntil = now.Add(authBlockDuration)
	}
}

func (l *authRateLimiter) reset(remoteAddr string) {
	ip := clientIP(remoteAddr)
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, ip)
}

func constantTimeEqual(a, b string) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func generateRandomHex(numBytes int) string {
	b := make([]byte, numBytes)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("Fatal error generating cryptographic bytes: %v", err)
	}
	return hex.EncodeToString(b)
}

const maxAuditEntries = 200

var errApprovalConsumed = errors.New("approval request has already been handled")

// AuditEntry records a security or remote-control event for the localhost admin.
type AuditEntry struct {
	Timestamp time.Time `json:"timestamp"`
	IP        string    `json:"ip"`
	Event     string    `json:"event"`
	Detail    string    `json:"detail"`
}

// Server provides HTTP and WebSocket handlers for Guaita.
type Server struct {
	mu                sync.RWMutex
	cfg               Config
	pairingClaimed    bool
	activeSessionID   string
	activeAdminCookie string
	selectedPane      string
	telegramNotifier  *notify.TelegramNotifier
	bannedIPs         map[string]time.Time
	auditLog          []AuditEntry
	hub               *Hub
	limiter           *authRateLimiter
	upgrader          websocket.Upgrader
	statusProvider    func() (herdr.AgentStatus, error)
	actionSender      func(string) error
	actionPaneSender  func(string, string) error
}

// New creates and configures a new Server instance.
func New(cfg Config) *Server {
	if cfg.SessionDays <= 0 {
		cfg.SessionDays = 7
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 1500 * time.Millisecond
	}
	if cfg.LocalIP == "" {
		cfg.LocalIP = "127.0.0.1"
	}
	if cfg.Port == "" {
		cfg.Port = "8085"
	}
	if cfg.NtfyServer == "" {
		cfg.NtfyServer = "https://ntfy.sh"
	}

	var adminCookie string
	if cfg.AdminToken != "" {
		adminCookie = generateRandomHex(32)
	}

	s := &Server{
		cfg:               cfg,
		activeAdminCookie: adminCookie,
		telegramNotifier:  cfg.TelegramNotifier,
		bannedIPs:         make(map[string]time.Time),
		auditLog:          make([]AuditEntry, 0, maxAuditEntries),
		hub:               NewHub(),
		limiter:           newAuthRateLimiter(),
	}
	s.upgrader = websocket.Upgrader{
		CheckOrigin: s.checkSameOrigin,
	}
	return s
}

func extractHostname(hostPort string) string {
	hostPort = strings.TrimSpace(hostPort)
	if hostPort == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(hostPort); err == nil {
		return strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	}
	return strings.TrimPrefix(strings.TrimSuffix(hostPort, "]"), "[")
}

func (s *Server) isAllowedHost(host string) bool {
	if pub := s.PublicURL(); pub != "" {
		if u, err := url.Parse(pub); err == nil && u.Host != "" && strings.EqualFold(host, u.Host) {
			return true
		}
	}
	return false
}

// isAllowedRequestHost mitigates DNS Rebinding attacks against localhost by
// ensuring the HTTP Host header matches an expected local or configured public host.
func (s *Server) isAllowedRequestHost(r *http.Request) bool {
	hostname := extractHostname(r.Host)
	if hostname == "" {
		return false
	}
	if strings.EqualFold(hostname, "localhost") || hostname == "127.0.0.1" || hostname == "::1" {
		return true
	}
	s.mu.RLock()
	localIP := s.cfg.LocalIP
	pubURL := s.cfg.PublicURL
	s.mu.RUnlock()

	if localIP != "" && strings.EqualFold(hostname, localIP) {
		return true
	}
	if pubURL != "" {
		if u, err := url.Parse(pubURL); err == nil && u.Hostname() != "" && strings.EqualFold(hostname, u.Hostname()) {
			return true
		}
	}
	return false
}

func (s *Server) hostValidationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAllowedRequestHost(r) {
			http.Error(w, "Forbidden: Untrusted Host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) checkSameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host) || s.isAllowedHost(u.Host)
}

func (s *Server) isSameOriginRequest(r *http.Request) bool {
	if sfs := r.Header.Get("Sec-Fetch-Site"); sfs != "" {
		if sfs != "same-origin" && sfs != "none" {
			return false
		}
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || (!strings.EqualFold(u.Host, r.Host) && !s.isAllowedHost(u.Host)) {
			return false
		}
	} else if ref := r.Header.Get("Referer"); ref != "" {
		u, err := url.Parse(ref)
		if err != nil || (!strings.EqualFold(u.Host, r.Host) && !s.isAllowedHost(u.Host)) {
			return false
		}
	}
	return true
}

func setCSPWithNonce(w http.ResponseWriter, nonce string) {
	if nonce != "" {
		w.Header().Set("Content-Security-Policy", fmt.Sprintf("default-src 'self'; script-src 'self' 'nonce-%s'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' ws: wss:; frame-ancestors 'none'", nonce))
		return
	}
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' ws: wss:; frame-ancestors 'none'")
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		setCSPWithNonce(w, "")
		next.ServeHTTP(w, r)
	})
}

// Routes returns the configured HTTP mux wrapped with Host validation and security headers.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.HandleIndex)
	mux.HandleFunc("/ws", s.HandleWebSocket)
	mux.HandleFunc("/action", s.HandleAction)
	mux.HandleFunc("/manifest.json", s.HandleManifest)
	mux.HandleFunc("/sw.js", s.HandleServiceWorker)
	mux.HandleFunc("/api/webpush/subscribe", s.HandleWebPushSubscribe)
	mux.HandleFunc("/api/clients/disconnect", s.HandleDisconnectClient)
	mux.HandleFunc("/api/clients/ban", s.HandleBanClient)
	mux.HandleFunc("/api/clients/unban", s.HandleUnbanClient)
	mux.HandleFunc("/api/token/regenerate", s.HandleRegenerateToken)
	mux.HandleFunc("/api/telegram/config", s.HandleTelegramConfig)
	mux.HandleFunc("/api/panes/select", s.HandleSelectPane)
	return securityHeadersMiddleware(s.hostValidationMiddleware(mux))
}

// Token returns the current session token thread-safely.
func (s *Server) Token() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.SessionToken
}

// SetToken updates the pairing token thread-safely and resets any active remote session.
func (s *Server) SetToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.SessionToken = token
	s.pairingClaimed = false
	s.activeSessionID = ""
}

func (s *Server) scheme() string {
	if s.cfg.TLSEnabled {
		return "https"
	}
	return "http"
}

// SetPublicURL updates the external tunnel/proxy base URL thread-safely.
func (s *Server) SetPublicURL(rawURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.PublicURL = strings.TrimSuffix(strings.TrimSpace(rawURL), "/")
}

// PublicURL returns the configured external URL, if any.
func (s *Server) PublicURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.PublicURL
}

// PairingURL constructs the full pairing URL containing the current one-time token.
func (s *Server) PairingURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.PublicURL != "" {
		return fmt.Sprintf("%s/?token=%s", s.cfg.PublicURL, s.cfg.SessionToken)
	}
	return fmt.Sprintf("%s://%s:%s/?token=%s", s.scheme(), s.cfg.LocalIP, s.cfg.Port, s.cfg.SessionToken)
}

// BaseURL constructs the base URL without any authentication token.
func (s *Server) BaseURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.PublicURL != "" {
		return s.cfg.PublicURL + "/"
	}
	return fmt.Sprintf("%s://%s:%s/", s.scheme(), s.cfg.LocalIP, s.cfg.Port)
}

// isLoopbackDirect returns true only for direct loopback connections without reverse-proxy headers
// and not targeting the configured public tunnel host.
func (s *Server) isLoopbackDirect(r *http.Request) bool {
	if !IsLoopback(r.RemoteAddr) {
		return false
	}
	proxyHeaders := []string{
		"CF-Connecting-IP",
		"Cf-Ray",
		"X-Forwarded-For",
		"X-Forwarded-Host",
		"X-Forwarded-Proto",
		"Forwarded",
		"Via",
	}
	for _, h := range proxyHeaders {
		if r.Header.Get(h) != "" {
			return false
		}
	}
	if pub := s.PublicURL(); pub != "" {
		if u, err := url.Parse(pub); err == nil && u.Hostname() != "" {
			if strings.EqualFold(extractHostname(r.Host), u.Hostname()) {
				return false
			}
		}
	}
	return true
}

// isLocalAdmin verifies direct loopback access and, when AdminToken is configured,
// requires a valid guaita_admin session cookie or admin token header.
func (s *Server) isLocalAdmin(r *http.Request) bool {
	if !s.isLoopbackDirect(r) {
		return false
	}
	s.mu.RLock()
	adminToken := s.cfg.AdminToken
	adminCookie := s.activeAdminCookie
	s.mu.RUnlock()

	if adminToken == "" {
		return true
	}
	if cookie, err := r.Cookie("guaita_admin"); err == nil && constantTimeEqual(cookie.Value, adminCookie) {
		return true
	}
	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		if constantTimeEqual(strings.TrimPrefix(authHeader, "Bearer "), adminToken) {
			return true
		}
	}
	if constantTimeEqual(r.Header.Get("X-Guaita-Admin-Token"), adminToken) {
		return true
	}
	return false
}

func (s *Server) effectiveRemoteIP(r *http.Request) string {
	if IsLoopback(r.RemoteAddr) {
		s.mu.RLock()
		hasPub := s.cfg.PublicURL != ""
		trustProxy := s.cfg.TrustProxy
		s.mu.RUnlock()

		isCloudflare := r.Header.Get("Cf-Ray") != "" || hasPub || trustProxy
		if cfIP := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cfIP != "" && isCloudflare {
			if parsed := net.ParseIP(cfIP); parsed != nil {
				return parsed.String()
			}
		}
		if hasPub || trustProxy {
			if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
				candidate := xff
				if idx := strings.Index(xff, ","); idx != -1 {
					candidate = strings.TrimSpace(xff[:idx])
				}
				if parsed := net.ParseIP(candidate); parsed != nil {
					return parsed.String()
				}
			}
		}
	}
	return clientIP(r.RemoteAddr)
}

// RecordAudit appends an entry to the bounded security audit log.
func (s *Server) RecordAudit(ip, event, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := AuditEntry{
		Timestamp: time.Now(),
		IP:        ip,
		Event:     event,
		Detail:    detail,
	}
	if len(s.auditLog) >= maxAuditEntries {
		copy(s.auditLog, s.auditLog[1:])
		s.auditLog[len(s.auditLog)-1] = entry
	} else {
		s.auditLog = append(s.auditLog, entry)
	}
}

// ListAudit returns a snapshot of the audit log (newest first).
func (s *Server) ListAudit() []AuditEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]AuditEntry, len(s.auditLog))
	for i := range s.auditLog {
		res[len(s.auditLog)-1-i] = s.auditLog[i]
	}
	return res
}

// BanIP bans an IP address and disconnects any active remote connections from it.
func (s *Server) BanIP(ip string) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return
	}
	s.mu.Lock()
	s.bannedIPs[ip] = time.Now()
	s.mu.Unlock()

	s.hub.DisconnectByIP(ip)
	if s.hub.RemoteCount() == 0 {
		s.mu.Lock()
		s.activeSessionID = ""
		s.mu.Unlock()
	}
	s.RecordAudit(ip, "CLIENT_BAN", "IP banned by localhost admin")
}

// UnbanIP removes an IP address from the ban list.
func (s *Server) UnbanIP(ip string) {
	ip = strings.TrimSpace(ip)
	s.mu.Lock()
	_, existed := s.bannedIPs[ip]
	delete(s.bannedIPs, ip)
	s.mu.Unlock()
	if existed {
		s.RecordAudit(ip, "CLIENT_UNBAN", "IP unbanned by localhost admin")
	}
}

// IsBanned checks if an IP address is currently banned.
func (s *Server) IsBanned(ip string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, banned := s.bannedIPs[ip]
	return banned
}

// ListBannedIPs returns all currently banned IP addresses.
func (s *Server) ListBannedIPs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]string, 0, len(s.bannedIPs))
	for ip := range s.bannedIPs {
		res = append(res, ip)
	}
	return res
}

// IsLoopback checks whether the connection originates from the local machine.
func IsLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}
	return host == "localhost"
}

// IsAuthorized verifies whether a request is from direct localhost or carries the single active remote session cookie/bearer.
func (s *Server) IsAuthorized(r *http.Request) bool {
	if s.isLocalAdmin(r) {
		return true
	}

	s.mu.RLock()
	activeID := s.activeSessionID
	s.mu.RUnlock()

	if activeID == "" {
		return false
	}

	if cookie, err := r.Cookie("guaita_session"); err == nil && constantTimeEqual(cookie.Value, activeID) {
		return true
	}
	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		if constantTimeEqual(strings.TrimPrefix(authHeader, "Bearer "), activeID) {
			return true
		}
	}
	if constantTimeEqual(r.Header.Get("X-Guaita-Token"), activeID) {
		return true
	}
	return false
}

// claimPairingToken atomically exchanges the one-time pairing token for a unique session ID.
func (s *Server) claimPairingToken(provided string) (string, bool) {
	if provided == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pairingClaimed || s.cfg.SessionToken == "" {
		return "", false
	}
	if !constantTimeEqual(provided, s.cfg.SessionToken) {
		return "", false
	}

	s.pairingClaimed = true
	s.activeSessionID = generateRandomHex(32)
	return s.activeSessionID, true
}

func (s *Server) checkRemoteAuth(w http.ResponseWriter, r *http.Request) bool {
	if s.isLocalAdmin(r) {
		return true
	}
	ip := s.effectiveRemoteIP(r)
	if s.IsBanned(ip) {
		s.RecordAudit(ip, "BANNED_BLOCKED", r.URL.Path)
		http.Error(w, "Forbidden: IP banned", http.StatusForbidden)
		return false
	}
	if s.limiter.isBlocked(ip) {
		s.RecordAudit(ip, "RATE_LIMITED", r.URL.Path)
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return false
	}
	if s.IsAuthorized(r) {
		s.limiter.reset(ip)
		return true
	}
	s.limiter.recordFailure(ip)
	s.RecordAudit(ip, "AUTH_FAILED", r.URL.Path)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
	return false
}

// HandleIndex serves the Admin Dashboard for localhost or the Mobile Gate for remote visitors.
func (s *Server) HandleIndex(w http.ResponseWriter, r *http.Request) {
	nonce := generateRandomHex(16)
	if s.isLoopbackDirect(r) {
		if !s.isLocalAdmin(r) {
			s.mu.RLock()
			adminToken := s.cfg.AdminToken
			adminCookie := s.activeAdminCookie
			s.mu.RUnlock()

			provided := r.URL.Query().Get("admin_token")
			if !constantTimeEqual(provided, adminToken) {
				s.RecordAudit("127.0.0.1", "ADMIN_AUTH_FAILED", "Missing or invalid admin_token on localhost")
				http.Error(w, "Unauthorized: Admin token required on localhost", http.StatusUnauthorized)
				return
			}
			isSecure := s.cfg.TLSEnabled || r.TLS != nil
			http.SetCookie(w, &http.Cookie{
				Name:     "guaita_admin",
				Value:    adminCookie,
				Expires:  time.Now().Add(time.Duration(s.cfg.SessionDays) * 24 * time.Hour),
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				Secure:   isSecure,
				Path:     "/",
			})
		}

		pairingURL := s.PairingURL()
		ntfyURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(s.cfg.NtfyServer, "/"), s.cfg.NtfyTopic)
		data := AdminPageData{
			PairingURL:     pairingURL,
			QRCodeBase64:   GenerateQRBase64(pairingURL),
			SessionDays:    s.cfg.SessionDays,
			SessionToken:   s.Token(),
			LocalIP:        s.cfg.LocalIP,
			Port:           s.cfg.Port,
			NtfyTopic:      s.cfg.NtfyTopic,
			NtfyURL:        ntfyURL,
			NtfyQRBase64:   GenerateQRBase64(ntfyURL),
			TelegramActive: s.cfg.TelegramEnabled,
			CSPNonce:       nonce,
		}
		setCSPWithNonce(w, nonce)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(RenderAdminHTML(data)))
		return
	}

	ip := s.effectiveRemoteIP(r)
	if s.IsBanned(ip) {
		s.RecordAudit(ip, "BANNED_BLOCKED", "/")
		http.Error(w, "Forbidden: IP banned", http.StatusForbidden)
		return
	}
	if s.limiter.isBlocked(ip) {
		s.RecordAudit(ip, "RATE_LIMITED", "/")
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return
	}

	// Do not let HEAD/OPTIONS or browser link-prefetch requests consume the one-time pairing token.
	if r.Method == http.MethodHead || r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if strings.Contains(strings.ToLower(r.Header.Get("Purpose")), "prefetch") ||
		strings.Contains(strings.ToLower(r.Header.Get("Sec-Purpose")), "prefetch") {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// If the client already holds the active session cookie, allow it even if ?token= is still in the URL.
	if s.IsAuthorized(r) {
		s.limiter.reset(ip)
		setCSPWithNonce(w, nonce)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(RenderMobileHTMLWithNonce(s.cfg.SessionDays, nonce)))
		return
	}

	if queryToken := r.URL.Query().Get("token"); queryToken != "" {
		if sessionID, ok := s.claimPairingToken(queryToken); ok {
			s.limiter.reset(ip)
			s.RecordAudit(ip, "PAIR_SUCCESS", r.UserAgent())
			isSecure := s.cfg.TLSEnabled ||
				r.TLS != nil ||
				strings.HasPrefix(s.PublicURL(), "https://") ||
				strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
			http.SetCookie(w, &http.Cookie{
				Name:     "guaita_session",
				Value:    sessionID,
				Expires:  time.Now().Add(time.Duration(s.cfg.SessionDays) * 24 * time.Hour),
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				Secure:   isSecure,
				Path:     "/",
			})
			setCSPWithNonce(w, nonce)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(RenderMobileHTMLWithNonce(s.cfg.SessionDays, nonce)))
			return
		}
		s.limiter.recordFailure(ip)
		s.RecordAudit(ip, "PAIR_FAILED", "Invalid or already claimed pairing token")
		http.Error(w, "Access Denied: Pairing token invalid or already claimed", http.StatusUnauthorized)
		return
	}

	s.limiter.recordFailure(ip)
	s.RecordAudit(ip, "AUTH_FAILED", "Missing token/cookie on /")
	http.Error(w, "Access Denied: Invalid token or expired session", http.StatusUnauthorized)
}

type wsPayload struct {
	Output        string               `json:"output"`
	Waiting       bool                 `json:"waiting"`
	PromptType    string               `json:"prompt_type,omitempty"`
	PromptText    string               `json:"prompt_text,omitempty"`
	PromptOptions []herdr.PromptOption `json:"prompt_options,omitempty"`
	PaneID        string               `json:"pane_id,omitempty"`
	SelectedPane  string               `json:"selected_pane,omitempty"`
	Panes         []herdr.PaneSummary  `json:"panes,omitempty"`
	AgentState    string               `json:"agent_state,omitempty"`
	Title         string               `json:"title,omitempty"`
	HerdrOnline   bool                 `json:"herdr_online"`
	Clients       []ClientInfo         `json:"clients,omitempty"`
	AuditLog      []AuditEntry         `json:"audit_log,omitempty"`
	BannedIPs     []string             `json:"banned_ips,omitempty"`
	PairingURL    string               `json:"pairing_url,omitempty"`
	QRBase64      string               `json:"qr_base64,omitempty"`
}

func (s *Server) pollStatus() (herdr.AgentStatus, []herdr.AgentStatus, string, error) {
	s.mu.RLock()
	preferred := s.selectedPane
	customProvider := s.statusProvider
	s.mu.RUnlock()

	if customProvider != nil {
		st, err := customProvider()
		var waiting []herdr.AgentStatus
		if st.Waiting {
			waiting = []herdr.AgentStatus{st}
		}
		return st, waiting, preferred, err
	}
	st, waiting, err := herdr.GetStatusWithPanes(preferred)
	return st, waiting, preferred, err
}

func (s *Server) sendPaneAction(paneID, action string) error {
	s.mu.RLock()
	customPaneSender := s.actionPaneSender
	customSender := s.actionSender
	s.mu.RUnlock()

	if customPaneSender != nil {
		return customPaneSender(paneID, action)
	}
	if paneID == "" && customSender != nil {
		return customSender(action)
	}
	if paneID != "" && customSender != nil && customPaneSender == nil {
		// Preserve compatibility when a test only overrides actionSender
		return customSender(action)
	}
	return herdr.SendActionToPane(paneID, action)
}

func (s *Server) sendCoordinatedAction(paneID, action string, requirePending bool) error {
	s.mu.RLock()
	dispatcher := s.cfg.Dispatcher
	s.mu.RUnlock()

	claim := notify.ClaimMissing
	if dispatcher != nil {
		claim = dispatcher.ClaimWaiting(paneID)
	}
	if claim == notify.ClaimConsumed {
		return errApprovalConsumed
	}
	if requirePending && claim == notify.ClaimMissing {
		return errApprovalConsumed
	}

	err := s.sendPaneAction(paneID, action)
	if claim == notify.ClaimAcquired {
		dispatcher.FinishWaiting(paneID, action, err == nil)
	}
	return err
}

// HandleTelegramAction routes a Telegram callback through the shared approval state.
func (s *Server) HandleTelegramAction(rawAction string) error {
	paneID, action, ok := notify.ParseTelegramAction(rawAction)
	if !ok {
		return errors.New("unsupported action")
	}
	if paneID == "" {
		s.mu.RLock()
		paneID = s.selectedPane
		s.mu.RUnlock()
	}
	return s.sendCoordinatedAction(paneID, action, true)
}

func (s *Server) maskConsumedWaiting(status *herdr.AgentStatus) {
	s.mu.RLock()
	dispatcher := s.cfg.Dispatcher
	s.mu.RUnlock()
	if dispatcher == nil {
		return
	}
	if status.Waiting && dispatcher.IsWaitingConsumed(status.PaneID) {
		status.Waiting = false
		status.PromptType = ""
		status.PromptText = ""
		status.PromptOptions = nil
	}
	if len(status.Panes) == 0 {
		return
	}
	status.Panes = append([]herdr.PaneSummary(nil), status.Panes...)
	for i := range status.Panes {
		if status.Panes[i].Waiting && dispatcher.IsWaitingConsumed(status.Panes[i].PaneID) {
			status.Panes[i].Waiting = false
			status.Panes[i].PromptType = ""
			status.Panes[i].PromptText = ""
			status.Panes[i].PromptOptions = nil
		}
	}
}

// StartMonitor polls statusProvider in the background and dispatches notifications
// even when no browser or mobile WebSocket client is connected.
func (s *Server) StartMonitor(stop <-chan struct{}) {
	go herdr.WatchAgentBlocked(stop, func() {
		_, waiting, _, _ := s.pollStatus()
		s.dispatchWaitingStatuses(waiting)
	})

	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()

	for {
		_, waiting, _, _ := s.pollStatus()
		s.dispatchWaitingStatuses(waiting)

		select {
		case <-stop:
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) dispatchWaitingStatuses(waiting []herdr.AgentStatus) {
	if s.cfg.Dispatcher == nil {
		return
	}
	events := make([]notify.NotificationEvent, 0, len(waiting))
	baseURL := s.BaseURL()
	for _, st := range waiting {
		label := "Approval Required"
		if len(st.PromptOptions) > 0 {
			label = fmt.Sprintf("Options Menu (%d Choices)", len(st.PromptOptions))
		} else if st.PromptType == "question" {
			label = "Question (Reply Needed)"
		}
		title := "🛡️ Guaita: " + label
		if st.PaneID != "" {
			title = fmt.Sprintf("🛡️ Guaita [%s]: %s", st.PaneID, label)
		}
		promptKey := ""
		if st.PromptType != "" || st.PromptText != "" {
			promptKey = st.PromptType + ":" + st.PromptText
		}

		notifOpts := make([]notify.NotificationOption, 0, len(st.PromptOptions))
		for _, o := range st.PromptOptions {
			notifOpts = append(notifOpts, notify.NotificationOption{
				Key:   o.Key,
				Label: o.Label,
				Style: o.Style,
			})
		}

		events = append(events, notify.NotificationEvent{
			Title:      title,
			Body:       st.Output,
			PaneID:     st.PaneID,
			PromptKey:  promptKey,
			PairingURL: baseURL,
			Options:    notifOpts,
		})
	}
	s.cfg.Dispatcher.SyncWaitingPanes(events)
}

// HandleWebSocket streams real-time status and telemetry to connected clients.
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !s.checkRemoteAuth(w, r) {
		return
	}

	isLocal := s.isLocalAdmin(r)
	remoteIP := s.effectiveRemoteIP(r)

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	clientAddr := remoteIP
	if !isLocal {
		s.RecordAudit(remoteIP, "WS_CONNECT", r.UserAgent())
	} else {
		clientAddr = r.RemoteAddr
	}
	s.hub.Register(conn, clientAddr, r.UserAgent(), isLocal)
	defer func() {
		s.hub.Unregister(conn)
		if !isLocal {
			s.RecordAudit(remoteIP, "WS_DISCONNECT", "Remote WebSocket closed")
		}
	}()

	// Start reader pump to consume client control frames (ping/pong/close)
	stopReader := make(chan struct{})
	go func() {
		defer close(stopReader)
		for {
			if _, _, err := conn.NextReader(); err != nil {
				return
			}
		}
	}()

	writeStatus := func() error {
		status, waiting, preferred, _ := s.pollStatus()
		s.dispatchWaitingStatuses(waiting)
		s.maskConsumedWaiting(&status)

		payload := wsPayload{
			Output:        status.Output,
			Waiting:       status.Waiting,
			PromptType:    status.PromptType,
			PromptText:    status.PromptText,
			PromptOptions: status.PromptOptions,
			PaneID:        status.PaneID,
			SelectedPane:  preferred,
			Panes:         status.Panes,
			AgentState:    status.AgentState,
			Title:         status.Title,
			HerdrOnline:   status.HerdrOnline,
		}
		if isLocal {
			payload.Clients = s.hub.ListClients()
			payload.AuditLog = s.ListAudit()
			payload.BannedIPs = s.ListBannedIPs()
			if pub := s.PublicURL(); pub != "" {
				payload.PairingURL = s.PairingURL()
				payload.QRBase64 = GenerateQRBase64(payload.PairingURL)
			}
		}

		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return conn.WriteJSON(payload)
	}

	// Send initial status frame immediately upon connection
	if err := writeStatus(); err != nil {
		return
	}

	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stopReader:
			return
		case <-ticker.C:
			if err := writeStatus(); err != nil {
				return
			}
		}
	}
}

func (s *Server) requirePostAndSameOrigin(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return false
	}
	if !s.isSameOriginRequest(r) {
		http.Error(w, "Forbidden: Cross-origin request blocked", http.StatusForbidden)
		return false
	}
	return true
}

// sanitizeActionInput bounds action length and strips dangerous ASCII control characters
// (e.g., Ctrl+C \x03, ESC \x1b, newlines) while preserving printable UTF-8, spaces, and tabs.
func sanitizeActionInput(raw string) (string, bool) {
	if len(raw) > maxActionBytes {
		return "", false
	}
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		if r == '\t' || (r >= 0x20 && r != 0x7f) {
			b.WriteRune(r)
		}
	}
	clean := strings.TrimSpace(b.String())
	if clean == "" || len(clean) > maxActionBytes {
		return "", false
	}
	return clean, true
}

// HandleAction receives actions from the web UI and forwards them to Herdr or tmux.
func (s *Server) HandleAction(w http.ResponseWriter, r *http.Request) {
	if !s.requirePostAndSameOrigin(w, r) {
		return
	}
	if !s.checkRemoteAuth(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	_ = r.ParseForm()
	action := r.FormValue("action")
	paneID := strings.TrimSpace(r.FormValue("pane_id"))
	if action == "" && r.Body != nil {
		var payload struct {
			Action string `json:"action"`
			PaneID string `json:"pane_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload.Action != "" {
			action = payload.Action
		}
		if paneID == "" {
			paneID = strings.TrimSpace(payload.PaneID)
		}
	}
	if action != "" {
		clean, ok := sanitizeActionInput(action)
		if !ok {
			http.Error(w, "Bad Request: invalid or oversized action", http.StatusBadRequest)
			return
		}
		if paneID == "" {
			s.mu.RLock()
			paneID = s.selectedPane
			s.mu.RUnlock()
		}
		detail := clean
		if paneID != "" {
			detail = fmt.Sprintf("[%s] %s", paneID, clean)
		}
		if err := s.sendCoordinatedAction(paneID, clean, false); err != nil {
			if errors.Is(err, errApprovalConsumed) {
				http.Error(w, "Conflict: approval request already handled", http.StatusConflict)
				return
			}
			http.Error(w, "Bad Gateway: failed to dispatch action", http.StatusBadGateway)
			return
		}
		s.RecordAudit(s.effectiveRemoteIP(r), "ACTION", detail)
	}
	w.WriteHeader(http.StatusOK)
}

// HandleSelectPane pins the monitored pane (or clears selection for Auto mode).
func (s *Server) HandleSelectPane(w http.ResponseWriter, r *http.Request) {
	if !s.requirePostAndSameOrigin(w, r) {
		return
	}
	if !s.checkRemoteAuth(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	_ = r.ParseForm()
	paneID := strings.TrimSpace(r.FormValue("pane_id"))
	if paneID == "" && r.Body != nil && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var payload struct {
			PaneID string `json:"pane_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		paneID = strings.TrimSpace(payload.PaneID)
	}

	s.mu.Lock()
	s.selectedPane = paneID
	s.mu.Unlock()

	label := paneID
	if label == "" {
		label = "auto"
	}
	s.RecordAudit(s.effectiveRemoteIP(r), "PANE_SELECT", label)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"selected_pane": paneID})
}

// HandleManifest serves the PWA manifest.
func (s *Server) HandleManifest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/manifest+json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(notify.ManifestJSON())
}

// HandleServiceWorker serves the PWA service worker.
func (s *Server) HandleServiceWorker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(notify.ServiceWorkerJS())
}

// HandleWebPushSubscribe registers a browser push subscription.
func (s *Server) HandleWebPushSubscribe(w http.ResponseWriter, r *http.Request) {
	if !s.requirePostAndSameOrigin(w, r) {
		return
	}
	if !s.checkRemoteAuth(w, r) {
		return
	}
	if s.cfg.WebPushStore == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var sub notify.WebPushSubscription
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		http.Error(w, "Invalid subscription payload", http.StatusBadRequest)
		return
	}
	s.cfg.WebPushStore.Subscribe(sub)
	w.WriteHeader(http.StatusOK)
}

// HandleDisconnectClient allows the localhost admin to disconnect a client and revoke its session.
func (s *Server) HandleDisconnectClient(w http.ResponseWriter, r *http.Request) {
	if !s.requirePostAndSameOrigin(w, r) {
		return
	}
	if !s.isLocalAdmin(r) {
		http.Error(w, "Forbidden: Localhost admin only", http.StatusForbidden)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	id := r.FormValue("id")
	ip, _ := s.hub.GetClientIP(id)
	if id != "" && s.hub.Disconnect(id) {
		s.RecordAudit(ip, "CLIENT_KICK", "Disconnected client id="+id)
		if s.hub.RemoteCount() == 0 {
			s.mu.Lock()
			s.activeSessionID = ""
			s.mu.Unlock()
		}
	}
	w.WriteHeader(http.StatusOK)
}

// HandleBanClient allows the localhost admin to permanently ban a client's IP and disconnect it.
func (s *Server) HandleBanClient(w http.ResponseWriter, r *http.Request) {
	if !s.requirePostAndSameOrigin(w, r) {
		return
	}
	if !s.isLocalAdmin(r) {
		http.Error(w, "Forbidden: Localhost admin only", http.StatusForbidden)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	ip := strings.TrimSpace(r.FormValue("ip"))
	if ip == "" && id != "" {
		if foundIP, ok := s.hub.GetClientIP(id); ok {
			ip = foundIP
		}
	}
	if ip == "" {
		http.Error(w, "Missing client id or ip", http.StatusBadRequest)
		return
	}
	s.BanIP(ip)
	w.WriteHeader(http.StatusOK)
}

// HandleUnbanClient allows the localhost admin to remove an IP from the ban list.
func (s *Server) HandleUnbanClient(w http.ResponseWriter, r *http.Request) {
	if !s.requirePostAndSameOrigin(w, r) {
		return
	}
	if !s.isLocalAdmin(r) {
		http.Error(w, "Forbidden: Localhost admin only", http.StatusForbidden)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	ip := strings.TrimSpace(r.FormValue("ip"))
	if ip != "" {
		s.UnbanIP(ip)
	}
	w.WriteHeader(http.StatusOK)
}

// HandleRegenerateToken generates a new one-time pairing token and revokes remote sessions.
func (s *Server) HandleRegenerateToken(w http.ResponseWriter, r *http.Request) {
	if !s.requirePostAndSameOrigin(w, r) {
		return
	}
	if !s.isLocalAdmin(r) {
		http.Error(w, "Forbidden: Localhost admin only", http.StatusForbidden)
		return
	}

	newToken := generateRandomHex(16)
	s.SetToken(newToken)
	s.hub.DisconnectRemote()
	s.RecordAudit("127.0.0.1", "TOKEN_REGENERATE", "Pairing token regenerated and remote sessions revoked")

	newURL := s.PairingURL()
	resp := map[string]string{
		"token":     newToken,
		"url":       newURL,
		"qr_base64": GenerateQRBase64(newURL),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// HandleTelegramConfig allows the localhost admin to configure or disable the Telegram Bot at runtime.
func (s *Server) HandleTelegramConfig(w http.ResponseWriter, r *http.Request) {
	if !s.requirePostAndSameOrigin(w, r) {
		return
	}
	if !s.isLocalAdmin(r) {
		http.Error(w, "Forbidden: Localhost admin only", http.StatusForbidden)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var payload struct {
		BotToken string `json:"bot_token"`
		ChatID   string `json:"chat_id"`
		Disable  bool   `json:"disable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	botToken := strings.TrimSpace(payload.BotToken)
	chatID := strings.TrimSpace(payload.ChatID)

	s.mu.Lock()
	if s.telegramNotifier != nil {
		s.telegramNotifier.Close()
		s.telegramNotifier = nil
	}
	if payload.Disable || (botToken == "" && chatID == "") {
		if s.cfg.Dispatcher != nil {
			s.cfg.Dispatcher.Unregister("telegram")
		}
		s.cfg.TelegramEnabled = false
		cfgPath := s.cfg.ConfigPath
		s.mu.Unlock()
		if cfgPath != "" {
			if err := config.UpdateTelegram(cfgPath, "", ""); err != nil {
				log.Printf("Warning: failed to persist Telegram config: %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"active": false})
		return
	}
	if botToken == "" || chatID == "" {
		s.mu.Unlock()
		http.Error(w, "Both bot_token and chat_id are required", http.StatusBadRequest)
		return
	}

	tg := notify.NewTelegram(botToken, chatID, func(rawAction string) error {
		return s.HandleTelegramAction(rawAction)
	})
	s.telegramNotifier = tg
	s.cfg.TelegramEnabled = true
	if s.cfg.Dispatcher != nil {
		s.cfg.Dispatcher.RegisterOrReplace(tg)
	}
	cfgPath := s.cfg.ConfigPath
	s.mu.Unlock()

	if cfgPath != "" {
		if err := config.UpdateTelegram(cfgPath, botToken, chatID); err != nil {
			log.Printf("Warning: failed to persist Telegram config: %v", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"active": true})
}
