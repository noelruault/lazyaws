package presentation

import (
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
)

func TestGetInstanceDisplayStrings(t *testing.T) {
	inst := &aws.Instance{
		ID:           "i-0123456789abcdef0",
		Name:         "web-1",
		State:        "running",
		InstanceType: "t3.micro",
		PrivateIP:    "10.0.0.5",
	}
	got := GetInstanceDisplayStrings(inst)
	want := []string{"web-1", "i-0123456789abcdef0", "t3.micro", "10.0.0.5"}
	for i, w := range want {
		if got[i+1] != w {
			t.Errorf("cell %d = %q, want %q", i+1, got[i+1], w)
		}
	}
}

func TestGetInstanceDisplayStringsUnnamed(t *testing.T) {
	inst := &aws.Instance{ID: "i-abc", State: "stopped"}
	got := GetInstanceDisplayStrings(inst)
	if got[1] != "(no name)" {
		t.Errorf("name cell = %q, want %q", got[1], "(no name)")
	}
}
