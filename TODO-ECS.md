- [x] Check Security Groups: Verify that the security group attached to your application tasks/containers allows outbound traffic on UDP and TCP port 53 to 10.69.0.2.


TASK_ARN="arn:aws:ecs:eu-west-1:111924088008:task/ipranger-cluster/32c780b76d7842e386ce83e158a2d92b"
AWS_PROFILE=security-staging aws ecs describe-tasks \
  --cluster ipranger-cluster \
  --tasks $TASK_ARN \
  --region eu-west-1 \
  --query "tasks[0].attachments[].details[?name=='networkInterfaceId'].value" \
  --output text
ENI_ID="eni-048dbc86fae20dd46"
AWS_PROFILE=security-staging aws ec2 describe-network-interfaces \
  --network-interface-ids $ENI_ID \
  --region eu-west-1 \
  --query "NetworkInterfaces[0].Groups"
```
[
    {
        "GroupId": "sg-0467537c5cfd5e329",
        "GroupName": "ipranger-tasks-sg"
    }
]
```
SG_ID="sg-0467537c5cfd5e329"
AWS_PROFILE=security-staging aws ec2 describe-security-groups \
  --group-ids $SG_ID \
  --region eu-west-1 \
  --query "SecurityGroups[0].IpPermissionsEgress"

- [] Test Consul Service: From within your VPC (using a bastion host or another container), test if the Consul DNS service is reachable:

```sh
telnet 10.69.0.2 53
dig pos-resetweb-platform-engineering.service.aws-webbeds-westeurope-stage.consul @10.69.0.2
```

- [] Verify Consul Configuration: Ensure the Consul service is properly configured and running on 10.69.0.2.
- [] Check Application DNS Configuration: Confirm your application is configured to use 10.69.0.2 as its DNS server for Consul service discovery.
- [] Review Container/Task Definitions: If using ECS or EKS, check the task definitions or pod specifications for proper DNS and networking configurations.
