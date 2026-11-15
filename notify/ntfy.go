package notify

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// NtfyNotifier implements push notifications via ntfy.sh or self-hosted ntfy servers.
type NtfyNotifier struct {
	ServerURL string
	Topic     string
	Client    *http.Client
}

// NewNtfy creates a configured NtfyNotifier.
func NewNtfy(serverURL, topic string) *NtfyNotifier {
	if serverURL == "" {
		serverURL = "https://ntfy.sh"
	}
	serverURL = strings.TrimSuffix(serverURL, "/")

	return &NtfyNotifier{
		ServerURL: serverURL,
		Topic:     topic,
		Client:    &http.Client{Timeout: 5 * time.Second},
	}
}

// Name returns the channel identifier.
func (n *NtfyNotifier) Name() string {
	return "ntfy"
}

// TopicURL returns the public subscription URL.
func (n *NtfyNotifier) TopicURL() string {
	return fmt.Sprintf("%s/%s", n.ServerURL, n.Topic)
}

// Notify publishes an actionable notification to the ntfy topic.
func (n *NtfyNotifier) Notify(event NotificationEvent) error {
	if n.Topic == "" {
		return nil
	}

	endpoint := fmt.Sprintf("%s/%s", n.ServerURL, n.Topic)
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(event.Body))
	if err != nil {
		return err
	}

	req.Header.Set("Title", event.Title)
	req.Header.Set("Priority", "high")
	req.Header.Set("Tags", "warning,robot")

	// Construct interactive actions for iOS notification banner
	actions := make([]string, 0)
	if event.ActionApproveURL != "" {
		actions = append(actions, fmt.Sprintf("http, ✓ Approve (Y), %s, method=POST", event.ActionApproveURL))
	}
	if event.ActionRejectURL != "" {
		actions = append(actions, fmt.Sprintf("http, ✕ Reject (N), %s, method=POST", event.ActionRejectURL))
	}
	if event.PairingURL != "" {
		actions = append(actions, fmt.Sprintf("view, 📱 Open Guaita, %s", event.PairingURL))
	}

	if len(actions) > 0 {
		req.Header.Set("Actions", strings.Join(actions, "; "))
	}

	resp, err := n.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy server returned HTTP %d", resp.StatusCode)
	}

	return nil
}
