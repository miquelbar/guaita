<p align="center">
  <img src="assets/logo.png" width="140" alt="Guaita Logo">
</p>

<h1 align="center">Guaita 🛡️</h1>

<p align="center">
  <a href="https://golang.org"><img src="https://img.shields.io/badge/go-1.24-blue.svg" alt="Go Version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green.svg" alt="License"></a>
  <a href="https://github.com/miquelbar/guaita/releases/latest"><img src="https://img.shields.io/github/v/release/miquelbar/guaita?label=release" alt="Release"></a>
  <a href="https://github.com/miquelbar/guaita/actions/workflows/ci.yml"><img src="https://github.com/miquelbar/guaita/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/miquelbar/guaita"><img src="https://img.shields.io/badge/architecture-mobile--first-purple.svg" alt="Architecture"></a>
</p>

> **Guaita** (*Catalan for "sentinel" or "lookout"*) is a lightweight, mobile-first human-in-the-loop sentinel written in pure Go. It monitors AI coding agents (Claude Code, Agy, or orchestrators) running in terminal workspaces via [Herdr](https://herdr.dev), notifies you on your mobile phone whenever an agent is waiting for human approval (`Yes/No` or text input), and allows instant, cryptographically secured responses.

---

## Highlights

* **🖥️ Localhost First-Class Admin:** Seamless admin dashboard when opening `http://localhost:8085/`. Zero token or QR prompt on loopback. Displays the mobile pairing QR code directly in the browser, live connected client telemetry, token revocation, and real-time terminal output.
* **📱 2026 Mobile Gate (ChatGPT-Grade UI):** Responsive dark interface (`#0b0f19`) designed for one-thumb mobile control. Features conversation bubbles, auto-scrolling terminal cards, quick action pills (`✓ Approve`, `✕ Reject`, `↵ Enter`), and a floating ChatGPT-style pill input bar.
* **🔔 Multi-Channel iOS Push Notifications:** 3 built-in notification pathways engineered specifically for iOS lock screens: **ntfy.sh**, **Native Web Push (PWA)**, and **Telegram Bot**.
* **⚡ Native Herdr Integration (with `tmux` Fallback):** Auto-discovers the installed `herdr` binary, queries the active session socket, identifies focused panes, parses lifecycle states (`blocked`, `idle`, `working`), and injects keyboard actions via `pane send-keys` and `pane send-text`. If `herdr` is unavailable or offline, Guaita automatically falls back to `tmux` (`list-panes`, `capture-pane`, `send-keys`).
* **🛡️ Hardened Single-Session Security:** One-time pairing tokens exchanged for a 256-bit `HttpOnly` `SameSite=Strict` session cookie (`Secure` when TLS is enabled), strict single-remote-session enforcement, built-in HTTPS/WSS (`-tls`), CSRF/CSWSH protection, and per-IP brute-force rate limiting.

---

## iOS Push Notifications: The 3 Supported Options

Apple imposes strict background execution and lock-screen restrictions on iOS. Guaita natively supports all three viable pathways to alert you on iPhone and Apple Watch when an agent requires approval:

### 1. `ntfy.sh` (Recommended — Instant & Zero-Friction) ⭐️
* **How it works:** Uses the free, open-source **[ntfy iOS App](https://apps.apple.com/app/ntfy/id1625396347)**. Requires **no accounts**, **no registration**, and **no API keys**.
* **Lock Screen Support:** Delivers high-priority native banners with sound and vibration even when your iPhone is locked or in your pocket.
* **Zero-Leak Notifications:** Alerts include a clean link (`http(s)://<LAN_IP>:<PORT>/`) without embedding session tokens in URLs or public `ntfy.sh` payloads. Once your phone is paired, opening Guaita uses your persistent `HttpOnly` session cookie.
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

### Download Prebuilt Binaries (Releases)
Prebuilt archives for macOS (`darwin/arm64`, `darwin/amd64`) and Linux (`linux/amd64`, `linux/arm64`) with SHA-256 checksums are automatically published on every push to `master`:
* **[Download Latest Release](https://github.com/miquelbar/guaita/releases/latest)** · [All Releases](https://github.com/miquelbar/guaita/releases)

### Build & Install from Source
```bash
make build
make install   # Installs `guaita` to ~/.local/bin
```

### Run (Encrypted HTTPS/WSS — Default)
```bash
./bin/guaita -port 8085 -days 7
```
*(Uses an auto-generated in-memory ECDSA P-256 certificate — zero setup required on your phone; just accept the browser's self-signed warning once).*

Or with your own trusted certificate (e.g., generated via `mkcert`):
```bash
./bin/guaita -tls-cert ./cert.pem -tls-key ./key.pem
```

### Run (Plain HTTP — Behind a Reverse Proxy / Tunnel Only)
```bash
./bin/guaita -tls=false -port 8085 -days 7
```

### Access
* **On your Mac (Admin Dashboard):** Open `https://localhost:8085/` (or `http://localhost:8085/` with `-tls=false`).
* **On your iPhone (Mobile Gate):** Scan the QR code shown in the terminal or on the Admin web page. The one-time pairing token is immediately exchanged for a single-device `HttpOnly` session cookie and stripped from the URL.

---

## Configuration & CLI Flags

| Flag | Default | Description |
| :--- | :--- | :--- |
| `-config` | `~/.config/guaita/config.json` | Path to persistent local JSON config (`0600` permissions) |
| `-port` | `8085` | HTTP/HTTPS & WebSocket server port |
| `-days` | `7` | Persistent session validity in days |
| `-tls` | `true` | Enable HTTPS/WSS with an auto-generated ephemeral ECDSA P-256 certificate |
| `-tls-cert` | `""` | Path to a custom TLS certificate PEM file (implies `-tls`) |
| `-tls-key` | `""` | Path to a custom TLS private key PEM file (implies `-tls`) |
| `-public-url` | `""` | External public URL (e.g., Cloudflare Tunnel or reverse proxy URL) |
| `-admin-auth` | `true` | Require an admin token on localhost dashboard (`admin_auth: false` in `config.json` to disable) |
| `-admin-token` | *(auto-generated)* | Custom localhost admin token (used when `-admin-auth=true`) |
| `-ntfy` | *(auto-generated)* | Private topic name on ntfy.sh |
| `-ntfy-server` | `https://ntfy.sh` | ntfy server URL (public or self-hosted) |
| `-telegram-token` | `""` or `$TELEGRAM_BOT_TOKEN` | Telegram Bot API token |
| `-telegram-chat-id` | `""` or `$TELEGRAM_CHAT_ID` | Telegram target Chat ID |

---

## Remote Access via Cloudflare Tunnel

You can expose Guaita securely outside your LAN using `cloudflared` (no port forwarding required, valid HTTPS certificate for iOS PWA & Safari).

### Option A: Plain HTTP locally (recommended when Cloudflare terminates TLS)
```bash
# Terminal 1: Start Guaita with TLS disabled locally and your public tunnel URL
guaita -tls=false -public-url https://guaita.yourdomain.com

# Terminal 2: Forward Cloudflare Tunnel to local Guaita
cloudflared tunnel --url http://127.0.0.1:8085
```

### Option B: Keep local self-signed TLS enabled
```bash
# Terminal 1: Start Guaita with default TLS
guaita -public-url https://guaita.yourdomain.com

# Terminal 2: Skip self-signed cert verification on loopback
cloudflared tunnel --url https://127.0.0.1:8085 --no-tls-verify
```

**Security notes for Cloudflare Tunnel:**
* Requests arriving through `cloudflared` (`127.0.0.1` with `CF-Connecting-IP` / `Cf-Ray` / `X-Forwarded-For` headers) are **never** treated as Localhost Admin — they still require the one-time pairing QR token and single-session `HttpOnly` cookie.
* Guaita uses `CF-Connecting-IP` for real client IP telemetry, per-IP rate limiting, IP banning (`/api/clients/ban`), and the Security Audit Log.

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
│   ├── handlers_test.go # HTTP, auth, and WebSocket unit tests
│   ├── security_test.go # Intrusion & security regression tests
│   ├── hub.go           # Active client telemetry and connection registry
│   └── templates.go     # Embedded ChatGPT-grade Web SPA (Admin + Mobile Gate)
└── herdr/
    ├── client.go        # Herdr CLI integration, pane detection, and key injection
    └── client_test.go   # Subprocess mock tests
```

---

## Security

1. **One-Time Pairing & Single Remote Session:** The QR pairing token can only be claimed once. Upon pairing, Guaita issues a unique 256-bit `HttpOnly`, `SameSite=Lax` (`Secure` under TLS or HTTPS proxy) session cookie and strips the token from browser history. A second device cannot reuse the QR token or open a concurrent remote session.
2. **Encrypted Transport (TLS/WSS):** Running with `-tls` auto-generates an in-memory ECDSA P-256 certificate valid for `localhost`, `127.0.0.1`, `::1`, and your LAN IP, preventing credential sniffing on local networks. You can also supply trusted certificates via `-tls-cert` and `-tls-key`.
3. **Localhost Isolation & DNS Rebinding Protection:** Admin endpoints require direct loopback (`127.0.0.1`, `::1`) without reverse-proxy headers, strict `Host` allowlist validation, and an ephemeral `-admin-auth` token by default.
4. **CSWSH, CSRF, Input & CSP Hardening:** WebSocket upgrades enforce same-origin `CheckOrigin`. State-changing endpoints require `POST`, validate `Origin`/`Referer`/`Sec-Fetch-Site`, cap request bodies at 64 KB, and sanitize action inputs (`<= 4096` bytes, control chars stripped). HTML responses use per-request CSP `nonce`s (`script-src 'self' 'nonce-...'`).
5. **Brute-Force & Bot Protection:** Per-IP rate limiting blocks repeated `401` attempts (`429 Too Many Requests`), token comparisons use constant-time `subtle.ConstantTimeCompare`, and Telegram callbacks verify `ChatID` and restrict actions to `approve`/`reject`.
6. **Zero-Leak Notifications:** Public `ntfy.sh` alerts redact raw terminal output, and neither `ntfy.sh` payloads nor browser `localStorage` ever store or transmit raw session tokens.

---

## License

MIT License. See [LICENSE](LICENSE) for details.
