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

// PaneSummary describes an active Herdr or tmux pane for multi-session selection.
type PaneSummary struct {
	PaneID     string `json:"pane_id"`
	Title      string `json:"title,omitempty"`
	Agent      string `json:"agent,omitempty"`
	AgentState string `json:"agent_state,omitempty"`
	Waiting    bool   `json:"waiting"`
	Focused    bool   `json:"focused,omitempty"`
	Backend    string `json:"backend,omitempty"`
}

// AgentStatus represents the state of the monitored AI agent via Herdr or tmux.
type AgentStatus struct {
	Output      string        `json:"output"`
	Waiting     bool          `json:"waiting"`
	PaneID      string        `json:"pane_id,omitempty"`
	AgentState  string        `json:"agent_state,omitempty"`
	Title       string        `json:"title,omitempty"`
	HerdrOnline bool          `json:"herdr_online"`
	Panes       []PaneSummary `json:"panes,omitempty"`
}

type paneMeta struct {
	isAgent bool
	backend string
}

var (
	execCommand       = exec.Command
	lookPath          = exec.LookPath
	activePaneMu      sync.RWMutex
	lastActivePane    string
	lastActiveIsAgent bool
	activeBackend     = "herdr"
	knownPanes        = make(map[string]paneMeta)
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
	status, _, err := GetStatusWithPanes("")
	return status, err
}

// GetStatusWithPanes returns the selected pane's status, a list of all waiting panes (for multi-session alerts),
// and populates status.Panes with every detected Herdr or tmux pane.
func GetStatusWithPanes(preferredPane string) (AgentStatus, []AgentStatus, error) {
	bin := FindHerdrBinary()

	// 1. Check Herdr server status
	cmdStatus := execCommand(bin, "status", "--json")
	statusRaw, err := cmdStatus.Output()
	if err != nil {
		// Fallback check for legacy herdr-core format
		var legacyStatus AgentStatus
		if jsonErr := json.Unmarshal(statusRaw, &legacyStatus); jsonErr == nil && legacyStatus.Output != "" {
			legacyStatus.HerdrOnline = true
			return legacyStatus, nil, nil
		}
		// Fallback to tmux if Herdr is unavailable
		if tmuxStatus, tmuxWaiting, ok := getTmuxStatus(preferredPane); ok {
			return tmuxStatus, tmuxWaiting, nil
		}
		return AgentStatus{
			Output:      "⚡ Herdr / tmux offline or inactive",
			Waiting:     false,
			HerdrOnline: false,
		}, nil, err
	}

	var statusData herdrStatusOutput
	if err := json.Unmarshal(statusRaw, &statusData); err != nil {
		// Try legacy format
		var legacyStatus AgentStatus
		if jsonErr := json.Unmarshal(statusRaw, &legacyStatus); jsonErr == nil && legacyStatus.Output != "" {
			legacyStatus.HerdrOnline = true
			return legacyStatus, nil, nil
		}
	}

	// 2. Prefer native `herdr agent list` for direct agent telemetry (agy, claude, etc.)
	panes := make([]PaneSummary, 0)
	metaMap := make(map[string]paneMeta)

	if agentsRaw, agentErr := execCommand(bin, "agent", "list").Output(); agentErr == nil {
		var agentsData agentListOutput
		if json.Unmarshal(agentsRaw, &agentsData) == nil && len(agentsData.Result.Agents) > 0 {
			for _, a := range agentsData.Result.Agents {
				title := a.TerminalTitle
				if title == "" {
					title = a.Agent
				}
				panes = append(panes, PaneSummary{
					PaneID:     a.PaneID,
					Title:      title,
					Agent:      a.Agent,
					AgentState: a.AgentStatus,
					Waiting:    a.AgentStatus == "blocked",
					Focused:    a.Focused,
					Backend:    "herdr",
				})
				metaMap[a.PaneID] = paneMeta{isAgent: true, backend: "herdr"}
			}
		}
	}

	// 2b. Fallback to `herdr pane list` if no active agents were returned
	if len(panes) == 0 {
		cmdPanes := execCommand(bin, "pane", "list")
		panesRaw, err := cmdPanes.Output()
		if err != nil {
			return AgentStatus{
				Output:      "⚡ Herdr server running (no active panes found)",
				Waiting:     false,
				HerdrOnline: true,
			}, nil, nil
		}

		var panesData paneListOutput
		if err := json.Unmarshal(panesRaw, &panesData); err != nil || len(panesData.Result.Panes) == 0 {
			return AgentStatus{
				Output:      "⚡ Herdr server running (no active panes)",
				Waiting:     false,
				HerdrOnline: true,
			}, nil, nil
		}

		for _, p := range panesData.Result.Panes {
			panes = append(panes, PaneSummary{
				PaneID:     p.PaneID,
				Title:      p.TerminalTitle,
				AgentState: p.AgentStatus,
				Waiting:    p.AgentStatus == "blocked",
				Focused:    p.Focused,
				Backend:    "herdr",
			})
			metaMap[p.PaneID] = paneMeta{isAgent: false, backend: "herdr"}
		}
	}

	// Pick selected pane: preferredPane -> first waiting/blocked -> focused -> first
	selectedIdx := 0
	foundPreferred := false
	if preferredPane != "" {
		for i, p := range panes {
			if p.PaneID == preferredPane {
				selectedIdx = i
				foundPreferred = true
				break
			}
		}
	}
	if !foundPreferred {
		for i, p := range panes {
			if p.Focused {
				selectedIdx = i
				break
			}
		}
		for i, p := range panes {
			if p.Waiting {
				selectedIdx = i
				break
			}
		}
	}

	selected := panes[selectedIdx]
	selectedMeta := metaMap[selected.PaneID]

	activePaneMu.Lock()
	lastActivePane = selected.PaneID
	lastActiveIsAgent = selectedMeta.isAgent
	activeBackend = "herdr"
	for k, v := range metaMap {
		knownPanes[k] = v
	}
	activePaneMu.Unlock()

	readPane := func(id string) string {
		raw, _ := execCommand(bin, "pane", "read", id, "--source", "recent-unwrapped", "--lines", "80").Output()
		return string(raw)
	}

	// 3. Read selected pane output and any other waiting/blocked panes for independent alerts
	outputText := readPane(selected.PaneID)
	if strings.TrimSpace(outputText) == "" {
		outputText = "Terminal active (waiting for agent activity in pane " + selected.PaneID + ")"
	}

	isWaiting := selected.Waiting
	if !isWaiting && promptRegex.MatchString(lastNonEmptyLines(outputText, 6)) {
		isWaiting = true
		panes[selectedIdx].Waiting = true
	}

	waitingStatuses := make([]AgentStatus, 0)
	for i := range panes {
		p := panes[i]
		if !p.Waiting {
			continue
		}
		pOut := outputText
		if p.PaneID != selected.PaneID {
			pOut = readPane(p.PaneID)
		}
		waitingStatuses = append(waitingStatuses, AgentStatus{
			Output:      pOut,
			Waiting:     true,
			PaneID:      p.PaneID,
			AgentState:  p.AgentState,
			Title:       p.Title,
			HerdrOnline: true,
		})
	}

	return AgentStatus{
		Output:      outputText,
		Waiting:     isWaiting,
		PaneID:      selected.PaneID,
		AgentState:  selected.AgentState,
		Title:       selected.Title,
		HerdrOnline: true,
		Panes:       panes,
	}, waitingStatuses, nil
}

// SendAction dispatches an action (approve, reject, enter, or text input) to the currently active pane.
func SendAction(action string) error {
	activePaneMu.RLock()
	paneID := lastActivePane
	activePaneMu.RUnlock()
	return SendActionToPane(paneID, action)
}

// SendActionToPane dispatches an action to a specific Herdr or tmux paneID.
func SendActionToPane(paneID, action string) error {
	activePaneMu.RLock()
	if paneID == "" {
		paneID = lastActivePane
	}
	meta, ok := knownPanes[paneID]
	isAgent := lastActiveIsAgent
	backend := activeBackend
	if ok {
		isAgent = meta.isAgent
		backend = meta.backend
	}
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

func getTmuxStatus(preferredPane string) (AgentStatus, []AgentStatus, bool) {
	tmuxBin, err := lookPath("tmux")
	if err != nil {
		return AgentStatus{}, nil, false
	}

	out, err := execCommand(tmuxBin, "list-panes", "-a", "-F", "#{pane_id}|#{pane_active}|#{session_attached}|#{session_name}:#{window_index}.#{pane_index} #{pane_title}").Output()
	if err != nil {
		return AgentStatus{}, nil, false
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	panes := make([]PaneSummary, 0, len(lines))
	outputs := make(map[string]string, len(lines))
	waitingStatuses := make([]AgentStatus, 0)

	for _, line := range lines {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 4)
		if len(parts) < 3 || parts[0] == "" {
			continue
		}
		id := parts[0]
		focused := parts[1] == "1" && parts[2] == "1"
		title := ""
		if len(parts) == 4 {
			title = strings.TrimSpace(parts[3])
		}

		readRaw, capErr := execCommand(tmuxBin, "capture-pane", "-p", "-t", id, "-S", "-80").Output()
		if capErr != nil {
			continue
		}
		outputText := string(readRaw)
		if strings.TrimSpace(outputText) == "" {
			outputText = "Terminal active (waiting for activity in tmux pane " + id + ")"
		}
		isWaiting := promptRegex.MatchString(lastNonEmptyLines(outputText, 6))
		state := "tmux"

		panes = append(panes, PaneSummary{
			PaneID:     id,
			Title:      title,
			AgentState: state,
			Waiting:    isWaiting,
			Focused:    focused,
			Backend:    "tmux",
		})
		outputs[id] = outputText

		if isWaiting {
			waitingStatuses = append(waitingStatuses, AgentStatus{
				Output:      outputText,
				Waiting:     true,
				PaneID:      id,
				AgentState:  state,
				Title:       title,
				HerdrOnline: true,
			})
		}
	}

	if len(panes) == 0 {
		return AgentStatus{}, nil, false
	}

	selectedIdx := 0
	foundPreferred := false
	if preferredPane != "" {
		for i, p := range panes {
			if p.PaneID == preferredPane {
				selectedIdx = i
				foundPreferred = true
				break
			}
		}
	}
	if !foundPreferred {
		for i, p := range panes {
			if p.Focused {
				selectedIdx = i
				break
			}
		}
		for i, p := range panes {
			if p.Waiting {
				selectedIdx = i
				break
			}
		}
	}

	selected := panes[selectedIdx]

	activePaneMu.Lock()
	lastActivePane = selected.PaneID
	activeBackend = "tmux"
	for _, p := range panes {
		knownPanes[p.PaneID] = paneMeta{isAgent: false, backend: "tmux"}
	}
	activePaneMu.Unlock()

	return AgentStatus{
		Output:      outputs[selected.PaneID],
		Waiting:     selected.Waiting,
		PaneID:      selected.PaneID,
		AgentState:  selected.AgentState,
		Title:       selected.Title,
		HerdrOnline: true,
		Panes:       panes,
	}, waitingStatuses, true
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
