package server

import (
	"log"
	"net/http"
	"time"

	"guaita/herdr"
	"github.com/gorilla/websocket"
)

type Config struct {
	SessionToken string
	SessionDays  int
	PollInterval time.Duration
}

type Server struct {
	cfg            Config
	upgrader       websocket.Upgrader
	statusProvider func() (herdr.AgentStatus, error)
	actionSender   func(string) error
}

func New(cfg Config) *Server {
	if cfg.SessionDays <= 0 {
		cfg.SessionDays = 7
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 1500 * time.Millisecond
	}

	return &Server{
		cfg: cfg,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		statusProvider: herdr.GetStatus,
		actionSender:   herdr.SendAction,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.HandleIndex)
	mux.HandleFunc("/ws", s.HandleWebSocket)
	mux.HandleFunc("/action", s.HandleAction)
	return mux
}

func (s *Server) IsAuthorized(r *http.Request) bool {
	if s.cfg.SessionToken == "" {
		return false
	}
	if token := r.URL.Query().Get("token"); token != "" && token == s.cfg.SessionToken {
		return true
	}
	cookie, err := r.Cookie("guaita_session")
	if err == nil && cookie.Value == s.cfg.SessionToken {
		return true
	}
	return false
}

func (s *Server) HandleIndex(w http.ResponseWriter, r *http.Request) {
	if !s.IsAuthorized(r) {
		http.Error(w, "Acceso Denegado: Token inválido o sesión expirada", http.StatusUnauthorized)
		return
	}

	if r.URL.Query().Get("token") == s.cfg.SessionToken {
		http.SetCookie(w, &http.Cookie{
			Name:     "guaita_session",
			Value:    s.cfg.SessionToken,
			Expires:  time.Now().Add(time.Duration(s.cfg.SessionDays) * 24 * time.Hour),
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			Path:     "/",
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(RenderHTML(s.cfg.SessionDays)))
}

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

	for {
		status, _ := s.statusProvider()
		if err := conn.WriteJSON(status); err != nil {
			break
		}
		time.Sleep(s.cfg.PollInterval)
	}
}

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
