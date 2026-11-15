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

	cmd := args[0]
	subCmd := ""
	if len(args) > 1 {
		subCmd = args[1]
	}

	if cmd == "herdr-core" {
		switch subCmd {
		case "status":
			mode := os.Getenv("TEST_HERDR_MODE")
			switch mode {
			case "json_waiting":
				fmt.Print(`{"output":"Approval required: edit file? (y/n)","waiting":true}`)
			case "json_running":
				fmt.Print(`{"output":"Building project...","waiting":false}`)
			case "raw_text":
				fmt.Print(`Running agent task 123...`)
			case "fail":
				os.Exit(1)
			default:
				fmt.Print(`{"output":"Default status","waiting":false}`)
			}
		case "approve", "reject", "input":
			if os.Getenv("TEST_HERDR_FAIL") == "1" {
				os.Exit(1)
			}
			os.Exit(0)
		default:
			os.Exit(1)
		}
	}
}

func TestGetStatus_JSONWaiting(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()
	execCommand = fakeExecCommand

	t.Setenv("TEST_HERDR_MODE", "json_waiting")
	status, err := GetStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Waiting {
		t.Errorf("expected waiting to be true")
	}
	if status.Output != "Approval required: edit file? (y/n)" {
		t.Errorf("unexpected output: %s", status.Output)
	}
}

func TestGetStatus_FallbackOnError(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()
	execCommand = fakeExecCommand

	t.Setenv("TEST_HERDR_MODE", "fail")
	status, err := GetStatus()
	if err == nil {
		t.Fatalf("expected error on failed herdr-core")
	}
	if status.Waiting {
		t.Errorf("expected waiting to be false on error")
	}
	if status.Output != "⚡ Herdr offline o inactivo" {
		t.Errorf("unexpected fallback output: %s", status.Output)
	}
}

func TestGetStatus_RawText(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()
	execCommand = fakeExecCommand

	t.Setenv("TEST_HERDR_MODE", "raw_text")
	status, err := GetStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Waiting {
		t.Errorf("expected waiting to be false")
	}
	if status.Output != "Running agent task 123..." {
		t.Errorf("unexpected output: %s", status.Output)
	}
}

func TestSendAction(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()
	execCommand = fakeExecCommand

	tests := []string{"approve", "reject", "custom input"}
	for _, action := range tests {
		t.Run(action, func(t *testing.T) {
			if err := SendAction(action); err != nil {
				t.Errorf("SendAction(%q) failed: %v", action, err)
			}
		})
	}
}
