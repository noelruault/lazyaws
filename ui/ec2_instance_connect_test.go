package ui

import (
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
)

func TestEC2InstanceConnectHost(t *testing.T) {
	cases := []struct {
		name string
		inst *aws.Instance
		want string
	}{
		{"prefers public IP", &aws.Instance{PublicIP: "1.2.3.4", PrivateIP: "10.0.0.1"}, "1.2.3.4"},
		{"falls back to private IP", &aws.Instance{PrivateIP: "10.0.0.1"}, "10.0.0.1"},
		{"empty when neither set", &aws.Instance{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ec2InstanceConnectHost(c.inst); got != c.want {
				t.Errorf("ec2InstanceConnectHost() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestEC2InstanceConnectUser(t *testing.T) {
	cases := []struct{ input, want string }{
		{"", "ec2-user"},
		{"   ", "ec2-user"},
		{"ubuntu", "ubuntu"},
		{"  ubuntu  ", "ubuntu"},
	}
	for _, c := range cases {
		if got := ec2InstanceConnectUser(c.input); got != c.want {
			t.Errorf("ec2InstanceConnectUser(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
