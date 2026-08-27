#!/usr/bin/env bash
# .seed.json is the journeys' side of this contract: moto invents the ids, so nothing downstream hardcodes one.
set -euo pipefail

: "${AWS_ENDPOINT_URL:?set AWS_ENDPOINT_URL to the fake AWS endpoint}"
export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-lazyaws-ui-test}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-lazyaws-ui-test}"
export AWS_REGION="${AWS_REGION:-eu-west-1}"
export AWS_PAGER=""

seed_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
q() { aws "$@" --output text; }

# A described ECS task walks one step down the lifecycle per read, so a fixture that is RUNNING when it is created is STOPPED by the time a journey looks at it.
curl -fsS -X POST "$AWS_ENDPOINT_URL/moto-api/state-manager/set-transition" \
	-H 'Content-Type: application/json' \
	-d '{"model_name":"ecs::task","transition":{"progression":"manual","times":1000000}}' >/dev/null

# --- VPCs -------------------------------------------------------------------------------------
# Two VPCs, and only the first is wired, so the panel shows a populated and a bare one side by side.
vpc_core=$(q ec2 create-vpc --cidr-block 10.0.0.0/16 \
	--tag-specifications 'ResourceType=vpc,Tags=[{Key=Name,Value=ui-core}]' --query Vpc.VpcId)
vpc_edge=$(q ec2 create-vpc --cidr-block 10.1.0.0/16 \
	--tag-specifications 'ResourceType=vpc,Tags=[{Key=Name,Value=ui-edge}]' --query Vpc.VpcId)
# moto ships a default VPC, so the panel holds three rows and a journey counting them has to expect this one too.
vpc_default=$(q ec2 describe-vpcs --filters Name=isDefault,Values=true --query 'Vpcs[0].VpcId')
# Also a prerequisite for the ECS task below: moto only gives an ENI a private DNS name when its VPC has hostnames on, and RunTask reads that field unguarded.
aws ec2 modify-vpc-attribute --vpc-id "$vpc_core" --enable-dns-hostnames >/dev/null
aws ec2 modify-vpc-attribute --vpc-id "$vpc_core" --enable-dns-support >/dev/null

igw=$(q ec2 create-internet-gateway --query InternetGateway.InternetGatewayId)
aws ec2 attach-internet-gateway --internet-gateway-id "$igw" --vpc-id "$vpc_core" >/dev/null

subnet_public=$(q ec2 create-subnet --vpc-id "$vpc_core" --cidr-block 10.0.1.0/24 \
	--availability-zone "${AWS_REGION}a" \
	--tag-specifications 'ResourceType=subnet,Tags=[{Key=Name,Value=ui-core-public}]' --query Subnet.SubnetId)
subnet_private=$(q ec2 create-subnet --vpc-id "$vpc_core" --cidr-block 10.0.2.0/24 \
	--availability-zone "${AWS_REGION}b" \
	--tag-specifications 'ResourceType=subnet,Tags=[{Key=Name,Value=ui-core-private}]' --query Subnet.SubnetId)

# The overview splits subnets by ROUTING, not by MapPublicIpOnLaunch, so only the public one gets a route table with a gateway route.
rtb_public=$(q ec2 create-route-table --vpc-id "$vpc_core" --query RouteTable.RouteTableId)
aws ec2 create-route --route-table-id "$rtb_public" --destination-cidr-block 0.0.0.0/0 --gateway-id "$igw" >/dev/null
aws ec2 associate-route-table --route-table-id "$rtb_public" --subnet-id "$subnet_public" >/dev/null

sg=$(q ec2 create-security-group --group-name lazyaws-ui-web --description 'lazyaws UI harness' \
	--vpc-id "$vpc_core" --query GroupId)

# --- EC2 --------------------------------------------------------------------------------------
ami=$(q ec2 describe-images --owners amazon --query 'Images[0].ImageId')
run_instance() { # run_instance <subnet> <public-ip-flag> [name-tag]
	local subnet="$1" public="$2" name="${3:-}" tags=()
	[ -n "$name" ] && tags=(--tag-specifications "ResourceType=instance,Tags=[{Key=Name,Value=$name}]")
	q ec2 run-instances --image-id "$ami" --instance-type t3.micro --subnet-id "$subnet" \
		"$public" --count 1 "${tags[@]}" --query 'Instances[0].InstanceId'
}
instance_web=$(run_instance "$subnet_public" --associate-public-ip-address ui-web-1)
instance_db=$(run_instance "$subnet_private" --no-associate-public-ip-address ui-db-1)
# No Name tag: the row has to fall back to "(no name)" rather than render blank.
instance_unnamed=$(run_instance "$subnet_private" --no-associate-public-ip-address)

# --- S3 ---------------------------------------------------------------------------------------
for bucket in lazyaws-ui-artifacts lazyaws-ui-logs lazyaws-ui-state; do
	aws s3api create-bucket --bucket "$bucket" \
		--create-bucket-configuration "LocationConstraint=$AWS_REGION" >/dev/null
done
aws s3api put-bucket-versioning --bucket lazyaws-ui-state \
	--versioning-configuration Status=Enabled >/dev/null

# --- ECR --------------------------------------------------------------------------------------
# One repository per mutability setting, because the row renders the badge from that field alone.
aws ecr create-repository --repository-name lazyaws/api --image-tag-mutability IMMUTABLE \
	--image-scanning-configuration scanOnPush=true >/dev/null
aws ecr create-repository --repository-name lazyaws/worker --image-tag-mutability MUTABLE >/dev/null

# --- Secrets ----------------------------------------------------------------------------------
secret_rotated=$(q secretsmanager create-secret --name lazyaws/ui/db \
	--description 'harness database password' --secret-string 'seeded' --query ARN)
aws secretsmanager rotate-secret --secret-id lazyaws/ui/db \
	--rotation-rules AutomaticallyAfterDays=7 >/dev/null
secret_plain=$(q secretsmanager create-secret --name lazyaws/ui/api-key \
	--secret-string 'seeded' --query ARN)

# --- ECS --------------------------------------------------------------------------------------
cluster=$(q ecs create-cluster --cluster-name lazyaws-ui --query cluster.clusterArn)
# The per-container cpu/memory are not decoration: moto sums them to size the task and adds int += None without them.
cat >"$seed_dir/.taskdef.json" <<JSON
{"family":"lazyaws-ui-web","networkMode":"awsvpc","requiresCompatibilities":["FARGATE"],
 "cpu":"256","memory":"512","containerDefinitions":[
  {"name":"web","image":"123456789012.dkr.ecr.$AWS_REGION.amazonaws.com/lazyaws/api:1.4.2","essential":true,"cpu":192,"memory":384},
  {"name":"logrouter","image":"public.ecr.aws/aws-observability/aws-for-fluent-bit:stable","essential":false,"cpu":64,"memory":128}]}
JSON
taskdef=$(q ecs register-task-definition --cli-input-json "file://$seed_dir/.taskdef.json" \
	--query taskDefinition.taskDefinitionArn)
rm -f "$seed_dir/.taskdef.json"
# securityGroups is optional to AWS and mandatory to moto, which indexes it without checking.
net="awsvpcConfiguration={subnets=[$subnet_public],securityGroups=[$sg],assignPublicIp=ENABLED}"
aws ecs create-service --cluster lazyaws-ui --service-name web --task-definition lazyaws-ui-web \
	--desired-count 1 --launch-type FARGATE --network-configuration "$net" >/dev/null
task=$(q ecs run-task --cluster lazyaws-ui --task-definition lazyaws-ui-web --launch-type FARGATE \
	--network-configuration "$net" --query 'tasks[0].taskArn')

cat >"$seed_dir/.seed.json" <<JSON
{
  "region": "$AWS_REGION",
  "vpcs": { "core": "$vpc_core", "edge": "$vpc_edge", "default": "$vpc_default" },
  "subnets": { "public": "$subnet_public", "private": "$subnet_private" },
  "securityGroup": "$sg",
  "internetGateway": "$igw",
  "instances": { "web": "$instance_web", "db": "$instance_db", "unnamed": "$instance_unnamed" },
  "instanceNames": { "web": "ui-web-1", "db": "ui-db-1" },
  "buckets": ["lazyaws-ui-artifacts", "lazyaws-ui-logs", "lazyaws-ui-state"],
  "repositories": ["lazyaws/api", "lazyaws/worker"],
  "secrets": { "rotated": "$secret_rotated", "plain": "$secret_plain" },
  "secretNames": { "rotated": "lazyaws/ui/db", "plain": "lazyaws/ui/api-key" },
  "ecs": { "cluster": "$cluster", "clusterName": "lazyaws-ui", "service": "web", "taskDefinition": "$taskdef", "task": "$task", "image": "lazyaws/api:1.4.2" },
  "eksClusters": []
}
JSON
echo "seeded $AWS_ENDPOINT_URL -> $seed_dir/.seed.json"
