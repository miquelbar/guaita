package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TelegramNotifier sends interactive messages with inline keyboards to Telegram.
type TelegramNotifier struct {
	Token    string
	ChatID   string
	Client   *http.Client
	onAction func(string) error
	stopOnce sync.Once
	stopChan chan struct{}
	mu       sync.Mutex
	messages map[string]telegramMessage
	resolved map[string]string
}

type telegramMessage struct {
	ID   int64
	Text string
}

// NewTelegram creates a TelegramNotifier and starts the callback query listener if enabled.
func NewTelegram(token, chatID string, onAction func(string) error) *TelegramNotifier {
	t := &TelegramNotifier{
		Token:    token,
		ChatID:   chatID,
		Client:   &http.Client{Timeout: 10 * time.Second},
		onAction: onAction,
		stopChan: make(chan struct{}),
		messages: make(map[string]telegramMessage),
		resolved: make(map[string]string),
	}

	if token != "" && chatID != "" && onAction != nil {
		go t.pollUpdates()
	}

	return t
}

// Name returns the channel identifier.
func (t *TelegramNotifier) Name() string {
	return "telegram"
}

// ParseTelegramAction parses "approve", "reject", "enter", "esc", "approve:<paneID>", "reject:<paneID>", or "act:<action>:<paneID>".
func ParseTelegramAction(raw string) (paneID, action string, ok bool) {
	if raw == "approve" || raw == "reject" || raw == "enter" || raw == "esc" {
		return "", raw, true
	}
	if strings.HasPrefix(raw, "approve:") {
		return strings.TrimPrefix(raw, "approve:"), "approve", true
	}
	if strings.HasPrefix(raw, "reject:") {
		return strings.TrimPrefix(raw, "reject:"), "reject", true
	}
	if strings.HasPrefix(raw, "act:") {
		trimmed := strings.TrimPrefix(raw, "act:")
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) == 2 {
			return parts[1], parts[0], true
		}
		return "", parts[0], true
	}
	return "", "", false
}

// Close stops the background update poller.
func (t *TelegramNotifier) Close() {
	t.stopOnce.Do(func() {
		close(t.stopChan)
	})
}

// BeginPending clears state from the preceding request before async delivery begins.
func (t *TelegramNotifier) BeginPending(paneID string) {
	key := paneKey(paneID)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.messages == nil {
		t.messages = make(map[string]telegramMessage)
	}
	if t.resolved == nil {
		t.resolved = make(map[string]string)
	}
	delete(t.messages, key)
	delete(t.resolved, key)
}

func truncateRunesTail(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[len(runes)-maxRunes:])
}

// Notify sends an alert message with inline buttons to the configured Telegram chat.
func (t *TelegramNotifier) Notify(event NotificationEvent) error {
	if t.Token == "" || t.ChatID == "" {
		return nil
	}

	body := truncateRunesTail(event.Body, 2200)

	headerLine := fmt.Sprintf("Agent in pane <code>%s</code> is paused awaiting human approval:", html.EscapeString(event.PaneID))
	if strings.Contains(strings.ToLower(event.Title), "question") {
		headerLine = fmt.Sprintf("Agent in pane <code>%s</code> asked a question and is waiting for your reply:", html.EscapeString(event.PaneID))
	}

	messageText := fmt.Sprintf("🛡️ <b>Guaita Sentinel Alert</b>\n\n%s\n\n<pre>%s</pre>",
		headerLine,
		html.EscapeString(body),
	)

	var keyboard [][]map[string]string
	if len(event.Options) > 0 {
		keyboard = make([][]map[string]string, 0, len(event.Options)+1)
		for _, opt := range event.Options {
			cbData := "act:" + opt.Key
			if event.PaneID != "" && len("act:"+opt.Key+":"+event.PaneID) <= 64 {
				cbData = fmt.Sprintf("act:%s:%s", opt.Key, event.PaneID)
			}
			icon := "• "
			switch opt.Style {
			case "approve":
				icon = "✓ "
			case "always":
				icon = "⚡ "
			case "reject":
				icon = "✕ "
			}
			label := icon + opt.Label
			if len([]rune(label)) > 36 {
				label = string([]rune(label)[:33]) + "..."
			}
			keyboard = append(keyboard, []map[string]string{
				{"text": label, "callback_data": cbData},
			})
		}
		enterData := "act:enter"
		escData := "act:esc"
		if event.PaneID != "" && len("act:enter:"+event.PaneID) <= 64 {
			enterData = "act:enter:" + event.PaneID
			escData = "act:esc:" + event.PaneID
		}
		keyboard = append(keyboard, []map[string]string{
			{"text": "↵ Send Enter", "callback_data": enterData},
			{"text": "⎋ Cancel (Esc)", "callback_data": escData},
		})
	} else {
		approveData := "approve"
		rejectData := "reject"
		if event.PaneID != "" && len("approve:"+event.PaneID) <= 64 {
			approveData = "approve:" + event.PaneID
			rejectData = "reject:" + event.PaneID
		}

		keyboard = [][]map[string]string{
			{
				{"text": "✓ APPROVE (Y)", "callback_data": approveData},
				{"text": "✕ REJECT (N)", "callback_data": rejectData},
			},
		}
	}

	payload := map[string]interface{}{
		"chat_id":    t.ChatID,
		"text":       messageText,
		"parse_mode": "HTML",
		"reply_markup": map[string]interface{}{
			"inline_keyboard": keyboard,
		},
	}
	key := paneKey(event.PaneID)

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.Token)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("telegram returned HTTP %d: %s", resp.StatusCode, string(errBody))
	}

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && result.OK && result.Result.MessageID != 0 {
		t.mu.Lock()
		if t.messages == nil {
			t.messages = make(map[string]telegramMessage)
		}
		t.messages[key] = telegramMessage{ID: result.Result.MessageID, Text: messageText}
		resolvedAction := t.resolved[key]
		t.mu.Unlock()
		if resolvedAction != "" {
			return t.Resolve(event.PaneID, resolvedAction)
		}
	}

	return nil
}

// Resolve replaces an active Telegram keyboard with a permanent action result.
func (t *TelegramNotifier) Resolve(paneID, action string) error {
	if t.Token == "" || t.ChatID == "" {
		return nil
	}

	key := paneKey(paneID)
	t.mu.Lock()
	if t.resolved == nil {
		t.resolved = make(map[string]string)
	}
	t.resolved[key] = action
	message, ok := t.messages[key]
	t.mu.Unlock()
	if !ok || message.ID == 0 {
		return nil
	}

	label := "✅ RESPONSE SENT"
	switch strings.ToLower(action) {
	case "approve", "yes", "y":
		label = "✅ APPROVED"
	case "reject", "no", "n":
		label = "❌ REJECTED"
	default:
		label = fmt.Sprintf("✅ OPTION SELECTED (%s)", html.EscapeString(action))
	}
	payload := map[string]interface{}{
		"chat_id":    t.ChatID,
		"message_id": message.ID,
		"text":       message.Text + "\n\n<b>" + label + "</b>",
		"parse_mode": "HTML",
		"reply_markup": map[string]interface{}{
			"inline_keyboard": [][]map[string]string{},
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/editMessageText", t.Token)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("telegram resolution update returned HTTP %d: %s", resp.StatusCode, string(errBody))
	}
	return nil
}

type telegramUpdate struct {
	UpdateID      int64 `json:"update_id"`
	CallbackQuery *struct {
		ID   string `json:"id"`
		Data string `json:"data"`
		From struct {
			ID        int64  `json:"id"`
			FirstName string `json:"first_name"`
		} `json:"from"`
		Message *struct {
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
	} `json:"callback_query"`
}

type telegramUpdatesResponse struct {
	OK     bool             `json:"ok"`
	Result []telegramUpdate `json:"result"`
}

func (t *TelegramNotifier) pollUpdates() {
	var offset int64 = 0

	for {
		select {
		case <-t.stopChan:
			return
		default:
		}

		url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=5", t.Token, offset)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}

		resp, err := t.Client.Do(req)
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}

		var data telegramUpdatesResponse
		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil && data.OK {
			for _, u := range data.Result {
				if u.UpdateID >= offset {
					offset = u.UpdateID + 1
				}

				if u.CallbackQuery != nil && t.onAction != nil {
					if !t.isAuthorizedCallback(u.CallbackQuery.From.ID, u.CallbackQuery.Message) {
						t.answerCallbackQuery(u.CallbackQuery.ID, "Unauthorized")
						continue
					}
					paneID, action, ok := ParseTelegramAction(u.CallbackQuery.Data)
					if !ok {
						t.answerCallbackQuery(u.CallbackQuery.ID, "Unsupported action")
						continue
					}
					if err := t.onAction(u.CallbackQuery.Data); err != nil {
						t.answerCallbackQuery(u.CallbackQuery.ID, err.Error())
					} else if paneID != "" {
						t.answerCallbackQuery(u.CallbackQuery.ID, fmt.Sprintf("Action '%s' sent to %s!", action, paneID))
					} else {
						t.answerCallbackQuery(u.CallbackQuery.ID, fmt.Sprintf("Action '%s' dispatched!", action))
					}
				}
			}
		}
		resp.Body.Close()
	}
}

func (t *TelegramNotifier) isAuthorizedCallback(fromID int64, msg *struct {
	Chat struct {
		ID int64 `json:"id"`
	} `json:"chat"`
}) bool {
	if msg != nil && strconv.FormatInt(msg.Chat.ID, 10) == t.ChatID {
		return true
	}
	return strconv.FormatInt(fromID, 10) == t.ChatID
}

func (t *TelegramNotifier) answerCallbackQuery(queryID, text string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/answerCallbackQuery", t.Token)
	payload := map[string]string{
		"callback_query_id": queryID,
		"text":              text,
	}
	b, _ := json.Marshal(payload)
	_, _ = t.Client.Post(url, "application/json", bytes.NewReader(b))
}
