package ui

import (
	"strings"
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
)

// Confirmation tokens must use visible names and IDs, never ARNs.
func TestEC2TerminateTokenIsWhatThePanelShows(t *testing.T) {
	named := &aws.Instance{ID: "i-0123456789abcdef0", Name: "web-1"}
	token := ec2InstanceToken(named)
	if !strings.Contains(token, named.Name) || !strings.Contains(token, named.ID) {
		t.Errorf("ec2InstanceToken(%+v) = %q, want both the name and the id", named, token)
	}

	unnamed := &aws.Instance{ID: "i-0123456789abcdef0"}
	if got := ec2InstanceToken(unnamed); got != unnamed.ID {
		t.Errorf("ec2InstanceToken(%+v) = %q, want just the id", unnamed, got)
	}
}
