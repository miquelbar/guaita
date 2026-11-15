package herdr

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// AgentStatus represents the state of the monitored AI agent via Herdr.
type AgentStatus struct {
	Output      string `json:"output"`
	Waiting     bool   `json:"waiting"`
	PaneID      string `json:"pane_id,omitempty"`
	AgentState  string `json:"agent_state,omitempty"`
	Title       string `json:"title,omitempty"`
	HerdrOnline bool   `json:"herdr_online"`
}

var (
	execCommand = exec.Command
	activePaneMu sync.RWMutex
	lastActivePane string
)

// FindHerdrBinary locates the Herdr CLI binary on the machine.
func FindHerdrBinary() string {
	if p, err := exec.LookPath("herdr"); err == nil {
		return p
	}
	if p, err := exec.LookPath("herdr-core"); err == nil {
		return p
	}

	home, err := os.UserHomeDir()
	if err == nil {
		userLocal := filepath.Join(home, ".local", "bin", "herdr")
		if _, err := os.Stat(userLocal); err == nil {
			return userLocal
		}
	}

	standardLocations := []string{
		"/usr/local/bin/herdr",
		"/opt/homebrew/bin/herdr",
		"/usr/bin/herdr",
	}
	for _, path := range standardLocations {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return "herdr"
}

type herdrStatusOutput struct {
	Server struct {
		Status  string `json:"status"`
		Running bool   `json:"running"`
		Socket  string `json:"socket"`
	} `json:"server"`
}

type paneListOutput struct {
	Result struct {
		Panes []struct {
			PaneID        string `json:"pane_id"`
			AgentStatus   string `json:"agent_status"`
			Focused       bool   `json:"focused"`
			TerminalTitle string `json:"terminal_title"`
		} `json:"panes"`
	} `json:"result"`
}

var promptRegex = regexp.MustCompile(`(?i)(\([yY]/[nN]\)|\[[yY]/[nN]\]|\[[yY]\]es|approve|proceed\?|are you sure|press enter)`)

// GetStatus checks the current status of Herdr or returns a defensive fallback when inactive.
func GetStatus() (AgentStatus, error) {
	bin := FindHerdrBinary()

	// 1. Check Herdr server status
	cmdStatus := execCommand(bin, "status", "--json")
	statusRaw, err := cmdStatus.Output()
	if err != nil {
		// Fallback check for legacy herdr-core format
		var legacyStatus AgentStatus
		if jsonErr := json.Unmarshal(statusRaw, &legacyStatus); jsonErr == nil && legacyStatus.Output != "" {
			legacyStatus.HerdrOnline = true
			return legacyStatus, nil
		}
		return AgentStatus{
			Output:      "⚡ Herdr offline or inactive",
			Waiting:     false,
			HerdrOnline: false,
		}, err
	}

	var statusData herdrStatusOutput
	if err := json.Unmarshal(statusRaw, &statusData); err != nil {
		// Try legacy format
		var legacyStatus AgentStatus
		if jsonErr := json.Unmarshal(statusRaw, &legacyStatus); jsonErr == nil && legacyStatus.Output != "" {
			legacyStatus.HerdrOnline = true
			return legacyStatus, nil
		}
	}

	// 2. Discover active panes
	cmdPanes := execCommand(bin, "pane", "list")
	panesRaw, err := cmdPanes.Output()
	if err != nil {
		return AgentStatus{
			Output:      "⚡ Herdr server running (no active panes found)",
			Waiting:     false,
			HerdrOnline: true,
		}, nil
	}

	var panesData paneListOutput
	if err := json.Unmarshal(panesRaw, &panesData); err != nil || len(panesData.Result.Panes) == 0 {
		return AgentStatus{
			Output:      "⚡ Herdr server running (no active panes)",
			Waiting:     false,
			HerdrOnline: true,
		}, nil
	}

	// Select focused pane or first available pane
	selectedPane := panesData.Result.Panes[0]
	for _, p := range panesData.Result.Panes {
		if p.Focused {
			selectedPane = p
			break
		}
	}

	activePaneMu.Lock()
	lastActivePane = selectedPane.PaneID
	activePaneMu.Unlock()

	// 3. Read pane output
	cmdRead := execCommand(bin, "pane", "read", selectedPane.PaneID, "--source", "recent-unwrapped", "--lines", "80")
	readRaw, _ := cmdRead.Output()
	outputText := string(readRaw)
	if strings.TrimSpace(outputText) == "" {
		outputText = "Terminal active (waiting for agent activity in pane " + selectedPane.PaneID + ")"
	}

	// Herdr reports "blocked" when it explicitly detects an approval or question prompt
	isWaiting := selectedPane.AgentStatus == "blocked"
	if !isWaiting && promptRegex.MatchString(outputText) {
		isWaiting = true
	}

	return AgentStatus{
		Output:      outputText,
		Waiting:     isWaiting,
		PaneID:      selectedPane.PaneID,
		AgentState:  selectedPane.AgentStatus,
		Title:       selectedPane.TerminalTitle,
		HerdrOnline: true,
	}, nil
}

// SendAction dispatches an action (approve, reject, enter, or text input) to herdr.
func SendAction(action string) error {
	bin := FindHerdrBinary()

	activePaneMu.RLock()
	paneID := lastActivePane
	activePaneMu.RUnlock()

	if paneID == "" {
		paneID = "w1:p1"
	}

	cleanAction := strings.TrimSpace(action)
	switch strings.ToLower(cleanAction) {
	case "approve", "y", "yes":
		return execCommand(bin, "pane", "send-keys", paneID, "y", "enter").Run()
	case "reject", "n", "no":
		return execCommand(bin, "pane", "send-keys", paneID, "n", "enter").Run()
	case "enter":
		return execCommand(bin, "pane", "send-keys", paneID, "enter").Run()
	case "esc", "escape":
		return execCommand(bin, "pane", "send-keys", paneID, "esc").Run()
	default:
		// Send literal text followed by enter
		return execCommand(bin, "pane", "send-text", paneID, cleanAction+"\n").Run()
	}
}
