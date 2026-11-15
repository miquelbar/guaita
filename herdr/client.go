package herdr

import (
	"encoding/json"
	"os/exec"
)

// AgentStatus represents the state of the monitored AI agent via Herdr.
type AgentStatus struct {
	Output  string `json:"output"`
	Waiting bool   `json:"waiting"`
}

var execCommand = exec.Command

// GetStatus checks the current status of Herdr or returns a defensive fallback when inactive.
func GetStatus() (AgentStatus, error) {
	cmd := execCommand("herdr-core", "status", "--json")
	out, err := cmd.Output()
	if err != nil {
		return AgentStatus{Output: "⚡ Herdr offline or inactive", Waiting: false}, err
	}

	var status AgentStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return AgentStatus{Output: string(out), Waiting: false}, nil
	}
	return status, nil
}

// SendAction dispatches an action (approve, reject, or text input) to herdr-core.
func SendAction(action string) error {
	var cmd *exec.Cmd
	switch action {
	case "approve":
		cmd = execCommand("herdr-core", "approve")
	case "reject":
		cmd = execCommand("herdr-core", "reject")
	default:
		cmd = execCommand("herdr-core", "input", action)
	}
	return cmd.Run()
}
