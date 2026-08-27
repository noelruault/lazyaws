package aws

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
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

func TestVolumeIDsSkipsDevicesWithNoVolume(t *testing.T) {
	devices := []BlockDevice{
		{DeviceName: "/dev/sda1", VolumeID: "vol-0fedcba9876543210"},
		{DeviceName: "/dev/sdb"}, // instance store, nothing to describe
		{DeviceName: "/dev/sdc", VolumeID: "vol-0abcdef1234567890"},
	}

	got := volumeIDs(devices)

	want := []string{"vol-0fedcba9876543210", "vol-0abcdef1234567890"}
	if len(got) != len(want) {
		t.Fatalf("volumeIDs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("volumeIDs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestVolumeIDsOfNothingAsksNothing(t *testing.T) {
	if got := volumeIDs([]BlockDevice{{DeviceName: "/dev/sdb"}}); got != nil {
		t.Errorf("volumeIDs() = %v, want nil so no call is made for a host with no EBS volumes", got)
	}
}

// DescribeVolumes answers in its own order, so a device must take its size and type from its own volume rather than from whichever came back in the same position.
func TestApplyVolumesMatchesByIDNotPosition(t *testing.T) {
	devices := []BlockDevice{
		{DeviceName: "/dev/sda1", VolumeID: "vol-0fedcba9876543210"},
		{DeviceName: "/dev/sdc", VolumeID: "vol-0abcdef1234567890"},
	}
	size8, iops100, iops3000, throughput125 := int32(8), int32(100), int32(3000), int32(125)
	encrypted := true
	volumes := []types.Volume{
		// Reversed relative to the request, which is what the API actually did.
		{VolumeId: &devices[1].VolumeID, Size: &size8, VolumeType: types.VolumeTypeGp3, Iops: &iops3000, Throughput: &throughput125},
		{VolumeId: &devices[0].VolumeID, Size: &size8, VolumeType: types.VolumeTypeGp2, Iops: &iops100, Encrypted: &encrypted},
	}

	applyVolumes(devices, volumes)

	if devices[0].VolumeType != "gp2" || devices[0].Iops != 100 || devices[0].Throughput != 0 || !devices[0].Encrypted {
		t.Errorf("/dev/sda1 = %+v, want its own gp2/100 IOPS/no throughput/encrypted volume", devices[0])
	}
	if devices[1].VolumeType != "gp3" || devices[1].Iops != 3000 || devices[1].Throughput != 125 || devices[1].Encrypted {
		t.Errorf("/dev/sdc = %+v, want its own gp3/3000 IOPS/125 throughput/unencrypted volume", devices[1])
	}
	if devices[0].VolumeSize != 8 || devices[1].VolumeSize != 8 {
		t.Errorf("volume sizes = %d/%d, want 8/8", devices[0].VolumeSize, devices[1].VolumeSize)
	}
}

// The devices carry values a bare zero-value device would not, or "left as it was" and "overwritten with zeroes" look identical.
func TestApplyVolumesLeavesUnansweredDevicesAlone(t *testing.T) {
	devices := []BlockDevice{
		{DeviceName: "/dev/sda1", VolumeID: "vol-0fedcba9876543210", VolumeSize: 8, VolumeType: "gp2", Iops: 100, Encrypted: true},
		{DeviceName: "/dev/sdb", VolumeSize: 4, VolumeType: "gp3", Throughput: 125},
	}
	want := append([]BlockDevice(nil), devices...)
	id, size := "vol-0000000000000cafe", int32(500)

	applyVolumes(devices, []types.Volume{{VolumeId: &id, Size: &size, VolumeType: types.VolumeTypeIo2}})

	for i, d := range devices {
		if d != want[i] {
			t.Errorf("%s = %+v, want %+v untouched when the response carried no volume for it", d.DeviceName, d, want[i])
		}
	}
}

func TestDescribeVolumesGuards(t *testing.T) {
	volumes, err := (&Client{}).DescribeVolumes(context.Background(), nil)
	if err != nil || volumes != nil {
		t.Errorf("DescribeVolumes(nil) = %v, %v; want no call and no error for a host with no EBS volumes", volumes, err)
	}
	if _, err := (&Client{}).DescribeVolumes(context.Background(), []string{"vol-0fedcba9876543210"}); err == nil {
		t.Error("DescribeVolumes() with nil EC2 client should error")
	}
}

func TestMapInstanceTypeInfo(t *testing.T) {
	vcpus, memory := int32(2), int64(1024)
	networkPerformance := "Up to 5 Gigabit"
	typeInfo := types.InstanceTypeInfo{
		InstanceType: types.InstanceTypeT3aMicro,
		VCpuInfo:     &types.VCpuInfo{DefaultVCpus: &vcpus},
		MemoryInfo:   &types.MemoryInfo{SizeInMiB: &memory},
		NetworkInfo:  &types.NetworkInfo{NetworkPerformance: &networkPerformance, EnaSupport: types.EnaSupportRequired},
		EbsInfo:      &types.EbsInfo{EbsOptimizedSupport: types.EbsOptimizedSupportDefault},
		ProcessorInfo: &types.ProcessorInfo{
			SupportedArchitectures: []types.ArchitectureType{types.ArchitectureTypeX8664},
		},
	}

	got := mapInstanceTypeInfo(typeInfo)

	if got.InstanceType != "t3a.micro" || got.VCpus != 2 || got.Memory != 1024 {
		t.Errorf("mapInstanceTypeInfo() = %+v, want t3a.micro/2 vCPU/1024 MiB", got)
	}
	if got.NetworkPerformance != "Up to 5 Gigabit (ENA)" {
		t.Errorf("NetworkPerformance = %q, want the ENA suffix appended", got.NetworkPerformance)
	}
	if !got.EbsOptimized {
		t.Error("EbsOptimized = false, want true for EbsOptimizedSupport=default")
	}
	if got.StorageType != "EBS Only" {
		t.Errorf("StorageType = %q, want %q when the type has no instance storage", got.StorageType, "EBS Only")
	}
	if len(got.SupportedArchitectures) != 1 || got.SupportedArchitectures[0] != "x86_64" {
		t.Errorf("SupportedArchitectures = %v, want [x86_64]", got.SupportedArchitectures)
	}
}

// Every optional block is a pointer the API may omit; a bare response must map, not panic.
func TestMapInstanceTypeInfoToleratesMissingSections(t *testing.T) {
	got := mapInstanceTypeInfo(types.InstanceTypeInfo{InstanceType: types.InstanceTypeT2Micro})

	if got.InstanceType != "t2.micro" {
		t.Errorf("InstanceType = %q, want t2.micro", got.InstanceType)
	}
	if got.StorageType != "EBS Only" {
		t.Errorf("StorageType = %q, want %q", got.StorageType, "EBS Only")
	}
	if got.SupportedArchitectures != nil {
		t.Errorf("SupportedArchitectures = %v, want nil when the response has no processor info", got.SupportedArchitectures)
	}
}

// A cached type is answered without the API, which is the only observable proof the cache is consulted at all.
func TestGetInstanceTypeInfoAnswersFromCache(t *testing.T) {
	c := &Client{}
	c.cacheInstanceType("t3a.micro", InstanceTypeInfo{InstanceType: "t3a.micro", VCpus: 2, Memory: 1024})

	got, err := c.GetInstanceTypeInfo(context.Background(), "t3a.micro")
	if err != nil {
		t.Fatalf("GetInstanceTypeInfo() error = %v, want the cached answer with no EC2 client", err)
	}
	if got.VCpus != 2 || got.Memory != 1024 {
		t.Errorf("GetInstanceTypeInfo() = %+v, want the cached t3a.micro", got)
	}

	// The caller holds its own copy: editing it must not rewrite what the next caller reads.
	got.VCpus = 99
	again, err := c.GetInstanceTypeInfo(context.Background(), "t3a.micro")
	if err != nil {
		t.Fatalf("GetInstanceTypeInfo() error = %v", err)
	}
	if again.VCpus != 2 {
		t.Errorf("cached VCpus = %d after a caller edited its copy, want 2", again.VCpus)
	}
}

// The fetch path's last step: what it maps, it must also store, or every selection re-asks for data that cannot change.
func TestRememberInstanceTypeCachesWhatItMapped(t *testing.T) {
	c := &Client{}
	vcpus := int32(2)

	got := c.rememberInstanceType("t3a.micro", types.InstanceTypeInfo{
		InstanceType: types.InstanceTypeT3aMicro,
		VCpuInfo:     &types.VCpuInfo{DefaultVCpus: &vcpus},
	})
	if got.VCpus != 2 {
		t.Errorf("rememberInstanceType() = %+v, want the mapped 2 vCPUs", got)
	}

	cached, ok := c.cachedInstanceType("t3a.micro")
	if !ok {
		t.Fatal("rememberInstanceType() mapped a response without caching it")
	}
	if !reflect.DeepEqual(cached, got) {
		t.Errorf("cached %+v, want the value it returned %+v", cached, got)
	}
}

func TestGetInstanceTypeInfoGuards(t *testing.T) {
	// Both guards are checked by the message they carry: with a nil client, an unguarded empty type still errors on the client, so "it errored" alone cannot tell which guard fired.
	_, err := (&Client{}).GetInstanceTypeInfo(context.Background(), "")
	if err == nil {
		t.Fatal("GetInstanceTypeInfo() with an empty instance type should error")
	}
	if !strings.Contains(err.Error(), "instance type required") {
		t.Errorf("GetInstanceTypeInfo(\"\") error = %v, want the empty-type guard to be what fired", err)
	}

	_, err = (&Client{}).GetInstanceTypeInfo(context.Background(), "t3a.micro")
	if err == nil {
		t.Fatal("GetInstanceTypeInfo() with nil EC2 client and a cold cache should error")
	}
	if !strings.Contains(err.Error(), "EC2 client") {
		t.Errorf("GetInstanceTypeInfo() nil-client error = %v, want the client guard to be what fired", err)
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
