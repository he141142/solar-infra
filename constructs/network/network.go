// Package network: VPC chuẩn cho một project account.
package network

import (
	"github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"github.com/he141142/solar-infra/config"
)

func NewVpc(scope constructs.Construct, id string, env config.Env) awsec2.IVpc {
	vpc := awsec2.NewVpc(scope, jsii.String(id), &awsec2.VpcProps{
		MaxAzs:      jsii.Number(env.MaxAzs),
		NatGateways: jsii.Number(env.NatGateways),
		SubnetConfiguration: &[]*awsec2.SubnetConfiguration{
			{Name: jsii.String("public"), SubnetType: awsec2.SubnetType_PUBLIC, CidrMask: jsii.Number(24)},
			{Name: jsii.String("app"), SubnetType: awsec2.SubnetType_PRIVATE_WITH_EGRESS, CidrMask: jsii.Number(22)},
		},
		// Gateway endpoint miễn phí: traffic DynamoDB/S3 không đi qua NAT.
		GatewayEndpoints: &map[string]*awsec2.GatewayVpcEndpointOptions{
			"S3":       {Service: awsec2.GatewayVpcEndpointAwsService_S3()},
			"DynamoDB": {Service: awsec2.GatewayVpcEndpointAwsService_DYNAMODB()},
		},
	})
	if env.Production {
		vpc.AddFlowLog(jsii.String("FlowLogs"), nil)
	}
	return vpc
}
