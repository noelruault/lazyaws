package aws

import (
	"context"
	"testing"
)

func TestInstanceTypeInfo(t *testing.T) {
	typeInfo := InstanceTypeInfo{
		InstanceType:       "t3.medium",
		VCpus:              2,
		Memory:             4096,
		NetworkPerformance: "Up to 5 Gigabit",
		StorageType:        "EBS Only",
		EbsOptimized:       true,
	}

	if typeInfo.InstanceType != "t3.medium" {
		t.Errorf("Expected instance type 't3.medium', got '%s'", typeInfo.InstanceType)
	}

	if typeInfo.VCpus != 2 {
		t.Errorf("Expected 2 vCPUs, got %d", typeInfo.VCpus)
	}

	if typeInfo.Memory != 4096 {
		t.Errorf("Expected 4096 MiB memory, got %d", typeInfo.Memory)
	}

	if !typeInfo.EbsOptimized {
		t.Error("Expected EBS Optimized to be true")
	}
}

func TestInstanceTypeInfoMemoryConversion(t *testing.T) {
	typeInfo := InstanceTypeInfo{
		Memory: 8192,
	}

	memoryGB := float64(typeInfo.Memory) / 1024.0
	expectedGB := 8.0

	if memoryGB != expectedGB {
		t.Errorf("Expected %.2f GiB, got %.2f GiB", expectedGB, memoryGB)
	}
}

func TestInstanceDetails(t *testing.T) {
	details := InstanceDetails{
		Instance: Instance{
			ID:           "i-1234567890",
			Name:         "test-instance",
			State:        "running",
			InstanceType: "t3.medium",
		},
		InstanceTypeInfo: &InstanceTypeInfo{
			InstanceType: "t3.medium",
			VCpus:        2,
			Memory:       4096,
		},
	}

	if details.InstanceTypeInfo == nil {
		t.Error("Expected InstanceTypeInfo to be set")
	}

	if details.InstanceTypeInfo.VCpus != 2 {
		t.Errorf("Expected 2 vCPUs, got %d", details.InstanceTypeInfo.VCpus)
	}
}

func TestInstanceDetailsWithoutTypeInfo(t *testing.T) {
	details := InstanceDetails{
		Instance: Instance{
			ID:           "i-1234567890",
			Name:         "test-instance",
			State:        "running",
			InstanceType: "t3.medium",
		},
		InstanceTypeInfo: nil,
	}

	if details.InstanceTypeInfo != nil {
		t.Error("Expected InstanceTypeInfo to be nil")
	}

	if details.ID != "i-1234567890" {
		t.Errorf("Expected instance ID 'i-1234567890', got '%s'", details.ID)
	}
}

// Instance-store-only hosts must not trigger a snapshot API call.
func TestListVolumeSnapshotsNoVolumes(t *testing.T) {
	c := &Client{}
	snapshots, err := c.ListVolumeSnapshots(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListVolumeSnapshots(nil) error = %v, want nil", err)
	}
	if snapshots != nil {
		t.Errorf("ListVolumeSnapshots(nil) = %v, want nil", snapshots)
	}
}
