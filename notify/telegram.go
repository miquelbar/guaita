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
	onAction func(string)
	stopOnce sync.Once
	stopChan chan struct{}
}

// NewTelegram creates a TelegramNotifier and starts the callback query listener if enabled.
func NewTelegram(token, chatID string, onAction func(string)) *TelegramNotifier {
	t := &TelegramNotifier{
		Token:    token,
		ChatID:   chatID,
		Client:   &http.Client{Timeout: 10 * time.Second},
		onAction: onAction,
		stopChan: make(chan struct{}),
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

// ParseTelegramAction parses "approve", "reject", or "approve:<paneID>" / "reject:<paneID>".
func ParseTelegramAction(raw string) (paneID, action string, ok bool) {
	if raw == "approve" || raw == "reject" {
		return "", raw, true
	}
	if strings.HasPrefix(raw, "approve:") {
		return strings.TrimPrefix(raw, "approve:"), "approve", true
	}
	if strings.HasPrefix(raw, "reject:") {
		return strings.TrimPrefix(raw, "reject:"), "reject", true
	}
	return "", "", false
}

// Close stops the background update poller.
func (t *TelegramNotifier) Close() {
	t.stopOnce.Do(func() {
		close(t.stopChan)
	})
}

// Notify sends an alert message with inline buttons to the configured Telegram chat.
func (t *TelegramNotifier) Notify(event NotificationEvent) error {
	if t.Token == "" || t.ChatID == "" {
		return nil
	}

	body := event.Body
	if len(body) > 3000 {
		body = body[len(body)-3000:]
	}

	messageText := fmt.Sprintf("🛡️ <b>Guaita Sentinel Alert</b>\n\nAgent in pane <code>%s</code> is paused awaiting human approval:\n\n<pre>%s</pre>",
		html.EscapeString(event.PaneID),
		html.EscapeString(body),
	)

	approveData := "approve"
	rejectData := "reject"
	if event.PaneID != "" && len("approve:"+event.PaneID) <= 64 {
		approveData = "approve:" + event.PaneID
		rejectData = "reject:" + event.PaneID
	}

	keyboard := [][]map[string]string{
		{
			{"text": "✓ APPROVE (Y)", "callback_data": approveData},
			{"text": "✕ REJECT (N)", "callback_data": rejectData},
		},
	}

	payload := map[string]interface{}{
		"chat_id":    t.ChatID,
		"text":       messageText,
		"parse_mode": "HTML",
		"reply_markup": map[string]interface{}{
			"inline_keyboard": keyboard,
		},
	}

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

		url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=15", t.Token, offset)
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
					t.onAction(u.CallbackQuery.Data)
					if paneID != "" {
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
