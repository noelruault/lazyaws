package aws

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type Instance struct {
	ID           string
	Name         string
	State        string
	InstanceType string
	AZ           string
	PublicIP     string
	PrivateIP    string
	Tags         []Tag
}

type Tag struct {
	Key   string
	Value string
}

type ElasticIP struct {
	PublicIP      string
	AllocationID  string
	AssociationID string
	NetworkIF     string
	PrivateIP     string
}

type InstanceDetails struct {
	Instance
	LaunchTime         string
	VpcID              string
	SubnetID           string
	KeyName            string
	Architecture       string
	Platform           string
	RootDeviceType     string
	Monitoring         string
	IamInstanceProfile string
	SecurityGroups     []SecurityGroup
	BlockDevices       []BlockDevice
	NetworkInterfaces  []NetworkInterface
	InstanceTypeInfo   *InstanceTypeInfo
	ElasticIPs         []ElasticIP
}

type InstanceTypeInfo struct {
	InstanceType           string
	VCpus                  int32
	Memory                 int64 // in MiB
	NetworkPerformance     string
	StorageType            string
	EbsOptimized           bool
	InstanceStorageGB      int64
	SupportedArchitectures []string
}

type SecurityGroup struct {
	ID   string
	Name string
}

type BlockDevice struct {
	DeviceName          string
	VolumeID            string
	VolumeSize          int32
	VolumeType          string
	DeleteOnTermination bool
	Iops                int32
	Throughput          int32
	Encrypted           bool
}

type NetworkInterface struct {
	ID             string
	PrivateIP      string
	PublicIP       string
	SubnetID       string
	VpcID          string
	MacAddress     string
	SecurityGroups []SecurityGroup
}

func (c *Client) ListInstances(ctx context.Context) ([]Instance, error) {
	input := &ec2.DescribeInstancesInput{}
	var instances []Instance
	for {
		result, err := c.EC2.DescribeInstances(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to describe instances: %w", err)
		}

		for _, reservation := range result.Reservations {
			for _, inst := range reservation.Instances {
				instance := Instance{
					ID:           getString(inst.InstanceId),
					State:        string(inst.State.Name),
					InstanceType: string(inst.InstanceType),
					AZ:           getString(inst.Placement.AvailabilityZone),
					PublicIP:     getString(inst.PublicIpAddress),
					PrivateIP:    getString(inst.PrivateIpAddress),
				}

				instance.Name = getNameTag(inst.Tags)

				for _, tag := range inst.Tags {
					instance.Tags = append(instance.Tags, Tag{Key: getString(tag.Key), Value: getString(tag.Value)})
				}

				instances = append(instances, instance)
			}
		}

		if result.NextToken == nil {
			break
		}
		input.NextToken = result.NextToken
	}

	return instances, nil
}

func getNameTag(tags []types.Tag) string {
	for _, tag := range tags {
		if tag.Key != nil && *tag.Key == "Name" && tag.Value != nil {
			return *tag.Value
		}
	}
	return ""
}

func getString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (c *Client) GetInstanceDetails(ctx context.Context, instanceID string) (*InstanceDetails, error) {
	input := &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	}

	result, err := c.EC2.DescribeInstances(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe instance: %w", err)
	}

	if len(result.Reservations) == 0 || len(result.Reservations[0].Instances) == 0 {
		return nil, fmt.Errorf("instance %s not found", instanceID)
	}

	inst := result.Reservations[0].Instances[0]

	details := &InstanceDetails{
		Instance: Instance{
			ID:           getString(inst.InstanceId),
			State:        string(inst.State.Name),
			InstanceType: string(inst.InstanceType),
			AZ:           getString(inst.Placement.AvailabilityZone),
			PublicIP:     getString(inst.PublicIpAddress),
			PrivateIP:    getString(inst.PrivateIpAddress),
			Name:         getNameTag(inst.Tags),
		},
		VpcID:          getString(inst.VpcId),
		SubnetID:       getString(inst.SubnetId),
		KeyName:        getString(inst.KeyName),
		Architecture:   string(inst.Architecture),
		Platform:       getString(inst.PlatformDetails),
		RootDeviceType: string(inst.RootDeviceType),
	}

	if inst.LaunchTime != nil {
		details.LaunchTime = inst.LaunchTime.Format("2006-01-02 15:04:05")
	}

	if inst.Monitoring != nil {
		details.Monitoring = string(inst.Monitoring.State)
	}

	if inst.IamInstanceProfile != nil {
		details.IamInstanceProfile = getString(inst.IamInstanceProfile.Arn)
	}

	for _, tag := range inst.Tags {
		details.Instance.Tags = append(details.Instance.Tags, Tag{
			Key:   getString(tag.Key),
			Value: getString(tag.Value),
		})
	}

	for _, sg := range inst.SecurityGroups {
		details.SecurityGroups = append(details.SecurityGroups, SecurityGroup{
			ID:   getString(sg.GroupId),
			Name: getString(sg.GroupName),
		})
	}

	for _, bd := range inst.BlockDeviceMappings {
		device := BlockDevice{
			DeviceName: getString(bd.DeviceName),
		}
		if bd.Ebs != nil {
			device.VolumeID = getString(bd.Ebs.VolumeId)
			device.DeleteOnTermination = bd.Ebs.DeleteOnTermination != nil && *bd.Ebs.DeleteOnTermination

			if device.VolumeID != "" {
				volInput := &ec2.DescribeVolumesInput{
					VolumeIds: []string{device.VolumeID},
				}
				volResult, err := c.EC2.DescribeVolumes(ctx, volInput)
				if err == nil && len(volResult.Volumes) > 0 {
					vol := volResult.Volumes[0]
					if vol.Size != nil {
						device.VolumeSize = *vol.Size
					}
					device.VolumeType = string(vol.VolumeType)
					device.Iops = getInt32Value(vol.Iops)
					device.Throughput = getInt32Value(vol.Throughput)
					device.Encrypted = vol.Encrypted != nil && *vol.Encrypted
				}
			}
		}
		details.BlockDevices = append(details.BlockDevices, device)
	}

	for _, ni := range inst.NetworkInterfaces {
		iface := NetworkInterface{
			ID:         getString(ni.NetworkInterfaceId),
			PrivateIP:  getString(ni.PrivateIpAddress),
			SubnetID:   getString(ni.SubnetId),
			VpcID:      getString(ni.VpcId),
			MacAddress: getString(ni.MacAddress),
		}

		if ni.Association != nil {
			iface.PublicIP = getString(ni.Association.PublicIp)
		}

		for _, sg := range ni.Groups {
			iface.SecurityGroups = append(iface.SecurityGroups, SecurityGroup{
				ID:   getString(sg.GroupId),
				Name: getString(sg.GroupName),
			})
		}

		details.NetworkInterfaces = append(details.NetworkInterfaces, iface)
	}

	typeInfo, err := c.GetInstanceTypeInfo(ctx, details.InstanceType)
	if err == nil {
		details.InstanceTypeInfo = typeInfo
	}

	eips, err := c.DescribeInstanceAddresses(ctx, instanceID)
	if err == nil {
		details.ElasticIPs = eips
	}

	return details, nil
}

func (c *Client) StartInstance(ctx context.Context, instanceID string) error {
	input := &ec2.StartInstancesInput{
		InstanceIds: []string{instanceID},
	}

	_, err := c.EC2.StartInstances(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to start instance: %w", err)
	}

	return nil
}

func (c *Client) StopInstance(ctx context.Context, instanceID string) error {
	input := &ec2.StopInstancesInput{
		InstanceIds: []string{instanceID},
	}

	_, err := c.EC2.StopInstances(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to stop instance: %w", err)
	}

	return nil
}

func (c *Client) RebootInstance(ctx context.Context, instanceID string) error {
	input := &ec2.RebootInstancesInput{
		InstanceIds: []string{instanceID},
	}

	_, err := c.EC2.RebootInstances(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to reboot instance: %w", err)
	}

	return nil
}

func (c *Client) TerminateInstance(ctx context.Context, instanceID string) error {
	input := &ec2.TerminateInstancesInput{
		InstanceIds: []string{instanceID},
	}

	_, err := c.EC2.TerminateInstances(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to terminate instance: %w", err)
	}

	return nil
}

// ChangeInstanceType stops and restarts because AWS permits type changes only while stopped.
func (c *Client) ChangeInstanceType(ctx context.Context, instanceID, newType string) error {
	if err := c.StopInstance(ctx, instanceID); err != nil {
		return err
	}

	waiter := ec2.NewInstanceStoppedWaiter(c.EC2)
	if err := waiter.Wait(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{instanceID}}, 5*time.Minute); err != nil {
		return fmt.Errorf("waiting for instance to stop: %w", err)
	}

	_, err := c.EC2.ModifyInstanceAttribute(ctx, &ec2.ModifyInstanceAttributeInput{
		InstanceId:   &instanceID,
		InstanceType: &types.AttributeValue{Value: &newType},
	})
	if err != nil {
		return fmt.Errorf("failed to change instance type: %w", err)
	}

	return c.StartInstance(ctx, instanceID)
}

func (c *Client) GetInstanceTerminationProtection(ctx context.Context, instanceID string) (bool, error) {
	result, err := c.EC2.DescribeInstanceAttribute(ctx, &ec2.DescribeInstanceAttributeInput{
		InstanceId: &instanceID,
		Attribute:  types.InstanceAttributeNameDisableApiTermination,
	})
	if err != nil {
		return false, fmt.Errorf("failed to describe termination protection: %w", err)
	}
	return result.DisableApiTermination != nil && result.DisableApiTermination.Value != nil && *result.DisableApiTermination.Value, nil
}

func (c *Client) SetInstanceTerminationProtection(ctx context.Context, instanceID string, enabled bool) error {
	_, err := c.EC2.ModifyInstanceAttribute(ctx, &ec2.ModifyInstanceAttributeInput{
		InstanceId:            &instanceID,
		DisableApiTermination: &types.AttributeBooleanValue{Value: &enabled},
	})
	if err != nil {
		return fmt.Errorf("failed to set termination protection: %w", err)
	}
	return nil
}

type InstanceStatus struct {
	InstanceID       string
	InstanceState    string
	SystemStatus     string
	InstanceStatus   string
	SystemStatusOk   bool
	InstanceStatusOk bool
	ScheduledEvents  []ScheduledEvent
}

type ScheduledEvent struct {
	Code        string
	Description string
	NotBefore   string
	NotAfter    string
}

func (c *Client) GetInstanceStatus(ctx context.Context, instanceID string) (*InstanceStatus, error) {
	input := &ec2.DescribeInstanceStatusInput{
		InstanceIds:         []string{instanceID},
		IncludeAllInstances: &[]bool{true}[0], // Otherwise stopped instances disappear from status results.
	}

	result, err := c.EC2.DescribeInstanceStatus(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe instance status: %w", err)
	}

	if len(result.InstanceStatuses) == 0 {
		return nil, fmt.Errorf("instance %s not found", instanceID)
	}

	status := result.InstanceStatuses[0]

	instanceStatus := &InstanceStatus{
		InstanceID:    getString(status.InstanceId),
		InstanceState: string(status.InstanceState.Name),
	}

	if status.SystemStatus != nil && status.SystemStatus.Status != "" {
		instanceStatus.SystemStatus = string(status.SystemStatus.Status)
		instanceStatus.SystemStatusOk = (string(status.SystemStatus.Status) == "ok")
	}

	if status.InstanceStatus != nil && status.InstanceStatus.Status != "" {
		instanceStatus.InstanceStatus = string(status.InstanceStatus.Status)
		instanceStatus.InstanceStatusOk = (string(status.InstanceStatus.Status) == "ok")
	}

	for _, event := range status.Events {
		scheduledEvent := ScheduledEvent{
			Code:        string(event.Code),
			Description: getString(event.Description),
		}
		if event.NotBefore != nil {
			scheduledEvent.NotBefore = event.NotBefore.Format("2006-01-02 15:04:05")
		}
		if event.NotAfter != nil {
			scheduledEvent.NotAfter = event.NotAfter.Format("2006-01-02 15:04:05")
		}
		instanceStatus.ScheduledEvents = append(instanceStatus.ScheduledEvents, scheduledEvent)
	}

	return instanceStatus, nil
}

func (c *Client) GetInstanceTypeInfo(ctx context.Context, instanceType string) (*InstanceTypeInfo, error) {
	input := &ec2.DescribeInstanceTypesInput{
		InstanceTypes: []types.InstanceType{types.InstanceType(instanceType)},
	}

	result, err := c.EC2.DescribeInstanceTypes(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe instance type: %w", err)
	}

	if len(result.InstanceTypes) == 0 {
		return nil, fmt.Errorf("instance type %s not found", instanceType)
	}

	typeInfo := result.InstanceTypes[0]

	info := &InstanceTypeInfo{
		InstanceType: string(typeInfo.InstanceType),
	}

	if typeInfo.VCpuInfo != nil && typeInfo.VCpuInfo.DefaultVCpus != nil {
		info.VCpus = *typeInfo.VCpuInfo.DefaultVCpus
	}

	if typeInfo.MemoryInfo != nil && typeInfo.MemoryInfo.SizeInMiB != nil {
		info.Memory = *typeInfo.MemoryInfo.SizeInMiB
	}

	if typeInfo.NetworkInfo != nil {
		if typeInfo.NetworkInfo.NetworkPerformance != nil {
			info.NetworkPerformance = *typeInfo.NetworkInfo.NetworkPerformance
		}
		if typeInfo.NetworkInfo.EnaSupport != "" {
			info.NetworkPerformance += " (ENA)"
		}
	}

	if typeInfo.EbsInfo != nil {
		if typeInfo.EbsInfo.EbsOptimizedSupport != "" {
			info.EbsOptimized = (string(typeInfo.EbsInfo.EbsOptimizedSupport) == "default" ||
				string(typeInfo.EbsInfo.EbsOptimizedSupport) == "supported")
		}
	}

	if typeInfo.InstanceStorageInfo != nil {
		if typeInfo.InstanceStorageInfo.TotalSizeInGB != nil {
			info.InstanceStorageGB = *typeInfo.InstanceStorageInfo.TotalSizeInGB
			info.StorageType = "Instance Store"
		}
	}
	if info.InstanceStorageGB == 0 {
		info.StorageType = "EBS Only"
	}

	for _, arch := range typeInfo.ProcessorInfo.SupportedArchitectures {
		info.SupportedArchitectures = append(info.SupportedArchitectures, string(arch))
	}

	return info, nil
}

func (c *Client) CreateImageFromInstance(ctx context.Context, instanceID, imageName string) (string, error) {
	input := &ec2.CreateImageInput{
		InstanceId: &instanceID,
		Name:       &imageName,
		NoReboot:   &[]bool{true}[0],
	}

	result, err := c.EC2.CreateImage(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to create image: %w", err)
	}

	return getString(result.ImageId), nil
}

type VolumeSnapshot struct {
	SnapshotID  string
	VolumeID    string
	State       string
	Progress    string
	StartTime   string
	Description string
	SizeGiB     int32
}

func (c *Client) ListVolumeSnapshots(ctx context.Context, volumeIDs []string) ([]VolumeSnapshot, error) {
	if len(volumeIDs) == 0 {
		return nil, nil
	}

	result, err := c.EC2.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{
		OwnerIds: []string{"self"},
		Filters: []types.Filter{
			{Name: &[]string{"volume-id"}[0], Values: volumeIDs},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe snapshots: %w", err)
	}

	snapshots := make([]VolumeSnapshot, len(result.Snapshots))
	for i, s := range result.Snapshots {
		snapshots[i] = VolumeSnapshot{
			SnapshotID:  getString(s.SnapshotId),
			VolumeID:    getString(s.VolumeId),
			State:       string(s.State),
			Progress:    getString(s.Progress),
			Description: getString(s.Description),
			SizeGiB:     getInt32Value(s.VolumeSize),
		}
		if s.StartTime != nil {
			snapshots[i].StartTime = s.StartTime.Format("2006-01-02 15:04:05")
		}
	}

	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].StartTime > snapshots[j].StartTime })

	return snapshots, nil
}

func (c *Client) CreateVolumeSnapshot(ctx context.Context, volumeID, description string) (string, error) {
	result, err := c.EC2.CreateSnapshot(ctx, &ec2.CreateSnapshotInput{
		VolumeId:    &volumeID,
		Description: &description,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create snapshot: %w", err)
	}
	return getString(result.SnapshotId), nil
}

func (c *Client) DescribeInstanceAddresses(ctx context.Context, instanceID string) ([]ElasticIP, error) {
	result, err := c.EC2.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{
		Filters: []types.Filter{
			{Name: &[]string{"instance-id"}[0], Values: []string{instanceID}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe addresses: %w", err)
	}

	eips := make([]ElasticIP, len(result.Addresses))
	for i, addr := range result.Addresses {
		eips[i] = ElasticIP{
			PublicIP:      getString(addr.PublicIp),
			AllocationID:  getString(addr.AllocationId),
			AssociationID: getString(addr.AssociationId),
			NetworkIF:     getString(addr.NetworkInterfaceId),
			PrivateIP:     getString(addr.PrivateIpAddress),
		}
	}
	return eips, nil
}

func (c *Client) AssociateElasticIP(ctx context.Context, instanceID, allocationID string) error {
	_, err := c.EC2.AssociateAddress(ctx, &ec2.AssociateAddressInput{
		InstanceId:   &instanceID,
		AllocationId: &allocationID,
	})
	if err != nil {
		return fmt.Errorf("failed to associate elastic IP: %w", err)
	}
	return nil
}

func (c *Client) DisassociateElasticIP(ctx context.Context, associationID string) error {
	_, err := c.EC2.DisassociateAddress(ctx, &ec2.DisassociateAddressInput{
		AssociationId: &associationID,
	})
	if err != nil {
		return fmt.Errorf("failed to disassociate elastic IP: %w", err)
	}
	return nil
}

// GetInstanceUserData returns the instance's user data base64-decoded, or "" when none is set.
func (c *Client) GetInstanceUserData(ctx context.Context, instanceID string) (string, error) {
	input := &ec2.DescribeInstanceAttributeInput{
		InstanceId: &instanceID,
		Attribute:  types.InstanceAttributeNameUserData,
	}

	result, err := c.EC2.DescribeInstanceAttribute(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to get instance user data: %w", err)
	}

	if result.UserData == nil || result.UserData.Value == nil {
		return "", nil
	}

	decoded, err := base64.StdEncoding.DecodeString(*result.UserData.Value)
	if err != nil {
		// Preserve malformed legacy values instead of hiding them.
		return *result.UserData.Value, nil
	}
	return string(decoded), nil
}

// SetInstanceUserData sets the user data script for an instance; the instance must be stopped.
func (c *Client) SetInstanceUserData(ctx context.Context, instanceID, userData string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(userData))

	input := &ec2.ModifyInstanceAttributeInput{
		InstanceId: &instanceID,
		UserData: &types.BlobAttributeValue{
			Value: []byte(encoded),
		},
	}

	_, err := c.EC2.ModifyInstanceAttribute(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to set instance user data: %w", err)
	}
	return nil
}

// GetConsoleOutput returns the instance's console output still base64-encoded, or "" when none is available.
func (c *Client) GetConsoleOutput(ctx context.Context, instanceID string) (string, error) {
	input := &ec2.GetConsoleOutputInput{
		InstanceId: &instanceID,
	}

	result, err := c.EC2.GetConsoleOutput(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to get console output: %w", err)
	}

	if result.Output == nil {
		return "", nil
	}
	return *result.Output, nil
}

// GetConsoleScreenshot returns the instance console screenshot as base64-encoded PNG bytes, or "" when not available.
func (c *Client) GetConsoleScreenshot(ctx context.Context, instanceID string) (string, error) {
	input := &ec2.GetConsoleScreenshotInput{
		InstanceId: &instanceID,
	}

	result, err := c.EC2.GetConsoleScreenshot(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to get console screenshot: %w", err)
	}

	if result.ImageData == nil {
		return "", nil
	}
	return *result.ImageData, nil
}
