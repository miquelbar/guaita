package server

import "fmt"

func RenderHTML(sessionDays int) string {
	return fmt.Sprintf(htmlTemplate, sessionDays)
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
    <title>Guaita // Control Center</title>
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
        <h1>🛡️ Guaita <span class="badge">SECURED</span></h1>
        <div id="connStatus" style="font-size: 0.75rem; color: var(--success);">● Live</div>
    </header>

    <div id="terminal" class="terminal-container">Estableciendo enlace seguro con Herdr...</div>

    <div id="controls" class="controls-panel" style="display: none;">
        <button class="approve" onclick="sendAction('approve')">✓ APROBAR (Y)</button>
        <button class="reject" onclick="sendAction('reject')">✕ RECHAZAR (N)</button>
    </div>

    <div id="inputControls" class="input-row" style="display: none;">
        <input type="text" id="customInput" class="custom-input" placeholder="Enviar respuesta o comando..." onkeydown="if(event.key==='Enter') sendCustomAction()">
        <button class="btn-send" onclick="sendCustomAction()">Enviar</button>
    </div>

    <div class="status-bar">Sesión persistente cifrada activa (Caduca en %d días)</div>

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
                term.innerText = data.output;
                term.scrollTop = term.scrollHeight;

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
                console.error("Error al procesar mensaje WS", err);
            }
        };

        ws.onclose = () => {
            connStatus.style.color = 'var(--danger)';
            connStatus.innerText = '● Reconectando...';
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
            console.error("Error al enviar accion", err);
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
