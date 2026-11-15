package herdr

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
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
	execCommand       = exec.Command
	lookPath          = exec.LookPath
	activePaneMu      sync.RWMutex
	lastActivePane    string
	lastActiveIsAgent bool
	activeBackend     = "herdr"
)

// FindHerdrBinary locates the Herdr CLI binary on the machine.
func FindHerdrBinary() string {
	if p, err := lookPath("herdr"); err == nil {
		return p
	}
	if p, err := lookPath("herdr-core"); err == nil {
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

type agentListOutput struct {
	Result struct {
		Agents []struct {
			Agent         string `json:"agent"`
			AgentStatus   string `json:"agent_status"`
			Focused       bool   `json:"focused"`
			PaneID        string `json:"pane_id"`
			TerminalTitle string `json:"terminal_title"`
		} `json:"agents"`
	} `json:"result"`
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

var promptRegex = regexp.MustCompile(`(?i)(\([yY]/[nN]\)|\[[yY]/[nN]\]|\[[yY]\]es|proceed\?|are you sure\??|press enter|enter to confirm|esc to cancel|allow once|allow always|allow this|yes,\s*allow|do you want to (run|execute|allow|create|edit|proceed))`)

func lastNonEmptyLines(text string, maxLines int) string {
	rawLines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	collected := make([]string, 0, maxLines)
	for i := len(rawLines) - 1; i >= 0 && len(collected) < maxLines; i-- {
		trimmed := strings.TrimSpace(rawLines[i])
		if trimmed != "" {
			collected = append(collected, trimmed)
		}
	}
	return strings.Join(collected, "\n")
}

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
		// Fallback to tmux if Herdr is unavailable
		if tmuxStatus, ok := getTmuxStatus(); ok {
			return tmuxStatus, nil
		}
		return AgentStatus{
			Output:      "⚡ Herdr / tmux offline or inactive",
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

	// 2. Prefer native `herdr agent list` for direct agent telemetry (agy, claude, etc.)
	var selectedID, selectedAgentStatus, selectedTitle string
	var isAgent bool

	if agentsRaw, agentErr := execCommand(bin, "agent", "list").Output(); agentErr == nil {
		var agentsData agentListOutput
		if json.Unmarshal(agentsRaw, &agentsData) == nil && len(agentsData.Result.Agents) > 0 {
			selected := agentsData.Result.Agents[0]
			for _, a := range agentsData.Result.Agents {
				if a.Focused {
					selected = a
					break
				}
			}
			for _, a := range agentsData.Result.Agents {
				if a.AgentStatus == "blocked" {
					selected = a
					break
				}
			}
			selectedID = selected.PaneID
			selectedAgentStatus = selected.AgentStatus
			selectedTitle = selected.TerminalTitle
			isAgent = true
		}
	}

	// 2b. Fallback to `herdr pane list` if no active agents were returned
	if selectedID == "" {
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

		selectedPane := panesData.Result.Panes[0]
		for _, p := range panesData.Result.Panes {
			if p.Focused {
				selectedPane = p
				break
			}
		}
		for _, p := range panesData.Result.Panes {
			if p.AgentStatus == "blocked" {
				selectedPane = p
				break
			}
		}
		selectedID = selectedPane.PaneID
		selectedAgentStatus = selectedPane.AgentStatus
		selectedTitle = selectedPane.TerminalTitle
	}

	activePaneMu.Lock()
	lastActivePane = selectedID
	lastActiveIsAgent = isAgent
	activeBackend = "herdr"
	activePaneMu.Unlock()

	// 3. Read pane output
	cmdRead := execCommand(bin, "pane", "read", selectedID, "--source", "recent-unwrapped", "--lines", "80")
	readRaw, _ := cmdRead.Output()
	outputText := string(readRaw)
	if strings.TrimSpace(outputText) == "" {
		outputText = "Terminal active (waiting for agent activity in pane " + selectedID + ")"
	}

	// Herdr reports "blocked" when it explicitly detects an approval or question prompt.
	// Fallback regex inspects the last 6 non-empty lines so TUI option dialogs (question + options + footer) are caught.
	isWaiting := selectedAgentStatus == "blocked"
	if !isWaiting && promptRegex.MatchString(lastNonEmptyLines(outputText, 6)) {
		isWaiting = true
	}

	return AgentStatus{
		Output:      outputText,
		Waiting:     isWaiting,
		PaneID:      selectedID,
		AgentState:  selectedAgentStatus,
		Title:       selectedTitle,
		HerdrOnline: true,
	}, nil
}

// SendAction dispatches an action (approve, reject, enter, or text input) to Herdr or tmux.
func SendAction(action string) error {
	activePaneMu.RLock()
	paneID := lastActivePane
	isAgent := lastActiveIsAgent
	backend := activeBackend
	activePaneMu.RUnlock()

	cleanAction := strings.TrimSpace(action)
	if backend == "tmux" {
		return sendTmuxAction(paneID, cleanAction)
	}

	bin := FindHerdrBinary()
	if paneID == "" {
		paneID = "w1:p1"
	}

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
		// Prefer native `herdr agent prompt` for AI agent panes so TUI apps submit instead of inserting a newline.
		if isAgent {
			if err := execCommand(bin, "agent", "prompt", paneID, cleanAction).Run(); err == nil {
				return nil
			}
		}
		// Fallback: send literal text WITHOUT trailing '\n', then press Enter key explicitly.
		if err := execCommand(bin, "pane", "send-text", paneID, cleanAction).Run(); err != nil {
			return err
		}
		return execCommand(bin, "pane", "send-keys", paneID, "enter").Run()
	}
}

// WatchAgentBlocked listens on `herdr agent wait <pane> --until blocked` for instant event-driven notifications.
func WatchAgentBlocked(stop <-chan struct{}, onBlocked func()) {
	for {
		select {
		case <-stop:
			return
		default:
		}

		activePaneMu.RLock()
		paneID := lastActivePane
		isAgent := lastActiveIsAgent
		backend := activeBackend
		activePaneMu.RUnlock()

		if backend != "herdr" || !isAgent || paneID == "" {
			select {
			case <-stop:
				return
			case <-time.After(1500 * time.Millisecond):
			}
			continue
		}

		bin := FindHerdrBinary()
		err := execCommand(bin, "agent", "wait", paneID, "--until", "blocked", "--timeout", "15000").Run()
		if err == nil && onBlocked != nil {
			onBlocked()
		}

		select {
		case <-stop:
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func getTmuxStatus() (AgentStatus, bool) {
	tmuxBin, err := lookPath("tmux")
	if err != nil {
		return AgentStatus{}, false
	}

	out, err := execCommand(tmuxBin, "list-panes", "-a", "-F", "#{pane_id}|#{pane_active}|#{session_attached}|#{pane_title}").Output()
	if err != nil {
		return AgentStatus{}, false
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var selectedID, selectedTitle string
	for _, line := range lines {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 4)
		if len(parts) < 3 || parts[0] == "" {
			continue
		}
		if selectedID == "" {
			selectedID = parts[0]
			if len(parts) == 4 {
				selectedTitle = parts[3]
			}
		}
		if parts[1] == "1" && parts[2] == "1" {
			selectedID = parts[0]
			if len(parts) == 4 {
				selectedTitle = parts[3]
			}
			break
		}
	}
	if selectedID == "" {
		return AgentStatus{}, false
	}

	activePaneMu.Lock()
	lastActivePane = selectedID
	activeBackend = "tmux"
	activePaneMu.Unlock()

	readRaw, err := execCommand(tmuxBin, "capture-pane", "-p", "-t", selectedID, "-S", "-80").Output()
	if err != nil {
		return AgentStatus{}, false
	}

	outputText := string(readRaw)
	if strings.TrimSpace(outputText) == "" {
		outputText = "Terminal active (waiting for activity in tmux pane " + selectedID + ")"
	}

	isWaiting := promptRegex.MatchString(lastNonEmptyLines(outputText, 2))
	return AgentStatus{
		Output:      outputText,
		Waiting:     isWaiting,
		PaneID:      selectedID,
		AgentState:  "tmux",
		Title:       selectedTitle,
		HerdrOnline: true,
	}, true
}

func sendTmuxAction(paneID, cleanAction string) error {
	tmuxBin, err := lookPath("tmux")
	if err != nil {
		tmuxBin = "tmux"
	}
	if paneID == "" {
		paneID = "0"
	}

	switch strings.ToLower(cleanAction) {
	case "approve", "y", "yes":
		return execCommand(tmuxBin, "send-keys", "-t", paneID, "y", "Enter").Run()
	case "reject", "n", "no":
		return execCommand(tmuxBin, "send-keys", "-t", paneID, "n", "Enter").Run()
	case "enter":
		return execCommand(tmuxBin, "send-keys", "-t", paneID, "Enter").Run()
	case "esc", "escape":
		return execCommand(tmuxBin, "send-keys", "-t", paneID, "Escape").Run()
	default:
		if err := execCommand(tmuxBin, "send-keys", "-t", paneID, "-l", cleanAction).Run(); err != nil {
			return err
		}
		return execCommand(tmuxBin, "send-keys", "-t", paneID, "Enter").Run()
	}
}
