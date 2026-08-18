// Package aws is the AWS-aware seam in the provider-agnostic resource registry.
package aws

import (
	"github.com/noelruault/lazyaws/ui/resources"
)

const Provider = "aws"

// Host stays narrow so providers cannot reach panel internals.
type Host interface {
	FocusProfiles(resources.Ref) error
	FocusECS(resources.Ref) error
	FocusEC2(resources.Ref) error
	FocusS3(resources.Ref) error
	FocusEKS(resources.Ref) error
	FocusECR(resources.Ref) error
	FocusSecrets(resources.Ref) error
	FocusVPC(resources.Ref) error
	FocusAmazonQ(resources.Ref) error
	FocusSettings(resources.Ref) error

	ProfileActions() []resources.Action
	ECSActions() []resources.Action
	EC2Actions() []resources.Action
	S3Actions() []resources.Action
	EKSActions() []resources.Action
	ECRActions() []resources.Action
	SecretsActions() []resources.Action
	VPCActions() []resources.Action
}

// Register follows panel order because registration order controls suggestions.
func Register(reg *resources.Registry, host Host) {
	reg.Register(
		&resources.Entry{
			Ref:     ref("profiles", ""),
			Title:   "Profiles",
			Aliases: []string{"profile", "creds"},
			Focus:   host.FocusProfiles,
			Actions: host.ProfileActions,
		},
		&resources.Entry{
			Ref:     ref("ecs", "clusters"),
			Title:   "ECS Clusters",
			Aliases: []string{"ecs", "cluster"},
			Focus:   host.FocusECS,
			Actions: host.ECSActions,
		},
		&resources.Entry{
			Ref:     ref("ec2", "instances"),
			Title:   "EC2 Instances",
			Aliases: []string{"ec2", "instance"},
			Focus:   host.FocusEC2,
			Actions: host.EC2Actions,
		},
		&resources.Entry{
			Ref:     ref("s3", "buckets"),
			Title:   "S3 Buckets",
			Aliases: []string{"s3", "bucket"},
			Focus:   host.FocusS3,
			Actions: host.S3Actions,
		},
		&resources.Entry{
			Ref:     ref("eks", "clusters"),
			Title:   "EKS Clusters",
			Aliases: []string{"eks", "kubernetes", "k8s"},
			Focus:   host.FocusEKS,
			Actions: host.EKSActions,
		},
		&resources.Entry{
			Ref:     ref("ecr", "repositories"),
			Title:   "ECR Repositories",
			Aliases: []string{"ecr", "repo", "repos"},
			Focus:   host.FocusECR,
			Actions: host.ECRActions,
		},
		&resources.Entry{
			Ref:     ref("secretsmanager", "secrets"),
			Title:   "Secrets",
			Aliases: []string{"secrets", "secret", "sm"},
			Focus:   host.FocusSecrets,
			Actions: host.SecretsActions,
		},
		&resources.Entry{
			Ref:     ref("vpc", "vpcs"),
			Title:   "VPCs",
			Aliases: []string{"vpc", "vpcs", "network"},
			Focus:   host.FocusVPC,
			Actions: host.VPCActions,
		},
		&resources.Entry{
			Ref:     ref("amazon-q", ""),
			Title:   "Amazon Q",
			Aliases: []string{"amazon-q", "chat", "ask"},
			Focus:   host.FocusAmazonQ,
		},
		&resources.Entry{
			Ref:     ref("settings", ""),
			Title:   "Settings",
			Aliases: []string{"settings", "config", "preferences"},
			Focus:   host.FocusSettings,
		},
	)
}

func ref(service, resource string) resources.Ref {
	return resources.Ref{Provider: Provider, Service: service, Resource: resource}
}
