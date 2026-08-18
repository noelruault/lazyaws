package ui

import (
	"strings"
	"testing"
)

func TestECSExecPrompt(t *testing.T) {
	prompt := ecsExecPrompt("web", "abc123")
	if !strings.Contains(prompt, "web") || !strings.Contains(prompt, "abc123") {
		t.Errorf("ecsExecPrompt() = %q, want it to contain both container and task id", prompt)
	}
}
