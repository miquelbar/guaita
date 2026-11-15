# Guaita 🛡️

[![Go Version](https://img.shields.io/badge/go-1.24-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Architecture](https://img.shields.io/badge/architecture-mobile--first-purple.svg)](https://github.com/miquelbar/guaita)

> **Guaita** (*Catalan for "sentinel" or "lookout"*) is a lightweight, mobile-first human-in-the-loop sentinel written in pure Go. It monitors AI coding agents (Claude Code, Agy, or orchestrators) running in terminal workspaces via [Herdr](https://herdr.dev), notifies you on your mobile phone whenever an agent is waiting for human approval (`Yes/No` or text input), and allows instant, cryptographically secured responses.

---

## Highlights

* **🖥️ Localhost First-Class Admin:** Seamless admin dashboard when opening `http://localhost:8085/`. Zero token or QR prompt on loopback. Displays the mobile pairing QR code directly in the browser, live connected client telemetry, token revocation, and real-time terminal output.
* **📱 2026 Mobile Gate (ChatGPT-Grade UI):** Responsive dark interface (`#0b0f19`) designed for one-thumb mobile control. Features conversation bubbles, auto-scrolling terminal cards, quick action pills (`✓ Approve`, `✕ Reject`, `↵ Enter`), and a floating ChatGPT-style pill input bar.
* **🔔 Multi-Channel iOS Push Notifications:** 3 built-in notification pathways engineered specifically for iOS lock screens: **ntfy.sh**, **Native Web Push (PWA)**, and **Telegram Bot**.
* **⚡ Native Herdr Integration:** Auto-discovers the installed `herdr` binary, queries the active session socket, identifies focused panes, parses lifecycle states (`blocked`, `idle`, `working`), and injects keyboard actions via `pane send-keys` and `pane send-text`.
* **🛡️ Hardened Session Security:** 128-bit random session secrets, persistent HTTP-only `SameSite=Strict` cookies (7 days default), and strict `401 Unauthorized` responses on unauthorized remote requests. Seamless access over UniFi Teleport or VPN without re-scanning.

---

## iOS Push Notifications: The 3 Supported Options

Apple imposes strict background execution and lock-screen restrictions on iOS. Guaita natively supports all three viable pathways to alert you on iPhone and Apple Watch when an agent requires approval:

### 1. `ntfy.sh` (Recommended — Instant & Zero-Friction) ⭐️
* **How it works:** Uses the free, open-source **[ntfy iOS App](https://apps.apple.com/app/ntfy/id1625396347)**. Requires **no accounts**, **no registration**, and **no API keys**.
* **Lock Screen Support:** Delivers high-priority native banners with sound and vibration even when your iPhone is locked or in your pocket.
* **Interactive Actions:** Notifications include direct actionable buttons:
  * `[ ✓ Approve ]`: Sends an HTTP POST to approve the agent turn directly from the notification.
  * `[ ✕ Reject ]`: Rejects the turn without opening a browser.
  * `[ 📱 Open Guaita ]`: Opens the web sentinel in Safari with your active session.
* **How to enable:**
  ```bash
  ./bin/guaita -ntfy my-private-topic
  ```
  Or let Guaita auto-generate a private topic for you. The QR code to subscribe in the ntfy app appears right inside the Localhost Admin dashboard.

---

### 2. Native Web Push PWA (iOS 16.4+)
* **How it works:** Uses Apple's native Web Push API over Apple Push Notification service (APNs).
* **Requirements:**
  1. Open the Guaita URL in Safari on iOS.
  2. Tap **Share** -> **Add to Home Screen** (*Añadir a pantalla de inicio*).
  3. Launch Guaita from the Home Screen icon.
  4. Tap **Enable Push Notifications** when prompted.
* **Network Requirement:** Apple requires HTTPS for Service Worker push subscriptions. When accessing over LAN without SSL, pair Guaita with UniFi Teleport, Tailscale HTTPS, or a reverse proxy (Caddy / Cloudflare Tunnel).
* **Built-in Support:** Guaita automatically serves a compliant Web App Manifest (`/manifest.json`), Service Worker (`/sw.js`), and subscription registration endpoint (`/api/webpush/subscribe`).

---

### 3. Telegram Bot Integration
* **How it works:** Sends interactive alerts to your personal Telegram account or private group.
* **Features:**
  * Delivers instant notifications to your iPhone, iPad, Mac, and **Apple Watch**.
  * Messages include Markdown summaries of the agent's prompt and interactive inline buttons: `[ ✓ APPROVE (Y) ]` and `[ ✕ REJECT (N) ]`.
  * Guaita listens for inline button callbacks; tapping a button on your Apple Watch or phone immediately unlocks the agent in Herdr.
* **How to enable:**
  ```bash
  export TELEGRAM_BOT_TOKEN="123456789:ABCdefGHIjklMNOpqrSTUvwxYZ"
  export TELEGRAM_CHAT_ID="987654321"
  ./bin/guaita
  ```

---

## Quick Start

### Build
```bash
make build
```

### Run
```bash
./bin/guaita -port 8085 -days 7
```

### Access
* **On your Mac (Admin Dashboard):** Open `http://localhost:8085/`.
* **On your iPhone (Mobile Gate):** Scan the QR code shown in the terminal or on the Admin web page, or navigate to:
  ```text
  http://<LAN_IP>:8085/?token=<TOKEN>
  ```

---

## Configuration & CLI Flags

| Flag | Default | Description |
| :--- | :--- | :--- |
| `-port` | `8085` | HTTP & WebSocket server port |
| `-days` | `7` | Persistent session validity in days |
| `-ntfy` | *(auto-generated)* | Private topic name on ntfy.sh |
| `-ntfy-server` | `https://ntfy.sh` | ntfy server URL (public or self-hosted) |
| `-telegram-token` | `""` or `$TELEGRAM_BOT_TOKEN` | Telegram Bot API token |
| `-telegram-chat-id` | `""` or `$TELEGRAM_CHAT_ID` | Telegram target Chat ID |

---

## Architecture & Project Structure

```text
guaita/
├── Makefile             # build, test, lint, clean, run
├── go.mod               # Go module definition
├── go.sum               # Checksums
├── main.go              # CLI flags, IP detection, ANSI QR, and startup
├── main_test.go         # Core tests
├── notify/              # Multi-channel notification engine
│   ├── dispatcher.go    # Event deduplication and multi-channel broadcast
│   ├── dispatcher_test.go
│   ├── ntfy.go          # ntfy.sh client with interactive action headers
│   ├── telegram.go      # Telegram bot client and callback query listener
│   └── webpush.go       # PWA manifest, service worker, and subscription store
├── server/
│   ├── handlers.go      # HTTP router, auth middleware, WebSockets, actions
│   ├── handlers_test.go # Comprehensive unit tests
│   ├── hub.go           # Active client telemetry and connection registry
│   └── templates.go     # Embedded ChatGPT-grade Web SPA (Admin + Mobile Gate)
└── herdr/
    ├── client.go        # Herdr CLI integration, pane detection, and key injection
    └── client_test.go   # Subprocess mock tests
```

---

## Security

1. **Localhost Isolation:** Only connections originating from loopback (`127.0.0.1`, `::1`) are granted Admin access. External requests spoofing headers are rejected as `RemoteAddr` is strictly checked against the physical TCP socket.
2. **Cryptographic Tokens:** Sessions use 128-bit cryptographic random tokens generated with `crypto/rand`.
3. **No Process Leaks:** Unauthorized scans receive `401 Unauthorized` without revealing process names, Herdr sockets, or local filesystem paths.

---

## License

MIT License. See [LICENSE](LICENSE) for details.
