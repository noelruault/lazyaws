module github.com/noelruault/lazyaws

go 1.27.0

// Superseded by v0.4.0.
retract (
	v0.3.0
	v0.2.0
	v0.1.0
)

require (
	github.com/aws/aws-sdk-go-v2 v1.43.6
	github.com/aws/aws-sdk-go-v2/config v1.32.37
	github.com/aws/aws-sdk-go-v2/credentials v1.19.36
	github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager v0.3.13
	github.com/aws/aws-sdk-go-v2/service/applicationautoscaling v1.45.6
	github.com/aws/aws-sdk-go-v2/service/autoscaling v1.72.1
	github.com/aws/aws-sdk-go-v2/service/bedrock v1.66.6
	github.com/aws/aws-sdk-go-v2/service/bedrockruntime v1.57.3
	// Deliberately pinned: v1.66+ is CBOR-only and moto 404s that protocol; unpin when moto serves RPCv2 CBOR.
	github.com/aws/aws-sdk-go-v2/service/cloudwatch v1.52.6
	github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs v1.82.2
	github.com/aws/aws-sdk-go-v2/service/codedeploy v1.38.6
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.321.2
	github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect v1.35.6
	github.com/aws/aws-sdk-go-v2/service/ecr v1.60.6
	github.com/aws/aws-sdk-go-v2/service/ecs v1.90.2
	github.com/aws/aws-sdk-go-v2/service/eks v1.91.1
	github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2 v1.58.7
	github.com/aws/aws-sdk-go-v2/service/s3 v1.107.2
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.44.6
	github.com/aws/aws-sdk-go-v2/service/ssm v1.73.6
	github.com/aws/aws-sdk-go-v2/service/sts v1.45.6
	github.com/aws/smithy-go v1.27.8
	github.com/fatih/color v1.19.0
	github.com/jesseduffield/gocui v0.3.1-0.20240418080333-8cd33929c513
	github.com/mattn/go-runewidth v0.0.27
	golang.org/x/term v0.45.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.18 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.37 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.37 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.37 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.38 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.30 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.37 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.38 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.5.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.33.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.38.6 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/gdamore/encoding v1.0.1 // indirect
	github.com/gdamore/tcell/v2 v2.13.10 // indirect
	github.com/go-errors/errors v1.5.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.1 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/stretchr/testify v1.8.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)
