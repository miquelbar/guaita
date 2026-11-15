package server

import (
	"encoding/base64"
	"fmt"

	"github.com/skip2/go-qrcode"
)

// AdminPageData contains parameters needed to render the Localhost Admin Control Center.
type AdminPageData struct {
	PairingURL   string
	QRCodeBase64 string
	SessionDays  int
	SessionToken string
	LocalIP      string
	Port         string
}

// GenerateQRBase64 encodes a target URL into a Base64-encoded PNG image string.
func GenerateQRBase64(targetURL string) string {
	pngBytes, err := qrcode.Encode(targetURL, qrcode.Medium, 240)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(pngBytes)
}

// RenderMobileHTML renders the mobile-optimized Sentinel Gate interface in English.
func RenderMobileHTML(sessionDays int) string {
	return fmt.Sprintf(mobileTemplate, sessionDays)
}

// RenderAdminHTML renders the full Localhost Admin & Control Center interface in English.
func RenderAdminHTML(data AdminPageData) string {
	return fmt.Sprintf(adminTemplate,
		data.LocalIP,
		data.Port,
		data.QRCodeBase64,
		data.PairingURL,
		data.SessionDays,
	)
}

const mobileTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
    <title>Guaita // Mobile Sentinel</title>
    <style>
        :root {
            --bg: #090d16;
            --surface: #131c31;
            --surface-border: #1e293b;
            --accent: #38bdf8;
            --success: #22c55e;
            --danger: #ef4444;
            --text: #f8fafc;
            --text-dim: #94a3b8;
        }
        * { box-sizing: border-box; -webkit-tap-highlight-color: transparent; }
        body {
            background-color: var(--bg);
            color: var(--text);
            font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            margin: 0;
            padding: 16px;
            display: flex;
            flex-direction: column;
            height: 100vh;
        }
        header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 12px;
        }
        h1 { font-size: 1.1rem; margin: 0; font-weight: 600; letter-spacing: -0.025em; display: flex; align-items: center; gap: 8px; }
        .badge { font-size: 0.7rem; background: rgba(56, 189, 248, 0.1); color: var(--accent); padding: 4px 8px; border-radius: 99px; border: 1px solid rgba(56, 189, 248, 0.2); }
        .terminal-container {
            flex: 1;
            background: var(--surface);
            border: 1px solid var(--surface-border);
            border-radius: 12px;
            padding: 12px;
            font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
            font-size: 11.5px;
            line-height: 1.5;
            overflow-y: auto;
            white-space: pre-wrap;
            word-break: break-all;
            color: #cbd5e1;
            box-shadow: inset 0 2px 4px rgba(0,0,0,0.4);
            transition: border-color 0.2s ease, background-color 0.2s ease;
        }
        .terminal-container.waiting {
            border-color: var(--danger);
            background: rgba(239, 68, 68, 0.05);
            animation: pulse-border 2s infinite;
        }
        @keyframes pulse-border {
            0%% { border-color: rgba(239, 68, 68, 0.4); }
            50%% { border-color: rgba(239, 68, 68, 1); }
            100%% { border-color: rgba(239, 68, 68, 0.4); }
        }
        .controls-panel {
            margin-top: 12px;
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 10px;
        }
        .input-row {
            margin-top: 10px;
            display: flex;
            gap: 8px;
        }
        .custom-input {
            flex: 1;
            background: var(--surface);
            border: 1px solid var(--surface-border);
            border-radius: 8px;
            color: var(--text);
            padding: 10px 12px;
            font-size: 0.9rem;
            outline: none;
        }
        .custom-input:focus { border-color: var(--accent); }
        .btn-send {
            background: var(--accent);
            color: #090d16;
            border: none;
            padding: 10px 16px;
            font-weight: 600;
            border-radius: 8px;
            cursor: pointer;
        }
        button {
            border: none;
            padding: 14px;
            font-size: 0.95rem;
            font-weight: 600;
            border-radius: 10px;
            cursor: pointer;
            transition: transform 0.1s active, opacity 0.2s;
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 6px;
        }
        button:active { transform: scale(0.97); }
        button.approve { background: var(--success); color: #fff; }
        button.reject { background: var(--danger); color: #fff; }
        .status-bar {
            margin-top: 8px;
            font-size: 0.75rem;
            color: var(--text-dim);
            text-align: center;
        }
    </style>
</head>
<body>
    <header>
        <h1>🛡️ Guaita <span class="badge">SECURED GATE</span></h1>
        <div id="connStatus" style="font-size: 0.75rem; color: var(--success);">● Live</div>
    </header>

    <div id="terminal" class="terminal-container">Establishing secure link with Herdr...</div>

    <div id="controls" class="controls-panel" style="display: none;">
        <button class="approve" onclick="sendAction('approve')">✓ APPROVE (Y)</button>
        <button class="reject" onclick="sendAction('reject')">✕ REJECT (N)</button>
    </div>

    <div id="inputControls" class="input-row" style="display: none;">
        <input type="text" id="customInput" class="custom-input" placeholder="Type response or input..." onkeydown="if(event.key==='Enter') sendCustomAction()">
        <button class="btn-send" onclick="sendCustomAction()">Send</button>
    </div>

    <div class="status-bar">Encrypted persistent session active (Expires in %d days)</div>

<script>
    let ws;
    let reconnectInterval = 1000;
    const term = document.getElementById('terminal');
    const controls = document.getElementById('controls');
    const inputControls = document.getElementById('inputControls');
    const connStatus = document.getElementById('connStatus');
    let wasWaiting = false;

    function connect() {
        const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
        ws = new WebSocket(` + "`" + `${protocol}//${location.host}/ws` + "`" + `);

        ws.onopen = () => {
            connStatus.style.color = 'var(--success)';
            connStatus.innerText = '● Live';
            reconnectInterval = 1000;
        };

        ws.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                if (data.output !== undefined) {
                    term.innerText = data.output;
                    term.scrollTop = term.scrollHeight;
                }

                if (data.waiting) {
                    term.classList.add('waiting');
                    controls.style.display = 'grid';
                    inputControls.style.display = 'flex';
                    if (!wasWaiting) {
                        if (navigator.vibrate) navigator.vibrate([250, 100, 250, 100, 300]);
                        wasWaiting = true;
                    }
                } else {
                    term.classList.remove('waiting');
                    controls.style.display = 'none';
                    inputControls.style.display = 'none';
                    wasWaiting = false;
                }
            } catch (err) {
                console.error("Failed to parse WebSocket message", err);
            }
        };

        ws.onclose = () => {
            connStatus.style.color = 'var(--danger)';
            connStatus.innerText = '● Reconnecting...';
            setTimeout(connect, reconnectInterval);
            reconnectInterval = Math.min(reconnectInterval * 1.5, 10000);
        };

        ws.onerror = () => { ws.close(); };
    }

    function sendAction(action) {
        fetch('/action', {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body: new URLSearchParams({action: action})
        }).then(() => {
            if (navigator.vibrate) navigator.vibrate(50);
        }).catch((err) => {
            console.error("Action dispatch failed", err);
        });
    }

    function sendCustomAction() {
        const input = document.getElementById('customInput');
        const val = input.value.trim();
        if (!val) return;
        sendAction(val);
        input.value = '';
    }

    connect();
</script>
</body>
</html>`

const adminTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Guaita // Localhost Admin & Control Center</title>
    <style>
        :root {
            --bg: #090d16;
            --surface: #131c31;
            --surface-hover: #18243e;
            --surface-border: #1e293b;
            --accent: #38bdf8;
            --accent-dim: rgba(56, 189, 248, 0.15);
            --success: #22c55e;
            --danger: #ef4444;
            --text: #f8fafc;
            --text-dim: #94a3b8;
        }
        * { box-sizing: border-box; }
        body {
            background-color: var(--bg);
            color: var(--text);
            font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            margin: 0;
            padding: 24px;
            display: flex;
            flex-direction: column;
            min-height: 100vh;
        }
        header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            border-bottom: 1px solid var(--surface-border);
            padding-bottom: 16px;
            margin-bottom: 24px;
        }
        .header-title {
            display: flex;
            align-items: center;
            gap: 12px;
        }
        h1 { font-size: 1.4rem; margin: 0; font-weight: 700; letter-spacing: -0.025em; }
        .badge {
            font-size: 0.75rem;
            background: var(--accent-dim);
            color: var(--accent);
            padding: 4px 10px;
            border-radius: 99px;
            border: 1px solid rgba(56, 189, 248, 0.3);
            font-weight: 600;
        }
        .badge-admin {
            background: rgba(34, 197, 94, 0.15);
            color: var(--success);
            border-color: rgba(34, 197, 94, 0.3);
        }
        .layout-grid {
            display: grid;
            grid-template-columns: 360px 1fr;
            gap: 24px;
            flex: 1;
        }
        @media (max-width: 900px) {
            .layout-grid { grid-template-columns: 1fr; }
        }
        .card {
            background: var(--surface);
            border: 1px solid var(--surface-border);
            border-radius: 12px;
            padding: 20px;
            margin-bottom: 20px;
            display: flex;
            flex-direction: column;
        }
        .card-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 16px;
            font-size: 0.95rem;
            font-weight: 600;
            color: var(--accent);
        }
        .qr-box {
            text-align: center;
            background: #fff;
            padding: 12px;
            border-radius: 8px;
            width: fit-content;
            margin: 0 auto 16px auto;
        }
        .qr-box img { display: block; max-width: 100%%; height: auto; }
        .url-field {
            display: flex;
            gap: 8px;
            background: var(--bg);
            border: 1px solid var(--surface-border);
            border-radius: 8px;
            padding: 8px 12px;
            font-size: 0.8rem;
            font-family: ui-monospace, monospace;
            align-items: center;
            word-break: break-all;
        }
        .url-field span { flex: 1; color: #cbd5e1; }
        .btn-copy {
            background: var(--accent);
            color: #090d16;
            border: none;
            padding: 6px 12px;
            font-size: 0.75rem;
            font-weight: 600;
            border-radius: 6px;
            cursor: pointer;
            white-space: nowrap;
        }
        .btn-copy:hover { opacity: 0.9; }
        .table-wrap {
            overflow-x: auto;
            border: 1px solid var(--surface-border);
            border-radius: 8px;
        }
        table {
            width: 100%%;
            border-collapse: collapse;
            font-size: 0.8rem;
            text-align: left;
        }
        th, td {
            padding: 10px 12px;
            border-bottom: 1px solid var(--surface-border);
        }
        th { background: rgba(0,0,0,0.2); color: var(--text-dim); font-weight: 600; }
        tr:last-child td { border-bottom: none; }
        .btn-disconnect {
            background: rgba(239, 68, 68, 0.15);
            color: var(--danger);
            border: 1px solid rgba(239, 68, 68, 0.3);
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 0.7rem;
            font-weight: 600;
            cursor: pointer;
        }
        .btn-disconnect:hover { background: var(--danger); color: #fff; }
        .terminal-container {
            flex: 1;
            min-height: 280px;
            background: #090d16;
            border: 1px solid var(--surface-border);
            border-radius: 8px;
            padding: 16px;
            font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
            font-size: 12px;
            line-height: 1.6;
            overflow-y: auto;
            white-space: pre-wrap;
            word-break: break-all;
            color: #e2e8f0;
            margin-bottom: 16px;
        }
        .terminal-container.waiting {
            border-color: var(--danger);
            background: rgba(239, 68, 68, 0.04);
        }
        .action-row {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 12px;
            margin-bottom: 12px;
        }
        .btn-action {
            border: none;
            padding: 12px;
            font-size: 0.9rem;
            font-weight: 600;
            border-radius: 8px;
            cursor: pointer;
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 6px;
        }
        .btn-approve { background: var(--success); color: #fff; }
        .btn-reject { background: var(--danger); color: #fff; }
        .input-bar {
            display: flex;
            gap: 8px;
        }
        .input-text {
            flex: 1;
            background: var(--bg);
            border: 1px solid var(--surface-border);
            border-radius: 8px;
            padding: 10px 14px;
            color: var(--text);
            font-size: 0.85rem;
            outline: none;
        }
        .input-text:focus { border-color: var(--accent); }
        .btn-submit {
            background: var(--accent);
            color: #090d16;
            border: none;
            padding: 10px 18px;
            font-weight: 600;
            border-radius: 8px;
            cursor: pointer;
        }
        .btn-regen {
            background: transparent;
            color: var(--accent);
            border: 1px solid rgba(56, 189, 248, 0.4);
            padding: 6px 12px;
            border-radius: 6px;
            font-size: 0.75rem;
            font-weight: 600;
            cursor: pointer;
        }
        .btn-regen:hover { background: var(--accent-dim); }
    </style>
</head>
<body>
    <header>
        <div class="header-title">
            <h1>🛡️ Guaita Control Center</h1>
            <span class="badge badge-admin">LOCALHOST ADMIN</span>
            <span class="badge">LAN: %s:%s</span>
        </div>
        <div id="connStatus" style="font-size: 0.8rem; color: var(--success); font-weight: 600;">● Live Stream</div>
    </header>

    <div class="layout-grid">
        <div>
            <!-- QR & Pairing Card -->
            <div class="card">
                <div class="card-header">
                    <span>📱 Mobile Pairing</span>
                    <button class="btn-regen" onclick="regenerateToken()">Regenerate Token</button>
                </div>
                <div class="qr-box">
                    <img id="qrImg" src="data:image/png;base64,%s" alt="Guaita Mobile Pairing QR">
                </div>
                <div class="url-field">
                    <span id="pairingURL">%s</span>
                    <button class="btn-copy" onclick="copyURL()">Copy Link</button>
                </div>
                <div style="font-size: 0.75rem; color: var(--text-dim); margin-top: 10px; line-height: 1.4;">
                    Scan with your mobile camera or copy link. Sessions persist for <strong>%d days</strong> across Wi-Fi and UniFi Teleport VPN.
                </div>
            </div>

            <!-- Active Clients Card -->
            <div class="card">
                <div class="card-header">
                    <span>👥 Connected Clients (<span id="clientCount">0</span>)</span>
                </div>
                <div class="table-wrap">
                    <table>
                        <thead>
                            <tr>
                                <th>Device / IP</th>
                                <th>Connected</th>
                                <th>Action</th>
                            </tr>
                        </thead>
                        <tbody id="clientsTable">
                            <tr><td colspan="3" style="text-align: center; color: var(--text-dim);">No active clients</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </div>

        <!-- Terminal & Sentinel Execution Card -->
        <div class="card" style="margin-bottom: 0;">
            <div class="card-header">
                <span>⚡ Agent Sentinel Execution</span>
                <span id="agentStatusBadge" class="badge">RUNNING</span>
            </div>
            <div id="terminal" class="terminal-container">Connecting to Herdr stream...</div>

            <div id="actionControls" class="action-row" style="display: none;">
                <button class="btn-action btn-approve" onclick="sendAction('approve')">✓ APPROVE (Y)</button>
                <button class="btn-action btn-reject" onclick="sendAction('reject')">✕ REJECT (N)</button>
            </div>

            <div class="input-bar">
                <input type="text" id="adminInput" class="input-text" placeholder="Send manual response or shell input to agent..." onkeydown="if(event.key==='Enter') sendAdminInput()">
                <button class="btn-submit" onclick="sendAdminInput()">Send</button>
            </div>
        </div>
    </div>

<script>
    let ws;
    let reconnectInterval = 1000;
    const term = document.getElementById('terminal');
    const actionControls = document.getElementById('actionControls');
    const agentBadge = document.getElementById('agentStatusBadge');
    const connStatus = document.getElementById('connStatus');
    const clientsTable = document.getElementById('clientsTable');
    const clientCount = document.getElementById('clientCount');

    function connect() {
        const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
        ws = new WebSocket(` + "`" + `${protocol}//${location.host}/ws` + "`" + `);

        ws.onopen = () => {
            connStatus.style.color = 'var(--success)';
            connStatus.innerText = '● Live Stream';
            reconnectInterval = 1000;
        };

        ws.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                if (data.output !== undefined) {
                    term.innerText = data.output;
                    term.scrollTop = term.scrollHeight;
                }

                if (data.waiting) {
                    term.classList.add('waiting');
                    actionControls.style.display = 'grid';
                    agentBadge.innerText = 'WAITING APPROVAL';
                    agentBadge.style.color = 'var(--danger)';
                    agentBadge.style.borderColor = 'rgba(239, 68, 68, 0.4)';
                } else {
                    term.classList.remove('waiting');
                    actionControls.style.display = 'none';
                    agentBadge.innerText = 'RUNNING';
                    agentBadge.style.color = 'var(--accent)';
                    agentBadge.style.borderColor = 'rgba(56, 189, 248, 0.3)';
                }

                if (data.clients) {
                    renderClients(data.clients);
                }
            } catch (err) {
                console.error("Failed to parse WS data", err);
            }
        };

        ws.onclose = () => {
            connStatus.style.color = 'var(--danger)';
            connStatus.innerText = '● Reconnecting...';
            setTimeout(connect, reconnectInterval);
            reconnectInterval = Math.min(reconnectInterval * 1.5, 10000);
        };

        ws.onerror = () => { ws.close(); };
    }

    function renderClients(clients) {
        clientCount.innerText = clients.length;
        if (!clients || clients.length === 0) {
            clientsTable.innerHTML = '<tr><td colspan="3" style="text-align: center; color: var(--text-dim);">No active clients</td></tr>';
            return;
        }
        clientsTable.innerHTML = clients.map(c => ` + "`" + `
            <tr>
                <td>
                    <div><strong>` + "${c.is_localhost ? 'Localhost (Admin)' : c.remote_addr}" + `</strong></div>
                    <div style="font-size: 0.7rem; color: var(--text-dim); max-width: 180px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">` + "${c.user_agent}" + `</div>
                </td>
                <td style="color: var(--text-dim); font-size: 0.75rem;">` + "${formatTime(c.connected_at)}" + `</td>
                <td>
                    ` + "${c.is_localhost ? '<span style=\"color: var(--success); font-size: 0.75rem;\">Self</span>' : `<button class=\"btn-disconnect\" onclick=\"disconnectClient('${c.id}')\">Kick</button>`}" + `
                </td>
            </tr>
        ` + "`" + `).join('');
    }

    function formatTime(isoString) {
        if (!isoString) return '';
        const d = new Date(isoString);
        return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
    }

    function disconnectClient(id) {
        fetch('/api/clients/disconnect', {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body: new URLSearchParams({id: id})
        });
    }

    function regenerateToken() {
        if (!confirm("Regenerate session token? All active remote clients will be disconnected.")) return;
        fetch('/api/token/regenerate', { method: 'POST' })
            .then(res => res.json())
            .then(data => {
                document.getElementById('pairingURL').innerText = data.url;
                document.getElementById('qrImg').src = 'data:image/png;base64,' + data.qr_base64;
            });
    }

    function sendAction(action) {
        fetch('/action', {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body: new URLSearchParams({action: action})
        });
    }

    function sendAdminInput() {
        const input = document.getElementById('adminInput');
        const val = input.value.trim();
        if (!val) return;
        sendAction(val);
        input.value = '';
    }

    function copyURL() {
        const urlText = document.getElementById('pairingURL').innerText;
        navigator.clipboard.writeText(urlText).then(() => {
            const btn = document.querySelector('.btn-copy');
            btn.innerText = 'Copied!';
            setTimeout(() => { btn.innerText = 'Copy Link'; }, 2000);
        });
    }

    connect();
</script>
</body>
</html>`
