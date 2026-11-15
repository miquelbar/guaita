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
	CSPNonce        string
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
	return RenderMobileHTMLWithNonce(sessionDays, "")
}

// RenderMobileHTMLWithNonce renders the mobile-optimized Sentinel Gate interface with a CSP script nonce.
func RenderMobileHTMLWithNonce(sessionDays int, nonce string) string {
	return fmt.Sprintf(mobileTemplate, sessionDays, nonce)
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
		data.CSPNonce,
	)
}

const mobileTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, viewport-fit=cover, interactive-widget=resizes-content">
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
        html, body {
            height: 100vh;
            height: 100dvh;
            margin: 0;
            padding: 0;
            background-color: var(--bg);
            color: var(--text);
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            overflow: hidden;
        }
        body {
            display: flex;
            flex-direction: column;
        }
        header {
            padding: 12px 16px;
            padding-top: max(12px, env(safe-area-inset-top));
            background: rgba(11, 15, 25, 0.95);
            backdrop-filter: blur(12px);
            border-bottom: 1px solid var(--surface-border);
            display: flex;
            justify-content: space-between;
            align-items: center;
            z-index: 50;
            flex-shrink: 0;
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
            touch-action: manipulation;
        }
        .toggle-btn.active {
            background: var(--accent);
            color: #0b0f19;
        }

        /* Chat area */
        .chat-container {
            flex: 1;
            overflow-y: auto;
            -webkit-overflow-scrolling: touch;
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
            -webkit-overflow-scrolling: touch;
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
            padding: 14px;
            font-size: 0.92rem;
            font-weight: 600;
            border-radius: 10px;
            cursor: pointer;
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 6px;
            touch-action: manipulation;
            transition: transform 0.1s active, opacity 0.2s;
        }
        .pill-btn:active { transform: scale(0.96); opacity: 0.9; }
        .pill-approve { background: var(--success); color: #fff; }
        .pill-reject { background: var(--danger); color: #fff; }

        /* Floating ChatGPT-style input bar */
        .bottom-bar {
            padding: 10px 16px;
            padding-bottom: max(16px, env(safe-area-inset-bottom));
            padding-left: max(16px, env(safe-area-inset-left));
            padding-right: max(16px, env(safe-area-inset-right));
            background: rgba(11, 15, 25, 0.98);
            backdrop-filter: blur(16px);
            border-top: 1px solid var(--surface-border);
            flex-shrink: 0;
            position: relative;
            z-index: 100;
        }
        .quick-chips {
            display: flex;
            gap: 6px;
            overflow-x: auto;
            padding-bottom: 8px;
            scrollbar-width: none;
            -webkit-overflow-scrolling: touch;
        }
        .quick-chips::-webkit-scrollbar { display: none; }
        .chip {
            background: var(--surface);
            border: 1px solid var(--surface-border);
            color: var(--text-dim);
            font-size: 0.76rem;
            padding: 6px 12px;
            border-radius: 99px;
            white-space: nowrap;
            cursor: pointer;
            font-weight: 500;
            touch-action: manipulation;
        }
        .chip:active { border-color: var(--accent); color: var(--accent); background: var(--surface-hover); }
        .input-pill-box {
            display: flex;
            align-items: center;
            background: var(--surface);
            border: 1px solid var(--surface-border);
            border-radius: 26px;
            padding: 4px 6px 4px 16px;
            gap: 8px;
            transition: border-color 0.2s;
        }
        .input-pill-box:focus-within { border-color: var(--accent); }
        .chat-input {
            flex: 1;
            background: transparent;
            border: none;
            color: #fff;
            font-size: 16px !important; /* Prevents auto-zoom and focus jump on iOS */
            outline: none;
            padding: 8px 0;
            -webkit-appearance: none;
        }
        .chat-input::placeholder { color: var(--text-dim); font-size: 0.9rem; }
        .btn-send-round {
            width: 36px;
            height: 36px;
            border-radius: 50%%;
            background: var(--accent);
            color: #0b0f19;
            border: none;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 18px;
            font-weight: bold;
            cursor: pointer;
            flex-shrink: 0;
            touch-action: manipulation;
            transition: transform 0.1s active, opacity 0.2s;
        }
        .btn-send-round:active { transform: scale(0.92); opacity: 0.9; }
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
            -webkit-overflow-scrolling: touch;
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
                <div id="connStatus" class="status-subtitle">● Connecting...</div>
            </div>
        </div>
        <div class="view-toggle">
            <button type="button" id="tabChat" class="toggle-btn active" data-view="chat">💬 Chat</button>
            <button type="button" id="tabTerminal" class="toggle-btn" data-view="terminal">💻 Raw</button>
        </div>
    </header>

    <!-- Main ChatGPT-style message stream -->
    <div id="chatView" class="chat-container">
        <div id="agentStatusRow" class="message-row agent">
            <div class="msg-avatar agent">🤖</div>
            <div class="msg-content">
                <div style="font-size: 0.72rem; color: var(--accent); font-weight: 600; margin-bottom: 4px;">HERDR SENTINEL <span id="agentTime" style="color: var(--text-dim); font-weight: 400; margin-left: 6px;"></span></div>
                <div id="agentText">Connected to Herdr agent workspace. Monitoring terminal for prompts...</div>
                <div id="terminalSnippet" class="terminal-block">Loading initial agent buffer...</div>
                <div id="approvalCard" class="approval-card" style="display: none;">
                    <div class="approval-title">⚠️ Approval Requested</div>
                    <div style="font-size: 0.8rem; color: #cbd5e1;">Agent is paused awaiting human confirmation:</div>
                    <div class="action-pills">
                        <button type="button" class="pill-btn pill-approve" data-action="approve">✓ APPROVE (Y)</button>
                        <button type="button" class="pill-btn pill-reject" data-action="reject">✕ REJECT (N)</button>
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
            <button type="button" class="chip" data-action="approve">✓ Approve (Y)</button>
            <button type="button" class="chip" data-action="reject">✕ Reject (N)</button>
            <button type="button" class="chip" data-action="enter">↵ Enter</button>
            <button type="button" class="chip" data-action="esc">⎋ Esc</button>
        </div>
        <form id="mobileInputForm" class="input-pill-box">
            <input type="text" id="userInput" class="chat-input" placeholder="Reply to agent or type command..." enterkeyhint="send" autocomplete="off" autocorrect="off" autocapitalize="off" spellcheck="false">
            <button type="submit" class="btn-send-round" title="Send">↑</button>
        </form>
        <div style="text-align: center; font-size: 0.65rem; color: var(--text-dim); margin-top: 6px;">
            Encrypted session active (Expires in %d days)
        </div>
    </div>

<script nonce="%s">
    // Strip any leftover token query param from browser history; auth relies on HttpOnly cookie.
    if (window.location.search.includes('token=')) {
        try { window.history.replaceState({}, document.title, window.location.pathname); } catch(e){}
    }
    try { localStorage.removeItem('guaita_session_token'); } catch(e){}

    let ws;
    let reconnectInterval = 1000;
    const connStatus = document.getElementById('connStatus');
    const termSnippet = document.getElementById('terminalSnippet');
    const rawTerminal = document.getElementById('rawTerminalView');
    const approvalCard = document.getElementById('approvalCard');
    const chatContainer = document.getElementById('chatView');
    const agentStatusRow = document.getElementById('agentStatusRow');
    const agentTimeEl = document.getElementById('agentTime');
    let wasWaiting = false;
    let lastOutput = '';
    let moveAgentAfterUser = false;

    function nowTimeStr() {
        return new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
    }

    // Viewport handling for iOS Virtual Keyboard
    if (window.visualViewport) {
        window.visualViewport.addEventListener('resize', () => {
            document.body.style.height = window.visualViewport.height + 'px';
            chatContainer.scrollTop = chatContainer.scrollHeight;
        });
        window.visualViewport.addEventListener('scroll', () => {
            window.scrollTo(0, 0);
        });
    }

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
                <div style="font-size: 0.7rem; color: #94a3b8; margin-bottom: 2px;">YOU • ${nowTimeStr()}</div>
                <div>${escapeHTML(text)}</div>
            </div>
        ` + "`" + `;
        chatContainer.appendChild(row);
        moveAgentAfterUser = true;
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
                const outputChanged = data.output !== undefined && data.output !== lastOutput;
                if (data.output !== undefined) {
                    lastOutput = data.output;
                    if (outputChanged && agentTimeEl) {
                        agentTimeEl.innerText = nowTimeStr();
                    }
                    termSnippet.innerText = data.output;
                    rawTerminal.innerText = data.output;
                    termSnippet.scrollTop = termSnippet.scrollHeight;
                    rawTerminal.scrollTop = rawTerminal.scrollHeight;
                }

                if (data.waiting) {
                    approvalCard.style.display = 'block';
                    if (!wasWaiting) {
                        chatContainer.appendChild(agentStatusRow);
                        moveAgentAfterUser = false;
                        if (navigator.vibrate) navigator.vibrate([250, 100, 250, 100, 300]);
                        wasWaiting = true;
                    }
                } else {
                    approvalCard.style.display = 'none';
                    wasWaiting = false;
                }

                if (moveAgentAfterUser && outputChanged) {
                    chatContainer.appendChild(agentStatusRow);
                    moveAgentAfterUser = false;
                }
                chatContainer.scrollTop = chatContainer.scrollHeight;
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

    function dispatchAction(action, displayLabel) {
        if (displayLabel) {
            appendUserMessage(displayLabel);
        }
        fetch('/action', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/x-www-form-urlencoded'
            },
            body: new URLSearchParams({action: action})
        }).then(res => {
            if (!res.ok) {
                console.error('Action failed:', res.status);
            } else if (navigator.vibrate) {
                navigator.vibrate(50);
            }
        }).catch(err => {
            console.error('Network error on action:', err);
        });
    }

    function sendAction(action) {
        dispatchAction(action, action.toUpperCase());
    }

    function sendInput() {
        const input = document.getElementById('userInput');
        const val = input.value.trim();
        if (!val) return;
        dispatchAction(val, val);
        input.value = '';
        input.blur();
    }

    document.addEventListener('click', (e) => {
        const viewBtn = e.target.closest('[data-view]');
        if (viewBtn) {
            switchView(viewBtn.dataset.view);
            return;
        }
        const actionBtn = e.target.closest('[data-action]');
        if (actionBtn) {
            sendAction(actionBtn.dataset.action);
        }
    });
    const mobileForm = document.getElementById('mobileInputForm');
    if (mobileForm) {
        mobileForm.addEventListener('submit', (e) => {
            e.preventDefault();
            sendInput();
        });
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
            display: grid;
            grid-template-columns: 1fr auto 1fr;
            align-items: center;
            gap: 16px;
            border-bottom: 1px solid var(--surface-border);
            padding-bottom: 14px;
            margin-bottom: 20px;
        }
        #connStatus {
            justify-self: end;
        }
        .header-title {
            display: flex;
            align-items: center;
            gap: 10px;
            flex-wrap: wrap;
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
            padding: 8px 10px;
            font-size: 0.76rem;
            font-family: ui-monospace, SFMono-Regular, monospace;
            align-items: center;
        }
        .url-field span {
            flex: 1;
            min-width: 0;
            color: #cbd5e1;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }
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
            padding: 14px 18px 16px 18px;
            border-top: 1px solid var(--surface-border);
            background: rgba(0,0,0,0.22);
        }
        .quick-actions {
            display: flex;
            justify-content: center;
            align-items: center;
            flex-wrap: wrap;
            gap: 8px;
            margin-bottom: 12px;
        }
        .btn-quick {
            background: var(--surface);
            border: 1px solid var(--surface-border);
            color: var(--text);
            padding: 7px 14px;
            border-radius: 99px;
            font-size: 0.76rem;
            font-weight: 600;
            cursor: pointer;
            display: inline-flex;
            align-items: center;
            gap: 6px;
            transition: all 0.15s ease;
        }
        .btn-quick:hover {
            background: var(--surface-hover);
            border-color: var(--accent);
            transform: translateY(-1px);
        }
        .btn-quick-approve {
            background: rgba(16, 185, 129, 0.12);
            border-color: rgba(16, 185, 129, 0.35);
            color: #34d399;
        }
        .btn-quick-approve:hover {
            background: rgba(16, 185, 129, 0.22);
            border-color: var(--success);
        }
        .btn-quick-reject {
            background: rgba(239, 68, 68, 0.12);
            border-color: rgba(239, 68, 68, 0.35);
            color: #f87171;
        }
        .btn-quick-reject:hover {
            background: rgba(239, 68, 68, 0.22);
            border-color: var(--danger);
        }
        .chat-input-pill {
            display: flex;
            align-items: center;
            background: var(--bg);
            border: 1px solid var(--surface-border);
            border-radius: 26px;
            padding: 4px 6px 4px 16px;
            gap: 8px;
            max-width: 820px;
            margin: 0 auto;
            width: 100%%;
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
        .top-nav {
            display: inline-flex;
            background: var(--surface);
            border: 1px solid var(--surface-border);
            border-radius: 10px;
            padding: 4px;
            gap: 4px;
        }
        .nav-tab {
            background: transparent;
            border: none;
            color: var(--text-dim);
            padding: 7px 14px;
            border-radius: 7px;
            font-size: 0.82rem;
            font-weight: 600;
            cursor: pointer;
            display: inline-flex;
            align-items: center;
            gap: 6px;
            transition: all 0.15s ease;
        }
        .nav-tab:hover { color: var(--text); background: rgba(255, 255, 255, 0.04); }
        .nav-tab.active {
            background: var(--accent-dim);
            color: var(--accent);
            box-shadow: inset 0 0 0 1px rgba(56, 189, 248, 0.3);
        }
        .section-pane {
            display: none;
            flex-direction: column;
            flex: 1;
            gap: 18px;
        }
        .section-pane.active { display: flex; }
        .clients-grid {
            display: grid;
            grid-template-columns: 1.5fr 1fr;
            gap: 18px;
        }
        .settings-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(340px, 1fr));
            gap: 18px;
        }
        @media (max-width: 960px) {
            header { display: flex; flex-wrap: wrap; justify-content: center; gap: 12px; }
            #connStatus { justify-self: auto; }
            .clients-grid { grid-template-columns: 1fr; }
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
        <nav class="top-nav" aria-label="Primary sections">
            <button id="navSentinel" class="nav-tab active" data-section="sentinel">🤖 Sentinel</button>
            <button id="navClients" class="nav-tab" data-section="clients">👥 Clients (<span id="clientCountTab">0</span>)</button>
            <button id="navSettings" class="nav-tab" data-section="settings">⚙️ Settings</button>
        </nav>
        <div id="connStatus" style="font-size: 0.8rem; color: var(--success); font-weight: 600;">● Live Herdr Link</div>
    </header>

    <section id="sectionSentinel" class="section-pane active">
        <div class="layout-grid">
            <div>
                <!-- Mobile Pairing QR Card -->
                <div class="card">
                    <div class="card-header">
                        <span>📱 Mobile Pairing</span>
                        <button class="btn-regen" data-fn="regenerateToken">Regenerate Token</button>
                    </div>
                    <div class="qr-box">
                        <img id="qrImg" src="data:image/png;base64,%s" alt="Guaita Mobile Pairing QR">
                    </div>
                    <div class="url-field">
                        <span id="pairingURL">%s</span>
                        <button class="btn-copy" data-fn="copyURL">Copy Link</button>
                    </div>
                    <div style="font-size: 0.72rem; color: var(--text-dim); margin-top: 10px; line-height: 1.4;">
                        Scan with your mobile camera. Sessions persist for <strong>%d days</strong> across Wi-Fi and UniFi Teleport VPN.
                    </div>
                </div>
            </div>

            <!-- Agent Sentinel & Conversation Panel -->
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
                        <button id="btnAdminChat" class="btn-view-toggle active" data-admin-view="chat">💬 Chat</button>
                        <button id="btnAdminTerm" class="btn-view-toggle" data-admin-view="term">💻 Raw</button>
                    </div>
                </div>

            <div id="adminChatBody" class="chat-body">
                <div id="adminAgentRow" class="msg-row agent">
                    <div style="font-size: 20px;">🤖</div>
                    <div class="bubble" style="width: 100%%;">
                        <div style="font-size: 0.75rem; color: var(--accent); font-weight: 600; margin-bottom: 4px;">AGENT EXECUTION STREAM <span id="adminAgentTime" style="color: var(--text-dim); font-weight: 400; margin-left: 6px;"></span></div>
                        <div id="agentTextAdmin">Monitoring active Herdr session. Output from current pane:</div>
                        <div id="terminalSnippetAdmin" class="terminal-viewport">Awaiting Herdr output...</div>
                        
                        <div id="approvalBannerAdmin" class="approval-banner" style="display: none;">
                            <div style="font-size: 0.82rem; font-weight: 700; color: var(--danger); text-transform: uppercase;">⚠️ Human Approval Required</div>
                            <div style="font-size: 0.8rem; color: #cbd5e1; margin-top: 4px;">Agent is waiting for your confirmation:</div>
                            <div class="action-grid">
                                <button class="btn-act btn-act-app" data-action="approve">✓ APPROVE (Y)</button>
                                <button class="btn-act btn-act-rej" data-action="reject">✕ REJECT (N)</button>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <div id="adminRawBody" class="terminal-viewport" style="display: none; height: 100%%; margin: 0; border: none; border-radius: 0;"></div>

            <div class="chat-footer">
                <div class="quick-actions">
                    <button class="btn-quick btn-quick-approve" data-action="approve">✓ Approve (Y)</button>
                    <button class="btn-quick btn-quick-reject" data-action="reject">✕ Reject (N)</button>
                    <button class="btn-quick" data-action="enter">↵ Enter</button>
                    <button class="btn-quick" data-action="esc">⎋ Esc</button>
                </div>
                <div class="chat-input-pill">
                    <input type="text" id="adminInput" class="txt-chat" placeholder="Reply to agent or send instruction...">
                    <button class="btn-arrow-send" data-fn="sendAdminInput" title="Send">↑</button>
                </div>
            </div>
        </div>
        </div>
    </section>

    <section id="sectionClients" class="section-pane">
        <div class="clients-grid">
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

            <div class="card" style="margin-bottom: 0;">
                <div class="card-header">
                    <span>🚫 Banned IPs (<span id="bannedCount">0</span>)</span>
                </div>
                <div id="bannedList" style="font-size: 0.78rem; color: var(--text-dim);">No banned IPs</div>
            </div>
        </div>

        <div class="card" style="margin-bottom: 0;">
            <div class="card-header">
                <span>📜 Security Audit Log</span>
            </div>
            <div class="table-wrap" style="max-height: 420px; overflow-y: auto;">
                <table>
                    <thead>
                        <tr>
                            <th>Time</th>
                            <th>IP</th>
                            <th>Event</th>
                        </tr>
                    </thead>
                    <tbody id="auditTable">
                        <tr><td colspan="3" style="text-align: center; color: var(--text-dim);">No events yet</td></tr>
                    </tbody>
                </table>
            </div>
        </div>
    </section>

    <section id="sectionSettings" class="section-pane">
        <div class="settings-grid">
            <div class="card" style="margin-bottom: 0;">
                <div class="card-header">
                    <span>⭐️ ntfy.sh (Interactive Push)</span>
                    <span style="color: var(--success); font-size: 0.75rem;">Active</span>
                </div>
                <div style="display: flex; gap: 14px; align-items: center;">
                    <img src="data:image/png;base64,%s" style="width: 84px; height: 84px; background: #fff; padding: 6px; border-radius: 8px;" alt="ntfy QR">
                    <div style="flex: 1; font-size: 0.8rem; color: var(--text-dim); line-height: 1.5;">
                        <div>Topic: <strong style="color: var(--text);">%s</strong></div>
                        <div style="margin-top: 6px;"><a href="%s" target="_blank" style="color: var(--accent); text-decoration: none;">Open in ntfy app →</a></div>
                    </div>
                </div>
            </div>

            <div class="card" style="margin-bottom: 0;">
                <div class="card-header">
                    <span>💬 Telegram Bot</span>
                    <span id="tgStatusBadge" style="color: var(--accent); font-size: 0.75rem;">%s</span>
                </div>
                <div style="font-size: 0.78rem; color: var(--text-dim); margin-bottom: 12px;">
                    Sends interactive inline approval buttons directly to your iPhone and Apple Watch. Saved in <code>~/.config/guaita/config.json</code>.
                </div>
                <div style="display: flex; flex-direction: column; gap: 8px;">
                    <input id="tgTokenInput" type="password" placeholder="Bot Token (@BotFather)" autocomplete="off" style="background: var(--bg); border: 1px solid var(--surface-border); color: var(--text); border-radius: 8px; padding: 8px 10px; font-size: 0.8rem; outline: none;" />
                    <input id="tgChatIdInput" type="text" placeholder="Chat ID (e.g. 987654321)" autocomplete="off" style="background: var(--bg); border: 1px solid var(--surface-border); color: var(--text); border-radius: 8px; padding: 8px 10px; font-size: 0.8rem; outline: none;" />
                    <div style="display: flex; gap: 8px; margin-top: 4px;">
                        <button class="btn-small" data-fn="saveTelegramConfig" style="flex: 1; padding: 8px;">Save &amp; Enable</button>
                        <button class="btn-small" data-fn="disableTelegramConfig" style="color: var(--danger); padding: 8px 14px;">Disable</button>
                    </div>
                </div>
            </div>

            <div class="card" style="margin-bottom: 0;">
                <div class="card-header">
                    <span>📱 Web Push PWA</span>
                    <span style="color: var(--text-dim); font-size: 0.75rem;">iOS 16.4+</span>
                </div>
                <div style="font-size: 0.78rem; color: var(--text-dim); line-height: 1.5;">
                    Add Guaita to Home Screen on iOS Safari after pairing to enable native APNs web notifications and standalone full-screen mode.
                </div>
            </div>

            <div class="card" style="margin-bottom: 0;">
                <div class="card-header">
                    <span>🔒 Session &amp; Security Policy</span>
                    <span style="color: var(--success); font-size: 0.75rem;">Hardened</span>
                </div>
                <div style="font-size: 0.78rem; color: var(--text-dim); line-height: 1.6;">
                    <div>• One-time pairing token exchanged for a 256-bit <code>HttpOnly</code> session cookie.</div>
                    <div>• Single active remote session enforced; older mobile sessions are auto-revoked.</div>
                    <div>• Admin controls restricted strictly to direct loopback connections.</div>
                </div>
            </div>
        </div>
    </section>

<script nonce="%s">
    // Strip any leftover admin_token query param from browser history; auth relies on HttpOnly cookie.
    if (window.location.search.includes('admin_token=')) {
        try { window.history.replaceState({}, document.title, window.location.pathname); } catch(e){}
    }
    let ws;
    let reconnectInterval = 1000;
    const connStatus = document.getElementById('connStatus');
    const paneBadge = document.getElementById('paneBadge');
    const termSnippetAdmin = document.getElementById('terminalSnippetAdmin');
    const rawBodyAdmin = document.getElementById('adminRawBody');
    const approvalBanner = document.getElementById('approvalBannerAdmin');
    const clientsTable = document.getElementById('clientsTable');
    const clientCount = document.getElementById('clientCount');
    const clientCountTab = document.getElementById('clientCountTab');
    const chatBody = document.getElementById('adminChatBody');
    const adminAgentRow = document.getElementById('adminAgentRow');
    const adminAgentTimeEl = document.getElementById('adminAgentTime');
    let adminLastOutput = '';
    let adminWasWaiting = false;
    let adminMoveAgentAfterUser = false;

    function nowTimeStr() {
        return new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
    }

    function switchAdminSection(section) {
        document.getElementById('sectionSentinel').classList.toggle('active', section === 'sentinel');
        document.getElementById('sectionClients').classList.toggle('active', section === 'clients');
        document.getElementById('sectionSettings').classList.toggle('active', section === 'settings');
        document.getElementById('navSentinel').classList.toggle('active', section === 'sentinel');
        document.getElementById('navClients').classList.toggle('active', section === 'clients');
        document.getElementById('navSettings').classList.toggle('active', section === 'settings');
    }

    function switchAdminView(mode) {
        chatBody.style.display = mode === 'chat' ? 'flex' : 'none';
        rawBodyAdmin.style.display = mode === 'term' ? 'block' : 'none';
        document.getElementById('btnAdminChat').classList.toggle('active', mode === 'chat');
        document.getElementById('btnAdminTerm').classList.toggle('active', mode === 'term');
    }

    function appendUserMessage(text) {
        const row = document.createElement('div');
        row.className = 'msg-row user';
        row.innerHTML = ` + "`" + `
            <div style="font-size: 18px;">👤</div>
            <div class="bubble">
                <div style="font-size: 0.7rem; color: #94a3b8; margin-bottom: 2px;">YOU • ${nowTimeStr()}</div>
                <div>${escapeHTML(text)}</div>
            </div>
        ` + "`" + `;
        chatBody.appendChild(row);
        adminMoveAgentAfterUser = true;
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
                const outputChanged = data.output !== undefined && data.output !== adminLastOutput;
                if (data.output !== undefined) {
                    adminLastOutput = data.output;
                    if (outputChanged && adminAgentTimeEl) {
                        adminAgentTimeEl.innerText = nowTimeStr();
                    }
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
                    if (!adminWasWaiting) {
                        chatBody.appendChild(adminAgentRow);
                        adminMoveAgentAfterUser = false;
                        adminWasWaiting = true;
                    }
                } else {
                    approvalBanner.style.display = 'none';
                    adminWasWaiting = false;
                }

                if (adminMoveAgentAfterUser && outputChanged) {
                    chatBody.appendChild(adminAgentRow);
                    adminMoveAgentAfterUser = false;
                }
                chatBody.scrollTop = chatBody.scrollHeight;

                if (data.clients) {
                    renderClients(data.clients);
                }
                if (data.banned_ips !== undefined) {
                    renderBanned(data.banned_ips);
                }
                if (data.audit_log !== undefined) {
                    renderAudit(data.audit_log);
                }
                if (data.pairing_url && data.qr_base64) {
                    const urlEl = document.getElementById('pairingURL');
                    const qrEl = document.getElementById('qrImg');
                    if (urlEl && urlEl.innerText !== data.pairing_url) {
                        urlEl.innerText = data.pairing_url;
                        qrEl.src = 'data:image/png;base64,' + data.qr_base64;
                    }
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

    function escapeAdminHTML(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
    }

    function renderClients(clients) {
        clientCount.innerText = clients.length;
        if (clientCountTab) clientCountTab.innerText = clients.length;
        if (!clients || clients.length === 0) {
            clientsTable.innerHTML = '<tr><td colspan="3" style="text-align: center; color: var(--text-dim);">No active clients</td></tr>';
            return;
        }
        clientsTable.innerHTML = '';
        clients.forEach(c => {
            const tr = document.createElement('tr');
            const tdInfo = document.createElement('td');
            const addrDiv = document.createElement('div');
            const strong = document.createElement('strong');
            strong.textContent = c.is_localhost ? 'Localhost (Admin)' : c.remote_addr;
            addrDiv.appendChild(strong);

            const uaDiv = document.createElement('div');
            uaDiv.style.cssText = 'font-size: 0.7rem; color: var(--text-dim); max-width: 170px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;';
            uaDiv.textContent = c.user_agent || '';
            tdInfo.appendChild(addrDiv);
            tdInfo.appendChild(uaDiv);

            const tdTime = document.createElement('td');
            tdTime.style.cssText = 'color: var(--text-dim); font-size: 0.72rem;';
            tdTime.textContent = formatTime(c.connected_at);

            const tdAction = document.createElement('td');
            if (c.is_localhost) {
                const selfSpan = document.createElement('span');
                selfSpan.style.cssText = 'color: var(--success); font-size: 0.75rem;';
                selfSpan.textContent = 'Self';
                tdAction.appendChild(selfSpan);
            } else {
                const wrap = document.createElement('div');
                wrap.style.cssText = 'display: flex; gap: 4px;';
                const btnKick = document.createElement('button');
                btnKick.className = 'btn-disconnect';
                btnKick.textContent = 'Kick';
                btnKick.addEventListener('click', () => disconnectClient(c.id));

                const btnBan = document.createElement('button');
                btnBan.className = 'btn-disconnect';
                btnBan.style.background = '#7f1d1d';
                btnBan.textContent = 'Ban';
                btnBan.addEventListener('click', () => banClient(c.id));

                wrap.appendChild(btnKick);
                wrap.appendChild(btnBan);
                tdAction.appendChild(wrap);
            }

            tr.appendChild(tdInfo);
            tr.appendChild(tdTime);
            tr.appendChild(tdAction);
            clientsTable.appendChild(tr);
        });
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

    function banClient(id) {
        if (!confirm('Ban this client IP and disconnect immediately?')) return;
        fetch('/api/clients/ban', {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body: new URLSearchParams({id: id})
        });
    }

    function unbanIP(ip) {
        fetch('/api/clients/unban', {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body: new URLSearchParams({ip: ip})
        });
    }

    function renderBanned(ips) {
        const list = document.getElementById('bannedList');
        const count = document.getElementById('bannedCount');
        if (!list || !count) return;
        count.innerText = ips.length;
        if (!ips || ips.length === 0) {
            list.textContent = 'No banned IPs';
            return;
        }
        list.innerHTML = '';
        ips.forEach(ip => {
            const row = document.createElement('div');
            row.style.cssText = 'display: flex; justify-content: space-between; align-items: center; padding: 4px 0; border-bottom: 1px solid var(--border);';
            const span = document.createElement('span');
            span.textContent = ip;
            const btn = document.createElement('button');
            btn.className = 'btn-small';
            btn.textContent = 'Unban';
            btn.addEventListener('click', () => unbanIP(ip));
            row.appendChild(span);
            row.appendChild(btn);
            list.appendChild(row);
        });
    }

    function renderAudit(entries) {
        const table = document.getElementById('auditTable');
        if (!table) return;
        if (!entries || entries.length === 0) {
            table.innerHTML = '<tr><td colspan="3" style="text-align: center; color: var(--text-dim);">No events yet</td></tr>';
            return;
        }
        table.innerHTML = '';
        entries.slice(0, 50).forEach(e => {
            const tr = document.createElement('tr');
            const tdTime = document.createElement('td');
            tdTime.style.cssText = 'color: var(--text-dim); font-size: 0.68rem; white-space: nowrap;';
            tdTime.textContent = formatTime(e.timestamp);

            const tdIP = document.createElement('td');
            tdIP.style.cssText = 'font-size: 0.7rem;';
            tdIP.textContent = e.ip || '-';

            const tdEvt = document.createElement('td');
            tdEvt.style.cssText = 'font-size: 0.7rem;';
            const evtStrong = document.createElement('strong');
            evtStrong.textContent = e.event + ' ';
            const detSpan = document.createElement('span');
            detSpan.style.color = 'var(--text-dim)';
            detSpan.textContent = e.detail || '';
            tdEvt.appendChild(evtStrong);
            tdEvt.appendChild(detSpan);

            tr.appendChild(tdTime);
            tr.appendChild(tdIP);
            tr.appendChild(tdEvt);
            table.appendChild(tr);
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

    function saveTelegramConfig() {
        const botToken = document.getElementById('tgTokenInput').value.trim();
        const chatId = document.getElementById('tgChatIdInput').value.trim();
        if (!botToken || !chatId) {
            alert('Please enter both Bot Token and Chat ID.');
            return;
        }
        fetch('/api/telegram/config', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ bot_token: botToken, chat_id: chatId })
        }).then(res => {
            if (!res.ok) throw new Error('HTTP ' + res.status);
            return res.json();
        }).then(data => {
            document.getElementById('tgStatusBadge').innerText = data.active ? 'Active' : 'Not configured';
            document.getElementById('tgTokenInput').value = '';
        }).catch(err => {
            alert('Failed to save Telegram config: ' + err.message);
        });
    }

    function disableTelegramConfig() {
        fetch('/api/telegram/config', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ disable: true })
        }).then(res => res.json()).then(() => {
            document.getElementById('tgStatusBadge').innerText = 'Not configured';
            document.getElementById('tgTokenInput').value = '';
            document.getElementById('tgChatIdInput').value = '';
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

    document.addEventListener('click', (e) => {
        const secBtn = e.target.closest('[data-section]');
        if (secBtn) {
            switchAdminSection(secBtn.dataset.section);
            return;
        }
        const viewBtn = e.target.closest('[data-admin-view]');
        if (viewBtn) {
            switchAdminView(viewBtn.dataset.adminView);
            return;
        }
        const actionBtn = e.target.closest('[data-action]');
        if (actionBtn) {
            sendAction(actionBtn.dataset.action);
            return;
        }
        const fnBtn = e.target.closest('[data-fn]');
        if (fnBtn) {
            const fnMap = { regenerateToken, copyURL, sendAdminInput, saveTelegramConfig, disableTelegramConfig };
            const fn = fnMap[fnBtn.dataset.fn];
            if (fn) fn();
        }
    });
    const adminInputEl = document.getElementById('adminInput');
    if (adminInputEl) {
        adminInputEl.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') sendAdminInput();
        });
    }

    connect();
</script>
</body>
</html>`
