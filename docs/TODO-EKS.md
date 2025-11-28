AWS_PROFILE=webbeds-stage \
    aws eks describe-cluster --name webbeds-eks-eu-west-1-stage-green \
    --region eu-west-1 --query 'cluster.resourcesVpcConfig'

{
    "subnetIds": [
        "subnet-0a314947456726b32",
        "subnet-02fb5923e2cbfde56",
        "subnet-07c525721f67b01da",
        "subnet-0e0839bce9a50a09a",
        "subnet-022d74fc6444f2e5d",
        "subnet-012ef2985d02dbf5d"
    ],
    "securityGroupIds": [
        "sg-03021e99d9a8f8c7d",
        "sg-0b516f2840440deed"
    ],
    "clusterSecurityGroupId": "sg-048a4037d0fcf505c",
    "vpcId": "vpc-03fc900440821c322",
    "endpointPublicAccess": true,
    "endpointPrivateAccess": true,
    "publicAccessCidrs": [
        "34.249.95.72/32",
        "54.216.237.73/32",
        "34.240.40.136/32",
        "54.247.69.22/32",
        "54.220.9.26/32",
        "89.140.187.10/32",
        "52.51.6.209/32",
        "3.255.15.193/32",
        "99.80.153.217/32",
        "3.251.51.2/32"
    ]
}


AWS_PROFILE=webbeds-stage \
    aws ec2 describe-security-groups --group-ids sg-0b516f2840440deed \
    --region eu-west-1
{
    "SecurityGroups": [
        {
            "GroupId": "sg-0b516f2840440deed",
            "IpPermissionsEgress": [
                {
                    "IpProtocol": "-1",
                    "UserIdGroupPairs": [ ],
                    "IpRanges": [
                        {
                            "CidrIp": "10.69.0.0/16"
                        }
                    ],
                    "Ipv6Ranges": [ ],
                    "PrefixListIds": [ ]
                }
            ],
            "VpcId": "vpc-03fc900440821c322",
            "SecurityGroupArn": "arn:aws:ec2:eu-west-1:946529865476:security-group/sg-0b516f2840440deed",
            "OwnerId": "946529865476",
            "GroupName": "ipranger",
            "Description": "Allows dns access for consul discovery",
            "IpPermissions": [
                {
                    "IpProtocol": "-1",
                    "UserIdGroupPairs": [ ],
                    "IpRanges": [
                        {
                            "CidrIp": "10.69.0.0/16"
                        }
                    ],
                    "Ipv6Ranges": [ ],
                    "PrefixListIds": [ ]
                }
            ]
        }
    ]
}

AWS_PROFILE=webbeds-stage \
    aws ec2 describe-security-groups --group-ids sg-0b516f2840440deed \
    --region eu-west-1 --query 'SecurityGroups[0].IpPermissions'

[
    {
        "IpProtocol": "-1",
        "UserIdGroupPairs": [ ],
        "IpRanges": [
            {
                "CidrIp": "10.69.0.0/16"
            }
        ],
        "Ipv6Ranges": [ ],
        "PrefixListIds": [ ]
    }
]

AWS_PROFILE=webbeds-stage \
    aws ec2 describe-vpc-peering-connections --vpc-peering-connection-ids pcx-0bd76f0a051013aed \
    --region eu-west-1
{
    "VpcPeeringConnections": [
        {
            "AccepterVpcInfo": {
                "CidrBlock": "10.192.0.0/16",
                "CidrBlockSet": [
                    {
                        "CidrBlock": "10.192.0.0/16"
                    }
                ],
                "OwnerId": "946529865476",
                "PeeringOptions": {
                    "AllowDnsResolutionFromRemoteVpc": false,
                    "AllowEgressFromLocalClassicLinkToRemoteVpc": false,
                    "AllowEgressFromLocalVpcToRemoteClassicLink": false
                },
                "VpcId": "vpc-03fc900440821c322",
                "Region": "eu-west-1"
            },
            "RequesterVpcInfo": {
                "CidrBlock": "10.69.0.0/16",
                "Ipv6CidrBlockSet": [
                    {
                        "Ipv6CidrBlock": "2a05:d018:1e1a:5700::/56"
                    }
                ],
                "CidrBlockSet": [
                    {
                        "CidrBlock": "10.69.0.0/16"
                    }
                ],
                "OwnerId": "111924088008",
                "VpcId": "vpc-06b385e04d8112bfa",
                "Region": "eu-west-1"
            },
            "Status": {
                "Code": "active",
                "Message": "Active"
            },
            "Tags": [
                    {
                        "Key": "Name",
                        "Value": "webbeds-stage-to-wbsecurity"
                    }
            ],
            "VpcPeeringConnectionId": "pcx-0bd76f0a051013aed"
        }
    ]
}
