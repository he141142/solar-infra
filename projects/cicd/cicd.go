// Package cicd: bootstrap CI/CD cho MỘT AWS account (làm một lần, giống `cdk bootstrap`).
//
//	GitHub OIDC provider (mỗi account chỉ một)
//	InfraDeployRole: workflow của repo solar-infra dùng để chạy `cdk deploy`.
//	                 Chỉ được assume các role bootstrap của CDK, mọi thay đổi đi qua CloudFormation.
//
// Role deploy của từng app nằm ở layer foundation của project sở hữu app đó (vd. trading).
package cicd

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"github.com/he141142/solar-infra/config"
	"github.com/he141142/solar-infra/constructs/githubci"
	"github.com/he141142/solar-infra/projects/layer"
)

func Build(stage constructs.Construct, env config.Env) awscdk.Stack {
	stack := layer.NewStack(stage, layer.Foundation, env, "CI/CD bootstrap - GitHub OIDC")

	githubci.NewProvider(stack, "GitHubOidc")

	role := githubci.NewDeployRole(stack, "InfraDeployRole", config.InfraRepo, env.Name)
	role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions: jsii.Strings("sts:AssumeRole"),
		Resources: &[]*string{awscdk.Fn_Join(jsii.String(""), &[]*string{
			jsii.String("arn:aws:iam::"), stack.Account(), jsii.String(":role/cdk-hnb659fds-*"),
		})},
	}))
	layer.Output(stack, "InfraDeployRoleArn", role.RoleArn())

	return stack
}
