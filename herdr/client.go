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

// PromptOption represents a selectable choice extracted from an interactive CLI menu or prompt.
type PromptOption struct {
	Key   string `json:"key"`             // Value to dispatch when clicked: e.g. "1", "2", "p", "esc"
	Label string `json:"label"`           // Clean human-readable button text: e.g. "1. Yes, proceed (y)"
	Style string `json:"style,omitempty"` // "approve", "always", "reject", "secondary"
}

// PaneSummary describes an active Herdr or tmux pane for multi-session selection.
type PaneSummary struct {
	PaneID        string         `json:"pane_id"`
	Title         string         `json:"title,omitempty"`
	Agent         string         `json:"agent,omitempty"`
	AgentState    string         `json:"agent_state,omitempty"`
	Waiting       bool           `json:"waiting"`
	PromptType    string         `json:"prompt_type,omitempty"`
	PromptText    string         `json:"prompt_text,omitempty"`
	PromptOptions []PromptOption `json:"prompt_options,omitempty"`
	Focused       bool           `json:"focused,omitempty"`
	Backend       string         `json:"backend,omitempty"`
}

// AgentStatus represents the state of the monitored AI agent via Herdr or tmux.
type AgentStatus struct {
	Output        string         `json:"output"`
	Waiting       bool           `json:"waiting"`
	PromptType    string         `json:"prompt_type,omitempty"`
	PromptText    string         `json:"prompt_text,omitempty"`
	PromptOptions []PromptOption `json:"prompt_options,omitempty"`
	PaneID        string         `json:"pane_id,omitempty"`
	AgentState    string         `json:"agent_state,omitempty"`
	Title         string         `json:"title,omitempty"`
	HerdrOnline   bool           `json:"herdr_online"`
	Panes         []PaneSummary  `json:"panes,omitempty"`
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

var (
	promptRegex              = regexp.MustCompile(`(?i)(\([yY]/[nN]\)|\[[yY]/[nN]\]|\([yY]es/[nN]o\)|\[[yY]es/[nN]o\]|\[[yY]\]es|proceed\?|are you sure\??|press enter|enter to confirm|esc to cancel|allow once|allow always|always allow|allow this|yes,\s*allow|do you want to (run|execute|allow|create|edit|delete|proceed)|approval required)`)
	yesNoChoiceRegex         = regexp.MustCompile(`(?i)(\([yY]/[nN]\)|\[[yY]/[nN]\]|\([yY]es/[nN]o\)|\[[yY]es/[nN]o\]|\[[yY]\]es|proceed\?|are you sure\??|allow once|allow always|always allow|allow this|yes,\s*allow|do you want to (run|execute|allow|create|edit|delete|proceed)|approval required|\b[123]\.\s*(yes|no|allow|always allow|approve|reject|confirm|cancel)\b)`)
	numberedMenuOptionRegex  = regexp.MustCompile(`^[>❯›»*]?\s*\d+\.\s+\S+`)
	numberedOptionLineRegex  = regexp.MustCompile(`^[>❯›»*•\-]?\s*(?:\[(\d+)\]|(\d+)[\.\)\:\-])\s+(.+)$`)
	letteredOptionLineRegex  = regexp.MustCompile(`^[>❯›»*•\-]?\s*(?:\[([a-zA-Z])\]|\(([a-zA-Z])\)|([a-zA-Z])[\.\)\:\-])\s+(.+)$`)
	trailingHotkeyRegex      = regexp.MustCompile(`\s*[\(\[]([a-zA-Z0-9]|esc|escape|enter)[\)\]]\s*$`)
	footerHintRegex          = regexp.MustCompile(`(?i)(press enter|enter to confirm|esc to cancel|use arrow keys|navigate with|arrow keys|\?\s+for\s+shortcuts|shift\+tab)`)
	inlineSlashChoiceRegex   = regexp.MustCompile(`(?i)[\[\(]([a-z0-9]+(?:/[a-z0-9]+)+)[\]\)]`)
	inputPromptRegex         = regexp.MustCompile(`(?i)(^(please\s+|por\s+favor\s+)?(enter|type|input|specify|provide|write|escribe|introduce|indica|ingresa|especifica)\b[^.!?\n]{2,90}[:?]\s*$|\b(enter|type|input|specify|provide|write|escribe|introduce|indica|ingresa|especifica)\s+(your|the|a|an|tu|el|la|los|las|un|una)\s+[^.!?\n]{2,70}[:?]\s*$)`)
	shellPromptRegex         = regexp.MustCompile(`(\S+@\S+.*[%$#]\s*$|^[\w.-]+:\S*\s*[%$#]\s*$|^bash-[0-9.]+[$#]\s*$)`)
	tuiBorderRegex           = regexp.MustCompile(`^[\s─━═╌╍┄┅┈┉╭╮╯╰┌┐┘└├┤┬┴┼│┃║\-_=*]+$`)
	tuiInputPlaceholderRegex = regexp.MustCompile(`(?i)^[│┃║]?\s*[>❯›»]\s*(type a message.*|reply.*|send a message.*|ask anything.*|enter a prompt.*)?\s*[│┃║]?$`)
	tuiFooterHintRegex       = regexp.MustCompile(`(?i)^\s*(\?\s+for\s+shortcuts|shift\+tab\b|ctrl\+[cd]\s+to\b|esc\s+to\s+interrupt\b|context:\s*\d+%|tokens:\s*\d+)`)
)

func isTUIChromeLine(line string) bool {
	if line == "" {
		return true
	}
	return tuiBorderRegex.MatchString(line) ||
		tuiInputPlaceholderRegex.MatchString(line) ||
		tuiFooterHintRegex.MatchString(line)
}

func lastNonEmptyLines(text string, maxLines int) string {
	rawLines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	collected := make([]string, 0, maxLines)
	for i := len(rawLines) - 1; i >= 0 && len(collected) < maxLines; i-- {
		trimmed := strings.TrimSpace(rawLines[i])
		if trimmed != "" {
			collected = append(collected, trimmed)
		}
	}
	for i, j := 0, len(collected)-1; i < j; i, j = i+1, j-1 {
		collected[i], collected[j] = collected[j], collected[i]
	}
	return strings.Join(collected, "\n")
}

func tailSliceJoin(lines []string, maxLines int) string {
	if len(lines) == 0 || maxLines <= 0 {
		return ""
	}
	start := len(lines) - maxLines
	if start < 0 {
		start = 0
	}
	return strings.Join(lines[start:], "\n")
}

func isQuestionLine(line string) bool {
	cleaned := strings.TrimSpace(strings.TrimRight(line, "\"'”’)*_` "))
	if len(cleaned) < 4 || !strings.HasSuffix(cleaned, "?") {
		return false
	}
	lower := strings.ToLower(cleaned)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return false
	}
	return true
}

func extractQuestionBlock(contentLines []string) string {
	if len(contentLines) == 0 {
		return ""
	}
	n := len(contentLines)
	start := n - 1
	// Include up to 2 preceding lines if they are part of a multi-line question or numbered options
	for start > 0 && (n-start) < 4 {
		prev := contentLines[start-1]
		if shellPromptRegex.MatchString(prev) {
			break
		}
		if numberedMenuOptionRegex.MatchString(contentLines[start]) ||
			numberedMenuOptionRegex.MatchString(prev) ||
			isQuestionLine(prev) ||
			(!strings.HasSuffix(prev, ".") && len(prev) > 20) {
			start--
		} else {
			break
		}
	}
	return strings.Join(contentLines[start:], "\n")
}

func detectOptionStyle(label string, key string) string {
	lower := strings.ToLower(label)
	if strings.Contains(lower, "always") ||
		strings.Contains(lower, "don't ask") ||
		strings.Contains(lower, "dont ask") ||
		strings.Contains(lower, "never ask") ||
		strings.Contains(lower, "remember") ||
		strings.Contains(lower, "all commands") ||
		strings.Contains(lower, "permanently") {
		return "always"
	}
	if strings.Contains(lower, "no") ||
		strings.Contains(lower, "reject") ||
		strings.Contains(lower, "deny") ||
		strings.Contains(lower, "cancel") ||
		strings.Contains(lower, "abort") ||
		strings.Contains(lower, "differently") ||
		strings.Contains(lower, "stop") ||
		strings.Contains(lower, "quit") ||
		strings.EqualFold(key, "esc") {
		return "reject"
	}
	if strings.Contains(lower, "yes") ||
		strings.Contains(lower, "approve") ||
		strings.Contains(lower, "proceed") ||
		strings.Contains(lower, "allow") ||
		strings.Contains(lower, "accept") ||
		strings.Contains(lower, "confirm") ||
		strings.Contains(lower, "run") ||
		strings.EqualFold(key, "y") {
		return "approve"
	}
	return "secondary"
}

func extractMenuOptions(contentLines []string) ([]PromptOption, int, int) {
	if len(contentLines) < 1 {
		return nil, 0, 0
	}

	endIdx := len(contentLines)
	for endIdx > 0 && footerHintRegex.MatchString(contentLines[endIdx-1]) {
		endIdx--
	}
	if endIdx < 1 {
		return nil, 0, 0
	}

	type rawOpt struct {
		key   string
		label string
	}

	// 1. Try numbered options (scanned backwards from endIdx - 1)
	collected := make([]rawOpt, 0)
	curr := endIdx - 1
	for curr >= 0 {
		line := contentLines[curr]
		if m := numberedOptionLineRegex.FindStringSubmatch(line); m != nil {
			num := m[1]
			if num == "" {
				num = m[2]
			}
			text := m[3]
			cleanLabel := num + ". " + text
			key := num
			if hk := trailingHotkeyRegex.FindStringSubmatch(text); hk != nil {
				hotkey := strings.ToLower(hk[1])
				if hotkey == "esc" || hotkey == "escape" {
					key = "esc"
				}
			}
			collected = append(collected, rawOpt{key: key, label: cleanLabel})
			curr--
		} else {
			break
		}
	}
	if len(collected) >= 2 {
		for i, j := 0, len(collected)-1; i < j; i, j = i+1, j-1 {
			collected[i], collected[j] = collected[j], collected[i]
		}
		opts := make([]PromptOption, len(collected))
		for i, r := range collected {
			opts[i] = PromptOption{
				Key:   r.key,
				Label: r.label,
				Style: detectOptionStyle(r.label, r.key),
			}
		}
		return opts, curr + 1, endIdx
	}

	// 2. Try lettered options: a) Yes b) No
	collected = make([]rawOpt, 0)
	curr = endIdx - 1
	for curr >= 0 {
		line := contentLines[curr]
		if m := letteredOptionLineRegex.FindStringSubmatch(line); m != nil {
			letter := m[1]
			if letter == "" {
				letter = m[2]
			}
			if letter == "" {
				letter = m[3]
			}
			text := m[4]
			cleanLabel := letter + ". " + text
			collected = append(collected, rawOpt{key: strings.ToLower(letter), label: cleanLabel})
			curr--
		} else {
			break
		}
	}
	if len(collected) >= 2 {
		for i, j := 0, len(collected)-1; i < j; i, j = i+1, j-1 {
			collected[i], collected[j] = collected[j], collected[i]
		}
		opts := make([]PromptOption, len(collected))
		for i, r := range collected {
			opts[i] = PromptOption{
				Key:   r.key,
				Label: r.label,
				Style: detectOptionStyle(r.label, r.key),
			}
		}
		return opts, curr + 1, endIdx
	}

	// 3. Try inline slash choice on last line, e.g. "Allow? [y/N/a]"
	lastLine := contentLines[endIdx-1]
	if m := inlineSlashChoiceRegex.FindStringSubmatch(lastLine); m != nil {
		parts := strings.Split(m[1], "/")
		if len(parts) >= 2 {
			opts := make([]PromptOption, 0, len(parts))
			for _, p := range parts {
				pTrim := strings.TrimSpace(p)
				if pTrim == "" {
					continue
				}
				key := strings.ToLower(pTrim)
				label := pTrim
				switch strings.ToLower(pTrim) {
				case "y", "yes":
					label = "Yes (" + pTrim + ")"
				case "n", "no":
					label = "No (" + pTrim + ")"
				case "a", "always":
					label = "Always (" + pTrim + ")"
				case "c", "cancel":
					label = "Cancel (" + pTrim + ")"
					key = "esc"
				}
				opts = append(opts, PromptOption{
					Key:   key,
					Label: label,
					Style: detectOptionStyle(label, key),
				})
			}
			if len(opts) >= 2 {
				return opts, endIdx - 1, endIdx
			}
		}
	}

	return nil, 0, 0
}

// analyzePrompt inspects terminal output and agent state to determine whether the agent is waiting,
// whether the prompt is a Yes/No confirmation ("confirm") or an open question/input ("question"),
// extracts the highlighted prompt text, and parses structured prompt options when available.
func analyzePrompt(outputText, agentState string, initiallyWaiting bool) (bool, string, string, []PromptOption) {
	trimmedOut := strings.TrimSpace(outputText)
	if trimmedOut == "" ||
		strings.HasPrefix(trimmedOut, "Terminal active (waiting for") ||
		strings.HasPrefix(trimmedOut, "⚡ Herdr") {
		if initiallyWaiting {
			return true, "confirm", "", nil
		}
		return false, "", "", nil
	}

	rawLines := strings.Split(strings.ReplaceAll(outputText, "\r\n", "\n"), "\n")
	nonEmpty := make([]string, 0, len(rawLines))
	for _, l := range rawLines {
		t := strings.TrimSpace(l)
		if t != "" {
			nonEmpty = append(nonEmpty, t)
		}
	}
	if len(nonEmpty) == 0 {
		if initiallyWaiting {
			return true, "confirm", "", nil
		}
		return false, "", "", nil
	}

	// If sitting at an OS shell prompt and not reported blocked by Herdr, it is not waiting on an agent prompt.
	if !initiallyWaiting && shellPromptRegex.MatchString(nonEmpty[len(nonEmpty)-1]) {
		return false, "", "", nil
	}

	// Strip trailing TUI chrome (borders, empty input prompt `❯`, shortcut hints) from the bottom.
	contentEnd := len(nonEmpty)
	for contentEnd > 0 && isTUIChromeLine(nonEmpty[contentEnd-1]) {
		contentEnd--
	}
	contentLines := nonEmpty[:contentEnd]
	if len(contentLines) == 0 {
		if initiallyWaiting {
			return true, "confirm", tailSliceJoin(nonEmpty, 3), nil
		}
		return false, "", "", nil
	}

	lastContent := contentLines[len(contentLines)-1]
	if !initiallyWaiting && shellPromptRegex.MatchString(lastContent) {
		return false, "", "", nil
	}

	tail6 := tailSliceJoin(nonEmpty, 6)
	contentTail5 := tailSliceJoin(contentLines, 5)

	// First, check if structured menu options exist (numbered, lettered, or inline slash choices)
	opts, optStart, _ := extractMenuOptions(contentLines)
	if len(opts) >= 2 {
		contextText := ""
		if optStart > 0 {
			contextText = strings.Join(contentLines[:optStart], "\n")
		} else {
			contextText = contentLines[0]
		}
		pType := "confirm"
		hasApprove := false
		for _, o := range opts {
			if o.Style == "approve" {
				hasApprove = true
				break
			}
		}
		if !hasApprove {
			pType = "question"
		}
		return true, pType, contextText, opts
	}

	// 1. Check for confirmation / TUI menu prompt
	if promptRegex.MatchString(tail6) {
		// Distinguish a Yes/No confirmation from a multiple-choice question menu
		hasNumberedOpts := false
		for _, l := range contentLines[max(0, len(contentLines)-5):] {
			if numberedMenuOptionRegex.MatchString(l) {
				hasNumberedOpts = true
				break
			}
		}
		if hasNumberedOpts && !yesNoChoiceRegex.MatchString(tail6) {
			return true, "question", contentTail5, nil
		}
		return true, "confirm", contentTail5, nil
	}

	// 2. Check for explicit text-input prompt (e.g. "Enter file path:", "Escribe el nombre:")
	if inputPromptRegex.MatchString(lastContent) {
		return true, "question", extractQuestionBlock(contentLines), nil
	}

	// 3. Check for conversational or interactive question when the agent is not actively streaming ("working")
	if agentState != "working" || initiallyWaiting {
		if isQuestionLine(lastContent) {
			return true, "question", extractQuestionBlock(contentLines), nil
		}
	}

	// 4. If Herdr explicitly reported the agent as blocked, inspect recent lines for question vs confirm
	if initiallyWaiting {
		for i := len(contentLines) - 1; i >= 0 && i >= len(contentLines)-4; i-- {
			if isQuestionLine(contentLines[i]) || inputPromptRegex.MatchString(contentLines[i]) {
				return true, "question", contentTail5, nil
			}
		}
		return true, "confirm", contentTail5, nil
	}

	return false, "", "", nil
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

	// 2. Query native `herdr agent list` for direct agent telemetry (agy, claude, etc.)
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

	// 2b. Merge or fallback to `herdr pane list` so non-registered panes are also monitored
	cmdPanes := execCommand(bin, "pane", "list")
	panesRaw, panesErr := cmdPanes.Output()
	if panesErr == nil {
		var panesData paneListOutput
		if json.Unmarshal(panesRaw, &panesData) == nil {
			for _, p := range panesData.Result.Panes {
				if _, exists := metaMap[p.PaneID]; exists {
					continue
				}
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
	}

	if len(panes) == 0 {
		if panesErr != nil {
			return AgentStatus{
				Output:      "⚡ Herdr server running (no active panes found)",
				Waiting:     false,
				HerdrOnline: true,
			}, nil, nil
		}
		return AgentStatus{
			Output:      "⚡ Herdr server running (no active panes)",
			Waiting:     false,
			HerdrOnline: true,
		}, nil, nil
	}

	readPane := func(id string) string {
		raw, _ := execCommand(bin, "pane", "read", id, "--source", "recent-unwrapped", "--lines", "80").Output()
		return string(raw)
	}

	// 3. Read and analyze all panes so prompts/questions on unfocused panes are always detected and notified
	outputs := make(map[string]string, len(panes))
	waitingStatuses := make([]AgentStatus, 0)
	for i := range panes {
		pOut := readPane(panes[i].PaneID)
		if strings.TrimSpace(pOut) == "" {
			pOut = "Terminal active (waiting for agent activity in pane " + panes[i].PaneID + ")"
		}
		outputs[panes[i].PaneID] = pOut
		isWait, pType, pText, pOpts := analyzePrompt(pOut, panes[i].AgentState, panes[i].Waiting)
		panes[i].Waiting = isWait
		panes[i].PromptType = pType
		panes[i].PromptText = pText
		panes[i].PromptOptions = pOpts
		if isWait {
			waitingStatuses = append(waitingStatuses, AgentStatus{
				Output:        pOut,
				Waiting:       true,
				PromptType:    pType,
				PromptText:    pText,
				PromptOptions: pOpts,
				PaneID:        panes[i].PaneID,
				AgentState:    panes[i].AgentState,
				Title:         panes[i].Title,
				HerdrOnline:   true,
			})
		}
	}

	// 4. Pick selected pane: preferredPane -> first waiting/blocked -> focused -> first
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

	return AgentStatus{
		Output:        outputs[selected.PaneID],
		Waiting:       selected.Waiting,
		PromptType:    selected.PromptType,
		PromptText:    selected.PromptText,
		PromptOptions: selected.PromptOptions,
		PaneID:        selected.PaneID,
		AgentState:    selected.AgentState,
		Title:         selected.Title,
		HerdrOnline:   true,
		Panes:         panes,
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
		state := "tmux"
		isWaiting, promptType, promptText, promptOpts := analyzePrompt(outputText, state, false)

		panes = append(panes, PaneSummary{
			PaneID:        id,
			Title:         title,
			AgentState:    state,
			Waiting:       isWaiting,
			PromptType:    promptType,
			PromptText:    promptText,
			PromptOptions: promptOpts,
			Focused:       focused,
			Backend:       "tmux",
		})
		outputs[id] = outputText

		if isWaiting {
			waitingStatuses = append(waitingStatuses, AgentStatus{
				Output:        outputText,
				Waiting:       true,
				PromptType:    promptType,
				PromptText:    promptText,
				PromptOptions: promptOpts,
				PaneID:        id,
				AgentState:    state,
				Title:         title,
				HerdrOnline:   true,
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
		Output:        outputs[selected.PaneID],
		Waiting:       selected.Waiting,
		PromptType:    selected.PromptType,
		PromptText:    selected.PromptText,
		PromptOptions: selected.PromptOptions,
		PaneID:        selected.PaneID,
		AgentState:    selected.AgentState,
		Title:         selected.Title,
		HerdrOnline:   true,
		Panes:         panes,
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
