package server

import (
	"encoding/base64"
	"fmt"

	"github.com/skip2/go-qrcode"
)

// AdminPageData contains parameters needed to render the Localhost Admin Control Center.
type AdminPageData struct {
	PairingURL      string
	QRCodeBase64    string
	SessionDays     int
	SessionToken    string
	LocalIP         string
	Port            string
	NtfyTopic       string
	NtfyURL         string
	NtfyQRBase64    string
	TelegramActive  bool
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
	tgStatus := "Not Configured"
	if data.TelegramActive {
		tgStatus = "Active (Connected)"
	}

	return fmt.Sprintf(adminTemplate,
		data.LocalIP,
		data.Port,
		data.QRCodeBase64,
		data.PairingURL,
		data.SessionDays,
		data.NtfyQRBase64,
		data.NtfyTopic,
		data.NtfyURL,
		tgStatus,
	)
}

const mobileTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
    <title>Guaita // Mobile Sentinel</title>
    <link rel="manifest" href="/manifest.json">
    <meta name="apple-mobile-web-app-capable" content="yes">
    <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
    <meta name="apple-mobile-web-app-title" content="Guaita">
    <style>
        :root {
            --bg: #0b0f19;
            --surface: #141c2e;
            --surface-hover: #1a253d;
            --surface-border: #22304d;
            --agent-bubble: #172138;
            --user-bubble: #1e3a5f;
            --accent: #38bdf8;
            --success: #10b981;
            --danger: #ef4444;
            --warning: #f59e0b;
            --text: #f8fafc;
            --text-dim: #94a3b8;
        }
        * { box-sizing: border-box; -webkit-tap-highlight-color: transparent; }
        body {
            background-color: var(--bg);
            color: var(--text);
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            margin: 0;
            padding: 0;
            display: flex;
            flex-direction: column;
            height: 100vh;
            overflow: hidden;
        }
        header {
            padding: 12px 16px;
            background: rgba(11, 15, 25, 0.85);
            backdrop-filter: blur(12px);
            border-bottom: 1px solid var(--surface-border);
            display: flex;
            justify-content: space-between;
            align-items: center;
            z-index: 10;
        }
        .header-brand {
            display: flex;
            align-items: center;
            gap: 10px;
        }
        .avatar-circle {
            width: 32px;
            height: 32px;
            border-radius: 50%%;
            background: linear-gradient(135deg, #0284c7, #0f172a);
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 14px;
            border: 1px solid rgba(56, 189, 248, 0.4);
        }
        .brand-text h1 { font-size: 0.95rem; margin: 0; font-weight: 600; letter-spacing: -0.01em; }
        .brand-text .status-subtitle { font-size: 0.7rem; color: var(--success); display: flex; align-items: center; gap: 4px; }
        .view-toggle {
            display: flex;
            background: var(--surface);
            border: 1px solid var(--surface-border);
            border-radius: 20px;
            padding: 2px;
        }
        .toggle-btn {
            background: transparent;
            border: none;
            color: var(--text-dim);
            padding: 4px 10px;
            font-size: 0.72rem;
            font-weight: 600;
            border-radius: 16px;
            cursor: pointer;
        }
        .toggle-btn.active {
            background: var(--accent);
            color: #0b0f19;
        }

        /* Chat area */
        .chat-container {
            flex: 1;
            overflow-y: auto;
            padding: 16px;
            display: flex;
            flex-direction: column;
            gap: 16px;
            scroll-behavior: smooth;
        }
        .message-row {
            display: flex;
            gap: 10px;
            max-width: 95%%;
        }
        .message-row.agent { align-self: flex-start; }
        .message-row.user { align-self: flex-end; flex-direction: row-reverse; }
        .msg-avatar {
            width: 28px;
            height: 28px;
            border-radius: 50%%;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 13px;
            flex-shrink: 0;
            margin-top: 2px;
        }
        .msg-avatar.agent { background: #0369a1; }
        .msg-avatar.user { background: #334155; }
        .msg-content {
            background: var(--agent-bubble);
            border: 1px solid var(--surface-border);
            border-radius: 14px;
            padding: 12px 14px;
            font-size: 0.88rem;
            line-height: 1.5;
            color: #e2e8f0;
            box-shadow: 0 2px 6px rgba(0,0,0,0.25);
        }
        .message-row.user .msg-content {
            background: var(--user-bubble);
            border-color: rgba(56, 189, 248, 0.4);
            color: #fff;
        }
        .terminal-block {
            font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
            font-size: 11px;
            line-height: 1.45;
            white-space: pre-wrap;
            word-break: break-all;
            background: #070a12;
            padding: 10px;
            border-radius: 8px;
            border: 1px solid rgba(255,255,255,0.06);
            margin-top: 6px;
            max-height: 280px;
            overflow-y: auto;
            color: #cbd5e1;
        }

        /* Action approval banner */
        .approval-card {
            background: rgba(239, 68, 68, 0.08);
            border: 1px solid var(--danger);
            border-radius: 12px;
            padding: 14px;
            margin-top: 8px;
            animation: pulse-border 2s infinite;
        }
        @keyframes pulse-border {
            0%% { border-color: rgba(239, 68, 68, 0.4); box-shadow: 0 0 0 0 rgba(239, 68, 68, 0.2); }
            50%% { border-color: rgba(239, 68, 68, 1); box-shadow: 0 0 12px 2px rgba(239, 68, 68, 0.3); }
            100%% { border-color: rgba(239, 68, 68, 0.4); box-shadow: 0 0 0 0 rgba(239, 68, 68, 0.2); }
        }
        .approval-title {
            font-size: 0.82rem;
            font-weight: 700;
            color: var(--danger);
            display: flex;
            align-items: center;
            gap: 6px;
            margin-bottom: 8px;
            text-transform: uppercase;
            letter-spacing: 0.04em;
        }
        .action-pills {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 8px;
            margin-top: 10px;
        }
        .pill-btn {
            border: none;
            padding: 12px;
            font-size: 0.88rem;
            font-weight: 600;
            border-radius: 8px;
            cursor: pointer;
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 6px;
            transition: transform 0.1s active, opacity 0.2s;
        }
        .pill-btn:active { transform: scale(0.96); }
        .pill-approve { background: var(--success); color: #fff; }
        .pill-reject { background: var(--danger); color: #fff; }

        /* Floating ChatGPT-style input bar */
        .bottom-bar {
            padding: 12px 16px 16px 16px;
            background: rgba(11, 15, 25, 0.95);
            backdrop-filter: blur(16px);
            border-top: 1px solid var(--surface-border);
        }
        .quick-chips {
            display: flex;
            gap: 6px;
            overflow-x: auto;
            padding-bottom: 8px;
            scrollbar-width: none;
        }
        .quick-chips::-webkit-scrollbar { display: none; }
        .chip {
            background: var(--surface);
            border: 1px solid var(--surface-border);
            color: var(--text-dim);
            font-size: 0.72rem;
            padding: 4px 10px;
            border-radius: 99px;
            white-space: nowrap;
            cursor: pointer;
            font-weight: 500;
        }
        .chip:hover, .chip:active { border-color: var(--accent); color: var(--accent); }
        .input-pill-box {
            display: flex;
            align-items: center;
            background: var(--surface);
            border: 1px solid var(--surface-border);
            border-radius: 24px;
            padding: 4px 6px 4px 14px;
            gap: 8px;
            transition: border-color 0.2s;
        }
        .input-pill-box:focus-within { border-color: var(--accent); }
        .chat-input {
            flex: 1;
            background: transparent;
            border: none;
            color: #fff;
            font-size: 0.92rem;
            outline: none;
            padding: 6px 0;
        }
        .chat-input::placeholder { color: var(--text-dim); font-size: 0.85rem; }
        .btn-send-round {
            width: 34px;
            height: 34px;
            border-radius: 50%%;
            background: var(--accent);
            color: #0b0f19;
            border: none;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 16px;
            font-weight: bold;
            cursor: pointer;
            flex-shrink: 0;
            transition: transform 0.1s active, opacity 0.2s;
        }
        .btn-send-round:active { transform: scale(0.92); }
        .terminal-fullscreen {
            flex: 1;
            padding: 14px;
            font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
            font-size: 11.5px;
            line-height: 1.5;
            white-space: pre-wrap;
            word-break: break-all;
            background: #050811;
            color: #cbd5e1;
            overflow-y: auto;
            display: none;
        }
    </style>
</head>
<body>
    <header>
        <div class="header-brand">
            <div class="avatar-circle">🛡️</div>
            <div class="brand-text">
                <h1>Guaita Sentinel</h1>
                <div id="connStatus" class="status-subtitle">● Live Herdr Link</div>
            </div>
        </div>
        <div class="view-toggle">
            <button id="tabChat" class="toggle-btn active" onclick="switchView('chat')">💬 Chat</button>
            <button id="tabTerminal" class="toggle-btn" onclick="switchView('terminal')">💻 Raw</button>
        </div>
    </header>

    <!-- Main ChatGPT-style message stream -->
    <div id="chatView" class="chat-container">
        <div class="message-row agent">
            <div class="msg-avatar agent">🤖</div>
            <div class="msg-content">
                <div style="font-size: 0.72rem; color: var(--accent); font-weight: 600; margin-bottom: 4px;">HERDR SENTINEL</div>
                <div id="agentText">Connected to Herdr agent workspace. Monitoring terminal for prompts...</div>
                <div id="terminalSnippet" class="terminal-block">Loading initial agent buffer...</div>
                <div id="approvalCard" class="approval-card" style="display: none;">
                    <div class="approval-title">⚠️ Approval Requested</div>
                    <div style="font-size: 0.8rem; color: #cbd5e1;">Agent is paused awaiting human confirmation:</div>
                    <div class="action-pills">
                        <button class="pill-btn pill-approve" onclick="sendAction('approve')">✓ APPROVE (Y)</button>
                        <button class="pill-btn pill-reject" onclick="sendAction('reject')">✕ REJECT (N)</button>
                    </div>
                </div>
            </div>
        </div>
    </div>

    <!-- Fullscreen terminal view -->
    <div id="rawTerminalView" class="terminal-fullscreen"></div>

    <!-- Bottom ChatGPT-style bar -->
    <div class="bottom-bar">
        <div class="quick-chips">
            <span class="chip" onclick="sendAction('approve')">✓ Approve (Y)</span>
            <span class="chip" onclick="sendAction('reject')">✕ Reject (N)</span>
            <span class="chip" onclick="sendAction('enter')">↵ Enter</span>
            <span class="chip" onclick="sendAction('esc')">⎋ Esc</span>
        </div>
        <div class="input-pill-box">
            <input type="text" id="userInput" class="chat-input" placeholder="Reply to agent or type command..." onkeydown="if(event.key==='Enter') sendInput()">
            <button class="btn-send-round" onclick="sendInput()" title="Send">↑</button>
        </div>
        <div style="text-align: center; font-size: 0.65rem; color: var(--text-dim); margin-top: 6px;">
            Encrypted session active (Expires in %d days)
        </div>
    </div>

<script>
    let ws;
    let reconnectInterval = 1000;
    const connStatus = document.getElementById('connStatus');
    const termSnippet = document.getElementById('terminalSnippet');
    const rawTerminal = document.getElementById('rawTerminalView');
    const approvalCard = document.getElementById('approvalCard');
    const chatContainer = document.getElementById('chatView');
    let wasWaiting = false;

    // Service Worker Registration for PWA / iOS Web Push
    if ('serviceWorker' in navigator) {
        navigator.serviceWorker.register('/sw.js').catch(err => console.log('SW registration error:', err));
    }

    function switchView(mode) {
        if (mode === 'chat') {
            chatContainer.style.display = 'flex';
            rawTerminal.style.display = 'none';
            document.getElementById('tabChat').classList.add('active');
            document.getElementById('tabTerminal').classList.remove('active');
        } else {
            chatContainer.style.display = 'none';
            rawTerminal.style.display = 'block';
            document.getElementById('tabTerminal').classList.add('active');
            document.getElementById('tabChat').classList.remove('active');
        }
    }

    function appendUserMessage(text) {
        const row = document.createElement('div');
        row.className = 'message-row user';
        row.innerHTML = ` + "`" + `
            <div class="msg-avatar user">👤</div>
            <div class="msg-content">
                <div style="font-size: 0.7rem; color: #94a3b8; margin-bottom: 2px;">YOU</div>
                <div>${escapeHTML(text)}</div>
            </div>
        ` + "`" + `;
        chatContainer.appendChild(row);
        chatContainer.scrollTop = chatContainer.scrollHeight;
    }

    function escapeHTML(str) {
        return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    function connect() {
        const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
        ws = new WebSocket(` + "`" + `${protocol}//${location.host}/ws` + "`" + `);

        ws.onopen = () => {
            connStatus.style.color = 'var(--success)';
            connStatus.innerText = '● Live Herdr Link';
            reconnectInterval = 1000;
        };

        ws.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                if (data.output !== undefined) {
                    termSnippet.innerText = data.output;
                    rawTerminal.innerText = data.output;
                    termSnippet.scrollTop = termSnippet.scrollHeight;
                    rawTerminal.scrollTop = rawTerminal.scrollHeight;
                }

                if (data.waiting) {
                    approvalCard.style.display = 'block';
                    if (!wasWaiting) {
                        if (navigator.vibrate) navigator.vibrate([250, 100, 250, 100, 300]);
                        wasWaiting = true;
                    }
                } else {
                    approvalCard.style.display = 'none';
                    wasWaiting = false;
                }
            } catch (err) {
                console.error("WS parse error", err);
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
        appendUserMessage(action.toUpperCase());
        fetch('/action', {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body: new URLSearchParams({action: action})
        }).then(() => {
            if (navigator.vibrate) navigator.vibrate(50);
        });
    }

    function sendInput() {
        const input = document.getElementById('userInput');
        const val = input.value.trim();
        if (!val) return;
        appendUserMessage(val);
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
            --bg: #0b0f19;
            --surface: #141c2e;
            --surface-hover: #1a253d;
            --surface-border: #22304d;
            --agent-bubble: #172138;
            --user-bubble: #1e3a5f;
            --accent: #38bdf8;
            --accent-dim: rgba(56, 189, 248, 0.12);
            --success: #10b981;
            --danger: #ef4444;
            --warning: #f59e0b;
            --text: #f8fafc;
            --text-dim: #94a3b8;
        }
        * { box-sizing: border-box; }
        body {
            background-color: var(--bg);
            color: var(--text);
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            margin: 0;
            padding: 20px 24px;
            display: flex;
            flex-direction: column;
            min-height: 100vh;
        }
        header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            border-bottom: 1px solid var(--surface-border);
            padding-bottom: 14px;
            margin-bottom: 20px;
        }
        .header-title {
            display: flex;
            align-items: center;
            gap: 12px;
        }
        h1 { font-size: 1.3rem; margin: 0; font-weight: 700; letter-spacing: -0.02em; display: flex; align-items: center; gap: 8px; }
        .badge {
            font-size: 0.72rem;
            background: var(--accent-dim);
            color: var(--accent);
            padding: 4px 10px;
            border-radius: 99px;
            border: 1px solid rgba(56, 189, 248, 0.25);
            font-weight: 600;
        }
        .badge-admin {
            background: rgba(16, 185, 129, 0.15);
            color: var(--success);
            border-color: rgba(16, 185, 129, 0.3);
        }
        .layout-grid {
            display: grid;
            grid-template-columns: 380px 1fr;
            gap: 20px;
            flex: 1;
        }
        @media (max-width: 960px) {
            .layout-grid { grid-template-columns: 1fr; }
        }
        .card {
            background: var(--surface);
            border: 1px solid var(--surface-border);
            border-radius: 14px;
            padding: 18px;
            margin-bottom: 18px;
            display: flex;
            flex-direction: column;
        }
        .card-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 14px;
            font-size: 0.9rem;
            font-weight: 600;
            color: var(--accent);
        }
        .qr-box {
            text-align: center;
            background: #fff;
            padding: 12px;
            border-radius: 10px;
            width: fit-content;
            margin: 0 auto 14px auto;
            box-shadow: 0 4px 12px rgba(0,0,0,0.3);
        }
        .qr-box img { display: block; max-width: 100%%; height: auto; }
        .url-field {
            display: flex;
            gap: 8px;
            background: #090d16;
            border: 1px solid var(--surface-border);
            border-radius: 8px;
            padding: 8px 12px;
            font-size: 0.78rem;
            font-family: ui-monospace, monospace;
            align-items: center;
            word-break: break-all;
        }
        .url-field span { flex: 1; color: #cbd5e1; }
        .btn-copy {
            background: var(--accent);
            color: #0b0f19;
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
            font-size: 0.78rem;
            text-align: left;
        }
        th, td {
            padding: 9px 12px;
            border-bottom: 1px solid var(--surface-border);
        }
        th { background: rgba(0,0,0,0.25); color: var(--text-dim); font-weight: 600; }
        tr:last-child td { border-bottom: none; }
        .btn-disconnect {
            background: rgba(239, 68, 68, 0.15);
            color: var(--danger);
            border: 1px solid rgba(239, 68, 68, 0.3);
            padding: 3px 8px;
            border-radius: 4px;
            font-size: 0.7rem;
            font-weight: 600;
            cursor: pointer;
        }
        .btn-disconnect:hover { background: var(--danger); color: #fff; }
        .btn-regen {
            background: transparent;
            color: var(--accent);
            border: 1px solid rgba(56, 189, 248, 0.4);
            padding: 5px 10px;
            border-radius: 6px;
            font-size: 0.75rem;
            font-weight: 600;
            cursor: pointer;
        }
        .btn-regen:hover { background: var(--accent-dim); }

        /* ChatGPT Layout for Admin */
        .chat-panel {
            background: var(--surface);
            border: 1px solid var(--surface-border);
            border-radius: 14px;
            display: flex;
            flex-direction: column;
            overflow: hidden;
            height: calc(100vh - 100px);
        }
        .chat-header {
            padding: 12px 18px;
            border-bottom: 1px solid var(--surface-border);
            display: flex;
            justify-content: space-between;
            align-items: center;
            background: rgba(0,0,0,0.15);
        }
        .agent-info {
            display: flex;
            align-items: center;
            gap: 10px;
        }
        .agent-info .avatar {
            width: 32px;
            height: 32px;
            border-radius: 50%%;
            background: linear-gradient(135deg, #0284c7, #0f172a);
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 14px;
        }
        .chat-body {
            flex: 1;
            overflow-y: auto;
            padding: 18px;
            display: flex;
            flex-direction: column;
            gap: 16px;
        }
        .msg-row {
            display: flex;
            gap: 12px;
            max-width: 90%%;
        }
        .msg-row.agent { align-self: flex-start; }
        .msg-row.user { align-self: flex-end; flex-direction: row-reverse; }
        .bubble {
            background: var(--agent-bubble);
            border: 1px solid var(--surface-border);
            border-radius: 12px;
            padding: 14px 16px;
            color: #e2e8f0;
            font-size: 0.9rem;
            line-height: 1.5;
        }
        .msg-row.user .bubble {
            background: var(--user-bubble);
            border-color: rgba(56, 189, 248, 0.4);
            color: #fff;
        }
        .terminal-viewport {
            font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
            font-size: 11.5px;
            line-height: 1.5;
            white-space: pre-wrap;
            word-break: break-all;
            background: #070a12;
            padding: 12px;
            border-radius: 8px;
            border: 1px solid rgba(255,255,255,0.06);
            margin-top: 8px;
            max-height: 340px;
            overflow-y: auto;
            color: #cbd5e1;
        }
        .approval-banner {
            background: rgba(239, 68, 68, 0.08);
            border: 1px solid var(--danger);
            border-radius: 10px;
            padding: 14px;
            margin-top: 10px;
            animation: pulse-border 2s infinite;
        }
        .action-grid {
            display: flex;
            gap: 10px;
            margin-top: 10px;
        }
        .btn-act {
            border: none;
            padding: 10px 18px;
            font-size: 0.85rem;
            font-weight: 600;
            border-radius: 8px;
            cursor: pointer;
            display: flex;
            align-items: center;
            gap: 6px;
        }
        .btn-act-app { background: var(--success); color: #fff; }
        .btn-act-rej { background: var(--danger); color: #fff; }
        .chat-footer {
            padding: 14px 18px;
            border-top: 1px solid var(--surface-border);
            background: rgba(0,0,0,0.2);
        }
        .chat-input-pill {
            display: flex;
            align-items: center;
            background: var(--bg);
            border: 1px solid var(--surface-border);
            border-radius: 26px;
            padding: 4px 6px 4px 16px;
            gap: 8px;
            transition: border-color 0.2s;
        }
        .chat-input-pill:focus-within { border-color: var(--accent); }
        .txt-chat {
            flex: 1;
            background: transparent;
            border: none;
            color: #fff;
            font-size: 0.9rem;
            outline: none;
            padding: 6px 0;
        }
        .txt-chat::placeholder { color: var(--text-dim); }
        .btn-arrow-send {
            width: 32px;
            height: 32px;
            border-radius: 50%%;
            background: var(--accent);
            color: #0b0f19;
            border: none;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 16px;
            font-weight: bold;
            cursor: pointer;
        }
        .view-pill {
            display: flex;
            background: var(--bg);
            border: 1px solid var(--surface-border);
            border-radius: 18px;
            padding: 2px;
        }
        .btn-view-toggle {
            background: transparent;
            border: none;
            color: var(--text-dim);
            padding: 4px 10px;
            font-size: 0.72rem;
            font-weight: 600;
            border-radius: 14px;
            cursor: pointer;
        }
        .btn-view-toggle.active {
            background: var(--accent);
            color: #0b0f19;
        }

        /* iOS Push Notification Card */
        .notify-channel {
            background: rgba(0,0,0,0.25);
            border: 1px solid var(--surface-border);
            border-radius: 8px;
            padding: 10px 12px;
            margin-top: 8px;
            font-size: 0.78rem;
        }
        .channel-title {
            display: flex;
            justify-content: space-between;
            align-items: center;
            font-weight: 600;
            color: var(--text);
            margin-bottom: 4px;
        }
    </style>
</head>
<body>
    <header>
        <div class="header-title">
            <h1>🛡️ Guaita Control Center</h1>
            <span class="badge badge-admin">LOCALHOST ADMIN</span>
            <span class="badge">LAN: %s:%s</span>
        </div>
        <div id="connStatus" style="font-size: 0.8rem; color: var(--success); font-weight: 600;">● Live Herdr Link</div>
    </header>

    <div class="layout-grid">
        <div>
            <!-- Mobile Pairing QR Card -->
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
                <div style="font-size: 0.72rem; color: var(--text-dim); margin-top: 10px; line-height: 1.4;">
                    Scan with your mobile camera. Sessions persist for <strong>%d days</strong> across Wi-Fi and UniFi Teleport VPN.
                </div>
            </div>

            <!-- iOS Notification Channels Card -->
            <div class="card">
                <div class="card-header">
                    <span>🔔 iOS Push Channels</span>
                </div>
                <div class="notify-channel">
                    <div class="channel-title">
                        <span>⭐️ ntfy.sh (Interactive Push)</span>
                        <span style="color: var(--success); font-size: 0.7rem;">Active</span>
                    </div>
                    <div style="display: flex; gap: 10px; align-items: center; margin-top: 6px;">
                        <img src="data:image/png;base64,%s" style="width: 64px; height: 64px; background: #fff; padding: 4px; border-radius: 6px;" alt="ntfy QR">
                        <div style="flex: 1; font-size: 0.7rem; color: var(--text-dim);">
                            <div>Topic: <strong>%s</strong></div>
                            <div style="margin-top: 4px;"><a href="%s" target="_blank" style="color: var(--accent); text-decoration: none;">Open in ntfy app →</a></div>
                        </div>
                    </div>
                </div>

                <div class="notify-channel">
                    <div class="channel-title">
                        <span>💬 Telegram Bot</span>
                        <span style="color: var(--accent); font-size: 0.7rem;">%s</span>
                    </div>
                    <div style="font-size: 0.7rem; color: var(--text-dim);">
                        Sends interactive inline buttons directly to your iPhone and Apple Watch.
                    </div>
                </div>

                <div class="notify-channel">
                    <div class="channel-title">
                        <span>📱 Web Push PWA</span>
                        <span style="color: var(--text-dim); font-size: 0.7rem;">iOS 16.4+</span>
                    </div>
                    <div style="font-size: 0.7rem; color: var(--text-dim);">
                        Add Guaita to Home Screen on Safari to enable APNs web notifications.
                    </div>
                </div>
            </div>

            <!-- Active Clients Telemetry Card -->
            <div class="card" style="margin-bottom: 0;">
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

        <!-- ChatGPT-style Agent Sentinel & Conversation Card -->
        <div class="chat-panel">
            <div class="chat-header">
                <div class="agent-info">
                    <div class="avatar">🤖</div>
                    <div>
                        <div style="font-weight: 600; font-size: 0.95rem;">Herdr Agent Sentinel</div>
                        <div id="paneBadge" style="font-size: 0.72rem; color: var(--accent);">Target: Auto-detecting pane...</div>
                    </div>
                </div>
                <div class="view-pill">
                    <button id="btnAdminChat" class="btn-view-toggle active" onclick="switchAdminView('chat')">💬 Chat</button>
                    <button id="btnAdminTerm" class="btn-view-toggle" onclick="switchAdminView('term')">💻 Raw</button>
                </div>
            </div>

            <div id="adminChatBody" class="chat-body">
                <div class="msg-row agent">
                    <div style="font-size: 20px;">🤖</div>
                    <div class="bubble" style="width: 100%%;">
                        <div style="font-size: 0.75rem; color: var(--accent); font-weight: 600; margin-bottom: 4px;">AGENT EXECUTION STREAM</div>
                        <div id="agentTextAdmin">Monitoring active Herdr session. Output from current pane:</div>
                        <div id="terminalSnippetAdmin" class="terminal-viewport">Awaiting Herdr output...</div>
                        
                        <div id="approvalBannerAdmin" class="approval-banner" style="display: none;">
                            <div style="font-size: 0.82rem; font-weight: 700; color: var(--danger); text-transform: uppercase;">⚠️ Human Approval Required</div>
                            <div style="font-size: 0.8rem; color: #cbd5e1; margin-top: 4px;">Agent is waiting for your confirmation:</div>
                            <div class="action-grid">
                                <button class="btn-act btn-act-app" onclick="sendAction('approve')">✓ APPROVE (Y)</button>
                                <button class="btn-act btn-act-rej" onclick="sendAction('reject')">✕ REJECT (N)</button>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <div id="adminRawBody" class="terminal-viewport" style="display: none; height: 100%%; margin: 0; border: none; border-radius: 0;"></div>

            <div class="chat-footer">
                <div style="display: flex; gap: 8px; margin-bottom: 8px;">
                    <button class="btn-view-toggle" onclick="sendAction('approve')">✓ Approve (Y)</button>
                    <button class="btn-view-toggle" onclick="sendAction('reject')">✕ Reject (N)</button>
                    <button class="btn-view-toggle" onclick="sendAction('enter')">↵ Enter</button>
                    <button class="btn-view-toggle" onclick="sendAction('esc')">⎋ Esc</button>
                </div>
                <div class="chat-input-pill">
                    <input type="text" id="adminInput" class="txt-chat" placeholder="Reply to agent or send instruction..." onkeydown="if(event.key==='Enter') sendAdminInput()">
                    <button class="btn-arrow-send" onclick="sendAdminInput()" title="Send">↑</button>
                </div>
            </div>
        </div>
    </div>

<script>
    let ws;
    let reconnectInterval = 1000;
    const connStatus = document.getElementById('connStatus');
    const paneBadge = document.getElementById('paneBadge');
    const termSnippetAdmin = document.getElementById('terminalSnippetAdmin');
    const rawBodyAdmin = document.getElementById('adminRawBody');
    const approvalBanner = document.getElementById('approvalBannerAdmin');
    const clientsTable = document.getElementById('clientsTable');
    const clientCount = document.getElementById('clientCount');
    const chatBody = document.getElementById('adminChatBody');

    function switchAdminView(mode) {
        if (mode === 'chat') {
            chatBody.style.display = 'flex';
            rawBodyAdmin.style.display = 'none';
            document.getElementById('btnAdminChat').classList.add('active');
            document.getElementById('btnAdminTerm').classList.remove('active');
        } else {
            chatBody.style.display = 'none';
            rawBodyAdmin.style.display = 'block';
            document.getElementById('btnAdminTerm').classList.add('active');
            document.getElementById('btnAdminChat').classList.remove('active');
        }
    }

    function appendUserMessage(text) {
        const row = document.createElement('div');
        row.className = 'msg-row user';
        row.innerHTML = ` + "`" + `
            <div style="font-size: 18px;">👤</div>
            <div class="bubble">
                <div style="font-size: 0.7rem; color: #94a3b8; margin-bottom: 2px;">YOU</div>
                <div>${escapeHTML(text)}</div>
            </div>
        ` + "`" + `;
        chatBody.appendChild(row);
        chatBody.scrollTop = chatBody.scrollHeight;
    }

    function escapeHTML(str) {
        return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    function connect() {
        const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
        ws = new WebSocket(` + "`" + `${protocol}//${location.host}/ws` + "`" + `);

        ws.onopen = () => {
            connStatus.style.color = 'var(--success)';
            connStatus.innerText = '● Live Herdr Link';
            reconnectInterval = 1000;
        };

        ws.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                if (data.output !== undefined) {
                    termSnippetAdmin.innerText = data.output;
                    rawBodyAdmin.innerText = data.output;
                    termSnippetAdmin.scrollTop = termSnippetAdmin.scrollHeight;
                    rawBodyAdmin.scrollTop = rawBodyAdmin.scrollHeight;
                }

                if (data.pane_id) {
                    paneBadge.innerText = ` + "`" + `Active Pane: ${data.pane_id} • Status: ${data.agent_state || 'ready'}` + "`" + `;
                }

                if (data.waiting) {
                    approvalBanner.style.display = 'block';
                } else {
                    approvalBanner.style.display = 'none';
                }

                if (data.clients) {
                    renderClients(data.clients);
                }
            } catch (err) {
                console.error("WS error", err);
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
                    <div><strong>${c.is_localhost ? 'Localhost (Admin)' : c.remote_addr}</strong></div>
                    <div style="font-size: 0.7rem; color: var(--text-dim); max-width: 170px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">${c.user_agent}</div>
                </td>
                <td style="color: var(--text-dim); font-size: 0.72rem;">${formatTime(c.connected_at)}</td>
                <td>
                    ${c.is_localhost ? '<span style="color: var(--success); font-size: 0.75rem;">Self</span>' : ('<button class="btn-disconnect" onclick="disconnectClient(\'' + c.id + '\')">Kick</button>')}
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
        appendUserMessage(action.toUpperCase());
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
        appendUserMessage(val);
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
