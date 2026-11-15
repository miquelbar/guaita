package herdr

import (
	"fmt"
	"os"
	"os/exec"
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
		fmt.Print(`{"server":{"status":"running","running":true,"socket":"/tmp/test.sock"}}`)
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
			default:
				fmt.Print(`{"result":{"panes":[{"pane_id":"w1:p1","agent_status":"idle","focused":true,"terminal_title":"agent-pane"}]}}`)
			}
		case "read":
			switch mode {
			case "pattern_prompt":
				fmt.Print(`Do you want to apply these changes? (y/n)`)
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

func TestGetStatus_FallbackOnError(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()
	execCommand = fakeExecCommand

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
	if status.Output != "⚡ Herdr offline or inactive" {
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
