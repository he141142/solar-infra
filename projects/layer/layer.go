// Package layer: quy ước chung cho mọi project.
//
// Mỗi project × env là một Stage (= một AWS account). Trong Stage có tối đa 3 stack theo volatility,
// deploy theo thứ tự và chỉ phụ thuộc một chiều:
//
//	infrastructure  ít đổi nhất: VPC, database, bus, secret, parameter
//	foundation      IAM: role + quyền trên tài nguyên của infrastructure
//	app             đổi nhiều nhất: Lambda, ECS, EC2, rule, queue, alarm
package layer

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"github.com/he141142/solar-infra/config"
)

const (
	Infrastructure = "infrastructure"
	Foundation     = "foundation"
	App            = "app"
)

// NewStage tạo stage "<project>-<env>" gắn với account của project.
func NewStage(scope constructs.Construct, project string, env config.Env, account string) awscdk.Stage {
	var acct *string
	if a := config.ResolveAccount(account); a != "" {
		acct = jsii.String(a)
	}
	stage := awscdk.NewStage(scope, jsii.String(project+"-"+env.Name), &awscdk.StageProps{
		Env: &awscdk.Environment{Account: acct, Region: jsii.String(env.Region)},
	})
	tags := awscdk.Tags_Of(stage)
	tags.Add(jsii.String("System"), jsii.String("solar"), nil)
	tags.Add(jsii.String("Project"), jsii.String(project), nil)
	tags.Add(jsii.String("Env"), jsii.String(env.Name), nil)
	return stage
}

// NewStack tạo stack cho một layer. Production khoá xoá nhầm ở 2 layer bền vững.
func NewStack(stage constructs.Construct, layer string, env config.Env, description string) awscdk.Stack {
	protect := env.Production && layer != App
	stack := awscdk.NewStack(stage, jsii.String(layer), &awscdk.StackProps{
		Description:           jsii.String(description),
		TerminationProtection: jsii.Bool(protect),
	})
	awscdk.Tags_Of(stack).Add(jsii.String("Layer"), jsii.String(layer), nil)
	return stack
}

// ImportRole dùng role của layer foundation trong layer app.
// Import dạng mutable để quyền trên tài nguyên của app (queue, X-Ray...) được tạo thành Policy trong
// stack app, không ghi ngược vào foundation -> tránh phụ thuộc vòng foundation <-> app.
func ImportRole(scope constructs.Construct, id string, role awsiam.IRole) awsiam.IRole {
	return awsiam.Role_FromRoleArn(scope, jsii.String(id), role.RoleArn(), &awsiam.FromRoleArnOptions{
		Mutable: jsii.Bool(true),
	})
}

func Output(stack awscdk.Stack, name string, value *string) {
	awscdk.NewCfnOutput(stack, jsii.String(name), &awscdk.CfnOutputProps{Value: value})
}
