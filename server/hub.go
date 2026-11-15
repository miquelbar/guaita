package server

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ClientInfo holds telemetry data for an active client connection.
type ClientInfo struct {
	ID          string    `json:"id"`
	RemoteAddr  string    `json:"remote_addr"`
	UserAgent   string    `json:"user_agent"`
	ConnectedAt time.Time `json:"connected_at"`
	IsLocalhost bool      `json:"is_localhost"`
}

// Hub manages active WebSocket connections and client telemetry.
type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]*ClientInfo
}

// NewHub creates a new thread-safe client connection registry.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[*websocket.Conn]*ClientInfo),
	}
}

// Register adds a connection to the hub and returns its telemetry descriptor.
func (h *Hub) Register(conn *websocket.Conn, remoteAddr, userAgent string, isLocal bool) *ClientInfo {
	idBytes := make([]byte, 4)
	_, _ = rand.Read(idBytes)
	info := &ClientInfo{
		ID:          hex.EncodeToString(idBytes),
		RemoteAddr:  remoteAddr,
		UserAgent:   userAgent,
		ConnectedAt: time.Now(),
		IsLocalhost: isLocal,
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[conn] = info
	return info
}

// Unregister removes a connection from the hub.
func (h *Hub) Unregister(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, conn)
}

// ListClients returns a snapshot of all currently active clients.
func (h *Hub) ListClients() []ClientInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]ClientInfo, 0, len(h.clients))
	for _, info := range h.clients {
		result = append(result, *info)
	}
	return result
}

// Disconnect closes and removes a client connection by its identifier.
func (h *Hub) Disconnect(clientID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	for conn, info := range h.clients {
		if info.ID == clientID {
			if conn != nil {
				_ = conn.Close()
			}
			delete(h.clients, conn)
			return true
		}
	}
	return false
}

// Count returns the number of active client connections.
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// RemoteCount returns the number of active non-loopback client connections.
func (h *Hub) RemoteCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	count := 0
	for _, info := range h.clients {
		if !info.IsLocalhost {
			count++
		}
	}
	return count
}

// DisconnectRemote closes and removes all non-loopback client connections.
func (h *Hub) DisconnectRemote() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	closed := 0
	for conn, info := range h.clients {
		if !info.IsLocalhost {
			if conn != nil {
				_ = conn.Close()
			}
			delete(h.clients, conn)
			closed++
		}
	}
	return closed
}

// GetClientIP returns the IP address (without port) for a connected client ID.
func (h *Hub) GetClientIP(clientID string) (string, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, info := range h.clients {
		if info.ID == clientID {
			return clientIP(info.RemoteAddr), true
		}
	}
	return "", false
}

// DisconnectByIP closes and removes all non-localhost connections matching the given IP.
func (h *Hub) DisconnectByIP(ip string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	closed := 0
	for conn, info := range h.clients {
		if !info.IsLocalhost && clientIP(info.RemoteAddr) == ip {
			if conn != nil {
				_ = conn.Close()
			}
			delete(h.clients, conn)
			closed++
		}
	}
	return closed
}
