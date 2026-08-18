package ui

import (
	"strings"
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
)

func TestSSMConnectivityErrorNilWhenConnected(t *testing.T) {
	if err := ssmConnectivityError("i-123", &aws.SSMConnectionStatus{Connected: true}); err != nil {
		t.Errorf("ssmConnectivityError(connected) = %v, want nil", err)
	}
}

func TestSSMConnectivityErrorWhenNotConnected(t *testing.T) {
	err := ssmConnectivityError("i-123", &aws.SSMConnectionStatus{Connected: false})
	if err == nil || !strings.Contains(err.Error(), "i-123") {
		t.Errorf("ssmConnectivityError(not connected) = %v, want error mentioning instance id", err)
	}
}

func TestSSMConnectivityErrorWhenNilStatus(t *testing.T) {
	if err := ssmConnectivityError("i-123", nil); err == nil {
		t.Errorf("ssmConnectivityError(nil) = nil, want error")
	}
}

func TestBuildSSMSessionCommand(t *testing.T) {
	cmd := buildSSMSessionCommand("i-0123456789abcdef0", "us-east-1")
	args := strings.Join(cmd.Args, " ")
	for _, want := range []string{"ssm", "start-session", "i-0123456789abcdef0", "us-east-1"} {
		if !strings.Contains(args, want) {
			t.Errorf("buildSSMSessionCommand args = %q, want it to contain %q", args, want)
		}
	}
}
