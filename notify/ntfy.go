package notify

import (
	"fmt"
	"net/http"
	"net/url"
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

// stripTokenParam removes any sensitive token query parameter before sending URLs over ntfy.
func stripTokenParam(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Del("token")
	u.RawQuery = q.Encode()
	return u.String()
}

// Notify publishes an actionable notification to the ntfy topic.
func (n *NtfyNotifier) Notify(event NotificationEvent) error {
	if n.Topic == "" {
		return nil
	}

	// Never leak raw terminal output, code, or potential secrets to the public ntfy.sh relay.
	body := event.Body
	if strings.EqualFold(n.ServerURL, "https://ntfy.sh") || strings.EqualFold(n.ServerURL, "http://ntfy.sh") {
		if event.PaneID != "" {
			body = fmt.Sprintf("Agent paused in pane %s waiting for human approval. Open Guaita to inspect and respond.", event.PaneID)
		} else {
			body = "Agent paused waiting for human approval. Open Guaita to inspect and respond."
		}
	}

	endpoint := fmt.Sprintf("%s/%s", n.ServerURL, n.Topic)
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Title", event.Title)
	req.Header.Set("Priority", "high")
	req.Header.Set("Tags", "warning,robot")

	// Construct interactive actions for iOS notification banner without leaking session tokens.
	actions := make([]string, 0)
	if safeApprove := stripTokenParam(event.ActionApproveURL); safeApprove != "" {
		actions = append(actions, fmt.Sprintf("http, ✓ Approve (Y), %s, method=POST", safeApprove))
	}
	if safeReject := stripTokenParam(event.ActionRejectURL); safeReject != "" {
		actions = append(actions, fmt.Sprintf("http, ✕ Reject (N), %s, method=POST", safeReject))
	}
	if safePairing := stripTokenParam(event.PairingURL); safePairing != "" {
		actions = append(actions, fmt.Sprintf("view, 📱 Open Guaita, %s", safePairing))
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
