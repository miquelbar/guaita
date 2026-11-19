package herdr

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func fakeExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	return cmd
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}

	if len(args) == 0 {
		os.Exit(2)
	}

	subCmd := ""
	if len(args) > 1 {
		subCmd = args[1]
	}

	mode := os.Getenv("TEST_HERDR_MODE")
	if mode == "fail" {
		os.Exit(1)
	}

	switch subCmd {
	case "status":
		if mode == "tmux_fallback" {
			os.Exit(1)
		}
		fmt.Print(`{"server":{"status":"running","running":true,"socket":"/tmp/test.sock"}}`)
	case "list-panes":
		fmt.Print("%0|1|1|claude-code\n")
	case "capture-pane":
		fmt.Print("Deploy to staging? (y/n)")
	case "send-keys":
		os.Exit(0)
	case "agent":
		agentAction := ""
		if len(args) > 2 {
			agentAction = args[2]
		}
		switch agentAction {
		case "list":
			if mode == "agent_blocked" {
				fmt.Print(`{"result":{"agents":[{"agent":"claude","agent_status":"idle","focused":true,"pane_id":"w1:p1","terminal_title":"claude"},{"agent":"agy","agent_status":"blocked","focused":false,"pane_id":"w1:p2","terminal_title":"agy"}]}}`)
			} else {
				os.Exit(1)
			}
		case "prompt":
			os.Exit(0)
		case "wait":
			os.Exit(0)
		default:
			os.Exit(0)
		}
	case "pane":
		paneAction := ""
		if len(args) > 2 {
			paneAction = args[2]
		}
		switch paneAction {
		case "list":
			switch mode {
			case "blocked":
				fmt.Print(`{"result":{"panes":[{"pane_id":"w1:p1","agent_status":"blocked","focused":true,"terminal_title":"agent-pane"}]}}`)
			case "pattern_prompt":
				fmt.Print(`{"result":{"panes":[{"pane_id":"w1:p1","agent_status":"working","focused":true,"terminal_title":"agent-pane"}]}}`)
			case "unfocused_pane_prompt":
				fmt.Print(`{"result":{"panes":[{"pane_id":"w1:p1","agent_status":"idle","focused":true,"terminal_title":"shell"},{"pane_id":"w1:p2","agent_status":"idle","focused":false,"terminal_title":"agy"}]}}`)
			default:
				fmt.Print(`{"result":{"panes":[{"pane_id":"w1:p1","agent_status":"idle","focused":true,"terminal_title":"agent-pane"}]}}`)
			}
		case "read":
			targetPane := ""
			if len(args) > 3 {
				targetPane = args[3]
			}
			switch mode {
			case "unfocused_pane_prompt":
				if targetPane == "w1:p2" {
					fmt.Print("Allow editing /tmp/a?\n❯ 1. Yes\n  2. Always allow\n  3. No\nEnter to confirm · Esc to cancel")
				} else {
					fmt.Print("miquel@mac ~ %")
				}
			case "pattern_prompt":
				fmt.Print(`Do you want to apply these changes? (y/n)`)
			case "old_scrollback":
				fmt.Print("Old prompt: Deploy? (y/n)\nResult: y\nStep 1\nStep 2\nCompiling...\nTests passed\nBuild succeeded\nmiquel@mac ~ %")
			case "agy_tui":
				fmt.Print("Allow editing /tmp/a?\n❯ 1. Yes\n  2. Always allow\n  3. No\nEnter to confirm · Esc to cancel")
			case "approve_word":
				fmt.Print("git log: fix(security): approve flow\nmiquel@mac ~ %")
			case "blocked":
				fmt.Print(`Approval required for file edit.`)
			default:
				fmt.Print(`Agent working normally...`)
			}
		case "send-keys", "send-text":
			os.Exit(0)
		default:
			os.Exit(0)
		}
	default:
		os.Exit(0)
	}
}

func TestGetStatus_BlockedState(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()
	execCommand = fakeExecCommand

	t.Setenv("TEST_HERDR_MODE", "blocked")
	status, err := GetStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.HerdrOnline {
		t.Errorf("expected HerdrOnline to be true")
	}
	if !status.Waiting {
		t.Errorf("expected waiting to be true for blocked agent")
	}
	if status.PaneID != "w1:p1" {
		t.Errorf("expected pane w1:p1, got %s", status.PaneID)
	}
}

func TestGetStatus_PromptPattern(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()
	execCommand = fakeExecCommand

	t.Setenv("TEST_HERDR_MODE", "pattern_prompt")
	status, err := GetStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Waiting {
		t.Errorf("expected waiting to be true from prompt pattern match")
	}
}

func TestGetStatus_OldScrollbackNoFalsePositive(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()
	execCommand = fakeExecCommand

	t.Setenv("TEST_HERDR_MODE", "old_scrollback")
	status, err := GetStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Waiting {
		t.Errorf("expected waiting to be false when (y/n) is only in old scrollback")
	}
}

func TestGetStatus_AgentListAndPromptSubmit(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()
	execCommand = fakeExecCommand

	t.Setenv("TEST_HERDR_MODE", "agent_blocked")
	status, err := GetStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Waiting {
		t.Errorf("expected waiting=true for blocked agent from agent list")
	}
	if status.PaneID != "w1:p2" {
		t.Errorf("expected blocked agent pane w1:p2, got %s", status.PaneID)
	}
	if err := SendAction("ahora escribe /tmp/b un hola"); err != nil {
		t.Errorf("expected SendAction via agent prompt to succeed, got %v", err)
	}
}

func TestGetStatus_AgyTUIPrompt(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()
	execCommand = fakeExecCommand

	t.Setenv("TEST_HERDR_MODE", "agy_tui")
	status, err := GetStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Waiting {
		t.Errorf("expected waiting=true for agy TUI approval dialog")
	}
}

func TestGetStatus_ApproveWordInOutputNoFalsePositive(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()
	execCommand = fakeExecCommand

	t.Setenv("TEST_HERDR_MODE", "approve_word")
	status, err := GetStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Waiting {
		t.Errorf("expected waiting to be false when output only mentions the word 'approve'")
	}
}

func TestGetStatus_TmuxFallback(t *testing.T) {
	oldExec := execCommand
	oldLook := lookPath
	defer func() {
		execCommand = oldExec
		lookPath = oldLook
	}()
	execCommand = fakeExecCommand
	lookPath = func(file string) (string, error) { return file, nil }

	t.Setenv("TEST_HERDR_MODE", "tmux_fallback")
	status, err := GetStatus()
	if err != nil {
		t.Fatalf("unexpected error on tmux fallback: %v", err)
	}
	if !status.HerdrOnline {
		t.Errorf("expected HerdrOnline=true on tmux fallback")
	}
	if status.AgentState != "tmux" || status.PaneID != "%0" {
		t.Errorf("expected tmux pane %%0, got state=%q pane=%q", status.AgentState, status.PaneID)
	}
	if !status.Waiting {
		t.Errorf("expected waiting=true for tmux (y/n) prompt")
	}

	if err := SendAction("approve"); err != nil {
		t.Errorf("SendAction(approve) over tmux failed: %v", err)
	}
	if err := SendAction("custom input"); err != nil {
		t.Errorf("SendAction(custom input) over tmux failed: %v", err)
	}
}

func TestGetStatus_FallbackOnError(t *testing.T) {
	oldExec := execCommand
	oldLook := lookPath
	defer func() {
		execCommand = oldExec
		lookPath = oldLook
	}()
	execCommand = fakeExecCommand
	lookPath = func(file string) (string, error) {
		if file == "tmux" {
			return "", fmt.Errorf("tmux not found")
		}
		return file, nil
	}

	t.Setenv("TEST_HERDR_MODE", "fail")
	status, err := GetStatus()
	if err == nil {
		t.Fatalf("expected error on failed herdr")
	}
	if status.HerdrOnline {
		t.Errorf("expected HerdrOnline to be false on error")
	}
	if status.Waiting {
		t.Errorf("expected waiting to be false on error")
	}
	if status.Output != "⚡ Herdr / tmux offline or inactive" {
		t.Errorf("unexpected fallback output: %s", status.Output)
	}
}

func TestSendAction(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()
	execCommand = fakeExecCommand

	actions := []string{"approve", "reject", "enter", "esc", "custom command text"}
	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			if err := SendAction(action); err != nil {
				t.Errorf("SendAction(%q) failed: %v", action, err)
			}
		})
	}
}

func TestAnalyzePrompt_QuestionAndConfirmClassification(t *testing.T) {
	cases := []struct {
		name             string
		output           string
		agentState       string
		initiallyWaiting bool
		wantWaiting      bool
		wantType         string
		wantTextContains string
		wantOptionCount  int
	}{
		{
			name:             "open question in idle agent with TUI chrome at bottom",
			output:           "Processed command.\n¿Querías ejecutar agy -i en una nueva sesión desde tu terminal, o necesitas que haga algo adicional aquí?\n────────────────────────────────────────\n❯ \n────────────────────────────────────────\n  ? for shortcuts",
			agentState:       "idle",
			initiallyWaiting: false,
			wantWaiting:      true,
			wantType:         "question",
			wantTextContains: "¿Querías ejecutar agy -i",
		},
		{
			name:             "explicit input prompt with colon",
			output:           "Initializing project...\nPlease enter the target output file path:",
			agentState:       "idle",
			initiallyWaiting: false,
			wantWaiting:      true,
			wantType:         "question",
			wantTextContains: "Please enter the target output file path:",
		},
		{
			name:             "multiple choice non-yes-no question menu",
			output:           "Which database engine should we configure?\n❯ 1. PostgreSQL\n  2. SQLite\n  3. Redis\nEnter to confirm · Esc to cancel",
			agentState:       "blocked",
			initiallyWaiting: true,
			wantWaiting:      true,
			wantType:         "question",
			wantTextContains: "Which database engine should we configure?",
			wantOptionCount:  3,
		},
		{
			name:             "yes/no TUI confirmation menu",
			output:           "Allow editing /tmp/a?\n❯ 1. Yes\n  2. Always allow\n  3. No\nEnter to confirm · Esc to cancel",
			agentState:       "blocked",
			initiallyWaiting: true,
			wantWaiting:      true,
			wantType:         "confirm",
			wantTextContains: "Allow editing /tmp/a?",
			wantOptionCount:  3,
		},
		{
			name:             "codex cli multi-option permission prompt",
			output:           "Would you like to run the following command?\n\nEnvironment: local\n\nReason: ¿Permites repetir la suite Playwright después de ajustar la comprobación para leer el color del tema seleccionado?\n\n$ npm --prefix e2e run test:e2e\n\n\n› 1. Yes, proceed (y)\n  2. Yes, and don't ask again for commands that start with `npm --prefix e2e run test:e2e` (p)\n  3. No, and tell Codex what to do differently (esc)\n\n  Press enter to confirm or esc to cancel",
			agentState:       "blocked",
			initiallyWaiting: true,
			wantWaiting:      true,
			wantType:         "confirm",
			wantTextContains: "$ npm --prefix e2e run test:e2e",
			wantOptionCount:  3,
		},
		{
			name:             "inline slash choice prompt",
			output:           "Apply database migration? [y/N/a]:",
			agentState:       "idle",
			initiallyWaiting: false,
			wantWaiting:      true,
			wantType:         "confirm",
			wantTextContains: "Apply database migration?",
			wantOptionCount:  3,
		},
		{
			name:             "normal idle completion statement",
			output:           "Se ha creado el archivo /tmp/b con el contenido hola.\n────────────────────\n❯ \n  ? for shortcuts",
			agentState:       "idle",
			initiallyWaiting: false,
			wantWaiting:      false,
			wantType:         "",
			wantTextContains: "",
		},
		{
			name:             "streaming question while agent is actively working",
			output:           "Thinking about: What is the best approach?",
			agentState:       "working",
			initiallyWaiting: false,
			wantWaiting:      false,
			wantType:         "",
			wantTextContains: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotWait, gotType, gotText, gotOpts := analyzePrompt(tc.output, tc.agentState, tc.initiallyWaiting)
			if gotWait != tc.wantWaiting {
				t.Errorf("waiting = %v, want %v (type=%q, text=%q)", gotWait, tc.wantWaiting, gotType, gotText)
			}
			if gotType != tc.wantType {
				t.Errorf("promptType = %q, want %q", gotType, tc.wantType)
			}
			if tc.wantTextContains != "" && !strings.Contains(gotText, tc.wantTextContains) {
				t.Errorf("promptText = %q, expected to contain %q", gotText, tc.wantTextContains)
			}
			if tc.wantOptionCount > 0 && len(gotOpts) != tc.wantOptionCount {
				t.Errorf("got %d options, want %d: %+v", len(gotOpts), tc.wantOptionCount, gotOpts)
			}
		})
	}
}

func TestGetStatusWithPanes_DetectsUnfocusedWaitingPaneForAlerts(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()
	execCommand = fakeExecCommand

	t.Setenv("TEST_HERDR_MODE", "unfocused_pane_prompt")
	status, waitingList, err := GetStatusWithPanes("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Waiting || status.PaneID != "w1:p2" {
		t.Fatalf("expected auto-selection of unfocused waiting pane w1:p2, got waiting=%v pane=%q", status.Waiting, status.PaneID)
	}
	if len(waitingList) != 1 || waitingList[0].PaneID != "w1:p2" {
		t.Fatalf("expected waitingStatuses to include unfocused pane w1:p2 for background alerts, got %+v", waitingList)
	}
}
