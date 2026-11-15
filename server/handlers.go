package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"guaita/herdr"
	"github.com/gorilla/websocket"
)

// Config specifies runtime parameters for the server.
type Config struct {
	SessionToken string
	SessionDays  int
	PollInterval time.Duration
	LocalIP      string
	Port         string
}

// Server provides HTTP and WebSocket handlers for Guaita.
type Server struct {
	mu             sync.RWMutex
	cfg            Config
	hub            *Hub
	upgrader       websocket.Upgrader
	statusProvider func() (herdr.AgentStatus, error)
	actionSender   func(string) error
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

	return &Server{
		cfg: cfg,
		hub: NewHub(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		statusProvider: herdr.GetStatus,
		actionSender:   herdr.SendAction,
	}
}

// Routes returns the configured HTTP mux.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.HandleIndex)
	mux.HandleFunc("/ws", s.HandleWebSocket)
	mux.HandleFunc("/action", s.HandleAction)
	mux.HandleFunc("/api/clients/disconnect", s.HandleDisconnectClient)
	mux.HandleFunc("/api/token/regenerate", s.HandleRegenerateToken)
	return mux
}

// Token returns the current session token thread-safely.
func (s *Server) Token() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.SessionToken
}

// SetToken updates the session token thread-safely.
func (s *Server) SetToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.SessionToken = token
}

// PairingURL constructs the full pairing URL containing the current token.
func (s *Server) PairingURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fmt.Sprintf("http://%s:%s/?token=%s", s.cfg.LocalIP, s.cfg.Port, s.cfg.SessionToken)
}

// IsLoopback checks whether the connection originates from the local machine.
func IsLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	// Strip IPv6 brackets if present
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}
	return host == "localhost"
}

// IsAuthorized verifies whether a request is from localhost or carries a valid token/cookie.
func (s *Server) IsAuthorized(r *http.Request) bool {
	if IsLoopback(r.RemoteAddr) {
		return true
	}

	expectedToken := s.Token()
	if expectedToken == "" {
		return false
	}

	if token := r.URL.Query().Get("token"); token != "" && token == expectedToken {
		return true
	}
	cookie, err := r.Cookie("guaita_session")
	if err == nil && cookie.Value == expectedToken {
		return true
	}
	return false
}

// HandleIndex serves the Admin Dashboard for localhost or the Mobile Gate for remote visitors.
func (s *Server) HandleIndex(w http.ResponseWriter, r *http.Request) {
	if IsLoopback(r.RemoteAddr) {
		pairingURL := s.PairingURL()
		data := AdminPageData{
			PairingURL:   pairingURL,
			QRCodeBase64: GenerateQRBase64(pairingURL),
			SessionDays:  s.cfg.SessionDays,
			SessionToken: s.Token(),
			LocalIP:      s.cfg.LocalIP,
			Port:         s.cfg.Port,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(RenderAdminHTML(data)))
		return
	}

	if !s.IsAuthorized(r) {
		http.Error(w, "Access Denied: Invalid token or expired session", http.StatusUnauthorized)
		return
	}

	if r.URL.Query().Get("token") == s.Token() {
		http.SetCookie(w, &http.Cookie{
			Name:     "guaita_session",
			Value:    s.Token(),
			Expires:  time.Now().Add(time.Duration(s.cfg.SessionDays) * 24 * time.Hour),
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			Path:     "/",
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(RenderMobileHTML(s.cfg.SessionDays)))
}

type wsPayload struct {
	Output  string       `json:"output"`
	Waiting bool         `json:"waiting"`
	Clients []ClientInfo `json:"clients,omitempty"`
}

// HandleWebSocket streams real-time status and telemetry to connected clients.
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !s.IsAuthorized(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	isLocal := IsLoopback(r.RemoteAddr)
	s.hub.Register(conn, r.RemoteAddr, r.UserAgent(), isLocal)
	defer s.hub.Unregister(conn)

	for {
		status, _ := s.statusProvider()
		payload := wsPayload{
			Output:  status.Output,
			Waiting: status.Waiting,
		}
		if isLocal {
			payload.Clients = s.hub.ListClients()
		}

		if err := conn.WriteJSON(payload); err != nil {
			break
		}
		time.Sleep(s.cfg.PollInterval)
	}
}

// HandleAction receives actions from the web UI and forwards them to Herdr.
func (s *Server) HandleAction(w http.ResponseWriter, r *http.Request) {
	if !s.IsAuthorized(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	action := r.FormValue("action")
	if action != "" {
		_ = s.actionSender(action)
	}
	w.WriteHeader(http.StatusOK)
}

// HandleDisconnectClient allows the localhost admin to disconnect a client.
func (s *Server) HandleDisconnectClient(w http.ResponseWriter, r *http.Request) {
	if !IsLoopback(r.RemoteAddr) {
		http.Error(w, "Forbidden: Localhost admin only", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	id := r.FormValue("id")
	if id != "" {
		s.hub.Disconnect(id)
	}
	w.WriteHeader(http.StatusOK)
}

// HandleRegenerateToken generates a new random token and revokes remote sessions.
func (s *Server) HandleRegenerateToken(w http.ResponseWriter, r *http.Request) {
	if !IsLoopback(r.RemoteAddr) {
		http.Error(w, "Forbidden: Localhost admin only", http.StatusForbidden)
		return
	}

	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	newToken := hex.EncodeToString(bytes)
	s.SetToken(newToken)

	// Disconnect non-localhost clients so they are forced to re-authenticate
	for _, client := range s.hub.ListClients() {
		if !client.IsLocalhost {
			s.hub.Disconnect(client.ID)
		}
	}

	newURL := s.PairingURL()
	resp := map[string]string{
		"token":     newToken,
		"url":       newURL,
		"qr_base64": GenerateQRBase64(newURL),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
